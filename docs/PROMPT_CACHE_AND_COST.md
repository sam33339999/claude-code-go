---
owner: @SivanCola
backup: @esengine
status: active
reviewed: 2026-11-18
---

# Prompt cache and cost control

## Purpose

Reasonix treats prompt-cache hit rate as product behavior rather than a later
optimization. Provider prefix caching bills a reused prefix at the cache-hit
rate instead of the miss rate, so every mechanism below exists to keep the
provider-visible prefix byte-stable and to keep the window small.

Two rules follow from that and are cited throughout this document:

1. The cache-stable prefix is composed once per runtime build and is never
   rewritten mid-session. Dynamic state rides the current turn's tail instead.
2. A voluntary prefix break is a cost decision, so it carries an economics
   argument and a guard test.

The mechanisms fall into four groups: keeping the prefix stable, keeping the
window small, making fewer and cheaper calls, and observing the result.

## Reference: keeping the prefix stable

| Mechanism | Where | Behavior |
| --- | --- | --- |
| Cache-first review gate | `CONTRIBUTING.md` (`Cache-first review gate`) | A change touching prompt construction, memory prefix, output styles, skill index, tool schemas, request serialization, compaction or MCP registration needs explicit cache review. |
| Stable prefix owner | `internal/assembly/boot/prompt.go` (`promptAssembly`) | The prompt is assembled once and documented as not rewritable mid-session. |
| Prefix-only-on-declared-config | `REASONIX.md` (`Context projection`) | Only declared prefix configuration may move the prefix; memory and skill writes ride the turn. |
| Turn-tail projection | `internal/session/control/turn_projection.go` | Date, workspace VCS, skill catalog, project rules, memory notes, plan marker and goal block are appended to the user turn, not the system prompt. |
| Persisted environment snapshot | `internal/platform/environment/snapshot.go` | Probe results are persisted per fingerprint for 24h so a slow tool or a different `PATH` cannot rewrite the prefix between relaunches. |
| Pinned MCP placeholders | `internal/ext/plugin/lazy.go` | Cached schema placeholders are pinned for the session; a differing live handshake is written to the schema cache and surfaces next session, not mid-session. |
| Extension reload stability | `docs/EXTENSIONS.md`, `docs/EXTENSION_RUNTIME_V2_PERF.md` | A no-op, interceptor-only or backend-only reload keeps the system prompt, tool schemas and `RuntimeSnapshot.CacheHash` byte-identical. |
| Separate planner and executor sessions | `docs/SPEC.md` (two-model collaboration) | The two models never share a conversation, because switching models inside one transcript breaks the prefix. |
| Prepend-only transcript | `docs/SPEC.md` (context management) | Below the compaction trigger, history is never rewritten: no summary, no projection rewrite, no maintenance event. |

## Reference: keeping the window small

| Mechanism | Where | Behavior |
| --- | --- | --- |
| Tool output cap | `internal/runtime/agent/agent.go` (`MaxToolOutputBytes`) | One tool result is capped at 32KiB, about 8K tokens. |
| Oversize output spill | `internal/runtime/agent/tool_output_spill.go` | A body past the cap is written beside the session and replaced by a pointer; the model reads it back with `read_file` or `grep`. |
| Deferred tool schemas | `internal/tools/builtin/bash_search_steer_test.go`, `internal/runtime/usecap` | `grep`, `glob`, `web_fetch` and `present_files` are not in the provider schema; they are reached as `tool:<name>` through `use_capability`, and the Bash description steers to that path. |
| Bounded reads | `internal/tools/builtin/readfile.go` | `read_file` returns 2000 lines by default and reports a pagination trailer. |
| Two-pass search | `internal/tools/builtin/grep.go` | The tracked tree is searched first; the ignored tree, which walks build output, is searched only when that answer is empty. |
| Skill one-liner index | `internal/ext/skill/index.go` | The skill surface lists one line per skill, not the skill bodies. |
| Subagent context capsule | `docs/SPEC.md` (sub-agent inheritance) | A child gets its system prompt, the task text and the workspace root; `ContextCapsule` carries references and digests, never copied parent context. |
| Fleet dependency edge | `docs/SPEC.md` (fleet) | A `depends_on` edge delivers the dependency's answer so the same research is not paid twice; a dependency fan-in shares one byte budget. |
| No dynamic fan-out | `docs/SPEC.md` (fleet) | Measured at 70%–98% more tokens than one child with a tool belt, so `for_each` was removed. |
| Bounded summarizer input | `docs/SPEC.md` (context management) | Tail budget is `clamp(window×10%, 32K, 96K)` and summary output is capped at 16K. |

## Reference: fewer and cheaper calls

| Mechanism | Where | Behavior |
| --- | --- | --- |
| Cheap model keeps the loop | `internal/tools/advisor/advisor.go` | The working model calls the strong model only when it judges the moment worth it; the strong model answers text and runs nothing. |
| Output budget ladder | `internal/contract/provider/provider.go` (`AutoOutputBudget`) | `max_output_tokens=0` maps to 16K, 32K or 64K by reasoning effort instead of an unbounded ceiling. |
| Minimal valid call on a schema miss | `internal/runtime/usecap` | A missing or mistyped argument returns a schema-derived example call, so the retry succeeds once. |
| Semantic route cache | `internal/runtime/capability/semantic.go` | A semantic routing decision is cached for 5 minutes with at most 256 output tokens per call. |
| Deterministic task policy | `internal/runtime/taskpolicy/policy.go` | Plan depth and verification level come from host structure, so no classifier model is paid for per turn. |
| Root token budget | `docs/ROOT_TOKEN_BUDGET_SECURITY.md`, `docs/BOUNDED_ROOT_TOKEN_BUDGET.md` | A goal or root delegation tree stops after its reported-token threshold is crossed. |
| Compaction economics | `internal/runtime/agent/compact.go`, `compact_projection.go` | A fold needs at least 100 tokens of region, and a second checkpoint waits for `refoldMinTokens` of new closed history, because each checkpoint costs the whole prefix cache. |
| Summary shares the prefix | `internal/runtime/agent/compact_projection.go` | The summarizer request reuses the already-sent prefix, so the digest is billed at the hit rate. |

## Reference: observing and gating

| Mechanism | Where | Behavior |
| --- | --- | --- |
| Cache split in the UI | `internal/frontend/tui/footer.go`, `internal/frontend/tui/render.go` | Cached and new input tokens are shown separately next to the hit rate. |
| `/status` cache field | `internal/frontend/cli` | The status line reports the session cache reading beside the model and effort. |
| Usage normalization | `internal/model/openai/openai.go` (`normaliseUsage`) | DeepSeek `prompt_cache_{hit,miss}_tokens`, OpenAI `prompt_tokens_details` and Anthropic-style counters fold into one `Usage`, so a hit is priced as a hit. |
| Cache-hit rate card | `internal/contract/pricing/rates.go` | Every shipped rate card carries a `CacheHit` price beside input and output. |
| Release cache guard | `internal/runtime/agent/cachehit_e2e_test.go`, `scripts/cache-guard.sh` | A mock provider derives hits from the byte-identical prefix shared with the previous request; the default threshold is 90%, overridable with `REASONIX_CACHE_GUARD_THRESHOLD`. |
| Boot prefix guard | `internal/assembly/boot/extension_dispatch_test.go` | `TestBootStableExtensionCacheGuard` proves independent reload generations reproduce identical prompt and schema bytes. |
| Provider-boundary effect tests | `internal/assembly/boot/effect_test.go` and siblings | Assert the request bytes that actually reach the provider, not component-level correctness. |

## Steps: reviewing a change against these rules

1. Classify the change: does it move the prefix, grow the window, or add a call?
2. If it moves the prefix, say in the pull request what it does to the
   provider-visible prefix and why that cost is worth paying.
3. Prefer a turn-tail projection over a prefix edit for per-turn data.
4. Add a focused guard test near the changed surface.
5. Run the release-level cache check when the change is prefix-bearing:

```bash
REASONIX_RELEASE_CACHE_GUARD=1 go test ./internal/runtime/agent -run '^TestReleaseCacheHitGuard$' -v
go test ./internal/assembly/boot -run '^TestBootStableExtensionCacheGuard$' -v
./scripts/cache-guard.sh
```

6. Run `make check` before pushing.

## Boundaries

1. These are cooperative mechanisms, not a spending ceiling. A running request
   and a finalization round may cross a token budget.
2. A cache hit is priced as a hit only where the adapter recognises the wire
   shape; a gateway that reports nothing reads as unmeasured, not as free.
3. Compaction is the one sanctioned prefix reset. Everything else trades a
   deliberate cold prefix plus a review note for its effect.
4. Deleting or renaming a tool, or editing a schema, moves the prefix even when
   the intent is unrelated, which is why the review gate names it.
