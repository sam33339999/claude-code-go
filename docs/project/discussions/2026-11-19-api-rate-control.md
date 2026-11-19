# API 速率控制＋retry 接線（qwen 429 痛點）

> 日期：2026-11-19
> 分支：feat/api-rate-control（merged feat/impl @ 2f1baf8）
> 狀態：已落地＋實機驗證

## 問題

使用者實機偶爾踩 qwen 速率上限。診斷兩個缺口：

1. **被動重试沒接上**：`api.WithRetry`（429 指數退避、retry-after 優先、max 10）
   完整實作＋測試，但 engine 從未呼叫——429 直接失敗。
2. **無主動限速**：平行 fork／sub-agent＝無上限併發 API 呼叫，是踩上限的放大器。

## 設計

```
engine.streamResponse
  └─ api.WithRetry（429/5xx/網路錯誤退避重试；401 fail-fast）
       └─ rateLimitedClient（provider-agnostic wrapper）
            ├─ RPM：strict spacing（burst 1，request-start 間距 = 60s/RPM）
            ├─ MaxConcurrent：semaphore（streaming 持 slot 至 reader.Close）
            └─ inner：direct / openai / bedrock / vertex / foundry
```

- **Provider 層「拉出來」的粒度**：內部抽象化（Client 介面＋factory＋wrapper seam），
  所有 provider 統一獲得限速，無需 provider-specific 程式碼。**不對外抽倉**——agtkeel
  教訓（内核 2026-11-18 才搬回本倉）。
- 401 fail-fast：`RetryOptions.RefreshToken` hook nil 時不烧退避梯（此前語義會重试
  10 次 ≈160s）；hook 預留給 OAuth refresh 接線。
- `foreground` 加入 529 foreground sources（Go engine 主 query source；此前主会話
  被當 background 於 529 即放棄）。

## 使用方式（env）

```bash
# 每分鐘最多 60 個請求（請求間距 ≥1s）
export CLAUDE_CODE_API_RPM=60

# 同時最多 2 個 in-flight 請求（含 streaming；平行 fork 會被排队）
export CLAUDE_CODE_API_MAX_CONCURRENT=2

# 兩者皆可選；未設＝不限速（被動 retry 仍生效）
```

`ClientConfig.RatePolicy` 程式化設定優先於 env。無 policy 時 client 不 wrap（零 overhead）。

建議 qwen 場景：先設 `CLAUDE_CODE_API_MAX_CONCURRENT=2`（擋平行 fork 爆量），
若仍踩 429 再加 `CLAUDE_CODE_API_RPM=<provider 上限的 80%>`。被動 retry 會吃
retry-after header 自動退避，雙保險。

## 實機驗證（bin/claude @ 2f1baf8，qwen4-max）

兩-turn prompt（Bash tool），request-start 間距 = 60/RPM：

| 設定 | 耗時 | 解讀 |
|------|------|------|
| 無 RPM | 7.86s | 基線 |
| RPM=6（10s 間距） | 13.43s | +5.6s＝10s 間距 − turn1 自身耗時（重疊） |
| RPM=3（20s 間距） | 23.03s | +15.2s；RPM6→RPM3 差 9.6s ≈ 10s 間距差 ✓ |

間距從 request-start 計量，與 turn 耗時重疊——線性縮放確認限速器精確生效。
守護測試×12（spacing／ctx cancel／併發峰值≤cap／錯誤路徑釋放／env 解析／
NewClient wrap／401 fail-fast／429 重试／engine 429→重试→成功／engine 401 單次失敗）。

## 後續（可選，未排程）

- TPM（tokens-per-minute）限速：需预估請求 token 數，複雜度高；RPM＋併發已覆蓋多數場景。
- OAuth RefreshToken hook 接線（bootstrap tm.CheckAndRefreshIfNeeded）。
- per-provider 預設 policy（provider registry 形式化）——目前 env 全域適用已夠用。
