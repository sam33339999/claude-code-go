package bootstrap

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// newInstallCmd creates the `claude install [target]` command (oracle shape:
// target = stable, latest, or a specific version; --force overrides an
// existing installation).
//
// The Go build ships as GitHub release archives (goreleaser), so install
// downloads the archive for the current platform, extracts the claude
// binary and places it in ~/.local/bin. Documented divergence (cases.md
// B10): no package-manager integration; the oracle's native installer
// manages symlinks/channels, ours is a direct binary install.
func newInstallCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "install [target]",
		Short: "Install Claude Code native build. Use [target] to specify version (stable, latest, or specific version)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "latest"
			if len(args) > 0 {
				target = args[0]
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("install: home dir: %w", err)
			}
			return runInstall(installOptions{
				target:       target,
				force:        force,
				apiBase:      releasesAPIBase,
				downloadBase: releasesDownloadBase,
				repo:         releasesRepo,
				destDir:      filepath.Join(home, ".local", "bin"),
				goos:         runtime.GOOS,
				goarch:       runtime.GOARCH,
				out:          cmd.OutOrStdout(),
			})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Force installation even if already installed")
	return cmd
}

// installOptions parameterises runInstall so tests can inject endpoints,
// platform and destination without touching the real environment.
type installOptions struct {
	target       string
	force        bool
	apiBase      string
	downloadBase string
	repo         string
	destDir      string
	goos         string
	goarch       string
	out          io.Writer
}

// runInstall resolves the requested version, downloads the platform archive,
// extracts the claude binary and installs it atomically into destDir.
func runInstall(opts installOptions) error {
	version, err := resolveInstallVersion(opts.target, opts.apiBase, opts.repo)
	if err != nil {
		return fmt.Errorf("install: %w", err)
	}

	binaryName := "claude"
	if opts.goos == "windows" {
		binaryName = "claude.exe"
	}
	dest := filepath.Join(opts.destDir, binaryName)
	if _, err := os.Stat(dest); err == nil && !opts.force {
		return fmt.Errorf("install: %s already exists (use --force to overwrite)", dest)
	}

	asset := releaseAssetName(version, opts.goos, opts.goarch)
	url := fmt.Sprintf("%s/%s/releases/download/v%s/%s", opts.downloadBase, opts.repo, version, asset)
	fmt.Fprintf(opts.out, "Downloading %s...\n", url)
	tmpAsset, err := downloadToTemp(url)
	if err != nil {
		return fmt.Errorf("install: %w", err)
	}
	defer os.Remove(tmpAsset)

	var binary []byte
	if strings.HasSuffix(asset, ".zip") {
		binary, err = extractBinaryFromZip(tmpAsset, binaryName)
	} else {
		binary, err = extractBinaryFromTarGz(tmpAsset, binaryName)
	}
	if err != nil {
		return fmt.Errorf("install: %w", err)
	}

	if err := os.MkdirAll(opts.destDir, 0o755); err != nil {
		return fmt.Errorf("install: create %s: %w", opts.destDir, err)
	}
	tmpBin, err := os.CreateTemp(opts.destDir, ".claude-install-*")
	if err != nil {
		return fmt.Errorf("install: temp file: %w", err)
	}
	tmpName := tmpBin.Name()
	defer os.Remove(tmpName)
	if _, err := tmpBin.Write(binary); err != nil {
		tmpBin.Close()
		return fmt.Errorf("install: write binary: %w", err)
	}
	if err := tmpBin.Close(); err != nil {
		return fmt.Errorf("install: close binary: %w", err)
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return fmt.Errorf("install: chmod: %w", err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("install: rename to %s: %w", dest, err)
	}

	fmt.Fprintf(opts.out, "Installed claude %s to %s\n", version, dest)
	if !pathContains(opts.destDir) {
		fmt.Fprintf(opts.out, "Note: %s is not in your PATH; add it to use `claude` directly.\n", opts.destDir)
	}
	return nil
}

// resolveInstallVersion maps stable/latest to the newest release tag and
// validates a specific version against the releases API.
func resolveInstallVersion(target, apiBase, repo string) (string, error) {
	switch target {
	case "", "latest", "stable":
		return fetchLatestVersion(apiBase, repo)
	default:
		ver := strings.TrimPrefix(target, "v")
		url := fmt.Sprintf("%s/repos/%s/releases/tags/v%s", apiBase, repo, ver)
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			return "", fmt.Errorf("verify version %s: %w", ver, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("version %s not found (status %d)", ver, resp.StatusCode)
		}
		return ver, nil
	}
}

// releaseAssetName mirrors the goreleaser archive name_template:
// claude_<version>_<os>_<arch>.tar.gz (amd64→x86_64, 386→i386; windows→zip).
func releaseAssetName(version, goos, goarch string) string {
	arch := goarch
	switch goarch {
	case "amd64":
		arch = "x86_64"
	case "386":
		arch = "i386"
	}
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("claude_%s_%s_%s%s", version, goos, arch, ext)
}

// downloadToTemp fetches url into a temporary file and returns its path.
func downloadToTemp(url string) (string, error) {
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: status %d", url, resp.StatusCode)
	}
	f, err := os.CreateTemp("", "claude-asset-*")
	if err != nil {
		return "", fmt.Errorf("temp file: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	return f.Name(), nil
}

// extractBinaryFromTarGz returns the named file's bytes from a .tar.gz archive.
func extractBinaryFromTarGz(path, name string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar: %w", err)
		}
		if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == name {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("binary %q not found in archive", name)
}

// extractBinaryFromZip returns the named file's bytes from a .zip archive.
func extractBinaryFromZip(path, name string) ([]byte, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("zip: %w", err)
	}
	defer r.Close()
	for _, f := range r.File {
		if filepath.Base(f.Name) == name {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("zip entry: %w", err)
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("binary %q not found in archive", name)
}

// pathContains reports whether dir is on the current PATH.
func pathContains(dir string) bool {
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p == dir {
			return true
		}
	}
	return false
}
