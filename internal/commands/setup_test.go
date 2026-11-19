package commands

import "testing"

func TestSetupCommand_OpensDialog(t *testing.T) {
	res := cmdSetup().Execute(CommandContext{}, "")
	if res.OpenDialog != "setup" || res.Display != DisplayNone {
		t.Errorf("result = %+v, want OpenDialog setup with DisplayNone", res)
	}
}

func TestSetupCommand_Registered(t *testing.T) {
	r := NewRegistry()
	RegisterBuiltins(r)
	if r.Lookup("setup") == nil {
		t.Error("/setup not registered")
	}
}
