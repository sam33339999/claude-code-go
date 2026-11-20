package tui

import (
	"reflect"
	"testing"
)

func TestBuildMainSystemPrompt_MatchesStartQuery(t *testing.T) {
	m := newTestModel()
	m.memdirPrompt = "project memory"
	fe := m.queryEngine.(*fakeEngine)
	want := buildMainSystemPrompt(&m)
	if len(want.Parts) != 1 || want.Parts[0].Text != "project memory" || want.Parts[0].CacheControl != "ephemeral" {
		t.Fatalf("helper = %+v", want)
	}
	cmd := startQueryCmd(&m, "hello", true)
	_ = cmd()
	if !reflect.DeepEqual(fe.lastParams.SystemPrompt, want) {
		t.Errorf("startQueryCmd system prompt = %+v, helper = %+v", fe.lastParams.SystemPrompt, want)
	}
}
