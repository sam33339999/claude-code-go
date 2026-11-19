<p align="center">
  <img src="assets/logo.png" alt="Glaude Logo" width="200">
</p>

<h1 align="center">Glaude</h1>

<p align="center">
  <strong>🤖 Claude Code の Go 実装 — ターミナルで動く AI コーディングアシスタント</strong>
</p>

<p align="center">
  <a href="https://golang.org/dl/"><img src="https://img.shields.io/badge/go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go Version"></a>
  <a href="https://goreportcard.com/report/github.com/tunsuy/glaude"><img src="https://goreportcard.com/badge/github.com/tunsuy/glaude?style=flat-square" alt="Go Report Card"></a>
  <a href="https://codecov.io/gh/tunsuy/glaude"><img src="https://codecov.io/gh/tunsuy/glaude/branch/main/graph/badge.svg?style=flat-square" alt="カバレッジ"></a>
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
  <img src="assets/image.png" alt="Glaude - インタラクティブセッション" width="800">
  <br>
  <em>ファイル読み込みとリアルタイム思考表示を備えたインタラクティブ TUI</em>
</p>

<p align="center">
  <img src="assets/image1.png" alt="Glaude - プロジェクト分析" width="800">
  <br>
  <em>アーキテクチャの詳細を含む包括的なプロジェクト分析</em>
</p>

---

## これは何ですか？

本プロジェクトは [Claude Code](https://claude.ai/code)（Anthropic 公式 TypeScript CLI）を **Go 言語で完全に再実装** したものです。TUI インターフェース、ツール呼び出し、権限システム、マルチエージェント連携、MCP プロトコル、セッション管理など、すべてのコア機能をカバーしています。

> **🤖 すべて AI エージェントによって構築 — 人間が書いた本番コードはゼロ。** 初回ビルド（2026 年 4 月）は、9 つの Claude AI エージェントが 9 日間かけて協力して構築したものです。2026 年 8 月以降は、単一のメイン Claude セッションが動作パリティギャップ台帳に沿ってプロジェクトを保守し、人間のオーナー 1 名がレビューとマージを担当しています。[詳しい経緯は下へ ↓](#ai-エージェントによる開発)

---

## ❤️ スポンサー

> [ここに掲載したい方は](https://github.com/tunsuy)

<table>
  <tr>
    <td width="180"><a href="https://www.orcarouter.ai/ref/ref_49eee7ba2e9927450075"><img src="assets/sponsors/orcarouter.png" alt="OrcaRouter" width="150"></a></td>
    <td><strong><a href="https://www.orcarouter.ai/ref/ref_49eee7ba2e9927450075">OrcaRouter</a></strong> — 1 つのゲートウェイで、すべてのモデルへ。OrcaRouter は各リクエストを 200 以上のモデル（フロンティアおよびオープンソース）から最適なものへルーティングし、料金は各プロバイダー価格のまま、フェイルオーバーとリクエストログも備えます。OpenAI 互換のため Glaude にそのまま接続できます — 具体的な設定は <a href="#api-プロバイダー">API プロバイダー</a> を参照してください。</td>
  </tr>
</table>

## 機能

- **インタラクティブ TUI** — [Bubble Tea](https://github.com/charmbracelet/bubbletea) で構築されたフル機能のターミナル UI、ダーク/ライトテーマ対応
- **エージェントツール使用** — 35 の組み込みツール（ファイル、シェル、検索、Web、タスク、サブエージェントなど）、すべて 9 層の権限パイプラインを通じて制御
- **Code Mode** — モデルが JavaScript/TypeScript を書き、1 回のサンドボックス実行で複数のツール呼び出しを合成（pure Go [moejs](https://github.com/MoeForge/moejs) エンジン）：replay、再利用スニペット、書き込み監査、バッチ承認——[Code Mode](#code-mode) 参照
- **CLI サブコマンド** — `glaude mcp …`、`glaude plugin …`、`glaude auth …`、`glaude doctor`、`glaude update`。`mcp` と `plugin` グループは TypeScript CLI とバイト単位で同一の出力を生成
- **プラグイン管理** — ローカルマーケットプレイスからプラグインをインストール/有効化/無効化、マニフェストの完全検証付き（`glaude plugin validate`）
- **マルチエージェント連携** — 並列タスク用のバックグラウンドサブエージェントを起動
- **MCP サポート** — [Model Context Protocol](https://modelcontextprotocol.io) 経由で外部ツールを接続（stdio + HTTP + WebSocket トランスポート）、Claude Desktop からインポート可能；サーバープッシュ通知がイベント駆動でエージェントを起動（ゼロポーリング）
- **GLAUDE.md メモリ** — ディレクトリツリー上の `GLAUDE.md` ファイルからプロジェクトコンテキストを自動読み込み
- **セッション管理** — 以前の会話を再開；長い履歴は自動圧縮
- **Vim モード** — 入力エリアでオプションの Vim キーバインディング
- **OAuth + API キー認証** — Anthropic OAuth でサインイン（`glaude auth login`）または `ANTHROPIC_API_KEY` を提供
- **組み込みスラッシュコマンド** — `/compact`、`/commit`、`/review`、`/model`、`/theme` など（下記参照）
- **ストリーミングレスポンス** — thinking ブロック表示付きのリアルタイムトークンストリーミング

## アーキテクチャ

Glaude は 6 層構造で構成されています：

```
┌─────────────────────────────────────┐
│  CLI (cmd/glaude)                   │  Cobra エントリポイント
├─────────────────────────────────────┤
│  TUI (internal/tui)                 │  Bubble Tea MVU インターフェース
├─────────────────────────────────────┤
│  Tools (internal/tools)             │  ファイル、シェル、検索、MCP ツール
├─────────────────────────────────────┤
│  Core Engine (internal/engine)      │  ストリーミング、ツールディスパッチ、コーディネーター
├─────────────────────────────────────┤
│  Services (internal/api, oauth,     │  Anthropic API、OAuth、MCP クライアント
│            mcp, compact)            │
├─────────────────────────────────────┤
│  Infra (pkg/types, internal/config, │  型、設定、状態、フック、プラグイン
│         state, session, hooks)      │
└─────────────────────────────────────┘
```

詳細は [`docs/project/architecture.md`](docs/project/architecture.md) を参照してください。

## 必要条件

- Go 1.26 以降
- [Anthropic API キー](https://console.anthropic.com/) **または** Claude.ai アカウント（OAuth）

## インストール

### ソースからビルド

```bash
git clone https://github.com/tunsuy/glaude.git
cd glaude
make build
# バイナリは ./bin/glaude に配置されます
```

`PATH` に追加：

```bash
export PATH="$PATH:$(pwd)/bin"
```

### `go install` を使用

```bash
go install github.com/tunsuy/glaude/cmd/glaude@latest
```

## クイックスタート

```bash
# API キーを設定（または以下の認証セクションを参照して OAuth を使用）
export ANTHROPIC_API_KEY=sk-ant-...

# 現在のディレクトリでインタラクティブセッションを開始
claude

# 一度だけ質問して終了
glaude -p "このプロジェクトのメインエントリポイントを説明してください"

# 最新のセッションを再開
glaude --resume
```

## 認証

**API キー（CI/スクリプト用に推奨）：**

```bash
export ANTHROPIC_API_KEY=sk-ant-...
# または永続的に保存：
glaude auth login --api-key sk-ant-...
```

**OAuth（インタラクティブ使用に推奨）：**

```bash
glaude auth login    # ブラウザで OAuth フローを開きます
glaude auth status   # サインイン中のアカウントを確認
```

## API プロバイダー

Glaude は複数の API プロバイダーをサポートしており、Anthropic の API だけでなく、OpenAI 互換の API も使用できます。

### サポートされるプロバイダー

| プロバイダー | 説明 | 環境変数 |
|-------------|------|----------|
| `direct`（デフォルト） | Anthropic Direct API | `ANTHROPIC_API_KEY`、`ANTHROPIC_BASE_URL` |
| `openai` | OpenAI および互換 API | `OPENAI_API_KEY`、`OPENAI_BASE_URL` |
| `bedrock` | AWS Bedrock | 環境変数経由の AWS 認証情報 |
| `vertex` | Google Cloud Vertex AI | 環境変数経由の GCP 認証情報 |

### OpenAI 互換 API の使用

OpenAI、DeepSeek、Qwen、Moonshot、または任意の OpenAI 互換 API を使用するには：

**方法 1：環境変数**

```bash
# プロバイダーを openai に設定
export CLAUDE_PROVIDER=openai

# API キーを設定
export OPENAI_API_KEY=sk-xxx

# オプション：カスタム Base URL を設定（OpenAI 互換サービス用）
export OPENAI_BASE_URL=https://api.deepseek.com  # DeepSeek
# export OPENAI_BASE_URL=https://api.moonshot.cn/v1  # Moonshot
# export OPENAI_BASE_URL=http://localhost:11434/v1  # Ollama

# モデルを設定
export OPENAI_MODEL=deepseek-chat

# Claude Code を実行
claude
```

**方法 2：設定ファイル**

`~/.config/claude-code/settings.json` を作成または編集：

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

### レート制限

クライアント側のレート制御はすべてのプロバイダーに一律適用されます。どちらの変数も任意で、未設定＝無制限です（429 時の自動バックオフ再試行は引き続き有効）。

| 環境変数 | 説明 |
|----------|------|
| `CLAUDE_CODE_API_RPM` | 1分あたりのリクエスト上限（リクエスト開始間隔を厳密に制御。例：`60`＝間隔 ≥1 秒） |
| `CLAUDE_CODE_API_MAX_CONCURRENT` | 同時実行リクエスト上限（ストリーミングを含む。上限を超える並列サブエージェントは待機） |

```bash
# レート制限のあるプロバイダー（例：Qwen）での推奨設定：
export CLAUDE_CODE_API_MAX_CONCURRENT=2   # 並列サブエージェントのバーストを抑制
export CLAUDE_CODE_API_RPM=60             # 任意：リクエスト間隔の調整
```

それでも 429 が発生したリクエストは指数バックオフで自動再試行されます。プロバイダーの `retry-after` ヘッダーが優先されます。

### プロバイダー別の設定例

**OpenAI：**
- すべての GPT-4 および GPT-3.5 モデルをサポート
- 完全なツール/関数呼び出しサポート
- ストリーミングレスポンス

**DeepSeek：**
```bash
export CLAUDE_PROVIDER=openai
export OPENAI_API_KEY=sk-xxx
export OPENAI_BASE_URL=https://api.deepseek.com
export OPENAI_MODEL=deepseek-chat
```

**Ollama（ローカル）：**
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

**OrcaRouter（AI ゲートウェイ）：**
[OrcaRouter](https://www.orcarouter.ai/ref/ref_49eee7ba2e9927450075) は AI ゲートウェイです。各リクエストを 200 以上のモデル（フロンティアおよびオープンソース）から最適なものへルーティングし、料金は各プロバイダー価格のまま、フェイルオーバーとリクエストログも備えます。OpenAI 互換のため、`openai` プロバイダー経由でそのまま使えます：

```bash
export CLAUDE_PROVIDER=openai
export OPENAI_API_KEY=<your-orcarouter-key>
export OPENAI_BASE_URL=https://api.orcarouter.ai/v1
export OPENAI_MODEL=orcarouter/auto   # OrcaRouter がリクエストごとにモデルを選択
```

## 使用方法

### インタラクティブモード

```
glaude [flags]
```

| フラグ | 説明 |
|--------|------|
| `--resume` | 最新のセッションを再開 |
| `--session <id>` | ID を指定してセッションを再開 |
| `--model <name>` | デフォルトの Claude モデルを上書き |
| `--dark` / `--light` | ダークまたはライトテーマを強制 |
| `--vim` | Vim キーバインディングを有効化 |
| `-p, --print <prompt>` | 非インタラクティブ：単一のプロンプトを実行して終了 |

### CLI サブコマンド

非インタラクティブな管理コマンド群です。`mcp` と `plugin` グループは TypeScript CLI とバイト単位で出力互換です：

| コマンド | 説明 |
|----------|------|
| `glaude mcp add <name> -- <command> [args…]` | stdio MCP サーバーを登録（リモートは `--transport http <name> <url>`、環境変数は `-e KEY=VAL`） |
| `glaude mcp list` / `glaude mcp get <name>` | 設定済みサーバーの一覧表示 / ヘルスチェック |
| `glaude mcp add-json <name> <json>` | 生の JSON からサーバーを登録 |
| `glaude mcp add-from-claude-desktop` | Claude Desktop の設定からサーバーをインポート |
| `glaude mcp remove <name>` / `reset-project-choices` | サーバーを削除 / プロジェクトローカルの選択をリセット |
| `glaude plugin install <plugin>` | 設定済みマーケットプレイスからプラグインをインストール |
| `glaude plugin list` / `enable` / `disable` / `uninstall` / `update` | インストール済みプラグインを管理 |
| `glaude plugin validate <path>` | プラグインまたはマーケットプレイスのマニフェストを検証（`--strict`、`--json`） |
| `glaude plugin marketplace add/list/remove/update` | プラグインマーケットプレイスを管理（ローカルパス） |
| `glaude auth login` / `logout` / `status` | OAuth または API キー認証 |
| `glaude doctor` | 環境のヘルスチェック |

### スラッシュコマンド

入力欄で `/` を入力すると、利用可能なすべてのコマンドが表示されます：

| コマンド | 説明 |
|----------|------|
| `/help` | すべてのコマンドを表示 |
| `/clear` | 会話履歴をクリア |
| `/compact` | 履歴を要約してコンテキスト使用量を削減 |
| `/exit` | Claude Code を終了 |
| `/model` | アクティブなモデルを表示または設定 |
| `/theme` | カラーテーマを切り替え |
| `/vim` | Vim キーバインディングを切り替え |
| `/effort` | effort レベルを設定（low / medium / high） |
| `/status` | セッションステータスを表示 |
| `/cost` | このセッションのトークン使用量を表示 |
| `/session` | 現在のセッション ID を表示 |
| `/memory` | 読み込まれた GLAUDE.md メモリファイルを表示 |
| `/dream` | メモリ統合を手動で実行 |
| `/review` | 現在の git diff を Claude でレビュー |
| `/commit` | git コミットの作成を Claude に依頼 |
| `/diff` | 現在の git diff を表示 |
| `/init` | このプロジェクト用の GLAUDE.md を生成 |

`/config`、`/mcp`、`/resume`、`/terminal-setup` はすべて完全に実装済みです（セッション再開、MCP ステータス、対話型設定、ターミナルキーバインド設定）。

## Code Mode

モデルは「1 回の LLM 往復で 1 ツール」ではなく、**JavaScript/TypeScript を書いて 1 回のサンドボックス実行で複数のツール呼び出しを合成**します——`CodeMode` ツール、pure Go の [moejs](https://github.com/MoeForge/moejs) エンジン（CGO 不要）。中間データはサンドボックス内に留まり、戻り値と `console.log` 出力だけがモデルに返ります（往復削減、コンテキスト節約）。

```js
// モデルが書き、サンドボックスで実行：
const src = await tools.Read({ file_path: "main.go" });
const todos = src.split("\n").filter(l => l.includes("TODO"));
console.log("found", todos.length);
return { count: todos.length, lines: todos };
```

### サンドボックス API

| サンドボックス内 | 機能 |
|------------------|------|
| `tools.<Name>(input)` | 登録済みツールを呼び出し（非同期）。`Promise.all` で並列実行。**すべての呼び出しが完全な権限パイプラインを通過。** |
| `codemode.search(query)` / `describe(name)` | 段階的ツール発見（キーワード順位 → 必要時に完全スキーマ） |
| `codemode.history(limit)` | 最近の実行履歴（id、code、結果）を再利用 |
| `codemode.replay(id)` | 記録された実行を再実行——権限は通常どおり適用 |
| `codemode.run(name)` / `snippets()` | 名前付き再利用スニペットの実行/一覧 |
| `codemode.audit(id)` | 1 回の実行で記録された書き込み副作用 |
| `codemode.approvals()` / `revoke(tool)` | CodeMode スコープの永続承認の一覧/取り消し |
| `import { x } from "name"` | ホスト管理ディレクトリの制御された共有モジュール |
| `console.log(...)` | キャプチャされる出力 |

ツール入力フィールド：`code`、`typescript`（型剥離コンパイル）、`replay`（履歴 id で再実行）、`save_as`（成功した実行を名前付きスニペットとして保存）、`batch_writes`（すべての書き込みを 1 回の承認で）。

### 安全モデル

- **サンドボックス化は認可ではない**——内部の各ツール呼び出しは直接呼び出しと同じ 9 層権限パイプラインを実行。ファイルシステム/ネットワーク/env/`eval` アクセスなし；実行はタイムアウトとメモリ上限で制限。
- **バッチ承認**——ツールがプロンプトを表示しあなたが承認すると、その実行内の*同じ*ツールへの後続呼び出しは事前承認（実行スコープのみ）。ルールによる直接許可は毎回再チェックされるため、Shift+Tab の権限モード切替が即座に反映。拒否は決してキャッシュしない。
- **永続承認**——権限ダイアログに「CodeMode 内で *tool* を常に許可」オプション：CodeMode スコープ、ユーザー自身が付与、取り消し可能（`codemode.revoke`）。Deny ルールが常に優先。
- **Write ゲート**——`batch_writes: true` の場合、最初の書き込み呼び出しで 1 回だけ確認（実行全体のスコープ説明付き）、その決定が実行内のすべての書き込みをカバー。読み取りは通常フロー。
- **結果スピル**——32 KiB 超の出力はファイルに書き込まれ、モデルは 4 KiB プレビュー＋ポインタを受け取る（`Read`/`Grep` 可能）。

### ワークフロー機能

- **Replay**：任意の記録済み実行を id で再実行（`replay` 入力または `codemode.replay`）、履歴に `replay_of` 来歴付き。
- **Snippets**：成功した実行を名前付き再利用スクリプトに（`save_as` → `codemode.run`）。スニペットは完全なコードを保存；ネスト実行は呼び出し元の承認スコープを共有（再帰深度上限 3）。
- **書き込み監査**：実際に実行された各書き込み呼び出しを記録（ツール＋入力要約）——*何が変わったか*の正直な追跡、`codemode.audit` で照会。

ランタイムデータは `~/.glaude/codemode/` 配下：`history.jsonl`、`spill/`、`modules/`、`snippets/`、`approvals.json`。

### イベント駆動スタンバイ（ゼロポーリング）

MCP サーバーのプッシュ通知はメッセージキューにルーティングされ、エージェントを自動で起こします——トークンを消費するポーリングループは不要。Code Mode と組み合わせ：スクリプトがハンドシェイク/サブスクリプションを行い、エージェントは待機、サービス通知が次の実行をトリガー（スニペットロジックを再利用可能）。

**既知の制限**：自動ロールバックなし（監査証跡が変更内容を伝える；git チェックポイント復元は評価中）、replay は記録されたコードを使用（≤2000 文字——長いワークフローはスニペットに保存）、write ゲートは完全な二段階計画プレビューではなく最初の具体的な書き込み＋スコープ説明に基づく。

設計詳細：[`docs/project/design/codemode.md`](docs/project/design/codemode.md)。

## AI エージェントによる開発

> **このリポジトリには、人間が書いた本番コードは一行もありません。**

プロジェクト全体（アーキテクチャ設計、詳細設計書、並列実装、コードレビュー、QA、統合テスト）は、構造化されたマルチエージェントワークフローで協力する **9 つの Claude AI エージェント** によって作成されました：

```
PM Agent          →  プロジェクト計画、マイルストーン、タスクスケジューリング
Tech Lead Agent   →  アーキテクチャ設計、設計書レビュー、コードレビュー
Agent-Infra       →  インフラ層（型、設定、状態、セッション）
Agent-Services    →  サービス層（API クライアント、OAuth、MCP、圧縮）
Agent-Core        →  コアエンジン（LLM ループ、ツールディスパッチ、コーディネーター）
Agent-Tools       →  ツール層（ファイル、シェル、検索、Web — ビルド当時 18 ツール）
Agent-TUI         →  UI 層（Bubble Tea MVU、テーマ、Vim モード）
Agent-CLI         →  エントリ層（Cobra CLI、DI、ブートストラップフェーズ）
QA Agent          →  テスト戦略、各層の受け入れテスト、統合テスト
```

結果：**9 日間で約 7,000 行の本番コード + フルテストスイート**、`go test -race ./...` がパス。

これは、本格的な多層 Go アプリケーションが、非同期で協調する AI エージェントによって設計・実装・レビュー・リリースまで完全に遂行できることの実証でもあります。2026 年 8 月以降、本プロジェクトは同じ精神で維持されています — 単一のメイン Claude セッションが、オリジナルの TypeScript CLI との動作パリティギャップ台帳に沿って開発を進め、人間のオーナー 1 名がレビューとマージを担当します。人間が書いた本番コードは依然としてゼロ（現在のコードベースは約 33,000 行）。完全な意思決定の記録は [`docs/project/`](docs/project/) にあります。

## 開発

### 前提条件

- Go 1.26+
- `golangci-lint`（オプション、リンティング用）

### ビルド＆テスト

```bash
# ビルド
make build

# 全テストを実行
make test

# カバレッジレポート付きでテストを実行
make test-cover

# Vet
make vet

# Lint（golangci-lint が必要）
make lint

# ビルド + テスト + vet
make all
```

## ロードマップ

初回ビルド（2026 年 4 月）では、完全な 6 層アーキテクチャを 9 日間で完成させました。2026 年 8 月以降、本プロジェクトは**保守モード**にあります：開発は、本 CLI をオリジナルの TypeScript 版と比較する動作パリティギャップ台帳（[`test/parity/cases.md`](test/parity/cases.md)）によって駆動されます — 閉じられたギャップはすべてテストでバイト単位に固定され、意図的な乖離はすべて理由付きで `waived` として記録されます。

最近のマイルストーン：

- **2026-11-18** — Code Mode v1→v3（サンドボックス JS/TS ツール合成：バッチ承認、TypeScript、履歴、スピル、制御 import、replay、スニペット、書き込み監査、永続承認、write ゲート）；MCP イベント駆動ウェイク（サーバー通知→エージェント、ゼロポーリング）；カーネルを本リポジトリに再内部化（外部エンジン依存なし）；Bubble Tea v2 移行；ユーザーに見えるスタブ 26 個をすべて埋め戻し（負債：スタブ 0）
- **2026-09-05** — `mcp`（7 コマンド）と `plugin`（8 コマンド + `marketplace` サブツリー）の CLI グループが投入され、出力は TS CLI とバイト単位で同一に。ユーザーに見えるスタブを 42 → 27 に削減
- **2026-08-30** — `--version` フォーマットのパリティ；TS CLI に対するサブコマンドの全インベントリ（§B11）
- **2026-08-29** — 保守モードのプロセスが導入：CI で強制される負債レッドライン、パリティテストスケルトン

元のフェーズ別計画（v0.2.0 → v1.0.0）は、フェーズごとの完了状況とともに [`docs/ROADMAP.md`](docs/ROADMAP.md) に保存されています。

### 現在の状態

```
✅ 完了: コアエンジン + ストリーミング、TUI、35 ツール + Code Mode サンドボックス（v3）、
         17 スラッシュコマンド、mcp/plugin/auth/doctor CLI サブコマンド、9 層権限パイプライン、
         コンテキスト圧縮（snip/micro/auto）、OAuth、セッション永続化、
         MCP イベント駆動ウェイク（ゼロポーリング）
🔧 モード: 保守 — TypeScript CLI に対するギャップ台帳駆動のパリティ作業
📉 負債: 11 TODO(dep) / 0 ユーザーに見えるスタブ、CI で強制される増加禁止レッドライン
🚀 リリース: v0.8.0、5 プラットフォーム向けバイナリ
```

## 貢献

貢献は大歓迎です！Pull Request を送る前に [CONTRIBUTING.md](CONTRIBUTING.md) をお読みください。

クイックチェックリスト：
- リポジトリをフォークしてフィーチャーブランチを作成
- `make test` と `make vet` がパスすることを確認
- 新機能にはテストを書く
- 既存のコードスタイルに従う（`gofmt ./...` を実行）
- 提供されたテンプレートを使用して Pull Request を開く

## セキュリティ

セキュリティ脆弱性を報告するには、[SECURITY.md](SECURITY.md) を参照してください。セキュリティバグについては公開 GitHub Issue を**開かないでください**。

## ライセンス

このプロジェクトは MIT ライセンスの下でライセンスされています — 詳細は [LICENSE](LICENSE) を参照してください。

## 関連プロジェクト

- [claude-code](https://github.com/anthropics/claude-code) — オリジナルの TypeScript CLI
- [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go) — Anthropic API 用公式 Go SDK
- [Model Context Protocol](https://modelcontextprotocol.io) — AI をツールに接続するためのオープンスタンダード
