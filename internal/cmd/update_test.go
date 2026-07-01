package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

func TestUpdateStatusCmdJSON(t *testing.T) {
	oldClient := updateHTTPClient
	oldLatestURL := updateLatestReleaseURL
	oldLatestWebURL := updateLatestWebURL
	oldVersion := version
	oldCommit := commit
	oldDate := date
	defer func() {
		updateHTTPClient = oldClient
		updateLatestReleaseURL = oldLatestURL
		updateLatestWebURL = oldLatestWebURL
		version = oldVersion
		commit = oldCommit
		date = oldDate
	}()

	version = "v0.31.0"
	commit = "abc1234"
	date = "2026-06-26T10:00:00Z"

	var serverURL string
	assetName := platformAssetName("v0.31.1", runtime.GOOS, runtime.GOARCH)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{
				"tag_name": "v0.31.1",
				"html_url": "https://github.com/openclaw/gogcli/releases/tag/v0.31.1",
				"assets": [
					{"name": %q, "browser_download_url": %q},
					{"name": "checksums.txt", "browser_download_url": %q}
				]
			}`, assetName, serverURL+"/download/"+assetName, serverURL+"/checksums.txt")
		case "/checksums.txt":
			_, _ = fmt.Fprintf(w, "%s  %s\n", strings.Repeat("a", 64), assetName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL
	updateHTTPClient = server.Client()
	updateLatestReleaseURL = server.URL + "/latest"

	result := executeWithTestRuntime(t, []string{"--json", "update", "status"}, nil)
	if result.err != nil {
		t.Fatalf("execute: %v\nstderr=%s", result.err, result.stderr)
	}

	var parsed updateStatusReport
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nstdout=%s", err, result.stdout)
	}
	if parsed.CurrentVersion != "v0.31.0" {
		t.Fatalf("current_version = %q", parsed.CurrentVersion)
	}
	if parsed.CurrentCommit != "abc1234" {
		t.Fatalf("current_commit = %q", parsed.CurrentCommit)
	}
	if parsed.LatestVersion != "v0.31.1" {
		t.Fatalf("latest_version = %q", parsed.LatestVersion)
	}
	if !parsed.UpdateAvailable {
		t.Fatalf("expected update_available")
	}
	if parsed.PlatformAsset != assetName {
		t.Fatalf("platform_asset = %q, want %q", parsed.PlatformAsset, assetName)
	}
	if parsed.PlatformAssetSHA256 != strings.Repeat("a", 64) {
		t.Fatalf("platform_asset_sha256 = %q", parsed.PlatformAssetSHA256)
	}
	if !parsed.ChecksumAvailable {
		t.Fatalf("expected checksum_available")
	}
}

func TestFetchAssetChecksumRejectsMalformedDigest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, "not-a-sha256  archive.tar.gz")
	}))
	defer server.Close()

	_, err := fetchAssetChecksum(t.Context(), server.Client(), server.URL, "archive.tar.gz")
	if err == nil || !strings.Contains(err.Error(), "invalid checksum") {
		t.Fatalf("expected invalid checksum error, got %v", err)
	}
}

func TestFetchLatestGitHubReleaseFallsBackToWebRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/latest":
			http.Error(w, "rate limited", http.StatusForbidden)
		case "/releases/latest":
			http.Redirect(w, r, "https://github.com/openclaw/gogcli/releases/tag/v0.31.1", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	oldWebURL := updateLatestWebURL
	updateLatestWebURL = server.URL + "/releases/latest"
	t.Cleanup(func() { updateLatestWebURL = oldWebURL })

	release, err := fetchLatestGitHubRelease(t.Context(), server.Client(), server.URL+"/api/latest")
	if err != nil {
		t.Fatalf("fetch latest release: %v", err)
	}
	if release.TagName != "v0.31.1" || !release.SyntheticAssets {
		t.Fatalf("unexpected fallback release: %#v", release)
	}
	checksum, ok := findReleaseAsset(release.Assets, "checksums.txt")
	if !ok || checksum.BrowserDownloadURL != "https://github.com/openclaw/gogcli/releases/download/v0.31.1/checksums.txt" {
		t.Fatalf("unexpected checksum asset: %#v", checksum)
	}
}

func TestUpdateStatusCheckAlias(t *testing.T) {
	oldClient := updateHTTPClient
	oldLatestURL := updateLatestReleaseURL
	oldLatestWebURL := updateLatestWebURL
	oldVersion := version
	defer func() {
		updateHTTPClient = oldClient
		updateLatestReleaseURL = oldLatestURL
		updateLatestWebURL = oldLatestWebURL
		version = oldVersion
	}()
	version = "v0.31.1"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/latest" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"tag_name":"v0.31.1","assets":[]}`)
	}))
	defer server.Close()
	updateHTTPClient = server.Client()
	updateLatestReleaseURL = server.URL + "/latest"

	result := executeWithTestRuntime(t, []string{"--json", "update", "check"}, nil)
	if result.err != nil {
		t.Fatalf("execute: %v\nstderr=%s", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, `"update_available": false`) {
		t.Fatalf("unexpected stdout: %s", result.stdout)
	}
}

func TestUpdateVersionComparison(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    bool
		ok      bool
	}{
		{current: "v0.31.0", latest: "v0.31.1", want: true, ok: true},
		{current: "v0.31.1", latest: "v0.31.1", want: false, ok: true},
		{current: "v0.31.2-dev", latest: "v0.31.1", want: false, ok: true},
		{current: "dev", latest: "v0.31.1", want: false, ok: false},
	}
	for _, tt := range tests {
		got, ok := updateAvailable(tt.current, tt.latest)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("updateAvailable(%q, %q) = (%t, %t), want (%t, %t)", tt.current, tt.latest, got, ok, tt.want, tt.ok)
		}
	}
}
