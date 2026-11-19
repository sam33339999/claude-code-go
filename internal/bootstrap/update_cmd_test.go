package bootstrap

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.1.0", "0.1.0", 0},
		{"v0.1.0", "0.1.0", 0},
		{"0.1.0", "0.2.0", -1},
		{"0.2.0", "0.1.9", 1},
		{"1.0.0", "0.9.9", 1},
		{"0.8", "0.8.0", 0},
		{"0.8.0", "0.8.1", -1},
		{"0.10.0", "0.9.0", 1},
		{"1.0.0-rc1", "1.0.0", 0}, // pre-release suffix ignored (documented)
	}
	for _, tc := range cases {
		if got := compareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("compareVersions(%q,%q)=%d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// withReleaseServer points the package-level release vars at a test server.
func withReleaseServer(t *testing.T, handler http.Handler) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	oldAPI, oldDL, oldRepo := releasesAPIBase, releasesDownloadBase, releasesRepo
	releasesAPIBase, releasesDownloadBase, releasesRepo = srv.URL, srv.URL, "test/repo"
	t.Cleanup(func() { releasesAPIBase, releasesDownloadBase, releasesRepo = oldAPI, oldDL, oldRepo })
}

func TestRunUpdate_NewerAvailable(t *testing.T) {
	withReleaseServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/test/repo/releases/latest" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"tag_name":"v9.9.9"}`))
	}))
	var buf bytes.Buffer
	if err := runUpdate(false, &buf); err != nil {
		t.Fatalf("runUpdate: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Current version:", "New version available: 9.9.9", "glaude install 9.9.9"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}
}

func TestRunUpdate_UpToDate(t *testing.T) {
	withReleaseServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v0.0.1"}`))
	}))
	var buf bytes.Buffer
	if err := runUpdate(false, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Already up to date") {
		t.Errorf("expected up-to-date message, got:\n%s", buf.String())
	}
}

func TestRunUpdate_CheckOnlyNoInstallHint(t *testing.T) {
	withReleaseServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v9.9.9"}`))
	}))
	var buf bytes.Buffer
	if err := runUpdate(true, &buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "glaude install") {
		t.Errorf("--check must not print install hint, got:\n%s", buf.String())
	}
}

func TestRunUpdate_APIError(t *testing.T) {
	withReleaseServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	var buf bytes.Buffer
	if err := runUpdate(false, &buf); err == nil {
		t.Error("expected error on API failure")
	}
}

func TestUpdateCmd_UpgradeAlias(t *testing.T) {
	cmd := newUpdateCmd()
	found := false
	for _, a := range cmd.Aliases {
		if a == "upgrade" {
			found = true
		}
	}
	if !found {
		t.Error("expected upgrade alias (oracle parity, closes cases.md second-level gap)")
	}
	if cmd.Flags().Lookup("check") == nil {
		t.Error("expected --check flag")
	}
}
