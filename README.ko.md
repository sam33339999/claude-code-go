<p align="center">
  <img src="assets/logo.png" alt="Glaude Logo" width="200">
</p>

<h1 align="center">Glaude</h1>

<p align="center">
  <strong>🤖 Claude Code의 Go 언어 구현 — 터미널에서 작동하는 AI 코딩 어시스턴트</strong>
</p>

<p align="center">
  <a href="https://golang.org/dl/"><img src="https://img.shields.io/badge/go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go Version"></a>
  <a href="https://goreportcard.com/report/github.com/tunsuy/glaude"><img src="https://goreportcard.com/badge/github.com/tunsuy/glaude?style=flat-square" alt="Go Report Card"></a>
  <a href="https://codecov.io/gh/tunsuy/glaude"><img src="https://codecov.io/gh/tunsuy/glaude/branch/main/graph/badge.svg?style=flat-square" alt="커버리지"></a>
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
  <img src="assets/image.png" alt="Glaude - 대화형 세션" width="800">
  <br>
  <em>파일 읽기 및 실시간 사고 표시를 지원하는 대화형 TUI</em>
</p>

<p align="center">
  <img src="assets/image1.png" alt="Glaude - 프로젝트 분석" width="800">
  <br>
  <em>아키텍처 세부 정보를 포함한 종합적인 프로젝트 분석</em>
</p>

---

## 소개

이 프로젝트는 [Claude Code](https://claude.ai/code)(Anthropic의 공식 TypeScript CLI)를 **Go 언어로 완전히 재구현**한 것입니다. TUI 인터페이스, 도구 호출, 권한 시스템, 멀티 에이전트 조정, MCP 프로토콜, 세션 관리 등 모든 핵심 기능을 포함합니다.

> **🤖 AI 에이전트로 완전히 구축 — 인간이 작성한 프로덕션 코드가 단 한 줄도 없습니다.** 초기 빌드(2026년 4월)는 9개의 Claude AI 에이전트가 9일간 협력하여 완성했으며, 2026년 8월부터는 한 개의 메인 Claude 세션이 행동 동등성 갭 원장에 따라 프로젝트를 유지 보수하고, 한 명의 인간 담당자가 리뷰하고 병합합니다. [전체 이야기 ↓](#ai-에이전트로-완전히-구축)

## ❤️ 스폰서

> [여기에 게시되길 원하시나요?](https://github.com/tunsuy)

<table>
  <tr>
    <td width="180"><a href="https://www.orcarouter.ai/ref/ref_49eee7ba2e9927450075"><img src="assets/sponsors/orcarouter.png" alt="OrcaRouter" width="150"></a></td>
    <td><strong><a href="https://www.orcarouter.ai/ref/ref_49eee7ba2e9927450075">OrcaRouter</a></strong> — 하나의 게이트웨이로 모든 모델을. OrcaRouter는 각 요청을 200개 이상의 모델(프론티어 및 오픈소스) 중 가장 적합한 모델로 라우팅하며, 요금은 프로바이더 가격 그대로이고 페일오버와 요청 로그도 제공합니다. OpenAI 호환이므로 Glaude에 바로 연결됩니다 — 구체적인 설정은 <a href="#api-프로바이더">API 프로바이더</a> 섹션을 참고하세요.</td>
  </tr>
</table>

## 기능

- **대화형 TUI** — [Bubble Tea](https://github.com/charmbracelet/bubbletea)로 구축된 완전한 터미널 UI, 다크/라이트 테마 지원
- **에이전트 도구 사용** — 35개 내장 도구(파일, 셸, 검색, 웹, 작업, 서브 에이전트 등), 모두 9계층 권한 파이프라인을 통해 중재
- **Code Mode** — 모델이 JavaScript/TypeScript를 작성해 한 번의 샌드박스 실행으로 여러 도구 호출을 합성(순수 Go [moejs](https://github.com/MoeForge/moejs) 엔진): replay, 재사용 스니펫, 쓰기 감사, 배치 승인 — [Code Mode](#code-mode) 참조
- **CLI 하위 명령** — `glaude mcp …`, `glaude plugin …`, `glaude auth …`, `glaude doctor`, `glaude update`; `mcp` 및 `plugin` 그룹은 TypeScript CLI와 바이트 단위까지 동일한 출력 생성
- **플러그인 관리** — 로컬 마켓플레이스에서 플러그인 설치/활성화/비활성화, 매니페스트 완전 검증 지원(`glaude plugin validate`)
- **멀티 에이전트 조정** — 병렬 작업을 위한 백그라운드 서브 에이전트 생성
- **MCP 지원** — [Model Context Protocol](https://modelcontextprotocol.io)을 통한 외부 도구 연결(stdio + HTTP + WebSocket 전송), Claude Desktop에서 가져오기 가능; 서버 푸시 알림이 이벤트 기반으로 에이전트를 깨움(제로 폴링)
- **GLAUDE.md 메모리** — 디렉터리 트리의 `GLAUDE.md` 파일에서 프로젝트 컨텍스트 자동 로드
- **세션 관리** — 이전 대화 재개; 긴 기록 자동 압축
- **Vim 모드** — 입력 영역에서 선택적 Vim 키 바인딩
- **OAuth + API 키 인증** — Anthropic OAuth로 로그인(`glaude auth login`)하거나 `ANTHROPIC_API_KEY` 제공
- **내장 슬래시 명령** — `/compact`, `/commit`, `/review`, `/model`, `/theme` 등(아래 참조)
- **스트리밍 응답** — thinking 블록 표시와 함께 실시간 토큰 스트리밍

## 아키텍처

Glaude는 6개 계층으로 구성됩니다:

```
┌─────────────────────────────────────┐
│  CLI (cmd/glaude)                   │  Cobra 진입점
├─────────────────────────────────────┤
│  TUI (internal/tui)                 │  Bubble Tea MVU 인터페이스
├─────────────────────────────────────┤
│  Tools (internal/tools)             │  파일, 셸, 검색, MCP 도구
├─────────────────────────────────────┤
│  Core Engine (internal/engine)      │  스트리밍, 도구 디스패치, 코디네이터
├─────────────────────────────────────┤
│  Services (internal/api, oauth,     │  Anthropic API, OAuth, MCP 클라이언트
│            mcp, compact)            │
├─────────────────────────────────────┤
│  Infra (pkg/types, internal/config, │  타입, 설정, 상태, 훅, 플러그인
│         state, session, hooks)      │
└─────────────────────────────────────┘
```

자세한 내용은 [`docs/project/architecture.md`](docs/project/architecture.md)를 참조하세요.

## 요구 사항

- Go 1.26 이상
- [Anthropic API 키](https://console.anthropic.com/) **또는** Claude.ai 계정(OAuth)

## 설치

### 소스에서 빌드

```bash
git clone https://github.com/tunsuy/glaude.git
cd glaude
make build
# 바이너리는 ./bin/glaude에 위치합니다
```

`PATH`에 추가:

```bash
export PATH="$PATH:$(pwd)/bin"
```

### `go install` 사용

```bash
go install github.com/tunsuy/glaude/cmd/glaude@latest
```

## 빠른 시작

```bash
# API 키 설정(또는 아래 인증 섹션을 참조하여 OAuth 사용)
export ANTHROPIC_API_KEY=sk-ant-...

# 현재 디렉터리에서 대화형 세션 시작
claude

# 한 번 질문하고 종료
glaude -p "이 프로젝트의 메인 진입점을 설명해주세요"

# 가장 최근 세션 재개
glaude --resume
```

## 인증

**API 키(CI/스크립트에 권장):**

```bash
export ANTHROPIC_API_KEY=sk-ant-...
# 또는 영구적으로 저장:
glaude auth login --api-key sk-ant-...
```

**OAuth(대화형 사용에 권장):**

```bash
glaude auth login    # 브라우저에서 OAuth 플로우 열기
glaude auth status   # 로그인된 계정 확인
```

## API 프로바이더

Glaude는 여러 API 프로바이더를 지원하여 Anthropic의 API뿐만 아니라 OpenAI 호환 API도 사용할 수 있습니다.

### 지원되는 프로바이더

| 프로바이더 | 설명 | 환경 변수 |
|-----------|------|----------|
| `direct` (기본값) | Anthropic Direct API | `ANTHROPIC_API_KEY`, `ANTHROPIC_BASE_URL` |
| `openai` | OpenAI 및 호환 API | `OPENAI_API_KEY`, `OPENAI_BASE_URL` |
| `bedrock` | AWS Bedrock | 환경 변수를 통한 AWS 자격 증명 |
| `vertex` | Google Cloud Vertex AI | 환경 변수를 통한 GCP 자격 증명 |

### OpenAI 호환 API 사용

OpenAI, DeepSeek, Qwen, Moonshot 또는 OpenAI 호환 API를 사용하려면:

**방법 1: 환경 변수**

```bash
# 프로바이더를 openai로 설정
export CLAUDE_PROVIDER=openai

# API 키 설정
export OPENAI_API_KEY=sk-xxx

# 선택사항: 커스텀 Base URL 설정 (OpenAI 호환 서비스용)
export OPENAI_BASE_URL=https://api.deepseek.com  # DeepSeek
# export OPENAI_BASE_URL=https://api.moonshot.cn/v1  # Moonshot
# export OPENAI_BASE_URL=http://localhost:11434/v1  # Ollama

# 모델 설정
export OPENAI_MODEL=deepseek-chat

# Claude Code 실행
claude
```

**방법 2: 설정 파일**

`~/.config/claude-code/settings.json`을 생성하거나 편집:

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

### 속도 제한

클라이언트 측 속도 제어는 모든 프로바이더에 일괄 적용됩니다. 두 변수 모두 선택 사항이며, 미설정＝무제한입니다(429 자동 백오프 재시도는 계속 유효).

| 환경 변수 | 설명 |
|-----------|------|
| `CLAUDE_CODE_API_RPM` | 분당 요청 상한(요청 시작 간격 엄격 제어, 예: `60`＝간격 ≥1초) |
| `CLAUDE_CODE_API_MAX_CONCURRENT` | 동시 실행 요청 상한(스트리밍 포함; 상한 초과 병렬 서브 에이전트는 대기) |

```bash
# 속도 제한이 있는 프로바이더(예: Qwen) 권장 설정:
export CLAUDE_CODE_API_MAX_CONCURRENT=2   # 병렬 서브 에이전트 폭주 억제
export CLAUDE_CODE_API_RPM=60             # 선택: 요청 간격 조절
```

그래도 429가 발생한 요청은 지수 백오프로 자동 재시도됩니다. 프로바이더의 `retry-after` 헤더가 우선합니다.

### 프로바이더별 설정 예시

**OpenAI:**
- 모든 GPT-4 및 GPT-3.5 모델 지원
- 완전한 도구/함수 호출 지원
- 스트리밍 응답

**DeepSeek:**
```bash
export CLAUDE_PROVIDER=openai
export OPENAI_API_KEY=sk-xxx
export OPENAI_BASE_URL=https://api.deepseek.com
export OPENAI_MODEL=deepseek-chat
```

**Ollama (로컬):**
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

**OrcaRouter (AI 게이트웨이):**
[OrcaRouter](https://www.orcarouter.ai/ref/ref_49eee7ba2e9927450075)는 AI 게이트웨이입니다. 각 요청을 200개 이상의 모델(프론티어 및 오픈소스) 중 가장 적합한 모델로 라우팅하며, 요금은 프로바이더 가격 그대로이고 페일오버와 요청 로그도 제공합니다. OpenAI 호환이므로 `openai` 프로바이더를 통해 바로 사용할 수 있습니다:

```bash
export CLAUDE_PROVIDER=openai
export OPENAI_API_KEY=<your-orcarouter-key>
export OPENAI_BASE_URL=https://api.orcarouter.ai/v1
export OPENAI_MODEL=orcarouter/auto   # OrcaRouter가 요청마다 모델을 선택
```

## 사용법

### 대화형 모드

```
glaude [flags]
```

| 플래그 | 설명 |
|--------|------|
| `--resume` | 가장 최근 세션 재개 |
| `--session <id>` | ID로 특정 세션 재개 |
| `--model <name>` | 기본 Claude 모델 재정의 |
| `--dark` / `--light` | 다크 또는 라이트 테마 강제 적용 |
| `--vim` | Vim 키 바인딩 활성화 |
| `-p, --print <prompt>` | 비대화형: 단일 프롬프트 실행 후 종료 |

### CLI 하위 명령

비대화형 관리 명령입니다. `mcp` 및 `plugin` 그룹은 TypeScript CLI와 출력이 바이트 단위까지 호환됩니다:

| 명령 | 설명 |
|------|------|
| `glaude mcp add <name> -- <command> [args…]` | stdio MCP 서버 등록(원격은 `--transport http <name> <url>`, 환경 변수는 `-e KEY=VAL`) |
| `glaude mcp list` / `glaude mcp get <name>` | 설정된 서버 목록 조회 / 개별 서버 헬스 체크 |
| `glaude mcp add-json <name> <json>` | raw JSON으로 서버 등록 |
| `glaude mcp add-from-claude-desktop` | Claude Desktop 설정에서 서버 가져오기 |
| `glaude mcp remove <name>` / `reset-project-choices` | 서버 제거 / 프로젝트 로컬 선택 초기화 |
| `glaude plugin install <plugin>` | 구성된 마켓플레이스에서 플러그인 설치 |
| `glaude plugin list` / `enable` / `disable` / `uninstall` / `update` | 설치된 플러그인 관리 |
| `glaude plugin validate <path>` | 플러그인 또는 마켓플레이스 매니페스트 검증(`--strict`, `--json`) |
| `glaude plugin marketplace add/list/remove/update` | 플러그인 마켓플레이스 관리(로컬 경로) |
| `glaude auth login` / `logout` / `status` | OAuth 또는 API 키 인증 |
| `glaude doctor` | 환경 상태 점검 |

### 슬래시 명령

입력창에서 `/`를 입력하면 사용 가능한 모든 명령을 볼 수 있습니다:

| 명령 | 설명 |
|------|------|
| `/help` | 모든 명령 표시 |
| `/clear` | 대화 기록 지우기 |
| `/compact` | 기록을 요약하여 컨텍스트 사용량 감소 |
| `/exit` | Claude Code 종료 |
| `/model` | 활성 모델 조회 또는 설정 |
| `/theme` | 색상 테마 전환 |
| `/vim` | Vim 키 바인딩 전환 |
| `/effort` | effort 레벨 설정(low / medium / high) |
| `/status` | 세션 상태 표시 |
| `/cost` | 이 세션의 토큰 사용량 표시 |
| `/session` | 현재 세션 ID 표시 |
| `/memory` | 로드된 GLAUDE.md 메모리 파일 표시 |
| `/dream` | 메모리 통합 수동 트리거 |
| `/review` | Claude로 현재 git diff 검토 |
| `/commit` | Claude에게 git 커밋 작성 및 생성 요청 |
| `/diff` | 현재 git diff 표시 |
| `/init` | 이 프로젝트용 GLAUDE.md 생성 |

`/config`, `/mcp`, `/resume`, `/terminal-setup`는 모두 완전히 구현되었습니다(세션 재개, MCP 상태, 대화형 설정, 터미널 키바인딩 설정).

## Code Mode

모델은 「LLM 왕복 한 번에 도구 하나」 대신 **JavaScript/TypeScript를 작성해 한 번의 샌드박스 실행으로 여러 도구 호출을 합성**합니다 — `CodeMode` 도구, 순수 Go [moejs](https://github.com/MoeForge/moejs) 엔진(CGO 불필요). 중간 데이터는 샌드박스 안에 남고, 반환값과 `console.log` 출력만 모델로 돌아옵니다(왕복 감소, 컨텍스트 절약).

```js
// 모델이 작성하고 샌드박스에서 실행:
const src = await tools.Read({ file_path: "main.go" });
const todos = src.split("\n").filter(l => l.includes("TODO"));
console.log("found", todos.length);
return { count: todos.length, lines: todos };
```

### 샌드박스 API

| 샌드박스 내부 | 기능 |
|---------------|------|
| `tools.<Name>(input)` | 등록된 모든 도구 호출(비동기). `Promise.all`로 병렬 실행. **모든 호출은 전체 권한 파이프라인을 통과.** |
| `codemode.search(query)` / `describe(name)` | 점진적 도구 탐색(키워드 순위 → 필요 시 전체 스키마) |
| `codemode.history(limit)` | 최근 실행 기록(id, code, 결과) 재사용 |
| `codemode.replay(id)` | 기록된 실행 재실행 — 권한은 그대로 적용 |
| `codemode.run(name)` / `snippets()` | 이름 붙은 재사용 스니펫 실행/목록 |
| `codemode.audit(id)` | 한 실행에서 기록된 쓰기 부작용 |
| `codemode.approvals()` / `revoke(tool)` | CodeMode 범위 영구 승인 목록/철회 |
| `import { x } from "name"` | 호스트 관리 디렉터리의 통제된 공유 모듈 |
| `console.log(...)` | 캡처되는 출력 |

도구 입력 필드: `code`, `typescript`(타입 스트리핑 컴파일), `replay`(기록 id로 재실행), `save_as`(성공한 실행을 이름 붙은 스니펫으로 저장), `batch_writes`(모든 쓰기를 한 번의 승인으로).

### 안전 모델

- **샌드박싱은 인가가 아니다** — 내부의 각 도구 호출은 직접 호출과 동일한 9계층 권한 파이프라인을 실행. 파일시스템/네트워크/env/`eval` 접근 없음; 실행은 타임아웃과 메모리 한도로 제한.
- **배치 승인** — 도구가 프롬프트를 띄우고 사용자가 승인하면, 해당 실행 내 *같은* 도구의 이후 호출은 사전 승인(실행 범위만). 규칙 기반 직접 허용은 매번 재확인되므로 Shift+Tab 권한 모드 전환이 즉시 반영. 거부는 절대 캐시하지 않음.
- **영구 승인** — 권한 대화상자에 「CodeMode 안에서 *tool* 항상 허용」 옵션: CodeMode 범위, 사용자가 직접 부여, 철회 가능(`codemode.revoke`). Deny 규칙이 항상 우선.
- **Write 게이트** — `batch_writes: true`일 때 첫 쓰기 호출에서 한 번만 확인(실행 전체 범위 설명 포함), 그 결정이 실행 내 모든 쓰기를 커버. 읽기는 정상 흐름.
- **결과 스필** — 32 KiB 초과 출력은 파일에 기록되고, 모델은 4 KiB 미리보기＋포인터를 받음(`Read`/`Grep` 가능).

### 워크플로 기능

- **Replay**: 기록된 실행을 id로 재실행(`replay` 입력 또는 `codemode.replay`), 기록에 `replay_of` 출처 포함.
- **Snippets**: 성공한 실행을 이름 붙은 재사용 스크립트로(`save_as` → `codemode.run`). 스니펫은 전체 코드 저장; 중첩 실행은 호출자의 승인 범위 공유(재귀 깊이 상한 3).
- **쓰기 감사**: 실제 실행된 각 쓰기 호출 기록(도구＋입력 요약) — *무엇이 바뀌었는지*에 대한 정직한 추적, `codemode.audit`로 조회.

런타임 데이터는 `~/.glaude/codemode/` 아래: `history.jsonl`, `spill/`, `modules/`, `snippets/`, `approvals.json`.

### 이벤트 기반 대기(제로 폴링)

MCP 서버 푸시 알림은 메시지 큐로 라우팅되어 에이전트를 자동으로 깨웁니다 — 토큰을 태우는 폴링 루프 불필요. Code Mode와 결합: 스크립트가 핸드셰이크/구독을 수행하고, 에이전트는 대기하며, 서비스 알림이 다음 실행을 트리거(스니펫 로직 재사용 가능).

**알려진 제한**: 자동 롤백 없음(감사 추적이 변경 내용을 알려줌; git 체크포인트 복원은 평가 중), replay는 기록된 코드 사용(≤2000자 — 긴 워크플로는 스니펫으로 저장), write 게이트는 완전한 2단계 계획 미리보기가 아니라 첫 구체적 쓰기＋범위 설명 기반.

설계 세부사항: [`docs/project/design/codemode.md`](docs/project/design/codemode.md).

## AI 에이전트로 완전히 구축

> **이 저장소에는 인간이 작성한 프로덕션 코드가 단 한 줄도 없습니다.**

전체 프로젝트(아키텍처 설계, 상세 설계 문서, 병렬 구현, 코드 리뷰, QA, 통합 테스트)는 구조화된 멀티 에이전트 워크플로우에서 협력하는 **9개의 Claude AI 에이전트**에 의해 생성되었습니다:

```
PM Agent          →  프로젝트 계획, 마일스톤, 작업 스케줄링
Tech Lead Agent   →  아키텍처 설계, 설계 문서 리뷰, 코드 리뷰
Agent-Infra       →  인프라 계층(타입, 설정, 상태, 세션)
Agent-Services    →  서비스 계층(API 클라이언트, OAuth, MCP, 압축)
Agent-Core        →  코어 엔진(LLM 루프, 도구 디스패치, 코디네이터)
Agent-Tools       →  도구 계층(파일, 셸, 검색, 웹 — 빌드 시점 기준 18개 도구)
Agent-TUI         →  UI 계층(Bubble Tea MVU, 테마, Vim 모드)
Agent-CLI         →  진입점 계층(Cobra CLI, DI, 부트스트랩 단계)
QA Agent          →  테스트 전략, 계층별 승인, 통합 테스트
```

결과: **9일 만에 약 7,000줄의 프로덕션 코드 + 전체 테스트 스위트** 완성, `go test -race ./...` 통과.

이는 규모가 있는 다계층 Go 애플리케이션이 AI 에이전트들의 비동기 협업만으로 설계, 구현, 리뷰, 출시까지 완전히 가능하다는 실제 사례입니다. 2026년 8월부터는 프로젝트가 같은 정신으로 유지 보수되고 있습니다 — 원본 TypeScript CLI와의 행동 동등성 갭 원장(behavior-parity gap ledger)을 한 개의 메인 Claude 세션이 작업하고, 한 명의 인간 담당자가 리뷰하고 병합합니다. 여전히 인간이 작성한 프로덕션 코드는 단 한 줄도 없습니다(현재 약 33,000줄). 전체 의사결정 기록은 [`docs/project/`](docs/project/)에서 확인할 수 있습니다.

## 개발

### 전제 조건

- Go 1.26+
- `golangci-lint`(선택사항, 린팅용)

### 빌드 및 테스트

```bash
# 빌드
make build

# 모든 테스트 실행
make test

# 커버리지 리포트와 함께 테스트 실행
make test-cover

# Vet
make vet

# Lint(golangci-lint 필요)
make lint

# 빌드 + 테스트 + vet
make all
```

## 로드맵

초기 빌드(2026년 4월)는 9일 만에 완전한 6계층 아키텍처를 완성했습니다. 2026년 8월부터 이 프로젝트는 **유지 보수 모드**에 있습니다: 개발은 이 CLI를 원본 TypeScript 버전과 비교하는 행동 동등성 갭 원장([`test/parity/cases.md`](test/parity/cases.md))이 주도하며 — 닫힌 갭은 모두 테스트로 바이트 단위까지 고정되고, 의도적인 차이는 그 이유와 함께 `waived`로 기록됩니다.

최근 마일스톤:

- **2026-11-18** — Code Mode v1→v3(샌드박스 JS/TS 도구 합성: 배치 승인, TypeScript, 기록, 스필, 통제 import, replay, 스니펫, 쓰기 감사, 영구 승인, write 게이트); MCP 이벤트 기반 웨이크(서버 알림→에이전트, 제로 폴링); 커널을 본 저장소로 재내부화(외부 엔진 의존성 없음); Bubble Tea v2 마이그레이션; 사용자 노출 스텁 26개 전부 백필(부채: 스텁 0)
- **2026-09-05** — `mcp`(7개 명령) 및 `plugin`(8개 명령 + `marketplace` 하위 트리) CLI 그룹 추가, 출력이 TS CLI와 바이트 단위까지 동일; 사용자 노출 스텁 42 → 27로 감소
- **2026-08-30** — `--version` 형식 동등화; TS CLI 대비 전체 하위 명령 인벤토리 작성(§B11)
- **2026-08-29** — 유지 보수 모드 프로세스 착수: CI 강제 부채 레드라인, 패리티 테스트 스켈레톤

원래의 단계별 계획(v0.2.0 → v1.0.0)은 단계별 완료 상태와 함께 [`docs/ROADMAP.md`](docs/ROADMAP.md)에 보존되어 있습니다.

### 현재 상태

```
✅ 완료:  코어 엔진 + 스트리밍, TUI, 35개 도구 + Code Mode 샌드박스(v3),
          17개 슬래시 명령, mcp/plugin/auth/doctor CLI 하위 명령, 9계층 권한 파이프라인,
          컨텍스트 압축(snip/micro/auto), OAuth, 세션 지속성,
          MCP 이벤트 기반 웨이크(제로 폴링)
🔧 모드:  유지 보수 — TypeScript CLI 대비 갭 원장 기반 동등성 작업
📉 부채:  11개 TODO(dep) / 0개 사용자 노출 스텁, CI 강제 감소 전용 레드라인
🚀 릴리스: v0.8.0, 5개 플랫폼 바이너리
```

## 기여

기여를 환영합니다! Pull Request를 제출하기 전에 [CONTRIBUTING.md](CONTRIBUTING.md)를 읽어주세요.

빠른 체크리스트:
- 저장소를 포크하고 기능 브랜치 생성
- `make test`와 `make vet`가 통과하는지 확인
- 새 기능에 대한 테스트 작성
- 기존 코드 스타일 준수(`gofmt ./...` 실행)
- 제공된 템플릿을 사용하여 Pull Request 열기

## 보안

보안 취약점을 보고하려면 [SECURITY.md](SECURITY.md)를 참조하세요. 보안 버그에 대해 공개 GitHub Issue를 **열지 마세요**.

## 라이선스

이 프로젝트는 MIT 라이선스에 따라 라이선스됩니다 — 자세한 내용은 [LICENSE](LICENSE)를 참조하세요.

## 관련 프로젝트

- [claude-code](https://github.com/anthropics/claude-code) — 원본 TypeScript CLI
- [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go) — Anthropic API 공식 Go SDK
- [Model Context Protocol](https://modelcontextprotocol.io) — AI를 도구에 연결하기 위한 오픈 스탠다드
