# 狀態列模型來源 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** 狀態列模型名標示「session 切換過」與「/setup 待重開」。
**Architecture:** `StatusBar` 加 `defaultModel`/`restartModel` 兩欄與 `modelLabel(full bool)`；`View` 依寬度選完整或 `*` 縮寫。`newAppModel` 設 `defaultModel`；`setupAppliedMsg` 帶 `mainModel`，`onSetupApplied` 成功時設 `restartModel`。
**Tech Stack:** Go、lipgloss v2。
**Spec:** `docs/superpowers/specs/2026-11-20-statusbar-model-source-design.md`

## Global Constraints
- 標記文字：`session · default <m>`、`restart → <m>`，括號包住、以 ` · ` 串接；窄螢幕縮寫為 `<model>*`。
- 完整版條件：左側（含完整標記）＋完整右側＋sep＋8 欄 cwd ≤ width。
- TUI 測試用詞斷言（`stripANSI` 後 `Contains`）。

## Review Focus
1. `defaultModel` 為空（舊測試路徑或沒設模型）→ 不顯示 session 標記（`TestStatusBar_NoDefaultNoMarker`）。
2. `/model` 切回預設 → 標記消失（`TestModelSwitch_BackToDefaultClearsMarker`）。
3. `/setup` 失敗或 `SettingsWritten=false` → 不設 restart（`TestSetupApplied_FailureNoRestartMarker`）。
4. 極窄寬度 → 不爆寬（既有 `View` 最後的 truncate 保底；`TestStatusBar_NarrowCollapsesAnnotation` 斷言寬度 ≤ width）。

---

### Task 1: StatusBar 標記與寬度
**Files:** Modify `internal/tui/statusbar.go`；Test `internal/tui/statusbar_source_test.go`
**Produces:** 欄位 `defaultModel, restartModel string`；`func (s StatusBar) modelLabel(full bool) string`
- [ ] 測試：`TestStatusBar_SameAsDefaultNoMarker`、`TestStatusBar_SessionSwitchShowsDefault`（含 `session`、`default b`）、`TestStatusBar_RestartPending`（含 `restart`、`r`）、`TestStatusBar_NoDefaultNoMarker`、`TestStatusBar_NarrowCollapsesAnnotation`（width 60：含 `a*`、不含 `session`、`displayWidth ≤ 60`）。
- [ ] 跑 `go test -race -run TestStatusBar_ ./internal/tui/` → FAIL；實作；→ PASS；commit `feat(tui): 狀態列標示模型來源`。

### Task 2: 接線（啟動預設、/model、/setup）
**Files:** Modify `internal/tui/model.go:305`（`defaultModel: model`）、`internal/tui/setup_dialog.go`（`setupAppliedMsg.mainModel`、`onSetupApplied`）；Test 同上檔
- [ ] 測試：`TestModelSwitch_MarksSession`（`switchModelRef("x")` 走 nil switcher 路徑 → View 含 `session`）、`TestModelSwitch_BackToDefaultClearsMarker`、`TestSetupApplied_SetsRestartMarker`、`TestSetupApplied_FailureNoRestartMarker`。
- [ ] FAIL → 實作 → `go test -race ./internal/tui/` PASS；commit `feat(tui): 啟動預設與 /setup 寫入接到狀態列標記`。

### Task 3: 文件與驗證
- [ ] `make docs`（只留有實質變動的 CONTEXT.md）＋ tui Design Notes；`docs/project/design/tui.md` 變更記錄 v2.9。
- [ ] `gofmt -l`、`make all`、`make docs-check`、`make debt-check` 全過；commit `docs: 狀態列模型來源 Design Notes`。
