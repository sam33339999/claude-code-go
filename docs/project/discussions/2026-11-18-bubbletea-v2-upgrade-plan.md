# Bubble Tea v2 升級計畫（UPD-01）

> **維護人**：PM Agent（主会话）
> **创建**：2026-11-18
> **状态**：planned（调查完成，待人类批准执行）
> **授权**：人类已同意更新 go.mod（2026-11-18）

---

## 1. 目标版本（已验证存在于 module proxy）

| 依赖 | 现状 | 目标 | 模组路径变更 |
|------|------|------|-------------|
| bubbletea | v1.1.0 | **v2.1.1** | `github.com/charmbracelet/bubbletea` → `charm.land/bubbletea/v2` |
| bubbles | v0.20.0 | **v2.2.1** | → `charm.land/bubbles/v2` |
| lipgloss | v0.13.0 | **v2.1.1** | → `charm.land/lipgloss/v2` |
| glamour | v0.8.0 | **v1.0.0** | 路径不变（内部用 lipgloss v1，与本仓 v2 **共存**，不同模组） |
| Go directive | 1.21 | **1.25+**（bubbles v2 要求 go 1.25.0） | 本机 toolchain go1.27.4 ✓ |

> v2 正式模组路径为 `charm.land/*`（github.com/charmbracelet/*/v2 是 vanity 别名，
> go.mod 声明的 module 是 charm.land）。官方升级指南已研读：
> bubbles `UPGRADE_GUIDE_V2.md`、lipgloss `UPGRADE_GUIDE_V2.md`（module cache 内）。

## 2. Breaking changes 映射（已逐项核实源码）

| # | 变更 | 本仓影响 | 迁移方式 | 风险 |
|---|------|---------|---------|------|
| 1 | import 路径 → charm.land/*/v2 | tui 17 檔＋root.go＋测试 | 机械替换 | 低 |
| 2 | `View() string` → `View() tea.View` | AppModel.View（**仅 1 处**） | `return tea.NewView(sb.String())`；`v.AltScreen=true`、`v.MouseMode=tea.MouseModeCellMotion` | 低 |
| 3 | `tea.WithAltScreen()/WithMouseCellMotion()` 移除 | root.go:245（1 处） | 改在 View 上设置（见 #2） | 低 |
| 4 | `tea.KeyMsg` → `tea.KeyPressMsg`（Key struct） | **非测试 51 处＋测试 29 处** | case 替换；`key.Type==tea.KeyRunes`→`key.Text!=""`；`key.Runes`→`[]rune(key.Text)`；`key.Alt`→`key.Mod.Contains(tea.ModAlt)` | **中**（量大） |
| 5 | key.String() 匹配字串 | keys.go 17 处＋input.go 4 处 | **已核实 ultraviolet keyTypeString：enter/esc/tab/backspace/up/down/pgup/shift+tab/ctrl+j 与 v1 一致**——switch 字串基本不动 | 低 |
| 6 | Mouse：`MouseMsg` 变 interface；滚轮独立 `MouseWheelMsg` | keys.go handleMouse（1 处）＋update.go case | type-switch `tea.MouseWheelMsg`，读方向字段 | 低 |
| 7 | `lipgloss.Color` 型别→函数（回传 `image/color.Color`） | theme.go Theme 12 欄位＋styles.go | 欄位型别 `lipgloss.Color`→`color.Color`（image/color）；`lipgloss.Color("12")` 呼叫式不变 | 低 |
| 8 | lipgloss v2 其余 API | JoinHorizontal/JoinVertical/Width/NewStyle/Render **均保留**（已核实）；**本仓未用 AdaptiveColor**（无需 compat 包） | 几乎不动 | 低 |
| 9 | bubbles viewport：`New(w,h)`→`New(opts...)`；Width/Height 欄位→方法 | model.go:232、update.go:23-24、view.go 读取 | `viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))`；`.Width=x`→`.SetWidth(x)`；读 `.Width`→`.Width()` | 低 |
| 10 | bubbles textarea：New(opts...)、DefaultKeyMap() 函数化 | input.go（New/Value/SetValue/SetWidth/SetHeight/Focus/Blur/Blink/Update） | `textarea.New()` 零参仍合法；Blink/Update 签名核实后微调 | 低 |
| 11 | spinner | **自研 SpinnerModel（仅 import time）——零影响** | 不动 | 无 |
| 12 | glamour v0.8→v1.0.0 | messagelist.go（NewTermRenderer/WithStyles/WithWordWrap/ansi.StyleConfig） | **API 已核实稳定**；渲染输出可能微差（word-based 断言应撑住） | 低 |
| 13 | tea.Quit/Tick/Batch/BatchMsg/WindowSizeMsg/Cmd/Msg/Update 签名 | 全仓 | **已核实不变**（Quit 仍 `func Quit() Msg` 作 Cmd 用） | 无 |
| 14 | Go 1.25 要求 | go.mod＋ci.yml ×5＋release.yml ×1＋CONTRIBUTING | go directive 1.25；CI go-version "1.25" | 低 |

## 3. 影响面盘点

- **生产代码**：internal/tui 17 檔（keys.go 最重：17 key 模式＋mouse；input.go textarea；
  model.go/update.go viewport；view.go View；theme.go/styles.go 颜色；messagelist.go glamour）
  ＋internal/bootstrap/root.go（program 选项）
- **测试**：tui_test.go、config_dialog_test.go、mode_cycle_test.go、question_dialog_test.go、
  mcp_resume_test.go、transcript_tui_test.go 等（tea.KeyMsg ×29 构造式迁移）
- **构建/CI**：go.mod、go.sum、ci.yml（5 处 go-version）、release.yml（1 处）、CONTRIBUTING.md
- **不受影响**：engine/tools/permissions/mcp 等非 TUI 层（零 bubbletea 依赖）；自研 spinner

## 4. 执行阶段（每阶段编译检查点）

| 阶段 | 内容 | 检查点 |
|------|------|--------|
| P0 | feat 分支；go.mod：go 1.25＋charm.land 三件 v2＋glamour v1.0.0；go mod tidy；CI/release go-version | `go mod download` ✓ |
| P1 | lipgloss v2：import＋Theme 欄位型别（image/color.Color）＋styles | tui 包编译（tea 部分暂报错可先 stub 隔离，或 P1/P2 合并） |
| P2 | bubbletea v2：import、View() tea.View＋AltScreen/MouseMode、root.go 选项移除、KeyPressMsg 全量替换、key 匹配迁移、MouseWheelMsg | `go build ./...` ✓ |
| P3 | bubbles v2：viewport New(opts)/SetWidth/Width()、textarea 微调 | `go build ./...` ✓ |
| P4 | glamour v1.0.0：import 路径不变，版本升；渲染 smoke | markdown 渲染测试 ✓ |
| P5 | 测试迁移（KeyMsg 构造式×29）＋全量门控＋TUI 视觉 smoke | 全仓 -race ✓＋docs-check ✓＋debt-check ✓ |
| P6 | 文档：design/tui.md v2.0（依赖版本＋API 变更）、AGENTS.md/CLAUDE.md Key Dependencies、CONTRIBUTING Go 要求、CONTEXT Notes；commit＋merge feat/impl | 三层门控＋人类 PR 确认 |

> P1/P2 可合并执行（lipgloss 与 bubbletea 型别互相牵连，分开会有中间态编译失败）。

## 5. 测试策略

- **既有测试全数保留语意**：word-based 断言（ANSI 容忍）是 v2 渲染差异的天然缓冲
- KeyMsg 构造映射：`tea.KeyMsg{Type: tea.KeyEnter}` → `tea.KeyPressMsg{Code: tea.KeyEnter}`；
  `{Type: tea.KeyRunes, Runes: []rune("x")}` → `{Code: 'x', Text: "x"}`
- 新增：View() 回传 tea.View 且 AltScreen/MouseMode 正确设置之断言
- 新增：KeyPressMsg 与 KeyReleaseMsg 分流（release 应被忽略，不误触发）
- 视觉 smoke：实际终端跑 ./bin/claude，验证 alt-screen、滚轮、Shift+Tab、对话框、spinner
- glamour 渲染对比：同一段 markdown v0.8 vs v1.0.0 输出差异记录（可接受则过）

## 6. 风险与缓解

| 风险 | 等级 | 缓解 |
|------|------|------|
| v2 renderer 布局微差破坏断言 | 中 | word-based 断言＋逐测试修正；视觉 smoke |
| KeyReleaseMsg（Kitty 协议）误触发 | 低 | Update 只 case KeyPressMsg；新增分流测试 |
| 滚轮方向语义变更 | 低 | 读 MouseWheelMsg 源码核实方向字段＋实测 |
| glamour v1 渲染差异 | 低 | 对比测试；ansi.StyleConfig 已核实不变 |
| Go 1.25 要求冲击 CI/贡献者 | 低 | CI 同步升；CONTRIBUTING 明示；go.mod toolchain 自动下载 |
| charm.land 域名依赖可用性 | 低 | module proxy 已验证可下载；github 别名亦可 |

## 7. 验收标准

- [ ] 全仓 `CGO_ENABLED=1 go test -race -count=1 ./...` 绿
- [ ] `make build` ✓；`./bin/claude` 互动 smoke（alt-screen/滚轮/Shift+Tab/对话框/markdown）
- [ ] `make docs-check` ✓；debt-check ✓（基线不升）
- [ ] gofmt 全仓 0；golangci-lint 绿
- [ ] CI（升 Go 1.25 后）全绿
- [ ] 文档同步（tui.md v2.0＋AGENTS/CLAUDE/CONTRIBUTING＋CONTEXT Notes）
- [ ] 人类 PR 审查确认

## 8. 工作量估计

中等偏大：~20 生产檔＋~10 测试檔＋CI/文档。机械替换占 70%（可脚本化），
key/mouse/View 语意迁移占 20%，测试修正与 smoke 占 10%。预计 1-2 个工作会话。

## 9. 决策记录

- 2026-11-18：人类指示「規劃更新 bubbletea v2，可以更新 go.mod」→ 本計畫。
- 选型：跟随 charm 官方 v2 生态（bubbletea/bubbles/lipgloss 三件必须同步升——官方指南明示）；
  glamour 升 v1.0.0（同路径，lipgloss v1 内部共存）。
- 不引入 compat 包（本仓无 AdaptiveColor 使用，直接 image/color.Color 更干净）。