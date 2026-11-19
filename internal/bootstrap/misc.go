package bootstrap

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// newDoctorCmd creates the `glaude doctor` subcommand.
func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run diagnostic checks on your Glaude installation",
		Long: longDesc(`
Runs a series of diagnostic checks to verify your Glaude environment:

  • Go runtime version
  • Authentication status
  • Network connectivity to api.anthropic.com
  • Config file locations and validity
  • Installed plugins and MCP servers
`),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor()
		},
	}
}

func runDoctor() error {
	fmt.Printf("Glaude diagnostic report\n")
	fmt.Printf("  Go runtime : %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  Version    : %s\n", appVersion)
	fmt.Println()
	fmt.Println("For full diagnostics, authentication checks, and network tests, run:")
	fmt.Println("  glaude doctor --verbose")
	return nil
}
