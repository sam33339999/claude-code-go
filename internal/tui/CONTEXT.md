---
package: tui
import_path: internal/tui
layer: tui
generated_at: 2026-11-19T08:08:11Z
source_files: [agentcolors.go, cmds.go, completion.go, coordinator.go, init.go, input.go, keys.go, memstat.go, memstat_darwin.go, memstat_linux.go, memstat_other.go, memstat_windows.go, messagelist.go, messages.go, model.go, permissions.go, rendercache.go, resumepicker.go, spinner.go, statusbar.go, styles.go, textwidth.go, theme.go, toolview.go, update.go, view.go, welcome.go]
---

# internal/tui

> Layer: **TUI** · Files: 27 · Interfaces: 0 · Structs: 38 · Functions: 6

## Structs

- **AgentColorManager** — 3 fields
- **AgentProgressMsg** — 3 fields: TaskID, Activity, Detail
- **AgentStatusMsg** — 5 fields: TaskID, Status, Description, Result, Summary
- **AgentTaskState** — 12 fields: ID, Name, AgentType, Description, Status, StartTime, ElapsedMs, OutputTokens, ...
- **AppModel** — 62 fields
- **CommandResultMsg** — 2 fields: Text, IsError
- **CompactDoneMsg** — 1 fields: Summary
- **CoordinatorPanel** — 3 fields: Tasks, SelectedIndex, TaskOrder
- **DreamDoneMsg** — 1 fields: Err
- **InputChangedMsg** — 1 fields: Text
- **InputModel** — 4 fields
- **InputSubmittedMsg** — 1 fields: Text
- **MCPStatusMsg** — 1 fields: Text
- **MemdirLoadedMsg** — 2 fields: Paths, ScopedFiles
- **MessageLookups** — 4 fields: ToolUseToResult, ResolvedToolUseIDs, ErroredToolUseIDs, InProgressToolUseIDs
- **PermissionDialog** — 8 fields
- **PermissionRequestMsg** — 8 fields: RequestID, ToolName, ToolUseID, Message, Input, ProjectPath, FromCodeMode, RespFn
- **QuestionRequestMsg** — 4 fields: RequestID, Question, Options, RespFn
- **SlashCommandMsg** — 2 fields: Name, Args
- **SpinnerModel** — 4 fields
- **StatusBar** — 10 fields
- **StreamAssistantTurnMsg** — 2 fields: FinalMessage, Usage
- **StreamDoneMsg** — 2 fields: FinalMessage, Usage
- **StreamErrorMsg** — 1 fields: Err
- **StreamThinkingMsg** — 1 fields: Delta
- **StreamTokenMsg** — 1 fields: Delta
- **StreamToolResultMsg** — 3 fields: ToolUseID, Content, IsError
- **StreamToolUseCompleteMsg** — 2 fields: ToolUseID, ToolInput
- **StreamToolUseInputDeltaMsg** — 3 fields: ToolUseID, ToolName, InputDelta
- **StreamToolUseStartMsg** — 3 fields: ToolUseID, ToolName, InputDelta
- **StreamUserTurnMsg** — 1 fields: FinalMessage
- **SystemTextMsg** — 1 fields: Text
- **TermResizedMsg** — 2 fields: Width, Height
- **Theme** — 12 fields: Name, Primary, Secondary, Accent, Muted, Error, Warning, Success, ...
- **TickMsg** — 1 fields: Time
- **TokenUsage** — 4 fields: Input, Output, CacheRead, CacheCreated
- **TranscriptErrMsg** — 1 fields: Err
- **WelcomeHeader** — 4 fields

## Functions

- `IsSlashCommand(text string) bool`
- `MessageListView(messages []types.Message, width int, darkMode bool, theme Theme, mdRenderer *glamour.TermRenderer, expandedToolResults map[string]bool) string`
- `New(qe engine.QueryEngine, appStore *state.AppStateStore, vimEnabled bool, dark bool, permAskCh <-chan permissions.AskRequest, permRespCh chan<- permissions.AskResponse, agentCoord tools.AgentCoordinator, agentEventCh <-chan coordinator.Event, mq *msgqueue.MessageQueue, qg *msgqueue.QueryGuard, memoryStore *memdir.MemoryStore, mcpPool commands.MCPServerLister, rec *transcript.Recorder, questionAskCh <-chan types.QuestionRequest, questionRespCh chan<- types.QuestionResponse) tea.Model`
- `NewAgentColorManager() *AgentColorManager`
- `NewInput(vimEnabled bool) InputModel`
- `NewWelcomeHeader(model string, cwd string) WelcomeHeader`

## Constants

- `AgentCompleted`
- `AgentFailed`
- `AgentPaused`
- `AgentRunning`
- `PermissionChoiceAlwaysAllow`
- `PermissionChoiceNo`
- `PermissionChoiceYes`
- `SpinnerModeBrief`
- `SpinnerModeNormal`
- `SpinnerModeTeammate`
- `ToolStatusError`
- `ToolStatusInProgress`
- `ToolStatusQueued`
- `ToolStatusResolved`
- `VimModeInsert`
- `VimModeNormal`
- `VimModeVisual`

## Dependencies

**Imports:** `internal/commands`, `internal/compact`, `internal/coordinator`, `internal/engine`, `internal/memdir`, `internal/msgqueue`, `internal/permissions`, `internal/state`, `internal/tools`, `internal/transcript`, `pkg/types`

**Imported by:** `internal/bootstrap`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **2026-09-23 基座换库（agtkeel import-swap）**：可复用 agent 内核已逐字提取到 `github.com/tunsuy/agtkeel`（MIT），本包 import 随之改写：`internal/{api,compact,msgqueue,engine,session,permissions,hooks,mcp,config}` → `agtkeel/*`、`internal/tools`（接口契约+注册表）→ `agtkeel/tools`、`pkg/types` → `agtkeel/types`、`pkg/utils/{fs,ids}` → `agtkeel/utils/*`。本包代码**仅 import 路径重写，零逻辑改动**，`go build`/`go vet`/`go test -race ./...` 全绿。已知表现局限：docgen 依赖图按本模块前缀分类，agtkeel 视作第三方库，故上方 Imports 栏不再展示内核依赖（与 cobra 等一致）。
- **2026-11-18 BF-03（/mcp、/resume 接线）**：`New()`/`newAppModel` 末尾新增 `mcpPool commands.MCPServerLister` 注入参数（bootstrap `runInteractive` 传入 `container.MCPPool`，与 memoryStore 同一注入模式）；`AppModel` 新增 `mcpPool` 字段。新增 `MCPStatusMsg` 与 `fetchMCPStatusCmd`（/mcp 报告在 Update 循环外生成，整体 15s 超时，MVU 合规）。`applyCommandResult` 新增：`FetchMCPStatus` → 异步 Cmd；`NewMessages`（`[]types.Message` 断言）→ 替换 `m.messages` + `queryEngine.SetMessages` + 采纳 `NewSessionID`，查询进行中拒绝恢复；/clear 的遗留 `[]interface{}{}` 断言失败被忽略，行为不变。未新增引擎契约（复用既有 `SetMessages`）。
- **2026-11-18 BF-03-F1 评审修复（CR-BF03-1）**：`applyCommandResult` 提取 `applyResumeResult`（NewMessages 消费＋isLoading 拒绝，返回 stop 标志）与 `applyDisplayResult`（Display 渲染）两个私有 helper，主函数 107→72 行（≤80 规范），逐路径行为不变。/mcp 显示-only 守护补强：TestSlashMCP_FetchesStatusViaCmd 追加引擎历史 DeepEqual 不变断言（cache gate）。
- **2026-11-18 BF-05 /config 面板**：view.go 新增 renderConfigDialog（dialogConfig overlay case），渲染 AppState 当前设置（Model/Theme/Vim/Effort/WorkingDir/Session），Esc 关闭沿用既有 keys.go。纯展示不触引擎历史（DeepEqual 守护测试）。只读面板，交互编辑为文档化后续项。
- **2026-11-18 BF-18 持久化接线**：AppModel.transcript 字段＋New/newAppModel 末位 rec 参数（可 nil）；StreamDone/StreamError 返回 persistTranscriptCmd（创建时快照拷贝 msgs 防引擎后续突变竞态）；/resume 采纳 NewSessionID 时经 switchTranscriptCmd 换文件（与 display/dream/mcp Cmd 用 tea.Batch 组合）；TranscriptErrMsg 降级为系统警告。sessionID 启动时取自 recorder。
- **2026-11-18 BF-07 Todo 面板**：renderTodoPanel 于 View 输入区上方渲染 AppState.Todos（[x] completed／[>] in_progress／[ ] pending），空列表不渲染。读 mutex 保护 store，纯展示（cache gate 三问全否）。
- **2026-11-18 BF-08 问答对话框**：questionAskCh/questionRespCh 字段＋New/newAppModel 参数；listenForQuestionRequest（RespFn callback 回传，权限 listener 同构）；dialogQuestion 状态机 questionState（选项 selected＋自由文本 answer）；handleQuestionKey（1-9/ runes/backspace/enter/esc）；renderQuestionDialog。
- **2026-11-18 BF-21 Shift+Tab 模式循环**：nextPermissionMode（default→acceptEdits→plan→bypassPermissions→default，空 Mode 视为 default；oracle 顺序，YOLO 纳入循环为人类明确要求之分歧）；cyclePermissionMode 更新 AppState＋系统訊息（YOLO 警告）；StatusBar.mode 指示器（⏵⏵ accept edits／⏸ plan／⚠ YOLO warning bold），View 每次读 live mode。
- **2026-11-18 UPD-01 Bubble Tea v2 迁移**：charm.land/bubbletea/v2 v2.1.1＋bubbles/v2＋lipgloss/v2＋glamour v1.0.0（Go 1.25）。View()→tea.View（screenView 设 AltScreen/MouseModeCellMotion）；KeyPressMsg（Code/Text/Mod 匹配；ctrl=字母+ModCtrl、shift+tab=Tab+ModShift、printable=Text!=""）；MouseWheelMsg 独立类型；viewport New(WithWidth/WithHeight)/SetWidth/ScrollUp/ScrollDown/HalfPageUp/Down；Theme 欄位 image/color.Color（lipgloss.Color 为函数）；textarea New() 零参兼容。自研 spinner 零影响。v2 守护测试×3。
- **2026-11-18 对话框可见性修复**：View 先渲染 renderActiveDialog＋todo/coordinator 面板并计行（renderedLines），viewport 以副本 SetHeight(原高−extras) 收缩，保证总渲染高度≤termHeight——修复批准对话框被挤出终端底部不可见的 bug。回归测试 dialog_visibility_test.go×4。
- **2026-11-18 EV-01**：processQueueCmd ModeExternalPrompt 分支——附加 display 訊息＋pinnedToBottom＋startQueryCmd，MCP 通知唤醒代理处理。
- **2026-11-18 CM-03**：PermissionRequestMsg +FromCodeMode；newPermissionDialog 按来源切换 always-allow 选项文案（CodeMode-scoped vs 项目级）；listenForPermissionRequest RespFn 映射 AlwaysInCodeMode/AlwaysAllow 互斥。
- **2026-11-18 coordinator 提示注入（feat/coordinator-fork-cache）**：startQueryCmd 於 coordinatorMode 時注入 GetCoordinatorSystemPrompt(false) 至穩定 system 前綴（GLAUDE.md 之後）；此前該 prompt 僅測試使用、未接生產。cache 佈局：穩定段（GLAUDE.md＋coordinator prompt）末尾掛單一 ephemeral 斷點，動態記憶 reminder 追加於斷點之後（turn 尾部、不進快取前綴）。守護測試 coordinator_prompt_test.go×2。
- **2026-11-18 WorkingDir 注入（feat/agent-cwd）**：startQueryCmd uctx.WorkingDir = appState.GetState().WorkingDir（live）——主會話工具（含 EnterWorktree 後）在會話目錄執行。守護測試 TestStartQueryCmd_WorkingDirFromAppState。
- **2026-11-19 協調者完成唤醒＋面板高度約束（feat/coordinator-completion-wake）**：(1) AgentStatusMsg completed/failed 時注入 coordinator.FormatTaskNotification 至 msgQueue（ModeExternalPrompt，复用 EV-01 唤醒路徑）——idle 立即 processQueueCmd 起 query；running 則排队待 StreamDone between-turn drain（不中斷 in-flight turn，同步 agent 经 WaitForAgent 取結果故不需 wake）。修「背景 agent 完成後協調者不繼續跑／派 2+ agent 後停擺」。AgentStatusMsg +Result/Summary；init.go +coordinatorStatus 反向映射。(2) CoordinatorPanel.View 上限 maxPanelTaskRows=5（running 優先排序＋"+N more"），View viewport 下限 minViewportLines=5——修「subagent 面板擠掉輸出」。守護測試 coordinator_wake_test.go×5。
- **2026-11-19 View 高度保證_fit（fix/subagent-result-layout）**：重寫高度分配防底部遮罩——topAvailable=termHeight-reservedLines-todoLines-dialogLines；coordPanel  truncateToLines(panelBudget=topAvailable-minViewportLines)；vpHeight=topAvailable-panelLines（≥1）。todo 亦可截斷（todoBudget=termHeight-reservedLines-dialogLines-1，display-only 譲位 interactive dialog）。優先序：dialog＞todo＞viewport(軟底5)＞panel（最先截）。修此前 minViewportLines 硬底反而總高溢出遮底部。守護測試 coordinator_layout_test.go×2（24/12/10 行總高≤termHeight；viewport 与 panel 共存）＋既有 dialog_visibility 全綠。
