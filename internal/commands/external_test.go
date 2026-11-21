package commands

import (
	"errors"
	"strings"
	"testing"
)

func TestRegisterExternalAgents(t *testing.T) {
	r := NewRegistry()
	RegisterBuiltins(r)
	var gotName, gotTask string
	spawn := func(name, task string) (string, error) {
		gotName, gotTask = name, task
		if task == "fail" {
			return "", errors.New("boom")
		}
		return "grok-123", nil
	}
	RegisterExternalAgents(r, []ExternalAgentCommand{{Name: "grok"}, {Name: "help"}, {Name: ""}}, spawn)

	cmd := r.Lookup("grok")
	if cmd == nil {
		t.Fatal("/grok not registered")
	}
	res := cmd.Execute(CommandContext{}, "  fix the bug  ")
	if res.Display != DisplayMessage || gotName != "grok" || gotTask != "fix the bug" || !strings.Contains(res.Text, "grok-123") {
		t.Fatalf("res = %+v name=%q task=%q", res, gotName, gotTask)
	}
	if res := cmd.Execute(CommandContext{}, "   "); res.Display != DisplayError || !strings.Contains(res.Text, "Usage") {
		t.Fatalf("empty task: %+v", res)
	}
	if res := cmd.Execute(CommandContext{}, "fail"); res.Display != DisplayError || !strings.Contains(res.Text, "boom") {
		t.Fatalf("spawn error: %+v", res)
	}
	// An existing command keeps its implementation.
	if h := r.Lookup("help"); h == nil || strings.Contains(h.Description, "external") {
		t.Fatalf("/help was overridden: %+v", h)
	}
}

func TestRegisterExternalAgents_NilSpawner(t *testing.T) {
	r := NewRegistry()
	RegisterExternalAgents(r, []ExternalAgentCommand{{Name: "grok"}}, nil)
	if r.Lookup("grok") != nil {
		t.Fatal("registered without a spawner")
	}
}
