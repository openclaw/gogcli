package cmd

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const (
	updateDefaultLatestReleaseURL = "https://api.github.com/repos/openclaw/gogcli/releases/latest"
	updateDefaultLatestWebURL     = "https://github.com/openclaw/gogcli/releases/latest"
	updateDefaultTimeout          = 10 * time.Second
	updateInstallUnknown          = "unknown"
)

var (
	updateHTTPClient       = http.DefaultClient
	updateLatestReleaseURL = updateDefaultLatestReleaseURL
	updateLatestWebURL     = updateDefaultLatestWebURL
)

type UpdateCmd struct {
	Status UpdateStatusCmd `cmd:"" name:"status" aliases:"check" help:"Show installed and latest gogcli release status"`
}

type UpdateStatusCmd struct {
	Timeout time.Duration `name:"timeout" help:"HTTP timeout for GitHub release metadata" default:"10s"`
}

type updateStatusReport struct {
	CurrentVersion      string   `json:"current_version"`
	CurrentCommit       string   `json:"current_commit,omitempty"`
	CurrentDate         string   `json:"current_date,omitempty"`
	LatestVersion       string   `json:"latest_version,omitempty"`
	LatestURL           string   `json:"latest_url,omitempty"`
	UpdateAvailable     bool     `json:"update_available"`
	Platform            string   `json:"platform"`
	PlatformAsset       string   `json:"platform_asset,omitempty"`
	PlatformAssetURL    string   `json:"platform_asset_url,omitempty"`
	ChecksumAvailable   bool     `json:"checksum_available"`
	ChecksumsURL        string   `json:"checksums_url,omitempty"`
	PlatformAssetSHA256 string   `json:"platform_asset_sha256,omitempty"`
	InstallMethod       string   `json:"install_method"`
	Executable          string   `json:"executable,omitempty"`
	Warnings            []string `json:"warnings,omitempty"`
}

type githubRelease struct {
	TagName         string               `json:"tag_name"`
	Name            string               `json:"name"`
	HTMLURL         string               `json:"html_url"`
	Assets          []githubReleaseAsset `json:"assets"`
	SyntheticAssets bool                 `json:"-"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func (c *UpdateStatusCmd) Run(ctx context.Context) error {
	report, err := buildUpdateStatusReport(ctx, c.Timeout)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), report)
	}
	writeUpdateStatusText(ctx, report)
	return nil
}

func buildUpdateStatusReport(ctx context.Context, timeout time.Duration) (updateStatusReport, error) {
	current := resolvedVersion()
	platform := runtime.GOOS + "/" + runtime.GOARCH
	installMethod, executable, installWarnings := detectUpdateInstallMethod()
	report := updateStatusReport{
		CurrentVersion: current,
		CurrentCommit:  strings.TrimSpace(commit),
		CurrentDate:    strings.TrimSpace(date),
		Platform:       platform,
		InstallMethod:  installMethod,
		Executable:     executable,
		Warnings:       installWarnings,
	}

	client := updateClient(timeout)
	release, err := fetchLatestGitHubRelease(ctx, client, updateLatestReleaseURL)
	if err != nil {
		return updateStatusReport{}, err
	}
	report.LatestVersion = strings.TrimSpace(release.TagName)
	report.LatestURL = strings.TrimSpace(release.HTMLURL)
	if report.LatestVersion == "" {
		report.Warnings = append(report.Warnings, "latest release did not include tag_name")
	}

	updateAvailable, versionsComparable := updateAvailable(current, report.LatestVersion)
	report.UpdateAvailable = updateAvailable
	if !versionsComparable {
		report.Warnings = append(report.Warnings, "could not compare current and latest release versions")
	}

	assetName := platformAssetName(report.LatestVersion, runtime.GOOS, runtime.GOARCH)
	if assetName != "" {
		report.PlatformAsset = assetName
		if asset, ok := findReleaseAsset(release.Assets, assetName); ok {
			report.PlatformAssetURL = asset.BrowserDownloadURL
		} else if release.SyntheticAssets {
			report.PlatformAssetURL = updateReleaseAssetURL(release.TagName, assetName)
		} else {
			report.Warnings = append(report.Warnings, "no release asset found for "+platform)
		}
	}

	if checksumAsset, ok := findReleaseAsset(release.Assets, "checksums.txt"); ok {
		report.ChecksumAvailable = true
		report.ChecksumsURL = checksumAsset.BrowserDownloadURL
		if report.PlatformAsset != "" {
			sum, checksumErr := fetchAssetChecksum(ctx, client, checksumAsset.BrowserDownloadURL, report.PlatformAsset)
			if checksumErr != nil {
				report.Warnings = append(report.Warnings, checksumErr.Error())
			} else {
				report.PlatformAssetSHA256 = sum
			}
		}
	} else {
		report.Warnings = append(report.Warnings, "checksums.txt not found on latest release")
	}

	return report, nil
}

func writeUpdateStatusText(ctx context.Context, report updateStatusReport) {
	u := ui.FromContext(ctx)
	if u == nil {
		return
	}
	u.Out().Linef("current_version\t%s", report.CurrentVersion)
	if report.CurrentCommit != "" {
		u.Out().Linef("current_commit\t%s", report.CurrentCommit)
	}
	if report.CurrentDate != "" {
		u.Out().Linef("current_date\t%s", report.CurrentDate)
	}
	u.Out().Linef("latest_version\t%s", report.LatestVersion)
	u.Out().Linef("update_available\t%t", report.UpdateAvailable)
	u.Out().Linef("platform\t%s", report.Platform)
	if report.PlatformAsset != "" {
		u.Out().Linef("platform_asset\t%s", report.PlatformAsset)
	}
	if report.PlatformAssetSHA256 != "" {
		u.Out().Linef("platform_asset_sha256\t%s", report.PlatformAssetSHA256)
	}
	u.Out().Linef("install_method\t%s", report.InstallMethod)
	for _, warning := range report.Warnings {
		u.Out().Linef("warning\t%s", warning)
	}
}

func updateClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = updateDefaultTimeout
	}
	if updateHTTPClient == nil {
		return &http.Client{Timeout: timeout}
	}
	if updateHTTPClient.Timeout != 0 {
		return updateHTTPClient
	}
	clone := *updateHTTPClient
	clone.Timeout = timeout
	return &clone
}

func fetchLatestGitHubRelease(ctx context.Context, client *http.Client, endpoint string) (githubRelease, error) {
	var release githubRelease
	apiErr := fetchUpdateJSON(ctx, client, endpoint, &release)
	if apiErr == nil {
		return release, nil
	}

	fallback, fallbackErr := fetchLatestGitHubReleaseRedirect(ctx, client, updateLatestWebURL)
	if fallbackErr != nil {
		return githubRelease{}, fmt.Errorf("fetch latest release: API: %s; web fallback: %w", apiErr.Error(), fallbackErr)
	}
	return fallback, nil
}

func fetchLatestGitHubReleaseRedirect(ctx context.Context, client *http.Client, latestURL string) (githubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestURL, nil)
	if err != nil {
		return githubRelease{}, err
	}
	req.Header.Set("User-Agent", "gogcli/"+resolvedVersion())

	redirectClient := *client
	redirectClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := redirectClient.Do(req)
	if err != nil {
		return githubRelease{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		return githubRelease{}, fmt.Errorf("github returned %s", resp.Status)
	}

	location := strings.TrimSpace(resp.Header.Get("Location"))
	if location == "" {
		return githubRelease{}, fmt.Errorf("github redirect did not include Location")
	}
	resolved, err := req.URL.Parse(location)
	if err != nil {
		return githubRelease{}, fmt.Errorf("parse github redirect: %w", err)
	}
	if !strings.EqualFold(resolved.Hostname(), "github.com") {
		return githubRelease{}, fmt.Errorf("unexpected github redirect host %q", resolved.Hostname())
	}
	const tagPath = "/openclaw/gogcli/releases/tag/"
	if !strings.HasPrefix(resolved.EscapedPath(), tagPath) {
		return githubRelease{}, fmt.Errorf("unexpected github release redirect path %q", resolved.EscapedPath())
	}
	tag, err := url.PathUnescape(strings.TrimPrefix(resolved.EscapedPath(), tagPath))
	if err != nil || tag == "" || strings.Contains(tag, "/") {
		return githubRelease{}, fmt.Errorf("invalid github release tag in redirect")
	}

	return githubRelease{
		TagName:         tag,
		HTMLURL:         resolved.String(),
		SyntheticAssets: true,
		Assets: []githubReleaseAsset{{
			Name:               "checksums.txt",
			BrowserDownloadURL: updateReleaseAssetURL(tag, "checksums.txt"),
		}},
	}, nil
}

func updateReleaseAssetURL(tag string, assetName string) string {
	return "https://github.com/openclaw/gogcli/releases/download/" +
		url.PathEscape(tag) + "/" + url.PathEscape(assetName)
}

func fetchUpdateJSON(ctx context.Context, client *http.Client, endpoint string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "gogcli/"+resolvedVersion())

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("github returned %s: %s", resp.Status, msg)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func fetchAssetChecksum(ctx context.Context, client *http.Client, endpoint string, assetName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("fetch checksums.txt: %w", err)
	}
	req.Header.Set("User-Agent", "gogcli/"+resolvedVersion())
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch checksums.txt: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fetch checksums.txt: github returned %s", resp.Status)
	}

	scanner := bufio.NewScanner(io.LimitReader(resp.Body, 1<<20))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if name == assetName {
			sum := strings.ToLower(fields[0])
			decoded, decodeErr := hex.DecodeString(sum)
			if decodeErr != nil || len(decoded) != sha256.Size {
				return "", fmt.Errorf("invalid checksum for %s in checksums.txt", assetName)
			}
			return sum, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read checksums.txt: %w", err)
	}
	return "", fmt.Errorf("checksum for %s not found in checksums.txt", assetName)
}

func findReleaseAsset(assets []githubReleaseAsset, name string) (githubReleaseAsset, bool) {
	for _, asset := range assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return githubReleaseAsset{}, false
}

func platformAssetName(tag string, goos string, goarch string) string {
	version := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	if version == "" {
		return ""
	}
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("gogcli_%s_%s_%s%s", version, goos, goarch, ext)
}

func updateAvailable(current string, latest string) (bool, bool) {
	cmp, ok := compareReleaseVersions(current, latest)
	if !ok {
		return false, false
	}
	return cmp < 0, true
}

func compareReleaseVersions(current string, latest string) (int, bool) {
	currentParts, okCurrent := releaseVersionParts(current)
	latestParts, okLatest := releaseVersionParts(latest)
	if !okCurrent || !okLatest {
		return 0, false
	}
	maxLen := len(currentParts)
	if len(latestParts) > maxLen {
		maxLen = len(latestParts)
	}
	for i := 0; i < maxLen; i++ {
		currentPart := 0
		if i < len(currentParts) {
			currentPart = currentParts[i]
		}
		latestPart := 0
		if i < len(latestParts) {
			latestPart = latestParts[i]
		}
		if currentPart < latestPart {
			return -1, true
		}
		if currentPart > latestPart {
			return 1, true
		}
	}
	return 0, true
}

func releaseVersionParts(value string) ([]int, bool) {
	v := strings.TrimSpace(value)
	v = strings.TrimPrefix(v, "v")
	if v == "" || v == sentinelDev {
		return nil, false
	}
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		v = v[:idx]
	}
	fields := strings.Split(v, ".")
	if len(fields) == 0 {
		return nil, false
	}
	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		if field == "" {
			return nil, false
		}
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 {
			return nil, false
		}
		parts = append(parts, n)
	}
	return parts, true
}

func detectUpdateInstallMethod() (method string, executable string, warnings []string) {
	exe, err := os.Executable()
	if err != nil {
		return updateInstallUnknown, "", []string{"could not determine executable path: " + err.Error()}
	}
	resolved := exe
	if resolvedExe, evalErr := filepath.EvalSymlinks(exe); evalErr == nil {
		resolved = resolvedExe
	}
	lower := strings.ToLower(resolved)
	switch {
	case isDockerRuntime():
		method = "docker"
	case strings.Contains(lower, "/cellar/") || strings.Contains(lower, "/homebrew/") || strings.Contains(lower, "/linuxbrew/"):
		method = "homebrew"
	case strings.Contains(lower, string(filepath.Separator)+"go-build") || strings.HasSuffix(lower, ".test"):
		method = "source"
	default:
		method = "standalone"
	}
	return method, resolved, nil
}

func isDockerRuntime() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	data, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	text := strings.ToLower(string(data))
	return strings.Contains(text, "docker") || strings.Contains(text, "kubepods") || strings.Contains(text, "containerd")
}
