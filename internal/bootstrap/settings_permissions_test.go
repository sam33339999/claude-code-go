package bootstrap

import (
	"testing"

	"github.com/tunsuy/glaude/internal/config"
	"github.com/tunsuy/glaude/internal/state"
	"github.com/tunsuy/glaude/pkg/types"
)

func TestApplySettingsPermissions_LoadsRulesPerLayer(t *testing.T) {
	settings := &config.LayeredSettings{
		User:    &config.SettingsJson{Permissions: &config.PermissionsConfig{Allow: []string{"Bash(go test:*)"}}},
		Project: &config.SettingsJson{Permissions: &config.PermissionsConfig{Allow: []string{"Read"}, Deny: []string{"WebFetch"}}},
		Local:   &config.SettingsJson{Permissions: &config.PermissionsConfig{Ask: []string{"Bash(git push:*)"}}},
		Merged:  &config.SettingsJson{Permissions: &config.PermissionsConfig{DefaultMode: types.PermissionModeAcceptEdits}},
	}
	pc := state.GetDefaultAppState().ToolPermissionContext
	applySettingsPermissions(&pc, settings)
	if got := pc.AlwaysAllowRules[types.RuleSourceUser]; len(got) != 1 || got[0] != "Bash(go test:*)" {
		t.Errorf("user allow = %v", got)
	}
	if got := pc.AlwaysAllowRules[types.RuleSourceProject]; len(got) != 1 || got[0] != "Read" {
		t.Errorf("project allow = %v", got)
	}
	if got := pc.AlwaysDenyRules[types.RuleSourceProject]; len(got) != 1 {
		t.Errorf("project deny = %v", got)
	}
	if got := pc.AlwaysAskRules[types.RuleSourceLocal]; len(got) != 1 {
		t.Errorf("local ask = %v", got)
	}
	if pc.Mode != types.PermissionModeAcceptEdits {
		t.Errorf("mode = %q", pc.Mode)
	}
}

func TestApplySettingsPermissions_ModeGuards(t *testing.T) {
	cases := []struct {
		mode    types.PermissionMode
		disable string
		want    types.PermissionMode
	}{
		{"bogus", "", types.PermissionModeDefault},
		{types.PermissionModeBypassPermissions, "disable", types.PermissionModeDefault},
		{types.PermissionModeBypassPermissions, "", types.PermissionModeBypassPermissions},
	}
	for _, c := range cases {
		pc := state.GetDefaultAppState().ToolPermissionContext
		applySettingsPermissions(&pc, &config.LayeredSettings{Merged: &config.SettingsJson{
			Permissions: &config.PermissionsConfig{DefaultMode: c.mode, DisableBypass: c.disable},
		}})
		if pc.Mode != c.want {
			t.Errorf("mode %q disable %q → %q, want %q", c.mode, c.disable, pc.Mode, c.want)
		}
	}
	applySettingsPermissions(nil, nil) // tolerates nil
}
