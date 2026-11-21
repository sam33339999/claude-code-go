# 記憶萃取（extract_memories）每輪重送整段歷史：token 成本問題

> 日期：2026-11-21
> 狀態：**待處理**（另開分支處理；本文只記錄問題與選項）
> 發現於：`fix/openai-tool-arguments`（同分支修了萃取 agent 觸發的 400 與 log 蓋畫面）

## 現象

使用者用 OpenAI 相容 provider（DashScope／Qwen coder）時，TUI 狀態列被這段錯誤蓋掉：

```
memdir: extract memories: engine: forked agent: engine: stream: API error 400:
"The \"function.arguments\" parameter of the code model must be in JSON format."
```

400 本身已在 `fix/openai-tool-arguments` 修掉（`normalizeToolArguments`）。追查時發現另一個更大的問題：**萃取在每一輪都會觸發，而且每次都把整段對話歷史重送一遍**。

## 機制（附出處）

1. **觸發點**：`internal/engine/query.go` 的 stop reason 是 `end_turn`／`stop_sequence`／`""` 時，會呼叫 `fireStopHooks`，讓每個 stop hook 在背景 goroutine 跑。
   `extract_memories` 只有遇到以下情況會跳過（`internal/memdir/extract.go`）：
   - 設定停用（`autoMemoryEnabled: false`）
   - bare 模式
   - `QuerySource` 不是 `foreground`
   - 這一輪主模型已經寫過 `MemoryWrite`
   - 摘要是空的

   **沒有節流**，也就是使用者每完成一輪，背景就多跑一個 forked agent。
2. **送了什麼**：`fireStopHooks` 用 `CreateCacheSafeParams(params, messages)` 建立 CacheParams，`ContextMessages` 是**完整的 messages**（`internal/engine/cache_params.go`）。
   `RunForkedAgent` 會把 `ContextMessages` 和 `PromptMessages` 串起來送出，所以一次請求的內容是：
   ```
   system prompt + 整段對話歷史 + BuildExtractionPrompt(最近 20 則訊息的摘要，≤4000 字)
   ```
   摘要涵蓋的範圍跟歷史尾端重複。
3. **回合數**：`MaxTurns: 3`。萃取 agent 呼叫 Memory* 工具後會再進下一回合，整包內容最多重送 3 次。
4. **prompt cache 的前提不成立**：原本的設計靠跟主對話共用前綴、命中快取來壓低成本，但目前：
   - `ExcludeTools` 把工具清單縮成 3 個 Memory* 工具，request 前綴（tools）已經跟主對話不一樣了。
   - OpenAI 相容 provider 是隱式前綴快取，命中規則各家不同，無法保證。

**粗估**：快取沒命中時，每輪 input token 大約是原本的 2～4 倍，對話越長越明顯。這是推算，**沒有實測**，處理前應先量一次（`--debug` 或 provider 的帳單／usage）。

`auto_dream` 也用同樣的 CacheParams，但有鎖檔節流（`minHoursBetweenDream = 24`），影響小。

## 緩解（使用者側，立即可用）

```json
{ "autoMemoryEnabled": false }
```

這個設定會同時關掉 extract 和 dream。

## 修法選項（待決定）

| 選項 | 做法 | 優點 | 缺點 |
|------|------|------|------|
| A. 只送摘要 | 萃取 agent 的 messages 只放 extraction prompt，不帶 `ContextMessages` | 最省，每次成本固定 | 只看得到最近 20 則、≤4000 字；跟主對話的快取共用徹底放棄 |
| B. 保留歷史＋節流 | 每 N 輪，或新增內容超過門檻才跑；同時拿掉重複的摘要 | 看到完整脈絡 | 仍然昂貴；要設計節流狀態（跨輪計數） |
| C. A＋節流 | 只送摘要，再加最低間隔 | 最省 | 可能漏掉短暫提及的資訊 |

建議方向：A 或 C。萃取只需要「最近發生了什麼」，摘要本來就是為這個設計的；整段歷史只是為了快取命中才帶，而快取在這裡已經不成立。

## 順帶查證（不是 bug）

最初懷疑 `fireStopHooks` 沒帶 `Model`／`Client`，會讓萃取 agent 跑在別的模型上。查證後**不成立**：
- 萃取只從主迴圈（`foreground`）觸發。
- forked query 的 `Model` 留空時，用的是當下的 `MainTarget`，跟剛結束的那一輪相同。
- 只有在「這一輪結束」到「fork 開始」之間剛好有人執行 `/model`，才可能不一致，可以忽略。

## 相關檔案

- `internal/engine/query.go`（`fireStopHooks` 觸發點）
- `internal/engine/stop_hooks.go`、`internal/engine/cache_params.go`、`internal/engine/forked_agent.go`
- `internal/memdir/extract.go`、`internal/memdir/dream.go`
- `internal/bootstrap/wire.go`（`registerMemoryStopHooks`）
