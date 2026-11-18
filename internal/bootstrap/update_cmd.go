package bootstrap

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// releaseSource locates Claude Code releases. Bases are vars so tests can
// point them at httptest servers; production uses GitHub.
var (
	releasesAPIBase      = "https://api.github.com"
	releasesDownloadBase = "https://github.com"
	releasesRepo         = "tunsuy/claude-code-go"
)

// newUpdateCmd creates the `claude update` command (alias `upgrade`, oracle
// parity). It checks the latest GitHub release and reports availability.
//
// Documented divergence (cases.md B8): the oracle auto-installs via its
// npm/native installer; the Go build is distributed as GitHub release
// archives, so update reports the new version and points at `claude install`
// (BF-06) instead of silently replacing the running binary.
func newUpdateCmd() *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"upgrade"},
		Short:   "Check for updates and install if available",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(check, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "Only check for a new version without installing")
	return cmd
}

// runUpdate prints the current version, checks the latest release and
// reports the result. checkOnly suppresses the install hint.
func runUpdate(checkOnly bool, out io.Writer) error {
	fmt.Fprintf(out, "Current version: %s\n", appVersion)
	fmt.Fprintln(out, "Checking for updates to latest version...")
	latest, err := fetchLatestVersion(releasesAPIBase, releasesRepo)
	if err != nil {
		return fmt.Errorf("update: check failed: %w", err)
	}
	if compareVersions(appVersion, latest) >= 0 {
		fmt.Fprintf(out, "Already up to date (%s).\n", appVersion)
		return nil
	}
	fmt.Fprintf(out, "New version available: %s (current %s)\n", latest, appVersion)
	fmt.Fprintf(out, "Release: %s/%s/releases/tag/v%s\n", releasesDownloadBase, releasesRepo, latest)
	if !checkOnly {
		fmt.Fprintf(out, "To install: claude install %s\n", latest)
	}
	return nil
}

// fetchLatestVersion queries the GitHub releases API and returns the latest
// tag with any leading "v" stripped.
func fetchLatestVersion(apiBase, repo string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", apiBase, repo)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("request %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode release: %w", err)
	}
	if payload.TagName == "" {
		return "", fmt.Errorf("release has no tag_name")
	}
	return strings.TrimPrefix(payload.TagName, "v"), nil
}

// compareVersions compares dot-separated numeric versions (leading "v" and
// pre-release suffixes ignored). Returns -1 if a<b, 0 if equal, 1 if a>b.
// Non-numeric segments compare as 0, keeping the check honest for the
// semver tags this project publishes.
func compareVersions(a, b string) int {
	pa := strings.SplitN(strings.TrimPrefix(a, "v"), "-", 2)[0]
	pb := strings.SplitN(strings.TrimPrefix(b, "v"), "-", 2)[0]
	sa := strings.Split(pa, ".")
	sb := strings.Split(pb, ".")
	for i := 0; i < len(sa) || i < len(sb); i++ {
		var na, nb int
		if i < len(sa) {
			na, _ = strconv.Atoi(sa[i])
		}
		if i < len(sb) {
			nb, _ = strconv.Atoi(sb[i])
		}
		if na != nb {
			if na < nb {
				return -1
			}
			return 1
		}
	}
	return 0
}
