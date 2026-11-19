<p align="center">
  <img src="assets/logo.png" alt="Glaude Logo" width="200">
</p>

<h1 align="center">Glaude</h1>

<p align="center">
  <strong>🤖 Claude Code 的 Go 重寫版——終端機裡的 AI 程式設計助手</strong>
</p>

<p align="center">
  <a href="https://golang.org/dl/"><img src="https://img.shields.io/badge/go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 版本"></a>
  <a href="https://goreportcard.com/report/github.com/tunsuy/glaude"><img src="https://goreportcard.com/badge/github.com/tunsuy/glaude?style=flat-square" alt="Go Report Card"></a>
  <a href="https://codecov.io/gh/tunsuy/glaude"><img src="https://codecov.io/gh/tunsuy/glaude/branch/main/graph/badge.svg?style=flat-square" alt="codecov"></a>
  <a href="https://pkg.go.dev/github.com/tunsuy/glaude"><img src="https://pkg.go.dev/badge/github.com/tunsuy/glaude.svg" alt="Go Reference"></a>
  <a href="https://github.com/tunsuy/glaude/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/tunsuy/glaude/ci.yml?branch=main&style=flat-square&logo=github&label=CI" alt="CI"></a>
  <a href="https://github.com/tunsuy/glaude/releases"><img src="https://img.shields.io/github/v/release/tunsuy/glaude?style=flat-square&logo=github" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green?style=flat-square" alt="License"></a>
  <a href="https://github.com/tunsuy/glaude/pulls"><img src="https://img.shields.io/badge/PRs-welcome-brightgreen?style=flat-square" alt="PRs Welcome"></a>
</p>

<p align="center">
  <a href="README.md">English</a> •
  <a href="README.zh-CN.md">简体中文</a> •
  <a href="README.zh-TW.md">繁體中文</a> •
  <a href="README.ja.md">日本語</a> •
  <a href="README.ko.md">한국어</a> •
  <a href="README.es.md">Español</a> •
  <a href="README.fr.md">Français</a>
</p>

---

<p align="center">
  <img src="assets/image.png" alt="Glaude - 互動工作階段" width="800">
  <br>
  <em>互動式 TUI，支援檔案讀取與即時思考顯示</em>
</p>

<p align="center">
  <img src="assets/image1.png" alt="Glaude - 專案分析" width="800">
  <br>
  <em>完整的專案分析與架構拆解</em>
</p>

---

## 這是什麼

本專案是 **[Claude Code](https://claude.ai/code)**（Anthropic 官方 TypeScript CLI）的**完整 Go 重寫版**——逐模組以 Go 改寫，涵蓋所有核心功能：TUI、工具使用、權限系統、多代理協調、MCP 協定、工作階段管理等。它理解你的程式碼庫、執行工具，透過自然對話協助你撰寫、審查與重構程式碼。

> **🤖 完全由 AI 代理建構——零人工撰寫的生產程式碼。** 初始建構（2026 年 4 月）由 9 個 Claude AI 代理協作 9 天完成；2026 年 8 月起由單一主 Claude 工作階段依據行為對等差距台賬維護專案，並由一位人類擁有者審查與合併。[完整故事 ↓](#完全由-ai-代理建構)

## ❤️ 贊助商

> [想出現在這裡嗎？](https://github.com/tunsuy)

<table>
  <tr>
    <td width="180"><a href="https://www.orcarouter.ai/ref/ref_49eee7ba2e9927450075"><img src="assets/sponsors/orcarouter.png" alt="OrcaRouter" width="150"></a></td>
    <td><strong><a href="https://www.orcarouter.ai/ref/ref_49eee7ba2e9927450075">OrcaRouter</a></strong>——一個閘道，所有模型。OrcaRouter 以供應商價格將每個請求路由到 200+ 模型（前沿與開源）中最適合的一個，具備故障轉移與請求日誌。它相容 OpenAI，可直接接入 Glaude——確切設定見 <a href="#api-供應商">API 供應商</a>。</td>
  </tr>
</table>

## 功能特性

- **互動式 TUI** — 以 [Bubble Tea](https://github.com/charmbracelet/bubbletea) 打造的全功能終端機 UI，支援深色/淺色主題
- **代理式工具使用** — 35 個內建工具（檔案、命令、搜尋、Web、任務、子代理等），所有操作均經過 9 層權限管線審批
- **Code Mode** — 模型撰寫 JavaScript/TypeScript，在一次沙箱執行中組合多個工具呼叫（純 Go [moejs](https://github.com/MoeForge/moejs) 引擎）：replay、可复用 snippet、寫入審計、批次批准——見 [Code Mode](#code-mode)
- **CLI 子命令** — `glaude mcp …`、`glaude plugin …`、`glaude auth …`、`glaude doctor`、`glaude update`；`mcp` 與 `plugin` 群組的輸出與 TypeScript CLI 位元組級一致
- **外掛管理** — 從本地市集安裝/啟用/停用外掛，完整 manifest 驗證（`glaude plugin validate`）
- **多代理協調** — 為平行任務啟動背景子代理
- **MCP 支援** — 透過 [Model Context Protocol](https://modelcontextprotocol.io) 接入外部工具（stdio + HTTP + WebSocket 傳輸），可從 Claude Desktop 匯入；伺服器推送通知以事件驅動喚醒代理（零輪詢）
- **GLAUDE.md 記憶** — 自動沿目錄樹向上載入 `GLAUDE.md` 檔案的專案上下文
- **工作階段管理** — 恢復先前的對話；長歷史自動壓縮
- **Vim 模式** — 輸入區可選 Vim 按鍵綁定
- **OAuth + API 金鑰認證** — 以 Anthropic OAuth 登入（`glaude auth login`）或提供 `ANTHROPIC_API_KEY`
- **內建斜線命令** — `/compact`、`/commit`、`/review`、`/model`、`/theme` 等（見下方）
- **串流回應** — 即時 token 串流，含思考區塊顯示

## 架構設計

Glaude 分為六層：

```
┌─────────────────────────────────────┐
│  CLI (cmd/glaude)                   │  cobra 進入點
├─────────────────────────────────────┤
│  TUI (internal/tui)                 │  Bubble Tea v2 MVU 介面
├─────────────────────────────────────┤
│  Tools (internal/tools/*)           │  檔案、命令、搜尋、MCP、CodeMode
├─────────────────────────────────────┤
│  Core Engine (internal/engine)      │  串流、工具分發、協調器
├─────────────────────────────────────┤
│  Services (internal/api, oauth,     │  Anthropic API、OAuth、MCP 用戶端
│            mcp, compact)            │
├─────────────────────────────────────┤
│  Infra (pkg/types, internal/config, │  型別、設定、狀態、hooks、外掛
│         state, session, hooks)      │
└─────────────────────────────────────┘
```

詳細拆解見 [`docs/project/architecture.md`](docs/project/architecture.md)。

## 環境要求

- Go 1.26 或更高版本
- [Anthropic API 金鑰](https://console.anthropic.com/)**或** Claude.ai 帳號（OAuth）

## 安裝

### 從原始碼建構

```bash
git clone https://github.com/tunsuy/glaude.git
cd glaude
make build
# 二進位檔位於 ./bin/glaude
```

加入 `PATH`：

```bash
export PATH="$PATH:$(pwd)/bin"
```

### 使用 `go install`

```bash
go install github.com/tunsuy/glaude/cmd/glaude@latest
```

## 快速開始

```bash
# 設定 API 金鑰（或使用 OAuth——見下方認證）
export ANTHROPIC_API_KEY=sk-ant-...

# 在目前目錄啟動互動工作階段
claude

# 問一個一次性問題後結束
glaude -p "解釋這個專案的主要進入點"

# 恢復最近的工作階段
glaude --resume
```

## 認證

**API 金鑰（建議用於 CI/腳本）：**

```bash
export ANTHROPIC_API_KEY=sk-ant-...
# 或永久儲存：
glaude auth login --api-key sk-ant-...
```

**OAuth（建議用於互動使用）：**

```bash
glaude auth login    # 在瀏覽器開啟 OAuth 流程
glaude auth status   # 查看目前登入身分
```

## API 供應商

Glaude 支援多個 API 供應商，除了 Anthropic API，也能使用 OpenAI 相容 API。

### 支援的供應商

| 供應商 | 說明 | 環境變數 |
|--------|------|----------|
| `direct`（預設） | Anthropic 直接 API | `ANTHROPIC_API_KEY`、`ANTHROPIC_BASE_URL` |
| `openai` | OpenAI 與相容 API | `OPENAI_API_KEY`、`OPENAI_BASE_URL` |
| `bedrock` | AWS Bedrock | 透過環境提供 AWS 憑證 |
| `vertex` | Google Cloud Vertex AI | 透過環境提供 GCP 憑證 |

### 使用 OpenAI 相容 API

要使用 OpenAI、DeepSeek、Qwen、Moonshot 或任何 OpenAI 相容 API：

**方法 1：環境變數**

```bash
# 設定供應商為 openai
export CLAUDE_PROVIDER=openai

# 設定 API 金鑰
export OPENAI_API_KEY=sk-xxx

# 可選：自訂 base URL（OpenAI 相容服務）
export OPENAI_BASE_URL=https://api.deepseek.com  # DeepSeek
# export OPENAI_BASE_URL=https://api.moonshot.cn/v1  # Moonshot
# export OPENAI_BASE_URL=http://localhost:11434/v1  # Ollama

# 設定模型
export OPENAI_MODEL=deepseek-chat

# 執行 Claude Code
claude
```

**方法 2：設定檔**

建立或編輯 `~/.config/claude-code/settings.json`：

```json
{
  "provider": "openai",
  "apiKey": "sk-xxx",
  "baseUrl": "https://api.openai.com",
  "model": "gpt-4-turbo",
  "openaiOrganization": "org-xxx",
  "openaiProject": "proj-xxx"
}
```

### 速率限制

客戶端速率控制統一適用於所有供應商。兩個變數皆選填；未設＝不限速（429 自動退避重试仍生效）。

| 環境變數 | 說明 |
|----------|------|
| `CLAUDE_CODE_API_RPM` | 每分鐘請求上限（請求啟動嚴格間距，如 `60`＝間距 ≥1 秒） |
| `CLAUDE_CODE_API_MAX_CONCURRENT` | 同時 in-flight 請求上限（含串流；平行 sub-agent 超過上限會排队） |

```bash
# 建議速率受限的供應商（如 Qwen）：
export CLAUDE_CODE_API_MAX_CONCURRENT=2   # 擋平行 sub-agent 爆量
export CLAUDE_CODE_API_RPM=60             # 選填：請求節流
```

仍踩 429 的請求會自動指數退避重试；provider 的 `retry-after` header 優先。

### 各供應商注意事項

**OpenAI：**
- 支援所有 GPT-4 與 GPT-3.5 模型
- 完整工具/函式呼叫支援
- 串流回應

**DeepSeek：**
```bash
export CLAUDE_PROVIDER=openai
export OPENAI_API_KEY=sk-xxx
export OPENAI_BASE_URL=https://api.deepseek.com
export OPENAI_MODEL=deepseek-chat
```

**Ollama（本地）：**
```bash
export CLAUDE_PROVIDER=openai
export OPENAI_BASE_URL=http://localhost:11434/v1
export OPENAI_MODEL=llama3
```

**Azure OpenAI：**
```bash
export CLAUDE_PROVIDER=openai
export OPENAI_API_KEY=your-azure-key
export OPENAI_BASE_URL=https://your-resource.openai.azure.com
export OPENAI_MODEL=your-deployment-name
```

**OrcaRouter（AI 閘道）：**
[OrcaRouter](https://www.orcarouter.ai/ref/ref_49eee7ba2e9927450075) 是 AI 閘道，以供應商價格將每個請求路由到 200+ 模型（前沿與開源）中最適合的一個，具備故障轉移與請求日誌。它相容 OpenAI，可直接透過 `openai` 供應商使用：

```bash
export CLAUDE_PROVIDER=openai
export OPENAI_API_KEY=<your-orcarouter-key>
export OPENAI_BASE_URL=https://api.orcarouter.ai/v1
export OPENAI_MODEL=orcarouter/auto   # 讓 OrcaRouter 逐請求挑選模型
```

## 使用說明

### 互動模式

```
glaude [flags]
```

| 旗標 | 說明 |
|------|------|
| `--resume` | 恢復最近的工作階段 |
| `--session <id>` | 恢復指定 ID 的工作階段 |
| `--model <name>` | 覆寫預設 Claude 模型 |
| `--dark` / `--light` | 強制深色/淺色主題 |
| `--vim` | 啟用 Vim 按鍵綁定 |
| `-p, --print <prompt>` | 非互動：執行單一 prompt 後結束 |

### CLI 子命令

非互動管理命令。`mcp` 與 `plugin` 群組的輸出與 TypeScript CLI 位元組級相容：

| 命令 | 說明 |
|------|------|
| `glaude mcp add <name> -- <command> [args…]` | 註冊 stdio MCP 伺服器（遠端用 `--transport http <name> <url>`，環境變數用 `-e KEY=VAL`） |
| `glaude mcp list` / `glaude mcp get <name>` | 列出已設定伺服器 / 健康檢查單一台 |
| `glaude mcp add-json <name> <json>` | 以原始 JSON 註冊伺服器 |
| `glaude mcp add-from-claude-desktop` | 從 Claude Desktop 設定匯入伺服器 |
| `glaude mcp remove <name>` / `reset-project-choices` | 移除伺服器 / 重置專案級選擇 |
| `glaude plugin install <plugin>` | 從已設定市集安裝外掛 |
| `glaude plugin list` / `enable` / `disable` / `uninstall` / `update` | 管理已安裝外掛 |
| `glaude plugin validate <path>` | 驗證外掛或市集 manifest（`--strict`、`--json`） |
| `glaude plugin marketplace add/list/remove/update` | 管理外掛市集（本地路徑） |
| `glaude auth login` / `logout` / `status` | OAuth 或 API 金鑰認證 |
| `glaude doctor` | 環境健康檢查 |

### 斜線命令

在輸入框打 `/` 查看所有可用命令：

| 命令 | 說明 |
|------|------|
| `/help` | 顯示所有命令 |
| `/clear` | 清除對話歷史 |
| `/compact` | 摘要歷史以降低上下文用量 |
| `/exit` | 離開 Claude Code |
| `/model` | 顯示或設定作用中模型 |
| `/theme` | 切換色彩主題 |
| `/vim` | 切換 Vim 按鍵綁定 |
| `/effort` | 設定努力等級（low / medium / high） |
| `/status` | 顯示工作階段狀態 |
| `/cost` | 顯示本工作階段 token 用量 |
| `/session` | 顯示目前工作階段 ID |
| `/memory` | 顯示已載入的 GLAUDE.md 記憶檔案 |
| `/dream` | 手動觸發記憶鞏固 |
| `/review` | 請 Claude 審查目前 git diff |
| `/commit` | 請 Claude 撰寫並建立 git commit |
| `/diff` | 顯示目前 git diff |
| `/init` | 為本專案產生 GLAUDE.md |

`/config`、`/mcp`、`/resume`、`/terminal-setup` 均已完整實作（工作階段恢復、MCP 狀態、互動式設定、終端機按鍵設定）。

## Code Mode

模型不再「一次 LLM 往返呼叫一個工具」，而是**撰寫 JavaScript/TypeScript，在一次沙箱執行中組合多個工具呼叫**——`CodeMode` 工具，基於純 Go 的 [moejs](https://github.com/MoeForge/moejs) 引擎（無 CGO）。中間資料留在沙箱內，只有回傳值與 `console.log` 輸出回到模型（更少往返、更省上下文）。

```js
// 由模型撰寫、在沙箱中執行：
const src = await tools.Read({ file_path: "main.go" });
const todos = src.split("\n").filter(l => l.includes("TODO"));
console.log("found", todos.length);
return { count: todos.length, lines: todos };
```

### 沙箱 API

| 沙箱內 | 作用 |
|--------|------|
| `tools.<Name>(input)` | 呼叫任何已註冊工具（非同步）。`Promise.all` 平行呼叫。**每次呼叫都走完整權限管線。** |
| `codemode.search(query)` / `describe(name)` | 漸進式工具發現（關鍵字排名 → 按需取完整 schema） |
| `codemode.history(limit)` | 近期執行記錄（id、code、結果）供复用 |
| `codemode.replay(id)` | 重跑某次歷史執行——權限照常審批 |
| `codemode.run(name)` / `snippets()` | 執行/列出具名可复用 snippet |
| `codemode.audit(id)` | 某次執行記錄的寫副作用清單 |
| `codemode.approvals()` / `revoke(tool)` | 列出/撤銷 CodeMode 範圍的持久批准 |
| `import { x } from "name"` | 受控共享模組（host 管理目錄） |
| `console.log(...)` | 擷取輸出 |

工具輸入欄位：`code`、`typescript`（型別剝離編譯）、`replay`（依歷史 id 重跑）、`save_as`（成功執行存為具名 snippet）、`batch_writes`（全部寫操作一次批准）。

### 安全模型

- **沙箱不等於授權**——每個內部工具呼叫走與直接呼叫相同的 9 層權限管線。無檔案系統/網路/env/`eval` 存取；執行受逾時與記憶體上限約束。
- **批次批准**——工具彈窗詢問而你批准後，本次執行內對*同一*工具的後續呼叫預批准（僅執行 scope）。規則直接放行每次重查，Shift+Tab 切換權限模式立即生效。拒絕永不快取。
- **持久批准**——權限對話框提供「在 CodeMode 內永遠允許 *tool*」：CodeMode 範圍、使用者親自授予、可撤銷（`codemode.revoke`）。Deny 規則永遠優先。
- **Write 閘門**——`batch_writes: true` 時，第一個寫呼叫詢問一次（附執行級範圍說明），決定涵蓋本次執行的所有寫操作；讀操作走正常流程。
- **結果 spill**——超過 32 KiB 的輸出寫入檔案，模型拿到 4 KiB 預覽＋檔案指標（可 `Read`/`Grep`）。

### 工作流功能

- **Replay**：依 id 重跑任何歷史執行（`replay` 輸入或 `codemode.replay`），歷史帶 `replay_of` 溯源。
- **Snippets**：把成功執行沉澱為具名可复用腳本（`save_as` → `codemode.run`）。snippet 存全量程式碼；巢狀執行共享呼叫方的批准 scope（遞迴深度上限 3）。
- **寫入審計**：每個實際執行的寫呼叫都記錄（工具＋輸入摘要）——誠實追溯*改了什麼*，`codemode.audit` 可查。

執行期資料位於 `~/.glaude/codemode/`：`history.jsonl`、`spill/`、`modules/`、`snippets/`、`approvals.json`。

### 事件驅動待命（零輪詢）

MCP 伺服器推送通知會路由進訊息佇列並自動喚醒代理——不跑輪詢迴圈燒 token。搭配 Code Mode：腳本完成握手/訂閱，代理待命，服務通知觸發下一次執行（可复用 snippet 邏輯）。

**已知局限**：無自動回滾（審計清單告訴你改了什麼；git checkpoint 還原評估中）、replay 使用記錄的程式碼（≤2000 字元——長工作流請存 snippet）、write 閘門基於首個具體寫＋範圍聲明而非兩階段完整計畫預覽。

設計細節：[`docs/project/design/codemode.md`](docs/project/design/codemode.md)。

## 完全由 AI 代理建構

> **本倉庫沒有任何一行生產程式碼是人類寫的。**

整個專案——架構設計、詳細設計文件、平行實作、程式碼審查、QA 與整合測試——由 **9 個 Claude AI 代理**在結構化多代理工作流中協作產出：

```
PM Agent          →  專案計畫、里程碑、任務排程
Tech Lead Agent   →  架構設計、設計文件審查、程式碼審查
Agent-Infra       →  基礎設施層（型別、設定、狀態、工作階段）
Agent-Services    →  服務層（API 用戶端、OAuth、MCP、壓縮）
Agent-Core        →  核心引擎（LLM 迴圈、工具分發、協調器）
Agent-Tools       →  工具層（檔案、命令、搜尋、Web——建構期 18 個工具）
Agent-TUI         →  UI 層（Bubble Tea MVU、主題、Vim 模式）
Agent-CLI         →  進入層（Cobra CLI、DI、bootstrap 階段）
QA Agent          →  測試策略、逐層驗收、整合測試
```

每個代理在獨立的 Git Worktree 分支上平行工作，透過共享程式碼庫、設計文件與 QA 報告協作。成果：**9 天內約 7,000 行生產程式碼＋完整測試套件**，`go test -race ./...` 全過。

這是真實世界的展示：一個非平凡的多層 Go 應用，能完全由 AI 代理非同步協作完成設計、實作、審查與交付。2026 年 8 月起專案以同樣精神維護——單一主 Claude 工作階段依據對照原版 TypeScript CLI 的行為對等差距台賬工作，一位人類擁有者審查與合併。依然零人工生產程式碼（今日約 33,000 行）。完整決策軌跡在 [`docs/project/`](docs/project/)。

## 開發指南

### 先決條件

- Go 1.26+
- `golangci-lint`（可選，用於 lint）

### 建構與測試

```bash
# 建構
make build

# 執行所有測試
make test

# 執行測試並產出覆蓋率報告
make test-cover

# Vet
make vet

# Lint（需要 golangci-lint）
make lint

# 建構 + 測試 + vet
make all
```

### 專案布局

```
glaude/
├── cmd/glaude/          # CLI 進入點
├── internal/
│   ├── api/             # Anthropic API 用戶端與串流
│   ├── bootstrap/       # 應用初始化
│   ├── commands/        # 斜線命令處理器
│   ├── compact/         # 對話壓縮
│   ├── config/          # 設定（檔案 + 環境）
│   ├── coordinator/     # 多代理協調器
│   ├── codemode/        # Code Mode 沙箱引擎（moejs）
│   ├── engine/          # 查詢引擎、工具分發
│   ├── hooks/           # 工具前/後 hooks
│   ├── mcp/             # MCP 伺服器管理
│   ├── memdir/          # GLAUDE.md 載入器
│   ├── oauth/           # OAuth2 流程
│   ├── permissions/     # 工具權限層
│   ├── plugin/          # 外掛系統
│   ├── session/         # 工作階段持久化
│   ├── state/           # 應用狀態
│   ├── tools/           # 工具介面、註冊表與內建實作
│   │   ├── agent/       #   子代理與訊息工具
│   │   ├── codemode/    #   CodeMode 工具（沙箱 JS/TS 工具編排）
│   │   ├── fileops/     #   檔案讀寫/編輯/glob/grep 工具
│   │   ├── interact/    #   使用者互動與 worktree 工具
│   │   ├── mcp/         #   MCP 工具適配器
│   │   ├── misc/        #   其他工具
│   │   ├── shell/       #   Bash 執行工具
│   │   ├── tasks/       #   任務清單工具
│   │   └── web/         #   web fetch 與搜尋工具
│   └── tui/             # Bubble Tea UI 元件
├── pkg/
│   └── types/           # 共享公開型別
├── docs/                # 設計文件與 QA 報告
├── Makefile
└── go.mod
```

## 路線圖

初始建構（2026 年 4 月）9 天交付完整六層架構。2026 年 8 月起專案進入**維護模式**：開發由行為對等差距台賬（[`test/parity/cases.md`](test/parity/cases.md)）驅動，對照原版 TypeScript CLI——每個關閉的差距由測試位元組級釘住，每個刻意分歧記錄為 `waived` 並附理由。

近期里程碑：

- **2026-11-18** — Code Mode v1→v3（沙箱 JS/TS 工具編排：批次批准、TypeScript、歷史、spill、受控 import、replay、snippet、寫入審計、持久批准、write 閘門）；MCP 事件驅動喚醒（伺服器通知→代理，零輪詢）；内核搬回本倉（無外部引擎依賴）；Bubble Tea v2 遷移；26 個使用者可見樁全部回填（債務：0 樁）
- **2026-09-05** — `mcp`（7 個命令）與 `plugin`（8 個命令 + `marketplace` 子樹）CLI 命令組落地，輸出與 TS 原版位元組級一致；使用者可見樁 42 → 27
- **2026-08-30** — `--version` 格式對等；對照 TS CLI 的完整子命令盤點（§B11）
- **2026-08-29** — 維護模式流程落地：CI 強制債務紅線、parity 測試骨架

原始階段計畫（v0.2.0 → v1.0.0）連同各階段完成狀態保留在 [`docs/ROADMAP.md`](docs/ROADMAP.md)。

### 目前狀態

```
✅ 已完成: 核心引擎 + 串流、TUI、35 個工具 + Code Mode 沙箱（v3）、
          17 個斜線命令、mcp/plugin/auth/doctor CLI 子命令、9 層權限管線、
          三級上下文壓縮（snip/micro/auto）、OAuth、工作階段持久化、
          MCP 事件驅動喚醒（零輪詢）
🔧 模式:  維護期——以差距台賬驅動的對等修復（對照 TypeScript CLI）
📉 欠賬:  11 個 TODO(dep) / 0 個使用者可見樁，CI 紅線只降不升
🚀 發布:  v0.8.0，5 平台二進位檔
```

## 貢獻指南

歡迎貢獻！提交 pull request 前請先閱讀 [CONTRIBUTING.md](CONTRIBUTING.md)。

快速檢查清單：
- Fork 倉庫並建立功能分支
- 確保 `make test` 與 `make vet` 通過
- 為新功能撰寫測試
- 遵循現有程式碼風格（執行 `gofmt ./...`）
- 使用提供的模板開啟 pull request

## 安全性

回報安全漏洞請見 [SECURITY.md](SECURITY.md)。**不要**為安全 bug 開啟公開 GitHub issue。

## 授權

本專案採 MIT 授權——詳見 [LICENSE](LICENSE)。

## 相關專案

- [claude-code](https://github.com/anthropics/claude-code) — 原版 TypeScript CLI
- [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go) — 官方 Go SDK
- [Model Context Protocol](https://modelcontextprotocol.io) — 連接 AI 與工具的開放標準
