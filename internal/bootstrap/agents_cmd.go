package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tunsuy/claude-code-go/internal/agenttype"
)

// newAgentsCmd creates the `claude agents` command.
//
// Shape aligned with the oracle (TS main.tsx): a single listing command with
// --setting-sources, NOT list/add/remove subcommands (the previous Go stub
// shape was a documented divergence, closed by BF-06 per cases.md guidance).
// Custom agents are managed by editing definition files under the agents
// directories, mirroring the oracle's file-based model.
func newAgentsCmd() *cobra.Command {
	var settingSources string
	cmd := &cobra.Command{
		Use:   "agents",
		Short: "List configured agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("agents: home dir: %w", err)
			}
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("agents: working dir: %w", err)
			}
			out, err := formatAgentsList(home, cwd, settingSources)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&settingSources, "setting-sources", "",
		"Comma-separated list of setting sources to load (user, project, local)")
	return cmd
}

// agentEntry is one displayable agent with its source group and shadow state.
type agentEntry struct {
	typeName   string
	model      string
	shadowedBy string // non-empty when a higher-priority source overrides it
}

// formatAgentsList renders the oracle-style agent listing:
//
//	N active agents
//
//	User agents:
//	  name · model
//
//	Project agents:
//	  ...
//
//	Built-in agents:
//	  ...
//
// Project definitions shadow user definitions of the same type (same
// precedence as loadCustomAgentProfiles in wire.go). Sources filter which
// custom groups load; "local" is accepted for oracle compatibility but Go
// has no separate local agents directory (documented divergence, cases.md).
// Built-in agents always list.
func formatAgentsList(homeDir, cwd, sources string) (string, error) {
	loadUser, loadProject := true, true
	if strings.TrimSpace(sources) != "" {
		loadUser, loadProject = false, false
		for _, s := range strings.Split(sources, ",") {
			switch strings.TrimSpace(s) {
			case "user":
				loadUser = true
			case "project":
				loadProject = true
			case "local":
				// Accepted for oracle parity; no local agents dir in Go.
			default:
				return "", fmt.Errorf("agents: unknown setting source %q (want user, project, local)", s)
			}
		}
	}

	var userEntries, projectEntries []agentEntry
	if loadUser {
		profiles, err := agenttype.LoadCustomAgents(filepath.Join(homeDir, ".claude", "agents"))
		if err != nil {
			return "", fmt.Errorf("agents: load user agents: %w", err)
		}
		userEntries = entriesFromProfiles(profiles)
	}
	if loadProject {
		profiles, err := agenttype.LoadCustomAgents(filepath.Join(cwd, ".claude", "agents"))
		if err != nil {
			return "", fmt.Errorf("agents: load project agents: %w", err)
		}
		projectEntries = entriesFromProfiles(profiles)
	}

	// Project shadows user (wire.go registration precedence).
	projectTypes := make(map[string]bool, len(projectEntries))
	for _, e := range projectEntries {
		projectTypes[e.typeName] = true
	}
	for i := range userEntries {
		if projectTypes[userEntries[i].typeName] {
			userEntries[i].shadowedBy = "project"
		}
	}

	builtinReg := agenttype.NewRegistry()
	agenttype.RegisterBuiltins(builtinReg)
	builtinEntries := entriesFromProfiles(builtinReg.All())

	active := len(projectEntries) + len(builtinEntries)
	for _, e := range userEntries {
		if e.shadowedBy == "" {
			active++
		}
	}
	if active == 0 {
		return "No agents found.\n", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d active agents\n", active)
	writeGroup := func(label string, entries []agentEntry) {
		if len(entries) == 0 {
			return
		}
		sb.WriteString("\n" + label + ":\n")
		for _, e := range entries {
			line := e.typeName
			if e.model != "" {
				line += " · " + e.model
			}
			if e.shadowedBy != "" {
				line = "(shadowed by " + e.shadowedBy + ") " + line
			}
			sb.WriteString("  " + line + "\n")
		}
	}
	writeGroup("User agents", userEntries)
	writeGroup("Project agents", projectEntries)
	writeGroup("Built-in agents", builtinEntries)
	return sb.String(), nil
}

// entriesFromProfiles converts profiles to sorted display entries.
func entriesFromProfiles(profiles []*agenttype.AgentProfile) []agentEntry {
	entries := make([]agentEntry, 0, len(profiles))
	for _, p := range profiles {
		entries = append(entries, agentEntry{typeName: string(p.Type), model: p.EffectiveModel()})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].typeName < entries[j].typeName })
	return entries
}
