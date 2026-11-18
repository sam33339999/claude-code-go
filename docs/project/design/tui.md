# TUI 层详细设计

> 负责 Agent：Agent-TUI
> 状态：已实现（维护期回填中）
> 版本：v1.3
> 日期：2026-11-18（初稿 2026-04-02）

---

## 目录

1. [BubbleTea 架构总览](#1-bubbletea-架构总览)
2. [internal/tui — 主界面](#2-internaltui--主界面)
3. [QueryEngine 接口依赖](#3-queryengine-接口依赖)
4. [internal/commands — Slash 命令系统](#4-internalcommands--slash-命令系统)
5. [internal/coordinator — 多 Agent 协调 UI](#5-internalcoordinator--多-agent-协调-ui)
6. [internal/memdir — 内存文件加载](#6-internalmemdir--内存文件加载)
7. [TS → Go 组件映射表](#7-ts--go-组件映射表)
8. [设计决策](#8-设计决策)

---

## 1. BubbleTea 架构总览

BubbleTea 采用 Elm 架构（Model-Update-View），与 React/Ink 的组件树模式相比，整个应用只有一个顶层 `Model`，状态变更通过 `Update()` 产生新 Model（不可变更新），副作用由 `tea.Cmd` 封装。

### 根 Model 结构

```go
// internal/tui/model.go

package tui

import (
    "github.com/charmbracelet/bubbletea"
    "claude-code-go/internal/state"
    "claude-code-go/pkg/types"
)

// AppModel 是整个 TUI 应用的根 Model，对应 TS 中 REPL.tsx 的组件状态聚合。
// TS 原版通过数十个 useState/useRef hook 分散管理状态；Go 版本统一到此结构体。
type AppModel struct {
    // --- 会话状态 ---
    sessionID   string
    messages    []types.Message      // 完整消息历史（含 assistant/user/system/tool）
    inputText   string               // 当前输入框内容
    isLoading   bool                 // 正在等待 LLM 响应
    abortFn     context.CancelFunc   // 当前请求的取消函数，nil 表示空闲

    // --- UI 子视图状态 ---
    activeDialog  dialogKind           // 当前弹出的对话框（权限/compact/退出等）
    permReq       *PermissionRequest   // 待确认的工具权限请求
    transcriptMode bool                // 是否处于 Transcript 模式（只读回放）
    showSpinner   bool                 // 是否显示加载动画

    // --- 滚动与布局 ---
    termWidth   int
    termHeight  int
    scrollOffset int                  // 消息列表滚动偏移（行数）
    pinnedToBottom bool               // 是否自动跟随最新输出

    // --- Slash 命令 ---
    commandRegistry *commands.Registry // 已注册命令表
    commandResult   *CommandResult     // 上一次命令的展示结果

    // --- 多 Agent ---
    coordinatorMode bool
    agentTasks      map[string]AgentTaskState // taskID → 状态

    // --- 样式 ---
    theme  lipgloss.Theme

    // --- 依赖（通过构造器注入）---
    queryEngine QueryEngine   // TODO(dep): 等待 Agent-Core #6
    appState    *state.Store
}

// dialogKind 枚举当前激活的模态对话框
type dialogKind int
const (
    dialogNone dialogKind = iota
    dialogPermission
    dialogCompact
    dialogExit
    dialogConfig
    dialogCostThreshold
)
```

**对应关系**：TS `REPL.tsx` 中散布在数十个 `useState` 的字段，对应 `AppModel` 的各字段；`useRef` 用于跨渲染持有可变引用，Go 中用 `*T` 指针字段或 `sync.Mutex` 保护的值替代。

---

### 消息类型（Msg）定义

BubbleTea 中的 `Msg` 是 `interface{}`，相当于 TS 中各种事件（键盘输入、流式 token 到达、权限请求等）。

```go
// internal/tui/messages.go

// --- 用户输入 ---
type InputSubmittedMsg struct{ Text string }     // 用户按下 Enter 提交
type InputChangedMsg  struct{ Text string }     // 输入框内容变化（Tab 补全等）
type SlashCommandMsg  struct{ Name string; Args string }

// --- LLM 流式响应 ---
// 对应 TS handleMessageFromStream 产生的各类事件
type StreamTokenMsg       struct{ Delta string }          // 文本增量
type StreamThinkingMsg    struct{ Delta string }          // Thinking block
type StreamToolUseStartMsg struct {
    ToolUseID string
    ToolName  string
    Input     json.RawMessage
}
type StreamToolUseDoneMsg  struct{ ToolUseID string }
type StreamDoneMsg         struct{ FinalMessage types.AssistantMessage }
type StreamErrorMsg        struct{ Err error }

// --- 权限请求（工具调用前） ---
type PermissionRequestMsg struct {
    ToolUseID   string
    ToolName    string
    Input       json.RawMessage
    RespondCh   chan<- PermissionDecision
}
type PermissionResponseMsg struct {
    Decision PermissionDecision
}

// --- 系统事件 ---
type TermResizedMsg  struct{ Width, Height int }  // tea.WindowSizeMsg 的包装
type TickMsg         struct{ Time time.Time }      // 周期 tick（Spinner 动画 / Agent 状态刷新）
type CompactDoneMsg  struct{ Summary string }      // 上下文压缩完成
type AgentStatusMsg  struct {                      // 子 Agent 状态推送
    TaskID string
    Status AgentStatus
}
```

**类比**：TS `REPL.tsx` 中通过 `setState` 触发重渲染，Go 中通过向 `tea.Program` 发送对应 `Msg` 触发 `Update()`。

---

### Update 逻辑流程

```go
// internal/tui/update.go

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {

    case tea.WindowSizeMsg:
        m.termWidth, m.termHeight = msg.Width, msg.Height
        return m, nil

    case tea.KeyMsg:
        return m.handleKey(msg)

    case InputSubmittedMsg:
        // 1. 检查是否 slash 命令 → 走命令执行路径
        if strings.HasPrefix(msg.Text, "/") {
            return m.handleSlashCommand(msg.Text)
        }
        // 2. 追加用户消息到列表
        m.messages = append(m.messages, types.NewUserMessage(msg.Text))
        m.isLoading = true
        m.pinnedToBottom = true
        // 3. 启动 LLM 查询 Cmd
        return m, m.startQueryCmd(msg.Text)

    case StreamTokenMsg:
        // 追加 delta 到最后一条 assistant 消息
        m = m.appendStreamDelta(msg.Delta)
        return m, nil

    case StreamToolUseStartMsg:
        m.messages = append(m.messages, types.NewToolUseMessage(msg))
        return m, nil

    case PermissionRequestMsg:
        // 暂停流、展示权限对话框
        m.activeDialog = dialogPermission
        m.permReq = &PermissionRequest{Msg: msg}
        return m, nil

    case PermissionResponseMsg:
        m.activeDialog = dialogNone
        m.permReq = nil
        return m, sendPermissionResponseCmd(msg.Decision)

    case StreamDoneMsg:
        m.isLoading = false
        m.messages = append(m.messages, msg.FinalMessage)
        return m, nil

    case StreamErrorMsg:
        m.isLoading = false
        m.messages = append(m.messages, types.NewErrorMessage(msg.Err))
        return m, nil

    case TickMsg:
        // Spinner 帧推进 + Agent 状态过期清理
        return m.handleTick(msg.Time)

    case AgentStatusMsg:
        m.agentTasks[msg.TaskID] = msg.Status
        return m, nil

    case CompactDoneMsg:
        // 替换消息列表为压缩摘要 + 边界标记
        return m.applyCompact(msg.Summary), nil
    }

    return m, nil
}
```

**要点**：每个 `Update` 分支返回 `(新Model, Cmd)`。`Cmd` 是异步工作单元，执行后产生新 `Msg` 再次进入 `Update`，形成事件循环。这与 TS 的 `useEffect` + `setState` 模式完全等价。

---

### View 渲染层次

```
AppModel.View()
├── StatusBar             // 顶部：模型名称 / CWD / Token 用量 / 权限模式
├── MessageList           // 主体：历史消息滚动列表
│   ├── UserTextMessage
│   ├── AssistantTextMessage  // Markdown 渲染
│   ├── AssistantThinkingMessage
│   ├── ToolUseMessage        // 工具调用展示
│   │   ├── BashToolMessage
│   │   ├── FileEditMessage
│   │   └── ...（其他工具）
│   └── SystemMessage
├── SpinnerRow            // 加载动画行（仅 isLoading 时显示）
├── [PermissionDialog]    // 模态：工具权限确认（覆盖输入区）
├── [CompactDialog]       // 模态：/compact 确认
├── [ExitDialog]          // 模态：退出确认
├── PromptInput           // 底部：输入框 + 命令提示
│   ├── InputField
│   ├── SlashCommandSuggestions  // 弹出候选列表
│   └── QueuedCommandsDisplay
└── CoordinatorTaskPanel  // 仅 coordinatorMode: 子 Agent 状态栏
```

渲染时各区域用 `lipgloss` 计算宽高，`strings.Join` 拼接各行，最终 `View()` 返回完整字符串交给 BubbleTea 刷新终端。

---

## 2. internal/tui — 主界面

### REPL 主界面组件

`internal/tui/` 是整个 TUI 层的核心包，对应 TS `src/screens/REPL.tsx`。

```
internal/tui/
├── model.go        // AppModel 结构体定义
├── update.go       // Update() 主分发函数
├── view.go         // View() 渲染入口，组装各子视图
├── messages.go     // 所有 Msg 类型定义
├── cmds.go         // tea.Cmd 工厂函数（startQueryCmd / tickCmd 等）
├── keys.go         // 键盘事件处理（handleKey）
├── input.go        // 输入框子 Model（PromptInput）
├── messagelist.go  // 消息列表渲染
├── spinner.go      // 加载动画
├── statusbar.go    // 顶部状态栏
├── permissions.go  // 权限对话框子 Model
├── theme.go        // Lip Gloss 样式定义
└── init.go         // New() 构造器 + Init()
```

**Init() 函数**：

```go
func (m AppModel) Init() tea.Cmd {
    return tea.Batch(
        m.loadMemdirCmd(),       // 异步加载 CLAUDE.md
        tea.EnterAltScreen,      // 进入备用屏幕（全屏模式）
        tickCmd(),               // 启动 1s 周期 tick
        listenWindowSize(),      // 监听终端 resize
    )
}
```

---

### 流式输出渲染机制

TS 中通过 `query()` 函数返回 AsyncGenerator，REPL 用 `for await` 消费后 `setState` 更新；Go 版本将相同语义映射到 `tea.Cmd` + channel：

```go
// internal/tui/cmds.go

// startQueryCmd 启动 LLM 流式查询，将 channel 事件转为 tea.Msg 推送到 Program
func (m AppModel) startQueryCmd(input string) tea.Cmd {
    return func() tea.Msg {
        // 1. 调用 QueryEngine（依赖注入，来自 Agent-Core #6）
        stream, err := m.queryEngine.Submit(context.Background(), input, m.messages)
        if err != nil {
            return StreamErrorMsg{Err: err}
        }
        // 2. 返回第一个事件（后续事件通过 Program.Send 推入）
        return waitForStreamEvent(stream)
    }
}

// waitForStreamEvent 从 channel 读取下一个事件并转为对应 Msg
// 每次 Update 收到 StreamTokenMsg 后，再 dispatch 此 Cmd，形成"拉取"循环
func waitForStreamEvent(stream <-chan QueryEngine.Event) tea.Cmd {
    return func() tea.Msg {
        event, ok := <-stream
        if !ok {
            return StreamDoneMsg{}
        }
        switch e := event.(type) {
        case QueryEngine.TokenEvent:
            return StreamTokenMsg{Delta: e.Text}
        case QueryEngine.ThinkingEvent:
            return StreamThinkingMsg{Delta: e.Text}
        case QueryEngine.ToolUseStartEvent:
            return StreamToolUseStartMsg{ToolUseID: e.ID, ToolName: e.Name, Input: e.Input}
        case QueryEngine.PermissionRequestEvent:
            return PermissionRequestMsg{
                ToolUseID: e.ToolUseID,
                ToolName:  e.ToolName,
                Input:     e.Input,
                RespondCh: e.RespondCh,
            }
        case QueryEngine.ErrorEvent:
            return StreamErrorMsg{Err: e.Err}
        }
        return nil
    }
}
```

**关键设计**：每次 `Update` 处理完一个 `StreamTokenMsg` 后，返回 `waitForStreamEvent(stream)` 作为新 Cmd，BubbleTea 继续执行，形成无阻塞的流式消费循环。这等价于 TS 的 `for await ... setState`，但不需要 goroutine 直接操作 UI。

---

### 权限确认对话框

对应 TS `src/components/permissions/PermissionRequest.tsx`，当工具调用需要用户确认时弹出。

```go
// internal/tui/permissions.go

type PermissionDialog struct {
    toolName  string
    toolInput json.RawMessage
    options   []string         // ["Yes, allow once", "Yes, always", "No"]
    cursor    int
    respondCh chan<- PermissionDecision
}

func (d PermissionDialog) View(width int) string {
    // 渲染工具名称、参数摘要、选项列表
    // 用 lipgloss.NewStyle().Border(lipgloss.RoundedBorder()) 包裹
}

// 在 AppModel.Update 中，收到 tea.KeyMsg{Type: tea.KeyEnter} 时：
// → 向 respondCh 发送 decision，关闭对话框，恢复流式消费
```

**权限类型映射**（对应 TS `permissionComponentForTool`）：

| 工具 | 权限对话框样式 |
|------|---------------|
| BashTool | 展示完整命令，高亮危险操作 |
| FileEditTool | 展示文件路径 + diff 预览 |
| FileWriteTool | 展示文件路径 + 新内容摘要 |
| WebFetchTool | 展示 URL |
| 其他工具 | 通用 FallbackPermissionRequest |

---

### 加载动画与状态栏

**Spinner（对应 TS `src/components/Spinner.tsx`）**：

```go
// internal/tui/spinner.go

type SpinnerModel struct {
    frames   []string          // e.g. ["⠋","⠙","⠹","⠸","⠼","⠴","⠦","⠧","⠇","⠏"]
    current  int
    verb     string            // 动态动词："Thinking" / "Searching" / "Writing"...
    elapsed  time.Duration
    mode     SpinnerMode       // normal | brief | teammate
}

func (s SpinnerModel) View() string {
    frame := s.frames[s.current % len(s.frames)]
    return lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render(frame) +
           " " + s.verb + "… " +
           dimStyle.Render(formatDuration(s.elapsed))
}
```

**StatusBar（对应 TS `src/components/StatusLine.tsx`）**：

```go
// internal/tui/statusbar.go

type StatusBar struct {
    model       string      // 当前模型名称
    cwd         string      // 工作目录
    tokenUsage  TokenUsage  // input/output token 计数
    permMode    string      // "default" | "auto" | "bypass"
    sessionName string
    cost        float64
}

func (s StatusBar) View(width int) string {
    left  := lipgloss.JoinHorizontal(lipgloss.Left,
        modelStyle.Render(s.model),
        dimStyle.Render(" | "),
        s.cwd,
    )
    right := dimStyle.Render(fmt.Sprintf(
        "%s tokens · $%.4f", formatTokens(s.tokenUsage), s.cost,
    ))
    gap := width - lipgloss.Width(left) - lipgloss.Width(right)
    return left + strings.Repeat(" ", max(0, gap)) + right
}
```

---

### 主题/样式系统（Lip Gloss）

对应 TS 中 `ThemeProvider` + `useTheme()` 的设计，Go 版本通过在 `AppModel` 中持有 `Theme` 结构体，各子视图渲染时从 Model 取样式：

```go
// internal/tui/theme.go

type Theme struct {
    // 语义色
    Primary     lipgloss.Color  // 用户消息、主交互元素
    Secondary   lipgloss.Color  // Assistant 消息
    Accent      lipgloss.Color  // Spinner、进度
    Muted       lipgloss.Color  // 时间戳、提示文字
    Error       lipgloss.Color
    Warning     lipgloss.Color
    Success     lipgloss.Color

    // 代码高亮（Markdown 渲染）
    CodeBG      lipgloss.Color
    CodeFG      lipgloss.Color

    // 工具调用
    ToolName    lipgloss.Color
    ToolInput   lipgloss.Color
}

var DefaultDarkTheme = Theme{
    Primary:   lipgloss.Color("12"),   // bright blue
    Secondary: lipgloss.Color("7"),    // white
    Accent:    lipgloss.Color("205"),  // pink
    Muted:     lipgloss.Color("8"),    // dark gray
    Error:     lipgloss.Color("9"),
    Warning:   lipgloss.Color("11"),
    Success:   lipgloss.Color("10"),
    CodeBG:    lipgloss.Color("236"),
    CodeFG:    lipgloss.Color("252"),
    ToolName:  lipgloss.Color("14"),
    ToolInput: lipgloss.Color("7"),
}

// 通过 /theme 命令切换内置主题
var BuiltinThemes = map[string]Theme{
    "dark":  DefaultDarkTheme,
    "light": DefaultLightTheme,
    "tokyo-night": TokyoNightTheme,
}
```

---

## 3. QueryEngine 接口依赖

```go
// internal/tui/query_engine.go

// TODO(dep): 等待 Agent-Core #6 定义 QueryEngine 接口后补全
// 预期接口：

package tui

import (
    "context"
    "claude-code-go/pkg/types"
)

// QueryEngine 是 TUI 层对 Core 层的唯一依赖接口。
// TUI 通过此接口提交用户输入并接收流式事件，不感知底层 LLM API、工具系统实现。
//
// TODO(dep): Agent-Core #6 负责实现此接口，当前以 stub 占位。
type QueryEngine interface {
    // Submit 提交一次用户输入，返回事件 channel。
    // channel 关闭表示本轮对话结束（正常/错误/中止）。
    Submit(ctx context.Context, input string, history []types.Message) (<-chan Event, error)

    // Abort 中止当前正在进行的查询（对应用户按 Ctrl+C / Esc）。
    Abort()

    // IsRunning 返回当前是否有活跃查询。
    IsRunning() bool
}

// Event 是 QueryEngine 推送给 TUI 的流式事件（sum type 模拟）
type Event interface{ isQueryEvent() }

type TokenEvent        struct{ Text string }
type ThinkingEvent     struct{ Text string }
type ToolUseStartEvent struct {
    ID    string
    Name  string
    Input json.RawMessage
}
type ToolUseDoneEvent  struct{ ID string }
type PermissionRequestEvent struct {
    ToolUseID string
    ToolName  string
    Input     json.RawMessage
    RespondCh chan<- PermissionDecision  // TUI 写入决策，Core 读取
}
type ErrorEvent  struct{ Err error }
type DoneEvent   struct{ Usage types.Usage }

// 实现私有标记方法（模拟 sealed interface）
func (TokenEvent) isQueryEvent()             {}
func (ThinkingEvent) isQueryEvent()          {}
func (ToolUseStartEvent) isQueryEvent()      {}
func (ToolUseDoneEvent) isQueryEvent()       {}
func (PermissionRequestEvent) isQueryEvent() {}
func (ErrorEvent) isQueryEvent()             {}
func (DoneEvent) isQueryEvent()              {}

// PermissionDecision 用户对权限请求的决策
type PermissionDecision int
const (
    PermissionAllow      PermissionDecision = iota // 允许一次
    PermissionAlwaysAllow                          // 永久允许此工具
    PermissionDeny                                 // 拒绝
)

// stubQueryEngine 开发阶段占位实现，仅回显输入
type stubQueryEngine struct{}

func (s *stubQueryEngine) Submit(_ context.Context, input string, _ []types.Message) (<-chan Event, error) {
    ch := make(chan Event, 2)
    go func() {
        ch <- TokenEvent{Text: "[stub] echo: " + input}
        ch <- DoneEvent{}
        close(ch)
    }()
    return ch, nil
}
func (s *stubQueryEngine) Abort()     {}
func (s *stubQueryEngine) IsRunning() bool { return false }
```

---

## 4. internal/commands — Slash 命令系统

对应 TS `src/commands.ts` + `src/commands/` 目录，实现 `/` 开头的交互式命令系统。

### 命令注册接口

```go
// internal/commands/registry.go

package commands

// Command 是一个可执行的 slash 命令描述符
type Command struct {
    Name        string              // 不含 "/" 前缀，如 "clear"
    Aliases     []string            // 别名
    Description string
    Hidden      bool                // 不在 /help 中显示

    // Execute 执行命令。返回 Result 供 TUI 层展示，或修改 AppModel。
    // 纯展示性命令返回 ResultDisplay；修改状态的命令返回 tea.Cmd。
    Execute func(ctx CommandContext) (Result, tea.Cmd)
}

// CommandContext 命令执行时的上下文（注入依赖，避免全局变量）
type CommandContext struct {
    Args        string              // 命令参数（"/clear foo" → "foo"）
    Messages    []types.Message     // 当前消息历史
    AppState    *state.Store
    QueryEngine QueryEngine         // 部分命令需要触发查询
    CWD         string
}

// Result 命令执行结果
type Result struct {
    Display  *ResultDisplay         // 可选：在消息列表中插入一条系统消息
    Error    error
}

// ResultDisplay 在消息流中展示命令结果
type ResultDisplay struct {
    Text      string
    IsError   bool
    Permanent bool               // false → 短暂显示后消失
}

// Registry 维护所有已注册命令
type Registry struct {
    commands map[string]*Command  // name → Command
}

func (r *Registry) Register(cmd *Command) { ... }
func (r *Registry) Lookup(name string) (*Command, bool) { ... }
func (r *Registry) All() []*Command { ... }
// CompletePrefix 用于 Tab 自动补全
func (r *Registry) CompletePrefix(prefix string) []*Command { ... }
```

### 内置命令列表

| Slash 命令 | 对应 TS 命令目录 | 功能描述 |
|-----------|----------------|---------|
| `/clear` | `commands/clear/` | 清空当前会话消息历史，重置状态 |
| `/compact [instructions]` | `commands/compact/` | 压缩上下文（调用 QueryEngine 生成摘要）|
| `/config` | `commands/config/` | 打开配置交互界面 |
| `/help` | `commands/help/` | 展示所有可用命令列表 |
| `/exit` | `commands/exit/` | 退出程序（弹出确认对话框）|
| `/memory` | `commands/memory/` | 管理 CLAUDE.md 记忆文件 |
| `/model` | `commands/model/` | 切换 LLM 模型 |
| `/theme` | `commands/theme/` | 切换 TUI 配色主题 |
| `/vim` | `commands/vim/` | 切换 Vim 键位模式 |
| `/status` | `commands/status/` | 展示会话状态摘要 |
| `/cost` | `commands/cost/` | 展示当前会话 API 费用统计 |
| `/session` | `commands/session/` | 会话管理（列表/切换/恢复）|
| `/mcp` | `commands/mcp/` | MCP 服务器管理 |
| `/resume` | `commands/resume/` | 恢复上一个会话 |
| `/diff` | `commands/diff/` | 展示当前会话文件修改 diff |
| `/init` | `commands/init.ts` | 初始化项目（生成 CLAUDE.md）|
| `/review` | `commands/review.ts` | 代码 review 模式 |
| `/commit` | `commands/commit.ts` | 生成 commit message 并提交 |
| `/terminal-setup` | `commands/terminalSetup/` | 终端环境配置检测 |

### 命令解析与执行流程

```
用户输入 "/compact --keep-recent 10 focus on tests"
         │
         ▼
input.go: isSlashCommand() 检测 "/" 前缀
         │
         ▼
parseSlashInput(text) → {name: "compact", args: "--keep-recent 10 focus on tests"}
         │
         ▼
registry.Lookup("compact") → *Command
         │
         ▼
cmd.Execute(CommandContext{Args: "--keep-recent 10 ...", ...})
         │
    ┌────┴───────────┐
    │ 纯 UI 命令      │ 需要异步/状态命令
    │ (返回 Result)   │ (返回 tea.Cmd)
    └────┬───────────┘
         │
         ▼
Update() 中 case CommandResultMsg: → 更新 m.commandResult → View() 展示
```

**Tab 补全**：输入框检测 `/` 前缀时，调用 `registry.CompletePrefix(partial)` 返回候选列表，用 `SlashCommandSuggestions` 组件渲染在输入框上方弹出层。

### 4.1 BF-03：/mcp 与 /resume 的真实实现与依赖注入（2026-11-18）

> 注意：上文 §4 的 `Command`/`CommandContext`/`Result` 代码块为初稿契约；实际生效的契约以
> `internal/commands/registry.go` 为准（`Execute func(ctx CommandContext, args string) Result`，
> 无 tea.Cmd 返回值；副作用经 `Result` 标志位由 `applyCommandResult` 驱动）。

**/mcp（异步状态报告，MVU）**：

- `commands.MCPServerLister` 是 /mcp 所需的最小只读池视图接口（`GetAll() []*mcp.ServerConnection`），
  agtkeel `*mcp.Pool` 天然满足（编译期断言钉住）；测试注入 fake。
- `/mcp` 的 Execute 只返回 `Result{Display: DisplayNone, FetchMCPStatus: true}`（新增标志位，
  仿照 `TriggerDream` 先例）。`applyCommandResult` 据此返回 `fetchMCPStatusCmd(m.mcpPool)`。
- `fetchMCPStatusCmd`（tui/cmds.go）在 Update 循环之外调用 `commands.FormatMCPStatus(ctx, pool)`
  渲染报告：每个 server 一行（名称、transport、✓ connected / ✗ failed / ⚠ needs authentication /
  … pending / ○ disabled），connected 的 server 附带 `ListTools` 工具数（每 server 2s 超时、
  整体 15s 上限，失败降级为 "tools unavailable"）。结果经新增 `MCPStatusMsg{Text}` 回到
  Update，作为系统消息展示。空池/nil 池输出友好提示。
- 选择异步的原因：`ListTools` 是网络 I/O，禁止在 Update() 内阻塞。
- **并发约束（CR-BF03-2，BF-03-F1 钉住）**：`GetAll` 返回新切片但元素是池内共享的
  **活 `*mcp.ServerConnection` 指针**；`FormatMCPStatus` 读取 `Status`/`Error`/`Client`/`Config`
  无同步。**调用方必须保证读取期间无并发池突变**（Connect/Reconnect/Disconnect 等写者不得
  与读取并发）。渲染侧已做廉价防御：每个连接值拷贝一次（单行字段读取瞬时一致），但这不是
  同步手段，约束仍然有效。根治（`GetAll` 返回值快照）在 agtkeel 侧，跨仓任务 **BF-19**；
  **BF-01 引入任何后台连接/健康检查/重连 goroutine 之前必须关闭该根治或显式复核本约束，
  否则升级为 P0**（评审定级说明）。
- **活状态依赖（CR-BF03-5）**：交互池的活连接状态依赖 **BF-01 的启动接线**；BF-01 落地前
  交互池无连接写者（wire.go 延迟连接、internal/tools/mcp 仍为桩），/mcp 恒显示
  "No MCP servers configured"（即使配置了 server）——此为**预期行为**，非功能缺陷。

**/resume（列表 + 按 ID 恢复）**：

- 裸 `/resume`：`listSessions(cwd, 10)` 扫描 `<cwd>/.claude/projects/<ProjectHash>/*.jsonl`
  （与 CLI `--continue` 同一布局，agtkeel `session`/`utils/fs` 直接复用），按 mtime 倒序取前 10，
  每行展示 id、修改时间、消息数、首条用户消息片段（60 rune 截断），并提示 `/resume <id>`。
- `/resume <id>`：`loadSessionMessages` 经 `session.Resume` + `ReadAll` 提取 user/assistant
  消息（跳过损坏/无关条目，与 bootstrap/session.go 的 CLI 路径行为一致，但在 commands 层独立实现，
  不 import bootstrap）。ID 做路径穿越校验（拒绝 `/`、`\`、`..`）。
  BF-03-F1 起错误处理略优于 CLI parity：`session.Resume` 底层错误经 `%w` 保留于友好文案中
  （CR-BF03-3），`mgr.Close` 失败显式返回错误（与 `listSessions` 的 ReadAll+Close 模式一致，
  CR-BF03-4）；用户可见的提示文案（not found + Tip）不变。
- 成功时返回 `Result{NewMessages: []types.Message, NewSessionID: id, Text: 确认文案}`。
  `NewMessages` 为既有 interface{} 字段（本次首次被 TUI 消费）；`NewSessionID` 为新增字段。
- `applyCommandResult` 对 `NewMessages` 做 `[]types.Message` 类型断言后：替换 `m.messages`、
  调用 `queryEngine.SetMessages(msgs)`（既有引擎能力，无需新契约）、更新 `m.sessionID`。
  **查询进行中（isLoading）拒绝恢复**，提示先按 Esc 中断。/clear 的遗留 `[]interface{}{}`
  值断言失败被忽略，行为不变。BF-03-F1 起该消费逻辑提取为私有 helper `applyResumeResult`
  （返回 stop 标志：仅 isLoading 拒绝时为 true，调用方据此短路后续 Result 处理），
  Display 渲染提取为 `applyDisplayResult`，使 `applyCommandResult` ≤80 行（CR-BF03-1）；
  行为与提取前逐路径一致。
- 已知局限：交互模式目前不落盘会话 JSONL（持久化接线属 bootstrap/engine 职责，见 BF-03 汇报），
  故 /resume 列表内容取决于其他路径写入的会话文件。

**/config（只读设置面板，BF-05）**：

- `cmdConfig` 返回 `Result{OpenDialog: "config"}`（既有 wiring），TUI 打开 `dialogConfig`，
  `renderConfigDialog` 渲染当前设置：Model（AppState.MainLoopModel）、Theme（dark/light）、
  Vim mode、Effort、Working dir、Session ID。Esc 关闭（既有 keys.go 处理）。
- **只读**：编辑经既有 /model /theme /vim /effort 命令与持久化设置文件（~/.claude/settings.json）；
  面板内交互编辑为文档化的后续项（与原版 /config 的可编辑面板有差距，属已知局限）。
- 纯展示：不触碰引擎历史与工具 schema（cache-first 闸门三问全否，有 DeepEqual 守护测试）。

**/terminal-setup（终端环境检测，BF-05）**：

- `DetectTerminal(lookup)` 纯函数从环境变量识别终端（VS Code/iTerm2/Apple Terminal/WezTerm/
  Ghostty/Windows Terminal/Konsole/GNOME/tmux/screen/generic/unknown）与 shell（SHELL basename）；
  lookup 注入（生产 os.Getenv），测试不改进程环境。未知环境诚实报告，不猜测。
- `FormatTerminalSetup` 输出检测报告＋非侵入指引：多行输入 Ctrl+J 提示、
  `claude completion <shell>` 补全安装指引（cobra 内建命令）。**不写任何文件、不改终端设置**——
  与原版「安装 Shift+Enter 键绑定」有差距，设计规格（§命令表「终端环境配置检测」）以检测为准，
  键绑定自动安装列为后续项。

**依赖注入路径（接口变更，已汇报）**：

- `tui.New(...)` 与内部 `newAppModel` 末尾新增参数 `mcpPool commands.MCPServerLister`（可为 nil），
  与 `memoryStore` 等既有依赖同一注入模式；`AppModel` 新增 `mcpPool` 字段。
- 唯一调用点 `internal/bootstrap/root.go runInteractive` 追加传入 `container.MCPPool`（一行接线）。
- commands 层新增对 agtkeel `mcp`/`session`/`types`/`utils/fs` 的向下依赖（内核库，合法）；
  未引入对 bootstrap / internal/tools 的任何依赖。

---

## 5. internal/coordinator — 多 Agent 协调 UI

对应 TS `src/coordinator/coordinatorMode.ts` + `src/components/CoordinatorAgentStatus.tsx`。

### 协调模式下的 UI 变化

当 `AppModel.coordinatorMode == true` 时（由环境变量 `CLAUDE_CODE_COORDINATOR_MODE` 或 CLI flag 激活）：

1. **底部追加 `CoordinatorTaskPanel`**：展示所有后台 Agent 的实时状态
2. **StatusBar** 增加 "Coordinator" 标记
3. **Spinner** 切换到 `teammate` 模式（展示多 Agent 树形进度）
4. **TeammateViewHeader** 在查看某个子 Agent 会话时显示在顶部

```go
// internal/coordinator/coordinator.go

package coordinator

// AgentStatus 子 Agent 的运行状态快照
type AgentStatus int
const (
    AgentRunning  AgentStatus = iota
    AgentPaused
    AgentCompleted
    AgentFailed
)

// AgentTaskState 对应 TS LocalAgentTaskState
type AgentTaskState struct {
    ID          string
    Name        string         // 来自 agentNameRegistry
    Status      AgentStatus
    StartTime   time.Time
    ElapsedMs   int64
    OutputTokens int
    EvictAfter  *time.Time     // nil = 永不自动隐藏；非 nil = 到期后从面板移除
}

// CoordinatorPanel 对应 TS CoordinatorTaskPanel
// 在 AppModel.View() 末尾条件渲染
type CoordinatorPanel struct {
    Tasks           map[string]AgentTaskState
    SelectedIndex   int
    ViewingTaskID   string     // 当前聚焦查看的子 Agent ID（空 = 看 Leader）
}

func (p CoordinatorPanel) View(width int) string {
    // 渲染：
    // ● [Main]       (当前 Leader 会话)
    // ● agent-1      Running  0:42  1.2k tok
    // ● agent-2      Paused   1:05  0.8k tok  ← 选中高亮
}
```

### 子 Agent 状态展示

```
┌─ Coordinator Panel ────────────────────────────────┐
│  ◉ [Main] (Leader)                                 │
│  ● refactor-agent    ⠼ Running    0:42  1,234 tok  │
│  ● test-agent       ⏸ Paused     1:05    891 tok  │
│  ● doc-agent         ✓ Done       2:11  2,102 tok  │
└────────────────────────────────────────────────────┘
  Enter: 切换查看  x: 解散选中  Tab: 循环选择
```

**键盘交互**：
- `↑`/`↓`：在 Agent 列表中移动光标
- `Enter`：进入选中 Agent 的会话视图（TeammateView）
- `x`：解散/隐藏选中的已完成 Agent（对应 TS `evictTerminalTask`）
- `Esc`：返回 Leader 视图

**状态更新机制**：Agent-Core 通过 `tea.Program.Send(AgentStatusMsg{...})` 向 TUI 推送状态更新，TUI 无需轮询。

---

## 6. internal/memdir — 内存文件加载

对应 TS `src/memdir/` 目录（`memdir.ts`、`paths.ts`、`memoryScan.ts` 等）。

### CLAUDE.md 文件发现逻辑（向上遍历目录树）

TS 原版通过 `getMemoryFiles()` / `loadMemoryPrompt()` 从当前目录向上遍历，收集所有 `CLAUDE.md` 文件：

```go
// internal/memdir/discover.go

package memdir

import (
    "os"
    "path/filepath"
)

const (
    MemoryFileName    = "CLAUDE.md"
    AutoMemFileName   = "MEMORY.md"
    MaxEntrypointLines = 200
    MaxEntrypointBytes = 25_000
)

// DiscoverClaudeMd 从 startDir 向上遍历到 root（home 目录或 git root），
// 收集沿途所有 CLAUDE.md 的路径，顺序为：从最内层→最外层（inner-first）。
// 对应 TS claudemd.ts getMemoryFiles()。
func DiscoverClaudeMd(startDir string) ([]string, error) {
    var files []string
    dir := startDir
    home, _ := os.UserHomeDir()

    for {
        candidate := filepath.Join(dir, MemoryFileName)
        if _, err := os.Stat(candidate); err == nil {
            files = append(files, candidate)
        }

        parent := filepath.Dir(dir)
        // 到达 home 目录或文件系统根后停止
        if dir == parent || dir == home {
            // 还检查 home 目录本身
            if dir == home {
                break
            }
            dir = home
            continue
        }
        dir = parent
    }

    // 全局配置目录：~/.claude/CLAUDE.md（如果不在遍历路径上）
    globalMem := filepath.Join(home, ".claude", MemoryFileName)
    if !contains(files, globalMem) {
        if _, err := os.Stat(globalMem); err == nil {
            files = append(files, globalMem)
        }
    }
    return files, nil
}

// LoadAndTruncate 读取文件内容并按行数/字节数双重截断
// 对应 TS truncateEntrypointContent()
func LoadAndTruncate(path string) (content string, wasTruncated bool, err error) {
    raw, err := os.ReadFile(path)
    if err != nil {
        return "", false, err
    }
    content = string(raw)
    lines := strings.Split(content, "\n")
    if len(lines) > MaxEntrypointLines {
        lines = lines[:MaxEntrypointLines]
        content = strings.Join(lines, "\n")
        wasTruncated = true
    }
    if len(content) > MaxEntrypointBytes {
        // 在最后一个换行符处截断
        cut := strings.LastIndexByte(content[:MaxEntrypointBytes], '\n')
        if cut < 0 { cut = MaxEntrypointBytes }
        content = content[:cut]
        wasTruncated = true
    }
    return content, wasTruncated, nil
}
```

### 加载与注入时机

```go
// internal/memdir/loader.go

// LoadMemoryPrompt 构建注入 LLM 系统提示的记忆内容字符串，
// 对应 TS memdir.ts loadMemoryPrompt()。
// 在每次 QueryEngine.Submit() 前调用，确保记忆文件是最新的。
func LoadMemoryPrompt(cwd string) (string, error) {
    paths, err := DiscoverClaudeMd(cwd)
    if err != nil {
        return "", err
    }

    var sections []string
    for _, p := range paths {
        content, truncated, err := LoadAndTruncate(p)
        if err != nil {
            continue  // 文件消失或无权限，跳过
        }
        header := fmt.Sprintf("<memory_file path=%q>", p)
        if truncated {
            header += " [truncated]"
        }
        sections = append(sections, header+"\n"+content+"\n</memory_file>")
    }
    if len(sections) == 0 {
        return "", nil
    }
    return "<memory>\n" + strings.Join(sections, "\n\n") + "\n</memory>", nil
}
```

**注入流程**：

```
用户提交输入
    │
    ▼
memdir.LoadMemoryPrompt(cwd)     ← 每次查询前重新读取（文件可能变化）
    │
    ▼
QueryEngine.Submit(ctx, input, history, memoryPrompt)
    │
    ▼
Core 层将 memoryPrompt 拼接到 system prompt
    │
    ▼
发送给 Anthropic API
```

**TUI 触发时机对应 TS 行为**：
- `Init()` 时：首次加载，显示已加载的 CLAUDE.md 列表（`/memory show` 效果）
- 每次 `Submit` 前：重新加载（支持实时编辑后生效）
- `/memory` 命令：交互式编辑/刷新记忆文件

---

## 7. TS → Go 组件映射表

| TS 原版组件/文件 | Go 对应模块/类型 | 说明 |
|----------------|----------------|------|
| `screens/REPL.tsx` | `internal/tui/model.go` + `update.go` + `view.go` | 主 REPL 界面，拆分为 Model/Update/View |
| `components/Messages.tsx` | `internal/tui/messagelist.go` | 消息列表渲染 |
| `components/Message.tsx` | `internal/tui/messagelist.go` `MessageView()` | 单条消息渲染分发 |
| `components/messages/AssistantTextMessage.tsx` | `internal/tui/messages/assistant.go` | Assistant 文本消息（含 Markdown）|
| `components/messages/UserTextMessage.tsx` | `internal/tui/messages/user.go` | 用户消息渲染 |
| `components/messages/AssistantThinkingMessage.tsx` | `internal/tui/messages/thinking.go` | Thinking block 渲染 |
| `components/messages/AssistantToolUseMessage.tsx` | `internal/tui/messages/tooluse.go` | 工具调用渲染 |
| `components/messages/UserToolResultMessage.tsx` | `internal/tui/messages/toolresult.go` | 工具结果渲染 |
| `components/PromptInput/PromptInput.tsx` | `internal/tui/input.go` | 输入框（含 Vim 模式、Tab 补全）|
| `components/Spinner.tsx` | `internal/tui/spinner.go` | 加载动画 |
| `components/StatusLine.tsx` | `internal/tui/statusbar.go` | 顶部状态栏 |
| `components/permissions/PermissionRequest.tsx` | `internal/tui/permissions.go` | 权限确认对话框 |
| `components/CoordinatorAgentStatus.tsx` | `internal/coordinator/panel.go` | 多 Agent 状态面板 |
| `components/TaskListV2.tsx` | `internal/coordinator/tasklist.go` | 任务列表子视图 |
| `components/CompactSummary.tsx` | `internal/tui/messages/compact.go` | 压缩边界展示 |
| `commands.ts` + `commands/` | `internal/commands/registry.go` + `commands/` | Slash 命令注册与实现 |
| `coordinator/coordinatorMode.ts` | `internal/coordinator/coordinator.go` | 协调模式逻辑 |
| `memdir/memdir.ts` | `internal/memdir/loader.go` | 记忆文件加载 |
| `memdir/paths.ts` | `internal/memdir/discover.go` | 路径发现逻辑 |
| `memdir/memoryScan.ts` | `internal/memdir/scan.go` | 记忆文件扫描 |
| `QueryEngine.ts` | `internal/tui/query_engine.go`（接口定义）| 接口由 Agent-Core #6 实现 |
| `ink.ts` + `ink/` | `github.com/charmbracelet/bubbletea` + `lipgloss` | 渲染引擎（三方库替代）|
| `components/design-system/ThemeProvider.tsx` | `internal/tui/theme.go` | 主题系统 |
| `components/Markdown.tsx` | `github.com/charmbracelet/glamour` | Markdown 渲染（三方库）|
| `components/StructuredDiff.tsx` | `internal/tui/messages/diff.go` | 文件 diff 展示 |
| `screens/Doctor.tsx` | `internal/tui/doctor.go` | 环境检查界面 |
| `screens/ResumeConversation.tsx` | `internal/tui/resume.go` | 恢复会话选择界面 |
| `state/AppState.ts` | `internal/state/` | 全局应用状态（架构中已规划）|

---

## 8. 设计决策

### 8.1 为什么整个 TUI 共用一个根 Model？

TS Ink 版本通过 React 组件树自然分解状态，每个组件各自持有 `useState`。BubbleTea 推荐单一顶层 Model，将所有状态提升到根。**代价**：`AppModel` 结构体字段较多；**收益**：状态变更的数据流完全可追踪，无需 Context/Provider 机制，测试简单（纯函数 `Update`）。

子组件（Spinner、PermissionDialog 等）实现为独立的 `View(params) string` 函数，而非独立的 `tea.Model`，保持"根 Model 统一调度"原则。对于复杂子交互（如 PromptInput 的 Vim 模式），可作为嵌套 `tea.Model`（`input.Model`）持有在根 Model 中，通过 `input.Update()` 委托处理。

### 8.2 流式输出：Cmd 拉取 vs goroutine 推送

两种方案均可行：
- **goroutine + `Program.Send()`**：QueryEngine goroutine 直接 `Send` Msg 到 Program
- **Cmd 拉取循环**（本方案）：每次 `Update` 返回新 Cmd 读取下一个事件

选择 **Cmd 拉取循环**，原因：与 BubbleTea 惯用法一致，`Update` 完全控制消费节奏，避免 goroutine 与 BubbleTea 渲染循环竞争，背压天然。权限等待（`PermissionRequestEvent`）时不消费下一个 Cmd，自然暂停。

### 8.3 Markdown 渲染策略

TS 版用自定义 `components/Markdown.tsx`（基于 remark/rehype）做 Ink 终端渲染。Go 版用 [`charmbracelet/glamour`](https://github.com/charmbracelet/glamour)，它原生支持终端 Markdown（ANSI 高亮、代码块、表格），无需自研。

### 8.4 Vim 模式输入

对应 TS `components/VimTextInput.tsx`。在 `input.Model` 内实现状态机：`Normal | Insert | Visual`，在 `input.Update()` 中处理 vim 按键序列（`i`/`a`/`o`/`dd`/`yy`/`p` 等）。通过 `/vim` 命令或 `--vim` flag 启用。

### 8.5 全屏模式 vs 普通模式

TS 版通过 `AlternateScreen` + `FullscreenLayout` 支持全屏（tmux-style 独立屏幕缓冲区）。Go 版通过 `tea.EnterAltScreen` Cmd 开启备用屏幕，通过 `tea.ExitAltScreen` 退出，行为完全一致。

### 8.6 测试策略

- `AppModel.Update()` 是纯函数（输入 Model + Msg，输出新 Model + Cmd），直接单元测试
- `View()` 输出字符串，可 golden-file 测试
- 集成测试通过 `bubbletea/teatest` 包驱动完整事件序列，断言最终输出
- 权限对话框、Slash 命令执行通过注入 `stubQueryEngine` 隔离 Core 层

### 8.7 与 Agent-Core #6 的集成边界

TUI 层对 Core 层的唯一接入点是 `QueryEngine` 接口（见第 3 节）。接口通过构造器注入 `AppModel`，在 `Agent-Core #6` 完成前，使用 `stubQueryEngine` 开发和测试 TUI 层所有功能，两层完全解耦。

---

## 变更记录

| 版本 | 日期 | 修改人 | 变更摘要 |
|------|------|--------|---------|
| v1.3 | 2026-11-18 | Agent-TUI（主会话代行） | BF-05：/config 只读设置面板（OpenDialog 既有 wiring＋renderConfigDialog，Esc 关闭，交互编辑为后续项）；/terminal-setup 终端环境检测（DetectTerminal 纯函数＋非侵入指引，不写文件；键绑定自动安装为后续项）。两桩消除，debt 24→22。cache 闸门三问全否＋守护测试。无接口签名变更。 |
| v1.2 | 2026-11-18 | Agent-TUI | BF-03-F1 评审修复（CR-BF03-1/2/3/4/5）：§4.1 钉住 `MCPServerLister`/`FormatMCPStatus` 的无并发池突变约束（活指针读取，快照根治待跨仓 BF-19；BF-01 并发写者落地前必须关闭）；补记 /mcp 活状态对 BF-01 启动接线的依赖（此前空池提示为预期行为）；`applyCommandResult` 提取 `applyResumeResult`/`applyDisplayResult` helper（≤80 行，行为不变）；/resume 错误经 %w 保留底层原因、Close 失败显式处理。无接口签名变更。 |
| v1.1 | 2026-11-18 | Agent-TUI | BF-03：/mcp、/resume 真实实现（§4.1）。新增 `commands.MCPServerLister`、`commands.FormatMCPStatus`、`Result.FetchMCPStatus`/`Result.NewSessionID`、`tui.MCPStatusMsg`；`tui.New`/`newAppModel` 新增 `mcpPool` 注入参数（bootstrap 传入 `container.MCPPool`）；`applyCommandResult` 开始消费 `Result.NewMessages`（含 isLoading 保护）。commands 层新增 agtkeel mcp/session/types/utils/fs 向下依赖。 |
| v1.0 | 2026-04-02 | Agent-TUI | 初稿 |
