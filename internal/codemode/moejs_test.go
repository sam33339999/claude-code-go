package codemode

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestMoejs_ReturnValueAndLogs(t *testing.T) {
	rt := NewMoejsRuntime()
	res, err := rt.Execute(context.Background(), `
		console.log("hello", 42);
		return { sum: 1 + 2, items: [1, 2, 3] };
	`, nil, Options{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(res.Logs) != 1 || !strings.Contains(res.Logs[0], "hello") || !strings.Contains(res.Logs[0], "42") {
		t.Errorf("logs: %v", res.Logs)
	}
	m, ok := res.Value.(map[string]any)
	if !ok || fmt.Sprintf("%v", m["sum"]) != "3" {
		t.Errorf("value: %#v", res.Value)
	}
}

func TestMoejs_BridgedAsyncCall(t *testing.T) {
	rt := NewMoejsRuntime()
	call := func(_ context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
		if name != "Echo" {
			t.Errorf("unexpected tool %q", name)
		}
		return json.Marshal(map[string]any{"echoed": string(input)})
	}
	res, err := rt.Execute(context.Background(), `
		const r = await tools.Echo({ msg: "hi" });
		return r;
	`, call, Options{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	m := res.Value.(map[string]any)
	if !strings.Contains(m["echoed"].(string), "hi") {
		t.Errorf("bridge round trip failed: %#v", res.Value)
	}
}

func TestMoejs_PromiseAllParallel(t *testing.T) {
	rt := NewMoejsRuntime()
	call := func(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
		time.Sleep(60 * time.Millisecond)
		return json.Marshal("done")
	}
	start := time.Now()
	res, err := rt.Execute(context.Background(), `
		const [a, b, c] = await Promise.all([tools.Slow(), tools.Slow(), tools.Slow()]);
		return [a, b, c];
	`, call, Options{})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if elapsed > 150*time.Millisecond {
		t.Errorf("3×60ms calls took %v — not parallel", elapsed)
	}
	if arr, ok := res.Value.([]any); !ok || len(arr) != 3 {
		t.Errorf("value: %#v", res.Value)
	}
}

func TestMoejs_BridgeErrorCatchable(t *testing.T) {
	rt := NewMoejsRuntime()
	call := func(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
		return nil, context.DeadlineExceeded
	}
	res, err := rt.Execute(context.Background(), `
		try {
			await tools.Fail();
			return "no-error";
		} catch (e) {
			return "caught: " + String(e);
		}
	`, call, Options{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if s, ok := res.Value.(string); !ok || !strings.Contains(s, "caught") {
		t.Errorf("bridge error must be catchable in JS, got %#v", res.Value)
	}
}

func TestMoejs_MemoryLimit(t *testing.T) {
	rt := NewMoejsRuntime()
	_, err := rt.Execute(context.Background(), `
		const a = [];
		for (;;) a.push(new Array(1000).fill(7));
	`, nil, Options{MemoryLimit: 1 << 20})
	if err == nil || !strings.Contains(err.Error(), "memory") {
		t.Errorf("expected memory-limit error, got %v", err)
	}
}

func TestMoejs_TimeoutInterruptsInfiniteLoop(t *testing.T) {
	rt := NewMoejsRuntime()
	start := time.Now()
	_, err := rt.Execute(context.Background(), `for (;;) {}`, nil, Options{Timeout: 100 * time.Millisecond})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if time.Since(start) > 2*time.Second {
		t.Error("timeout did not interrupt promptly")
	}
}

func TestMoejs_ContextCancel(t *testing.T) {
	rt := NewMoejsRuntime()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	_, err := rt.Execute(ctx, `for (;;) {}`, nil, Options{})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestMoejs_EvalDisabled(t *testing.T) {
	rt := NewMoejsRuntime()
	_, err := rt.Execute(context.Background(), `return eval("1+1");`, nil, Options{})
	if err == nil {
		t.Error("eval must be disabled in the sandbox")
	}
}

func TestMoejs_SyntaxErrorReported(t *testing.T) {
	rt := NewMoejsRuntime()
	_, err := rt.Execute(context.Background(), `const x = ;`, nil, Options{})
	if err == nil || !strings.Contains(err.Error(), "compile") {
		t.Errorf("expected compile error, got %v", err)
	}
}

func TestMoejs_SearchDescribeHooks(t *testing.T) {
	rt := NewMoejsRuntime()
	opts := Options{
		Search:   func(q string) (any, error) { return []string{"Echo", "Read"}, nil },
		Describe: func(n string) (any, error) { return map[string]any{"name": n}, nil },
	}
	res, err := rt.Execute(context.Background(), `
		const names = codemode.search("echo");
		const d = codemode.describe(names[0]);
		return d.name;
	`, nil, opts)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Value != "Echo" {
		t.Errorf("search/describe round trip: %#v", res.Value)
	}
}
