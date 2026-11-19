package bootstrap

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeTarGz builds a .tar.gz archive containing the given files.
func makeTarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReleaseAssetName(t *testing.T) {
	cases := []struct {
		ver, os, arch, want string
	}{
		{"1.2.3", "linux", "amd64", "glaude_1.2.3_linux_x86_64.tar.gz"},
		{"1.2.3", "darwin", "arm64", "glaude_1.2.3_darwin_arm64.tar.gz"},
		{"1.2.3", "windows", "amd64", "glaude_1.2.3_windows_x86_64.zip"},
		{"1.2.3", "linux", "386", "glaude_1.2.3_linux_i386.tar.gz"},
	}
	for _, tc := range cases {
		if got := releaseAssetName(tc.ver, tc.os, tc.arch); got != tc.want {
			t.Errorf("releaseAssetName(%s,%s,%s)=%q, want %q", tc.ver, tc.os, tc.arch, got, tc.want)
		}
	}
}

// installTestServer serves the releases API + a linux/amd64 archive whose
// glaude binary is fake but byte-verifiable.
func installTestServer(t *testing.T, version string, archive []byte) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/test/repo/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v` + version + `"}`))
	})
	mux.HandleFunc("/repos/test/repo/releases/tags/v"+version, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v` + version + `"}`))
	})
	asset := releaseAssetName(version, "linux", "amd64")
	mux.HandleFunc("/test/repo/releases/download/v"+version+"/"+asset, func(w http.ResponseWriter, r *http.Request) {
		w.Write(archive)
	})
	withReleaseServer(t, mux)
}

func testInstallOpts(dest string) installOptions {
	return installOptions{
		target:       "latest",
		apiBase:      releasesAPIBase,
		downloadBase: releasesDownloadBase,
		repo:         releasesRepo,
		destDir:      dest,
		goos:         "linux",
		goarch:       "amd64",
		out:          &bytes.Buffer{},
	}
}

func TestRunInstall_Latest(t *testing.T) {
	fakeBin := []byte("#!/bin/sh\necho fake-claude\n")
	installTestServer(t, "1.2.3", makeTarGz(t, map[string][]byte{"glaude": fakeBin, "README.md": []byte("x")}))
	dest := t.TempDir()
	opts := testInstallOpts(dest)
	if err := runInstall(opts); err != nil {
		t.Fatalf("runInstall: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "glaude"))
	if err != nil {
		t.Fatalf("installed binary missing: %v", err)
	}
	if !bytes.Equal(got, fakeBin) {
		t.Error("installed binary content mismatch")
	}
	info, _ := os.Stat(filepath.Join(dest, "glaude"))
	if info.Mode().Perm() != 0o755 {
		t.Errorf("expected 0755, got %v", info.Mode().Perm())
	}
	out := opts.out.(*bytes.Buffer).String()
	if !strings.Contains(out, "Installed glaude 1.2.3") {
		t.Errorf("expected install confirmation, got:\n%s", out)
	}
	if !strings.Contains(out, "not in your PATH") {
		t.Errorf("expected PATH hint for temp dest, got:\n%s", out)
	}
}

func TestRunInstall_ExistingRequiresForce(t *testing.T) {
	fakeBin := []byte("new")
	installTestServer(t, "1.2.3", makeTarGz(t, map[string][]byte{"glaude": fakeBin}))
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, "glaude"), []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	opts := testInstallOpts(dest)
	if err := runInstall(opts); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("expected --force error, got %v", err)
	}
	opts.force = true
	if err := runInstall(opts); err != nil {
		t.Fatalf("force install: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dest, "glaude"))
	if string(got) != "new" {
		t.Error("force install should overwrite")
	}
}

func TestRunInstall_SpecificVersion(t *testing.T) {
	fakeBin := []byte("v2")
	installTestServer(t, "2.0.0", makeTarGz(t, map[string][]byte{"glaude": fakeBin}))
	dest := t.TempDir()
	opts := testInstallOpts(dest)
	opts.target = "v2.0.0"
	if err := runInstall(opts); err != nil {
		t.Fatalf("specific version install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "glaude")); err != nil {
		t.Errorf("binary missing: %v", err)
	}
}

func TestRunInstall_UnknownVersion(t *testing.T) {
	withReleaseServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	opts := testInstallOpts(t.TempDir())
	opts.target = "99.99.99"
	if err := runInstall(opts); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestRunInstall_MissingBinaryInArchive(t *testing.T) {
	installTestServer(t, "1.2.3", makeTarGz(t, map[string][]byte{"README.md": []byte("no binary")}))
	opts := testInstallOpts(t.TempDir())
	if err := runInstall(opts); err == nil || !strings.Contains(err.Error(), "not found in archive") {
		t.Errorf("expected missing-binary error, got %v", err)
	}
}

func TestExtractBinaryFromZip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("glaude.exe")
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("windows-binary"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "a.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := extractBinaryFromZip(path, "glaude.exe")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if string(got) != "windows-binary" {
		t.Errorf("content mismatch: %q", got)
	}
	if _, err := extractBinaryFromZip(path, "absent"); err == nil {
		t.Error("expected error for absent entry")
	}
}
