<p align="center">
  <img src="assets/logo.png" alt="Claude Code Go Logo" width="200">
</p>

<h1 align="center">Claude Code Go</h1>

<p align="center">
  <strong>🤖 A Go reimplementation of Claude Code — AI coding assistant in your terminal</strong>
</p>

<p align="center">
  <a href="https://golang.org/dl/"><img src="https://img.shields.io/badge/go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go Version"></a>
  <a href="https://goreportcard.com/report/github.com/tunsuy/claude-code-go"><img src="https://goreportcard.com/badge/github.com/tunsuy/claude-code-go?style=flat-square" alt="Go Report Card"></a>
  <a href="https://codecov.io/gh/tunsuy/claude-code-go"><img src="https://codecov.io/gh/tunsuy/claude-code-go/branch/main/graph/badge.svg?style=flat-square" alt="codecov"></a>
  <a href="https://pkg.go.dev/github.com/tunsuy/claude-code-go"><img src="https://pkg.go.dev/badge/github.com/tunsuy/claude-code-go.svg" alt="Go Reference"></a>
  <a href="https://github.com/tunsuy/claude-code-go/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/tunsuy/claude-code-go/ci.yml?branch=main&style=flat-square&logo=github&label=CI" alt="CI"></a>
  <a href="https://github.com/tunsuy/claude-code-go/releases"><img src="https://img.shields.io/github/v/release/tunsuy/claude-code-go?style=flat-square&logo=github" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green?style=flat-square" alt="License"></a>
  <a href="https://github.com/tunsuy/claude-code-go/pulls"><img src="https://img.shields.io/badge/PRs-welcome-brightgreen?style=flat-square" alt="PRs Welcome"></a>
</p>

<p align="center">
  <a href="README.md">English</a> •
  <a href="README.zh-CN.md">简体中文</a> •
  <a href="README.ja.md">日本語</a> •
  <a href="README.ko.md">한국어</a> •
  <a href="README.es.md">Español</a> •
  <a href="README.fr.md">Français</a>
</p>

---

<p align="center">
  <img src="assets/image.png" alt="Claude Code Go - Interactive Session" width="800">
  <br>
  <em>Interactive TUI with file reading and real-time thinking display</em>
</p>

<p align="center">
  <img src="assets/image1.png" alt="Claude Code Go - Project Analysis" width="800">
  <br>
  <em>Comprehensive project analysis with architecture breakdown</em>
</p>

---

## What is this?

This project is a **complete Go reimplementation of [Claude Code](https://claude.ai/code)** — Anthropic's official TypeScript CLI — rewritten module-by-module in Go, covering all core features: TUI, tool use, permission system, multi-agent coordination, MCP protocol, session management, and more. It understands your codebase, runs tools, and helps you write, review, and refactor code through natural conversation.

> **🤖 Built entirely by AI agents — zero human-written production code.** The initial build (April 2026) was produced by 9 Claude AI agents collaborating in 9 days; since August 2026 a single main Claude session maintains the project against a behavior-parity ledger, with one human owner reviewing and merging. [Full story ↓](#built-entirely-by-ai-agents)

## ❤️ Sponsors

> [Want to be featured here?](https://github.com/tunsuy)

<table>
  <tr>
    <td width="180"><a href="https://www.orcarouter.ai/ref/ref_49eee7ba2e9927450075"><img src="assets/sponsors/orcarouter.png" alt="OrcaRouter" width="150"></a></td>
    <td><strong><a href="https://www.orcarouter.ai/ref/ref_49eee7ba2e9927450075">OrcaRouter</a></strong> — one gateway, every model. OrcaRouter routes each request to the best-fit model among 200+ (frontier and open-source) models at provider prices, with failover and request logs. It is OpenAI-compatible and plugs straight into Claude Code Go — see <a href="#api-providers">API Providers</a> for the exact configuration.</td>
  </tr>
</table>

## Features

- **Interactive TUI** — Full-featured terminal UI built with [Bubble Tea](https://github.com/charmbracelet/bubbletea), with dark/light themes
- **Agentic tool use** — 35 built-in tools (file, shell, search, web, tasks, sub-agents, …), all mediated through a 9-layer permission pipeline
- **Code Mode** — The model writes JavaScript/TypeScript that composes many tool calls in ONE sandboxed execution (pure-Go [moejs](https://github.com/MoeForge/moejs) engine): replay, reusable snippets, write auditing, batch approvals — see [Code Mode](#code-mode)
- **CLI subcommands** — `claude mcp …`, `claude plugin …`, `claude auth …`, `claude doctor`, `claude update`; the `mcp` and `plugin` groups produce byte-for-byte identical output to the TypeScript CLI
- **Plugin management** — Install/enable/disable plugins from local marketplaces, with full manifest validation (`claude plugin validate`)
- **Multi-agent coordination** — Spawn background sub-agents for parallel tasks
- **MCP support** — Connect external tools via the [Model Context Protocol](https://modelcontextprotocol.io) (stdio + HTTP + WebSocket transports), importable from Claude Desktop; server push notifications wake the agent event-driven (zero polling)
- **CLAUDE.md memory** — Auto-loads project context from `CLAUDE.md` files up the directory tree
- **Session management** — Resume previous conversations; compact long histories automatically
- **Vim mode** — Optional Vim key bindings in the input area
- **OAuth + API key auth** — Sign in with Anthropic OAuth (`claude auth login`) or supply an `ANTHROPIC_API_KEY`
- **Built-in slash commands** — `/compact`, `/commit`, `/review`, `/model`, `/theme`, and more (see below)
- **Streaming responses** — Real-time token streaming with thinking-block display

## Architecture

Claude Code Go is organized in six layers:

```
┌─────────────────────────────────────┐
│  CLI (cmd/claude)                   │  cobra entry point
├─────────────────────────────────────┤
│  TUI (internal/tui)                 │  Bubble Tea v2 MVU interface
├─────────────────────────────────────┤
│  Tools (internal/tools/*)           │  file, shell, search, MCP, CodeMode
├─────────────────────────────────────┤
│  Core Engine (internal/engine)      │  streaming, tool dispatch, coordinator
├─────────────────────────────────────┤
│  Services (internal/api, oauth,     │  Anthropic API, OAuth, MCP client
│            mcp, compact)            │
├─────────────────────────────────────┤
│  Infra (pkg/types, internal/config, │  types, config, state, hooks, plugins
│         state, session, hooks)      │
└─────────────────────────────────────┘
```

See [`docs/project/architecture.md`](docs/project/architecture.md) for a detailed breakdown.

## Requirements

- Go 1.26 or later
- An [Anthropic API key](https://console.anthropic.com/) **or** Claude.ai account (OAuth)

## Installation

### From source

```bash
git clone https://github.com/tunsuy/claude-code-go.git
cd claude-code-go
make build
# Binary is placed at ./bin/claude
```

Add to your `PATH`:

```bash
export PATH="$PATH:$(pwd)/bin"
```

### Using `go install`

```bash
go install github.com/tunsuy/claude-code-go/cmd/claude@latest
```

## Quick Start

```bash
# Set your API key (or use OAuth — see Authentication below)
export ANTHROPIC_API_KEY=sk-ant-...

# Start an interactive session in the current directory
claude

# Ask a one-shot question and exit
claude -p "Explain the main entry point of this project"

# Resume the most recent session
claude --resume
```

## Authentication

**API key (recommended for CI/scripts):**

```bash
export ANTHROPIC_API_KEY=sk-ant-...
# or store it persistently:
claude auth login --api-key sk-ant-...
```

**OAuth (recommended for interactive use):**

```bash
claude auth login    # opens the OAuth flow in your browser
claude auth status   # check who you're signed in as
```

## API Providers

Claude Code Go supports multiple API providers, allowing you to use not just Anthropic's API, but also OpenAI-compatible APIs.

### Supported Providers

| Provider | Description | Environment Variables |
|----------|-------------|----------------------|
| `direct` (default) | Anthropic Direct API | `ANTHROPIC_API_KEY`, `ANTHROPIC_BASE_URL` |
| `openai` | OpenAI & compatible APIs | `OPENAI_API_KEY`, `OPENAI_BASE_URL` |
| `bedrock` | AWS Bedrock | AWS credentials via environment |
| `vertex` | Google Cloud Vertex AI | GCP credentials via environment |

### Using OpenAI-Compatible APIs

To use OpenAI, DeepSeek, Qwen, Moonshot, or any OpenAI-compatible API:

**Method 1: Environment Variables**

```bash
# Set provider to openai
export CLAUDE_PROVIDER=openai

# Set your API key
export OPENAI_API_KEY=sk-xxx

# Optionally set a custom base URL (for OpenAI-compatible services)
export OPENAI_BASE_URL=https://api.deepseek.com  # DeepSeek
# export OPENAI_BASE_URL=https://api.moonshot.cn/v1  # Moonshot
# export OPENAI_BASE_URL=http://localhost:11434/v1  # Ollama

# Set the model
export OPENAI_MODEL=deepseek-chat

# Run Claude Code
claude
```

**Method 2: Configuration File**

Create or edit `~/.config/claude-code/settings.json`:

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

### Provider-Specific Notes

**OpenAI:**
- Supports all GPT-4 and GPT-3.5 models
- Full tool/function calling support
- Streaming responses

**DeepSeek:**
```bash
export CLAUDE_PROVIDER=openai
export OPENAI_API_KEY=sk-xxx
export OPENAI_BASE_URL=https://api.deepseek.com
export OPENAI_MODEL=deepseek-chat
```

**Ollama (local):**
```bash
export CLAUDE_PROVIDER=openai
export OPENAI_BASE_URL=http://localhost:11434/v1
export OPENAI_MODEL=llama3
```

**Azure OpenAI:**
```bash
export CLAUDE_PROVIDER=openai
export OPENAI_API_KEY=your-azure-key
export OPENAI_BASE_URL=https://your-resource.openai.azure.com
export OPENAI_MODEL=your-deployment-name
```

**OrcaRouter (AI gateway):**
[OrcaRouter](https://www.orcarouter.ai/ref/ref_49eee7ba2e9927450075) is an AI gateway that routes each request to the best-fit model among 200+ (frontier and open-source) models at provider prices, with failover and request logs. It is OpenAI-compatible, so it works directly through the `openai` provider:

```bash
export CLAUDE_PROVIDER=openai
export OPENAI_API_KEY=<your-orcarouter-key>
export OPENAI_BASE_URL=https://api.orcarouter.ai/v1
export OPENAI_MODEL=orcarouter/auto   # let OrcaRouter pick the model per request
```

## Usage

### Interactive mode

```
claude [flags]
```

| Flag | Description |
|------|-------------|
| `--resume` | Resume the most recent session |
| `--session <id>` | Resume a specific session by ID |
| `--model <name>` | Override the default Claude model |
| `--dark` / `--light` | Force dark or light theme |
| `--vim` | Enable Vim key bindings |
| `-p, --print <prompt>` | Non-interactive: run a single prompt and exit |

### CLI subcommands

Non-interactive management commands. The `mcp` and `plugin` groups are byte-for-byte output-compatible with the TypeScript CLI:

| Command | Description |
|---------|-------------|
| `claude mcp add <name> -- <command> [args…]` | Register a stdio MCP server (`--transport http <name> <url>` for remote, `-e KEY=VAL` for env) |
| `claude mcp list` / `claude mcp get <name>` | List configured servers / health-check one |
| `claude mcp add-json <name> <json>` | Register a server from raw JSON |
| `claude mcp add-from-claude-desktop` | Import servers from Claude Desktop's config |
| `claude mcp remove <name>` / `reset-project-choices` | Remove a server / reset project-local choices |
| `claude plugin install <plugin>` | Install a plugin from a configured marketplace |
| `claude plugin list` / `enable` / `disable` / `uninstall` / `update` | Manage installed plugins |
| `claude plugin validate <path>` | Validate a plugin or marketplace manifest (`--strict`, `--json`) |
| `claude plugin marketplace add/list/remove/update` | Manage plugin marketplaces (local paths) |
| `claude auth login` / `logout` / `status` | OAuth or API-key authentication |
| `claude doctor` | Environment health check |

### Slash commands

Type `/` in the input to see all available commands:

| Command | Description |
|---------|-------------|
| `/help` | Show all commands |
| `/clear` | Clear conversation history |
| `/compact` | Summarise history to reduce context usage |
| `/exit` | Exit Claude Code |
| `/model` | Show or set the active model |
| `/theme` | Switch color theme |
| `/vim` | Toggle Vim key bindings |
| `/effort` | Set effort level (low / medium / high) |
| `/status` | Show session status |
| `/cost` | Show token usage for this session |
| `/session` | Show current session ID |
| `/memory` | Show loaded CLAUDE.md memory files |
| `/dream` | Manually trigger memory consolidation |
| `/review` | Review the current git diff with Claude |
| `/commit` | Ask Claude to write and create a git commit |
| `/diff` | Show the current git diff |
| `/init` | Generate a CLAUDE.md for this project |

`/config`, `/mcp`, `/resume`, and `/terminal-setup` are fully implemented (session resume, MCP status, interactive settings, terminal keybinding setup).

## Code Mode

Instead of one LLM round trip per tool call, the model writes **JavaScript or TypeScript that composes many tool calls in a single sandboxed execution** — the `CodeMode` tool, backed by the pure-Go [moejs](https://github.com/MoeForge/moejs) engine (no CGO). Intermediate data stays inside the sandbox; only the return value and `console.log` output come back to the model (fewer round trips, less context).

```js
// Written by the model, executed in the sandbox:
const src = await tools.Read({ file_path: "main.go" });
const todos = src.split("\n").filter(l => l.includes("TODO"));
console.log("found", todos.length);
return { count: todos.length, lines: todos };
```

### Sandbox API

| Inside the sandbox | What it does |
|--------------------|--------------|
| `tools.<Name>(input)` | Call any registered tool (async). `Promise.all` runs calls in parallel. **Every call goes through the full permission pipeline.** |
| `codemode.search(query)` / `describe(name)` | Progressive tool discovery (ranked names → full schema on demand) |
| `codemode.history(limit)` | Recent executions (id, code, outcome) for reuse |
| `codemode.replay(id)` | Re-run a recorded execution — permissions still apply |
| `codemode.run(name)` / `snippets()` | Run / list named reusable snippets |
| `codemode.audit(id)` | Write side effects recorded for one execution |
| `codemode.approvals()` / `revoke(tool)` | List / revoke persistent CodeMode-scoped approvals |
| `import { x } from "name"` | Controlled shared modules from a host-managed directory |
| `console.log(...)` | Captured output |

Tool input fields: `code`, `typescript` (type-stripping compile), `replay` (re-run by history id), `save_as` (save a successful run as a named snippet), `batch_writes` (one approval for all writes).

### Safety model

- **Sandboxing is not authorization** — every inner tool call runs the same 9-layer permission pipeline as a direct call. No filesystem, network, env, or `eval` access; execution is bounded by a timeout and a memory limit.
- **Batch approval** — when a tool prompts and you approve, later calls to the *same* tool within that execution are pre-approved (execution scope only). Rule-based allows are re-checked on every call, so live permission-mode changes (Shift+Tab) apply immediately. Denials are never cached.
- **Persistent approvals** — the permission dialog offers "always allow *tool* inside CodeMode": a CodeMode-scoped, user-granted, revocable approval (`codemode.revoke`). Deny rules always win over it.
- **Write gate** — with `batch_writes: true`, the first write call asks once (with an execution-wide scope note); the decision covers all writes of that execution. Reads keep the normal flow.
- **Result spill** — outputs over 32 KiB are written to a file; the model gets a 4 KiB preview plus a pointer it can `Read`/`Grep`.

### Workflow features

- **Replay**: re-run any recorded execution by id (`replay` input or `codemode.replay`), with full provenance (`replay_of`) in the history.
- **Snippets**: turn successful runs into named, reusable scripts (`save_as` → `codemode.run`). Snippets store the full code; nested runs share the caller's approval scope (recursion depth capped at 3).
- **Write audit**: every executed write call is recorded (`tool` + input summary) per execution — an honest trail of *what changed*, queryable via `codemode.audit`.

Runtime data lives under `~/.claude/codemode/`: `history.jsonl`, `spill/`, `modules/`, `snippets/`, `approvals.json`.

### Event-driven standby (zero polling)

MCP server push notifications are routed into the message queue and wake the agent automatically — no polling loops burning tokens. Combined with Code Mode: a script performs the handshake/subscription, the agent idles, and the service's notification triggers the next run (which can reuse snippet logic).

**Known limitations**: no automatic rollback (the audit trail tells you what changed; git-checkpoint restore is under evaluation), replay uses the recorded code (≤2000 chars — save long workflows as snippets), and the write gate approves on the first concrete write plus a scope note rather than a full two-phase plan preview.

Design details: [`docs/project/design/codemode.md`](docs/project/design/codemode.md).

## Built entirely by AI agents

> **No human wrote a single line of production code in this repository.**

The entire project — architecture design, detailed design docs, parallel implementation, code review, QA, and integration testing — was produced by **9 Claude AI agents** collaborating in a structured multi-agent workflow:

```
PM Agent          →  project plan, milestones, task scheduling
Tech Lead Agent   →  architecture design, design-doc review, code review
Agent-Infra       →  infrastructure layer (types, config, state, session)
Agent-Services    →  services layer (API client, OAuth, MCP, compaction)
Agent-Core        →  core engine (LLM loop, tool dispatch, coordinator)
Agent-Tools       →  tools layer (file, shell, search, web — 18 tools at build time)
Agent-TUI         →  UI layer (Bubble Tea MVU, themes, Vim mode)
Agent-CLI         →  entry layer (Cobra CLI, DI, bootstrap phases)
QA Agent          →  test strategy, per-layer acceptance, integration tests
```

Each agent worked on an isolated Git Worktree branch in parallel, collaborating through the shared codebase, design docs, and QA reports. The result: ~**7,000 lines of production code + a full test suite in 9 days**, with `go test -race ./...` passing.

This is a real-world demonstration that a non-trivial, multi-layer Go application can be fully designed, implemented, reviewed, and shipped by AI agents collaborating asynchronously. Since August 2026 the project is maintained in the same spirit — a single main Claude session working through a behavior-parity gap ledger against the original TypeScript CLI, with one human owner reviewing and merging. Still zero human-written production code (~33,000 lines today). The complete decision trail lives in [`docs/project/`](docs/project/).

## Development

### Prerequisites

- Go 1.26+
- `golangci-lint` (optional, for linting)

### Build & test

```bash
# Build
make build

# Run all tests
make test

# Run tests with coverage report
make test-cover

# Vet
make vet

# Lint (requires golangci-lint)
make lint

# Build + test + vet
make all
```

### Project layout

```
claude-code-go/
├── cmd/claude/          # CLI entry point
├── internal/
│   ├── api/             # Anthropic API client & streaming
│   ├── bootstrap/       # App initialisation
│   ├── commands/        # Slash command handlers
│   ├── compact/         # Conversation compaction
│   ├── config/          # Configuration (file + env)
│   ├── coordinator/     # Multi-agent coordinator
│   ├── codemode/        # Code Mode sandbox engine (moejs)
│   ├── engine/          # Query engine, tool dispatch
│   ├── hooks/           # Pre/post-tool hooks
│   ├── mcp/             # MCP server management
│   ├── memdir/          # CLAUDE.md loader
│   ├── oauth/           # OAuth2 flow
│   ├── permissions/     # Tool permission layer
│   ├── plugin/          # Plugin system
│   ├── session/         # Session persistence
│   ├── state/           # Application state
│   ├── tools/           # Tool interface, registry & built-in implementations
│   │   ├── agent/       #   sub-agent & send-message tools
│   │   ├── codemode/    #   CodeMode tool (sandboxed JS/TS tool composition)
│   │   ├── fileops/     #   file read/write/edit/glob/grep tools
│   │   ├── interact/    #   user-interaction & worktree tools
│   │   ├── mcp/         #   MCP tool adapter
│   │   ├── misc/        #   miscellaneous tools
│   │   ├── shell/       #   Bash execution tool
│   │   ├── tasks/       #   task-list tools
│   │   └── web/         #   web fetch & search tools
│   └── tui/             # Bubble Tea UI components
├── pkg/
│   └── types/           # Shared public types
├── docs/                # Design docs and QA reports
├── Makefile
└── go.mod
```

## Roadmap

The initial build (April 2026) delivered the full six-layer architecture in 9 days. Since August 2026 the project is in **maintenance mode**: development is driven by a behavior-parity gap ledger ([`test/parity/cases.md`](test/parity/cases.md)) that compares this CLI against the original TypeScript version — every closed gap is byte-pinned by tests, every deliberate divergence is recorded as `waived` with a reason.

Recent milestones:

- **2026-11-18** — Code Mode v1→v3 (sandboxed JS/TS tool composition: batch approval, TypeScript, history, spill, controlled imports, replay, snippets, write audit, persistent approvals, write gate); event-driven MCP wake (server notifications → agent, zero polling); kernel re-internalized (no external engine dependency); Bubble Tea v2 migration; all 26 remaining user-visible stubs backfilled (debt: 0 stubs)
- **2026-09-05** — `mcp` (7 commands) and `plugin` (8 commands + `marketplace` subtree) CLI groups landed, output byte-identical to the TS CLI; user-visible stubs reduced 42 → 27
- **2026-08-30** — `--version` format parity; full subcommand inventory against the TS CLI (§B11)
- **2026-08-29** — maintenance-mode process landed: CI-enforced debt red lines, parity test skeleton

The original phased plan (v0.2.0 → v1.0.0) is preserved with per-phase completion status in [`docs/ROADMAP.md`](docs/ROADMAP.md).

### Current Status

```
✅ Done:  Core engine + streaming, TUI, 35 tools + Code Mode sandbox (v3),
          17 slash commands, mcp/plugin/auth/doctor CLI subcommands,
          9-layer permission pipeline, context compaction (snip/micro/auto),
          OAuth, session persistence, event-driven MCP wake (zero polling)
🔧 Mode:  Maintenance — gap-ledger-driven parity work vs. the TypeScript CLI
📉 Debt:  11 TODO(dep) / 0 user-visible stubs, CI-enforced only-down red lines
🚀 Release: v0.8.0, 5-platform binaries
```

## Contributing

We welcome contributions! Please read [CONTRIBUTING.md](CONTRIBUTING.md) before submitting a pull request.

Quick checklist:
- Fork the repo and create a feature branch
- Make sure `make test` and `make vet` pass
- Write tests for new functionality
- Follow existing code style (run `gofmt ./...`)
- Open a pull request using the provided template

## Security

To report a security vulnerability, please see [SECURITY.md](SECURITY.md). **Do not** open a public GitHub issue for security bugs.

## License

This project is licensed under the MIT License — see [LICENSE](LICENSE) for details.

## Related projects

- [claude-code](https://github.com/anthropics/claude-code) — the original TypeScript CLI
- [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go) — official Go SDK for the Anthropic API
- [Model Context Protocol](https://modelcontextprotocol.io) — open standard for connecting AI to tools
