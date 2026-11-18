// Package codemode implements Code Mode (CM-01): the model writes JavaScript
// that composes tool calls inside a sandbox, instead of issuing one tool call
// per LLM round trip. See docs/project/discussions/2026-11-18-codemode-feasibility.md.
//
// The engine is wrapped behind the Runtime interface so it can be swapped
// (moejs primary, goja fallback) without touching the bridge or the tool.
// Sandboxed code reaches the outside world ONLY through the CallFunc bridge,
// which enforces the full permission pipeline — sandboxing is not authorization.
package codemode

import (
	"context"
	"encoding/json"
	"time"
)

// CallFunc dispatches one tool call from sandboxed code. input is the JSON
// arguments; the returned JSON is the tool result content. An error rejects
// the JS promise (the code may catch it). Implementations MUST run the full
// permission pipeline (validate → tool CheckPermissions → checker → call).
type CallFunc func(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error)

// Options bounds one sandbox execution.
type Options struct {
	// Timeout bounds the whole execution (default 120s when zero).
	Timeout time.Duration
	// MemoryLimit caps JS heap retention in bytes (default 64 MiB when zero).
	// Exceeding it interrupts the script with an uncatchable error.
	MemoryLimit int64
	// Search, when set, backs codemode.search(query) inside the sandbox
	// (progressive discovery over the tool catalog).
	Search func(query string) (any, error)
	// Describe, when set, backs codemode.describe(name) inside the sandbox.
	Describe func(name string) (any, error)
	// Typescript compiles the code as TypeScript (Node-style type stripping,
	// CM-02 v2). Errors point into the TS source.
	Typescript bool
	// ImportModule, when set, resolves import specifiers to module source
	// (CM-02 v2 controlled imports). The host decides what is reachable;
	// nil keeps imports disabled. Never expose arbitrary filesystem reads.
	ImportModule func(specifier string) (string, error)
	// History, when set, backs codemode.history(limit) inside the sandbox
	// (CM-02 v2 execution history for reuse).
	History func(limit int) (any, error)
	// Replay, when set, backs codemode.replay(execID) (CM-03 v3): re-run a
	// recorded execution through the SAME permission-gated bridge.
	Replay func(id string) (any, error)
	// Run, when set, backs codemode.run(name) (CM-03 v3): execute a named
	// snippet from the host snippet library, sharing the caller's approval
	// scope.
	Run func(name string) (any, error)
	// Snippets, when set, backs codemode.snippets() (CM-03 v3): list the
	// available snippet names.
	Snippets func() (any, error)
	// Audit, when set, backs codemode.audit(execID) (CM-03 v3): the recorded
	// write side effects of one execution.
	Audit func(id string) (any, error)
	// Approvals, when set, backs codemode.approvals() (CM-03 v3): list the
	// persisted CodeMode-scoped tool approvals.
	Approvals func() (any, error)
	// Revoke, when set, backs codemode.revoke(tool) (CM-03 v3): remove a
	// persisted CodeMode-scoped approval (safe direction: tightening).
	Revoke func(tool string) (any, error)
}

// Result is what one sandbox execution produced.
type Result struct {
	// Value is the JSON-decoded return value of the code (nil if none).
	Value any
	// Logs holds console.log output lines, in order.
	Logs []string
}

// Runtime executes model-written code in a sandbox. Implementations must be
// safe to call concurrently (each Execute creates its own isolated runtime).
type Runtime interface {
	// Execute runs code (the body of an async function; `return` yields the
	// result). call bridges tools.* invocations. Cancellation of ctx or the
	// timeout interrupts the script.
	Execute(ctx context.Context, code string, call CallFunc, opts Options) (*Result, error)
}

// Defaults for Options.
const (
	DefaultTimeout     = 120 * time.Second
	DefaultMemoryLimit = 64 << 20 // 64 MiB
)
