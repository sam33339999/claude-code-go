# 狀態列標示模型來源（短版）

日期：2026-11-20 ｜ 規模：小改動

## 意圖

session 裡用 `/model` 切換不會寫回設定檔；`/setup` 寫了設定檔但要重開才生效。兩種情況下，狀態列只顯示模型名，使用者分不出「現在實際在用哪個、跟設定檔是否一致」。要一眼看出來。

## 目標 / 非目標

- 目標：狀態列模型名後標示 (1) session 中途切換過、與啟動預設不同；(2) `/setup` 已寫入新模型、需要重開。
- 非目標：`/setup` 後即時套用；新增 `/status` 指令；改變 `/model` 寫不寫檔的行為。

## 做法

| 做法 | 取捨 | 結果 |
|---|---|---|
| 狀態列標記 | 一眼可見；佔寬度，需窄螢幕縮寫 | **採用**（使用者選定） |
| `/status` 指令 | 不佔版面；要主動查 | 不採用 |

## 設計

- `StatusBar` 新增 `defaultModel`（啟動時的 `MainLoopModel.ModelID`，即設定檔＋環境變數＋`--model` 合併後的結果）與 `restartModel`。
- 標記規則（括號內以 ` · ` 串接）：
  - `model != defaultModel` → `session · default <defaultModel>`
  - `restartModel != "" && restartModel != model` → `restart → <restartModel>`
  - 都沒有 → 只顯示模型名（與現在相同）。
- 例：`qwen4-max (session · default deepseek-v4.1-flash-1017)`、`qwen4-max (restart → deepseek-v4.1-flash-1017)`。
- 寬度：完整標記放得下「左側＋完整右側＋至少 8 欄 cwd」才用完整版，否則縮成 `qwen4-max*`。
- `/setup` 成功且 `SettingsWritten` 時，`restartModel = plan.MainModel`（經 `setupAppliedMsg` 帶回）。
- `/model` 切回預設模型時標記消失。

## Cache-first

純 TUI 顯示，不動 prompt、工具、請求。
