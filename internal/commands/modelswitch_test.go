package commands

import "testing"

func TestModelCommand_NoArgsOpensPicker(t *testing.T) {
	res := cmdModel().Execute(CommandContext{Model: "m"}, "")
	if res.OpenDialog != "model" || res.Display != DisplayNone {
		t.Errorf("result = %+v", res)
	}
}

func TestModelCommand_RefPassesThrough(t *testing.T) {
	res := cmdModel().Execute(CommandContext{}, " pro ")
	if res.ModelRef != "pro" || res.NewModel != "" || res.Display != DisplayNone {
		t.Errorf("result = %+v", res)
	}
}
