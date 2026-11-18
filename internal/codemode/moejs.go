package codemode

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/MoeForge/moejs"
	"github.com/MoeForge/moejs/eventloop"
)

// MoejsRuntime is the primary Code Mode engine (CM-01 §9): pure Go, zero CGO,
// native async/await + Promise, MemoryLimit, cross-goroutine promise settling
// (true parallel tool calls), eval/Function disabled. Alpha API is pinned in
// go.mod and isolated behind the Runtime interface.
type MoejsRuntime struct{}

// NewMoejsRuntime returns the moejs-backed Code Mode engine.
func NewMoejsRuntime() *MoejsRuntime { return &MoejsRuntime{} }

// sandboxPrelude installs the only outward channels: tools.* (bridged,
// permission-gated), codemode.search/describe (progressive discovery) and
// console.log (captured). No fs, no net, no env, no eval.
const sandboxPrelude = `
globalThis.__logs = [];
globalThis.console = {
	log: (...a) => __log(a.map(x => typeof x === "string" ? x : JSON.stringify(x)).join(" ")),
	info: (...a) => __log(a.map(x => typeof x === "string" ? x : JSON.stringify(x)).join(" ")),
	warn: (...a) => __log(a.map(x => typeof x === "string" ? x : JSON.stringify(x)).join(" ")),
	error: (...a) => __log(a.map(x => typeof x === "string" ? x : JSON.stringify(x)).join(" ")),
};
globalThis.tools = new Proxy({}, {
	get: (_t, name) => (...args) => __call(String(name), JSON.stringify(args[0] ?? {})),
});
globalThis.codemode = {
	search: (q) => __search(String(q)),
	describe: (n) => __describe(String(n)),
	history: (n) => __history(Number(n) || 10),
};
`

// Execute runs code as the body of an async function. The resolved return
// value is captured via .then; rejections and runtime errors are returned as
// Go errors. Each call gets a fresh isolated runtime (cheap: ~µs / ~KiB).
func (m *MoejsRuntime) Execute(ctx context.Context, code string, call CallFunc, opts Options) (*Result, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	memLimit := opts.MemoryLimit
	if memLimit <= 0 {
		memLimit = DefaultMemoryLimit
	}

	mopts := moejs.Options{
		MemoryLimit:        memLimit,
		DisableDynamicCode: true, // no eval / new Function in the sandbox
	}
	if opts.ImportModule != nil {
		resolve := opts.ImportModule
		mopts.Importer = &moejs.Importer{
			Resolve: func(_ moejs.Referrer, specifier string) (*moejs.Module, error) {
				src, err := resolve(specifier)
				if err != nil {
					return nil, err
				}
				return moejs.Compile("codemode:"+specifier, src)
			},
		}
	}
	rt := moejs.NewRuntime(mopts)
	loop, err := eventloop.New(rt, eventloop.Options{})
	if err != nil {
		return nil, fmt.Errorf("codemode: event loop: %w", err)
	}

	var (
		value   any
		logs    []string
		codeErr error
	)

	// __log captures console output (runs on the loop).
	if err := rt.SetGlobal("__log", moejs.NativeFunc(func(_ *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
		logs = append(logs, moejs.Arg(args, 0).String())
		return moejs.Undefined(), nil
	})); err != nil {
		return nil, fmt.Errorf("codemode: install __log: %w", err)
	}

	// __call bridges one tool invocation: returns a promise, does the blocking
	// call on a goroutine, settles on the loop. Parallel Promise.all works.
	if err := rt.SetGlobal("__call", moejs.NativeFunc(func(_ *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
		name := moejs.Arg(args, 0).String()
		input := json.RawMessage(moejs.Arg(args, 1).String())
		p, settle := loop.NewPromise()
		go func() {
			out, callErr := call(ctx, name, input)
			settle(func(rt *moejs.Runtime) (moejs.Value, error) {
				if callErr != nil {
					return moejs.Undefined(), callErr
				}
				var decoded any
				if len(out) > 0 {
					if err := json.Unmarshal(out, &decoded); err != nil {
						// Non-JSON tool content: pass through as a string.
						return rt.FromGo(string(out))
					}
				}
				return rt.FromGo(decoded)
			})
		}()
		return p, nil
	})); err != nil {
		return nil, fmt.Errorf("codemode: install __call: %w", err)
	}

	// Progressive discovery hooks (synchronous, in-memory catalog).
	if err := rt.SetGlobal("__search", moejs.NativeFunc(func(_ *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
		if opts.Search == nil {
			return moejs.Undefined(), fmt.Errorf("codemode: search is not available")
		}
		v, err := opts.Search(moejs.Arg(args, 0).String())
		if err != nil {
			return moejs.Undefined(), err
		}
		return rt.FromGo(v)
	})); err != nil {
		return nil, fmt.Errorf("codemode: install __search: %w", err)
	}
	if err := rt.SetGlobal("__describe", moejs.NativeFunc(func(_ *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
		if opts.Describe == nil {
			return moejs.Undefined(), fmt.Errorf("codemode: describe is not available")
		}
		v, err := opts.Describe(moejs.Arg(args, 0).String())
		if err != nil {
			return moejs.Undefined(), err
		}
		return rt.FromGo(v)
	})); err != nil {
		return nil, fmt.Errorf("codemode: install __describe: %w", err)
	}

	// Result capture: the async body's resolved value / rejection.
	if err := rt.SetGlobal("__history", moejs.NativeFunc(func(_ *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
		if opts.History == nil {
			return moejs.Undefined(), fmt.Errorf("codemode: history is not available")
		}
		n, _ := strconv.Atoi(moejs.Arg(args, 0).String())
		v, err := opts.History(n)
		if err != nil {
			return moejs.Undefined(), err
		}
		return rt.FromGo(v)
	})); err != nil {
		return nil, fmt.Errorf("codemode: install __history: %w", err)
	}
	if err := rt.SetGlobal("__capture", moejs.NativeFunc(func(_ *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
		v, err := rt.ToGoWith(moejs.Arg(args, 0), moejs.DecodeOptions{})
		if err != nil {
			codeErr = fmt.Errorf("codemode: decode result: %w", err)
			return moejs.Undefined(), nil
		}
		value = v
		return moejs.Undefined(), nil
	})); err != nil {
		return nil, fmt.Errorf("codemode: install __capture: %w", err)
	}
	if err := rt.SetGlobal("__captureErr", moejs.NativeFunc(func(_ *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
		codeErr = fmt.Errorf("codemode: %s", moejs.Arg(args, 0).String())
		return moejs.Undefined(), nil
	})); err != nil {
		return nil, fmt.Errorf("codemode: install __captureErr: %w", err)
	}

	source := sandboxPrelude + "\n(async () => {\n" + code + "\n})().then(v => __capture(v), e => __captureErr(e && e.message ? String(e.message) : String(e)));\n"

	// Guards: timeout and caller cancellation both interrupt the script.
	timer := time.AfterFunc(timeout, func() { rt.Interrupt("codemode: execution timed out") })
	defer timer.Stop()
	stopCtxWatch := context.AfterFunc(ctx, func() { rt.Interrupt("codemode: cancelled") })
	defer stopCtxWatch()

	var runErr error
	if opts.Typescript {
		mod, err := moejs.CompileTS("codemode.ts", source)
		if err != nil {
			return nil, fmt.Errorf("codemode: compile: %w", err)
		}
		runErr = loop.Run(func(rt *moejs.Runtime) error { return rt.Load(mod) })
	} else {
		script, err := moejs.CompileScript("codemode.js", source)
		if err != nil {
			return nil, fmt.Errorf("codemode: compile: %w", err)
		}
		runErr = loop.Run(func(rt *moejs.Runtime) error {
			_, err := rt.RunScript(script)
			return err
		})
	}
	if codeErr != nil {
		return nil, codeErr
	}
	if runErr != nil {
		return nil, fmt.Errorf("codemode: %w", runErr)
	}
	return &Result{Value: value, Logs: logs}, nil
}
