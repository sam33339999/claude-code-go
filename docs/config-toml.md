# `~/.glaude/config.toml`：provider 註冊與模型等級

這個檔案讓 sub-agent 依**等級**（`lite` / `pro` / `max`）把請求送到不同的 provider 和模型。主腦（main loop）的 provider 和模型仍由 `settings.json` 決定，不受這個檔案影響；`[main]` 段只影響主腦的上下文長度。

- 只讀全域這一份，沒有專案層或本地層。
- 檔案不存在：沒有 provider、沒有等級，行為和沒有這個功能時完全一樣。
- 格式錯誤或欄位不合法：啟動時在 stderr 印警告，忽略有問題的部分，程式照常啟動。
- 新增或修改 provider 要手動編輯檔案並**重新啟動**；`/tier` 只能在已註冊的 provider 之間改綁定。

設計背景見 `docs/superpowers/specs/2026-11-19-provider-tiers-design.md`。

## 範例

```toml
# Provider 註冊：id 自訂（"main" 是保留字，不能用）
[providers.qwencloud]
format      = "openai"          # "openai" | "anthropic"
base_url    = "https://dashscope.aliyuncs.com/compatible-mode/v1"
api_key_env = "DASHSCOPE_API_KEY"

[providers.openrouter]
format      = "openai"
base_url    = "https://openrouter.ai/api/v1"
api_key_env = "OPENROUTER_API_KEY"

[providers.zai]
format      = "openai"
base_url    = "https://api.z.ai/api/paas/v4"
api_key_env = "ZAI_API_KEY"

# 選填：provider 層的模型表
[providers.openrouter.models."qwen/qwen3.8-27b"]
context_window = 262144
max_tokens     = 16384

# 選填：只管主腦的上下文長度
[main]
context_window = 131072

# 等級綁定：provider id + 模型 ID（模型 ID 原樣傳給該端點）
[tiers.lite]
provider   = "zai"
model      = "glm-5.3-flash"
max_tokens = 8192

[tiers.pro]
provider = "openrouter"
model    = "qwen/qwen3.8-27b"

[tiers.max]
provider       = "main"              # 保留 id：沿用主腦的 client 與認證（含 OAuth）
model          = "claude-opus-5-5"
context_window = 200000
```

## 欄位

| 位置 | 欄位 | 說明 |
|------|------|------|
| `[providers.<id>]` | `format` | 必填。`"anthropic"`（Anthropic Messages）或 `"openai"`（OpenAI Chat Completions）。`bedrock` / `vertex` 不支援 |
| | `base_url` | 端點網址；結尾的 `/v1` 會自動去掉 |
| | `api_key_env` | 存放金鑰的**環境變數名稱**。不接受明文 `api_key`（出現時整個 provider 會被忽略）。留空表示這個端點不需要金鑰 |
| `[providers.<id>.models."<模型>"]` | `context_window`、`max_tokens` | 選填，正整數 |
| `[main]` | `context_window` | 選填。主腦的壓縮門檻和狀態列顯示用；不改主腦的請求內容 |
| `[tiers.<名稱>]` | `provider`、`model` | 兩個都寫才算綁定；都不寫（空表）＝已知但未綁定 |
| | `context_window`、`max_tokens` | 選填，優先於 provider 模型表 |

等級名稱和數量由檔案決定，`[tiers.<任意名>]` 都合法；`lite` / `pro` / `max` 永遠是已知等級，即使沒寫。

## 值的解析順序

- **上下文長度**（sub-agent）：`CLAUDE_CODE_AUTO_COMPACT_WINDOW` > 等級的 `context_window` > provider 模型表 > 內建表 > 預設值 200000。
- **上下文長度**（主腦）：`CLAUDE_CODE_AUTO_COMPACT_WINDOW` > `[main] context_window` > 內建表 > 預設值。
- **`max_tokens`**：等級 > provider 模型表 > engine 預設。摘要請求的輸出上限取 `min(20000, max_tokens)`。

綁到 `main:<模型>` 的等級用等級自己的值，不套用 `[main]`。

## sub-agent 怎麼選模型

順序不變：`CLAUDE_CODE_SUBAGENT_MODEL` > Agent 工具的 `model` 參數 > agent profile 的 `model` > 繼承主腦。拿到的值：

- 是**已綁定的等級**（或舊別名 `haiku`→`lite`、`sonnet`→`pro`、`opus`→`max`）→ 用該等級的 provider 和模型。
- 是 `providerID:modelID`（只以第一個 `:` 切開，而且 provider 已註冊或是 `main`）→ 直接用。
- 是**未綁定的** `lite` / `pro` / `max` → 照舊別名的規則：Anthropic 系 provider 用對應的模型家族，openai provider 則繼承主腦。
- 是未綁定的自訂等級 → 繼承主腦。
- 其他字串 → 當成模型 ID 原樣傳給主腦的 provider。

內建的 explore agent 預設用 `lite`。fork agent 永遠繼承主腦（才能共用 prompt cache），不受等級影響。

provider 的金鑰環境變數沒設時，啟動照常進行，但派工到那個等級會直接失敗，錯誤訊息會點名環境變數，**不會**默默改用主腦。

## `/tier` 指令

```
/tier                                  列出等級綁定、provider 與金鑰狀態
/tier <等級> <providerID>:<modelID>    改綁定並寫回檔案
/tier <等級> none                      解除綁定（之後繼承主腦）
```

- 改綁定在**下一個派出的 sub-agent** 生效；已在跑的 agent（包括之後被續跑的）沿用派工當下的綁定。
- 改綁定會清掉該等級原本寫的 `context_window` / `max_tokens`（那是舊模型的值），需要的話請寫在 provider 模型表。
- 寫回檔案時會重新輸出整個 TOML，**檔案裡的註解會遺失**。
