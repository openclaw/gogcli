package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// UNSTABLE: docsTabExportBaseURL is the base for the undocumented per-tab
// export endpoint. If Google changes or removes it, callers will see an HTTP
// 302 redirect to a login page or an HTTP 404.
const docsTabExportBaseURL = "https://docs.google.com/document/d"

// maxRedirects matches net/http.defaultCheckRedirect (10 hops).
const maxRedirects = 10

// newDocsHTTPClient is swapped in tests to avoid real auth.
var newDocsHTTPClient = func(ctx context.Context, email string) (*http.Client, error) {
	return googleapi.NewHTTPClient(ctx, googleauth.ServiceDocs, email)
}

// googleExportRedirectPolicy allows redirects within Google's serving
// infrastructure (*.google.com, *.googleusercontent.com, *.googleapis.com)
// but rejects redirects to unrecognised domains, which typically indicate
// an auth-wall redirect.
func googleExportRedirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return errors.New("too many redirects")
	}
	if len(via) > 0 && isGoogleAuthHost(req.URL.Host) {
		return fmt.Errorf("refusing redirect from %s to Google sign-in host %s (try re-authenticating with 'gog auth login')", via[0].URL.Host, req.URL.Host)
	}
	if len(via) > 0 && !isGoogleHost(req.URL.Host) {
		return fmt.Errorf("refusing redirect from %s to non-Google host %s (possible auth redirect; try re-authenticating)", via[0].URL.Host, req.URL.Host)
	}
	return nil
}

func isGoogleHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, suffix := range []string{".google.com", ".googleusercontent.com", ".googleapis.com"} {
		if host == suffix[1:] || strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

func isGoogleAuthHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return host == "accounts.google.com" || host == "myaccount.google.com"
}

type tabExportParams struct {
	DocID    string
	OutFlag  string
	Format   string
	TabQuery string
}

// sanitizeFilenameComponent replaces characters unsafe for filenames with
// underscores, collapsing runs. Only ASCII word chars ([0-9A-Za-z_]), dots,
// @, and hyphens are kept; non-ASCII characters are replaced.
var unsafeFilenameChars = regexp.MustCompile(`[^\w.@-]+`)

func sanitizeFilenameComponent(s string) string {
	return unsafeFilenameChars.ReplaceAllString(s, "_")
}

const formatHTML = "html"

func tabExportFormatParam(format string) (string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "pdf", "docx", "txt", formatHTML:
		return format, nil
	case "md":
		return "markdown", nil
	default:
		return "", fmt.Errorf("--tab export does not support format %q (supported: pdf|docx|txt|md|"+formatHTML+")", format)
	}
}

func docsTabExportURL(docID, format, tabID string) string {
	v := url.Values{}
	v.Set("format", format)
	v.Set("tab", tabID)
	return fmt.Sprintf("%s/%s/export?%s", docsTabExportBaseURL, url.PathEscape(docID), v.Encode())
}

func resolveTabID(ctx context.Context, docsSvc *docs.Service, docID, tabQuery string) (string, error) {
	doc, err := docsSvc.Documents.Get(docID).
		Fields("tabs(tabProperties(tabId,title,index),childTabs)").
		Context(ctx).
		Do()
	if err != nil {
		if isDocsNotFound(err) {
			return "", fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return "", fmt.Errorf("resolve tab: %w", err)
	}

	tabs := flattenTabs(doc.Tabs)
	tab, err := findTab(tabs, tabQuery)
	if err != nil {
		return "", err
	}
	return tab.TabProperties.TabId, nil
}

func tabExportOutPath(outFlag, docID, tabQuery, format string) (string, error) {
	defaultBase := docID + "_" + sanitizeFilenameComponent(tabQuery) + "." + format

	outPath := strings.TrimSpace(outFlag)
	if outPath != "" {
		expanded, err := config.ExpandPath(outPath)
		if err != nil {
			return "", err
		}
		if st, statErr := os.Stat(expanded); statErr == nil && st.IsDir() {
			return filepath.Join(expanded, defaultBase), nil
		}
		return expanded, nil
	}
	dir, err := config.EnsureDriveDownloadsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, defaultBase), nil
}

// runDocsTabExport performs a per-tab export using the undocumented Docs export
// URL. It resolves the tab, downloads the content, and writes it to a file.
func runDocsTabExport(ctx context.Context, flags *RootFlags, p tabExportParams) error {
	u := ui.FromContext(ctx)

	p.DocID = normalizeGoogleID(strings.TrimSpace(p.DocID))
	if p.DocID == "" {
		return usage("empty docId")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	format := p.Format
	if format == "" || format == formatAuto {
		format = "pdf"
	}
	format = strings.ToLower(strings.TrimSpace(format))

	formatParam, fmtErr := tabExportFormatParam(format)
	if fmtErr != nil {
		return fmtErr
	}

	outPath, pathErr := tabExportOutPath(p.OutFlag, p.DocID, p.TabQuery, format)
	if pathErr != nil {
		return pathErr
	}
	if outfmt.IsJSON(ctx) && isStdoutPath(outPath) {
		return usage("can't combine --json with --out -")
	}

	if dryErr := dryRunExit(ctx, flags, "docs.tab-export", map[string]any{
		"docID":  p.DocID,
		"tab":    p.TabQuery,
		"format": format,
		"out":    outPath,
	}); dryErr != nil {
		return dryErr
	}

	docsSvc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	u.Err().Printf("Resolving tab %q…", p.TabQuery)
	tabID, err := resolveTabID(ctx, docsSvc, p.DocID, p.TabQuery)
	if err != nil {
		return err
	}

	httpClient, err := newDocsHTTPClient(ctx, account)
	if err != nil {
		return err
	}
	httpClient.CheckRedirect = googleExportRedirectPolicy

	u.Err().Printf("Exporting tab %q as %s…", p.TabQuery, format)
	exportURL := docsTabExportURL(p.DocID, formatParam, tabID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, exportURL, nil)
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if checkErr := checkTabExportResponse(resp, format); checkErr != nil {
		return checkErr
	}

	if isStdoutPath(outPath) {
		_, copyErr := io.Copy(os.Stdout, resp.Body)
		return copyErr
	}

	f, outPath, writeErr := createUserOutputFile(outPath)
	if writeErr != nil {
		return writeErr
	}
	defer f.Close()

	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"path": outPath, "size": n})
	}
	u.Out().Printf("path\t%s", outPath)
	u.Out().Printf("size\t%s", formatDriveSize(n))
	return nil
}

func checkTabExportResponse(resp *http.Response, format string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		mediaType := strings.ToLower(resp.Header.Get("Content-Type"))
		if parsed, _, err := mime.ParseMediaType(mediaType); err == nil {
			mediaType = parsed
		}
		if format != formatHTML && mediaType == mimeHTML {
			snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			return fmt.Errorf("tab export returned unexpected text/html (possible auth redirect; try 'gog auth login'): %s", strings.TrimSpace(string(snippet)))
		}
		return nil
	}

	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	body := strings.TrimSpace(string(snippet))
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("tab export failed: %s: %s (check sharing settings or re-authenticate with 'gog auth login')", resp.Status, body)
	}
	return fmt.Errorf("tab export failed: %s: %s (undocumented endpoint may have changed)", resp.Status, body)
}
