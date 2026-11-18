package codemode

import (
	"fmt"

	"github.com/MoeForge/moejs"
)

// installStringHook registers a native sandbox hook taking one string argument
// (CM-03 v3). A nil fn makes the hook raise "not available" inside the sandbox
// — the feature is disabled, never silently ignored.
func installStringHook(rt *moejs.Runtime, name string, fn func(string) (any, error)) error {
	return rt.SetGlobal(name, moejs.NativeFunc(func(_ *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
		if fn == nil {
			return moejs.Undefined(), fmt.Errorf("codemode: %s is not available", name)
		}
		v, err := fn(moejs.Arg(args, 0).String())
		if err != nil {
			return moejs.Undefined(), err
		}
		return rt.FromGo(v)
	}))
}

// installNullaryHook registers a native sandbox hook taking no arguments
// (CM-03 v3). Same nil semantics as installStringHook.
func installNullaryHook(rt *moejs.Runtime, name string, fn func() (any, error)) error {
	return rt.SetGlobal(name, moejs.NativeFunc(func(_ *moejs.Realm, _ moejs.Value, _ []moejs.Value) (moejs.Value, error) {
		if fn == nil {
			return moejs.Undefined(), fmt.Errorf("codemode: %s is not available", name)
		}
		v, err := fn()
		if err != nil {
			return moejs.Undefined(), err
		}
		return rt.FromGo(v)
	}))
}
