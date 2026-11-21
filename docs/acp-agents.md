# 外部 agent（ACP）：把工作派給 Grok Build / opencode

glaude 可以扮演 [Agent Client Protocol](https://agentclientprotocol.com) 的 client，把任務派給另一個 coding agent，讓它用自己的模型和工具來做。認證走那個 agent 自己的登入（例如 `~/.grok/auth.json`），glaude 不會碰它的 token。

和每次重開的 `grok -p` 不同，**同一個 agent 在整個 glaude 會話裡只開一個行程、一個 session**。之後每次派工都在同一個 session 裡繼續，它已經讀過的檔案還在 context 裡，不必重新掃專案。

## 用法

```
/grok 把 internal/acpclient 的錯誤訊息改成中文
/opencode 幫 cmd/docgen 補單元測試
```

- `/<agent> <任務>` 會在背景派工。進度（文字、工具呼叫、計畫）即時顯示在 agent 面板，完成後結果回到對話，由主模型轉述。
- 主模型也可以自己派工：Agent 工具填 `agent_type: "grok"`。工具說明裡會列出目前可用的外部 agent。
- 用 SendMessage 追加指令時，會送進同一個 session 繼續做。
- 外部 agent 不適用 `system_prompt`、`allowed_tools`、`max_turns`、`model` 這幾個參數。

## 內建 agent

| 名稱 | 啟動方式 | 為了「一件事做到底」做的設定 |
|------|----------|------------------------------|
| `grok` | `grok agent stdio` | `GROK_SUBAGENTS=0`、`GROK_WORKFLOWS=0`（不開 sub agent 和 workflow）；模型設為 `grok-4.7`（預設的 `build-fast` 是 2 倍價） |
| `opencode` | `opencode acp` | 透過 `OPENCODE_CONFIG_CONTENT` 注入 `permission.task = deny`（不開 sub agent） |

指令要在 PATH 裡，而且要先在 glaude 外面登入（`grok login`、`opencode auth login`）。沒有安裝的 agent，要到第一次派工時才會報錯。

## 權限

外部 agent 要寫檔、跑指令時，會發 `session/request_permission` 回 glaude，由 **glaude 的權限系統**決定：

- 請求會轉成 glaude 工具的形狀：execute → `Bash`、edit → `Edit`/`Write`、read → `Read`、search → `Grep`、fetch → `WebFetch`，其餘 → `ExternalAgent`。所以既有的規則照樣適用，例如 `Bash(go test:*)`，受保護路徑、Bash 安全檢查、權限模式也都會生效。
- 沒有命中規則的請求，互動模式跳出確認框，`-p` 模式直接拒絕。
- 在確認框選「永遠允許」時，規則由 glaude 自己記錄（寫進專案的 `.glaude/settings.json`）。
- 就算你自己的 agent 設定是全放行（例如 `~/.grok/config.toml` 的 `permission_mode = "always-approve"`），glaude 也會強制它們回來詢問：grok 送 `_meta.yoloMode=false`，opencode 注入 `edit/bash/webfetch = ask`。
- `ExternalAgent` 這條規則會放行所有對應不到 glaude 工具的呼叫，請小心使用。

## 設定（settings.json `acpAgents`）

```json
{
  "acpAgents": {
    "grok":     { "model": "grok-4.7-build-fast" },
    "opencode": { "disabled": true },
    "my-agent": {
      "command": "my-agent",
      "args": ["--acp"],
      "env": { "MY_AGENT_MODE": "strict" },
      "description": "什麼時候該派給它（給主模型看）"
    }
  }
}
```

- 跟內建同名的條目逐欄覆寫：`env` 合併，`args` 有填就整個取代。新的名稱會新增一個 agent，`"disabled": true` 會移除它。
- 欄位：`command`、`args`、`env`、`model`（用 `session/set_config_option` 設定）、`description`、`disabled`。
- 名稱不能和 glaude 的 agent 類型（`worker`、`explore`、`plan`、`verify`、`guide`、`fork`）相同，撞名的會被略過。
- 各層照一般 settings 的順序合併（user → project → local → policy）。policy 可以用 `disabled` 停用某個 agent。
- 注意：專案層的 `.glaude/settings.json` 可以指定要執行的指令。clone 別人的專案時，派工前先看一下這個檔案（風險和 hooks 相同）。

## 行為細節

- 一個「agent + 工作目錄」對應一個行程和一個 session。切換到 worktree 會開新的 session。
- 送給同一個 session 的 prompt 會排隊依序處理。
- agent 行程掛掉的話，下次派工會自動重啟。結果會標註「新 session，沒有先前的 context」。
- 停止 agent（StopAgent / 中斷）會送出 `session/cancel`，session 本身會保留。
- glaude 結束時會關閉所有 agent 行程。
- agent 的 stderr 不會出現在終端畫面，只保留最後 4 KiB，附在錯誤訊息裡。`--debug` 會印出 ACP 的連線紀錄。

## 端到端測試（會消耗真實 token）

```bash
go test -tags acpe2e -run TestRealAgents -v ./internal/acpclient/
GLAUDE_ACP_E2E_AGENTS=grok go test -tags acpe2e -run TestRealAgents -v ./internal/acpclient/
```

測試會讓 agent 跑一個無害的指令，並驗證三件事：權限請求真的有回到 glaude、檔案真的建立了、第二輪記得第一輪做過什麼。
