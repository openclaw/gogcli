package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	gapi "google.golang.org/api/googleapi"
	searchconsoleapi "google.golang.org/api/searchconsole/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type SearchConsoleCmd struct {
	Sites           SearchConsoleSitesCmd           `cmd:"" name:"sites" aliases:"list,ls" help:"List and inspect Search Console sites"`
	SearchAnalytics SearchConsoleSearchAnalyticsCmd `cmd:"" name:"searchanalytics" aliases:"analytics" help:"Search Analytics queries"`
	Query           SearchConsoleQueryCmd           `cmd:"" name:"query" aliases:"report" help:"Run a Search Analytics query"`
	Sitemaps        SearchConsoleSitemapsCmd        `cmd:"" name:"sitemaps" help:"List/get/submit/delete sitemaps"`
}

type SearchConsoleSitesCmd struct {
	List SearchConsoleSitesListCmd `cmd:"" default:"withargs" aliases:"ls" help:"List accessible Search Console sites"`
	Get  SearchConsoleSitesGetCmd  `cmd:"" name:"get" aliases:"info,show" help:"Get a specific Search Console site"`
}

type SearchConsoleSitesListCmd struct {
	FailEmpty bool `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *SearchConsoleSitesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := searchConsoleService(ctx, account)
	if err != nil {
		return err
	}
	resp, err := svc.Sites.List().Context(ctx).Do()
	if err != nil {
		return wrapSearchConsoleError(err)
	}

	rows := resp.SiteEntry
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"sites": rows,
		}); err != nil {
			return err
		}
		if len(rows) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(rows) == 0 {
		u.Err().Println("No Search Console sites")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "SITE\tPERMISSION")
	for _, item := range rows {
		if item == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\n", sanitizeTab(item.SiteUrl), sanitizeTab(item.PermissionLevel))
	}
	return nil
}

type SearchConsoleSitesGetCmd struct {
	SiteURL string `arg:"" name:"siteUrl" help:"Search Console property URL (e.g. https://example.com/ or sc-domain:example.com)"`
}

func (c *SearchConsoleSitesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	siteURL := strings.TrimSpace(c.SiteURL)
	if siteURL == "" {
		return usage("empty siteUrl")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := searchConsoleService(ctx, account)
	if err != nil {
		return err
	}
	site, err := svc.Sites.Get(siteURL).Context(ctx).Do()
	if err != nil {
		return wrapSearchConsoleError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"site": site,
		})
	}

	return writeResult(ctx, u,
		kv("site_url", site.SiteUrl),
		kv("permission_level", site.PermissionLevel),
	)
}

type SearchConsoleSearchAnalyticsCmd struct {
	Query SearchConsoleQueryCmd `cmd:"" name:"query" default:"withargs" aliases:"run" help:"Run a Search Analytics query"`
}

type SearchConsoleQueryCmd struct {
	SiteURL string `arg:"" name:"siteUrl" help:"Search Console property URL (e.g. https://example.com/ or sc-domain:example.com)"`

	From        string   `name:"from" aliases:"start" help:"Start date (YYYY-MM-DD)"`
	To          string   `name:"to" aliases:"end" help:"End date (YYYY-MM-DD)"`
	Dimensions  string   `name:"dimensions" help:"Comma-separated dimensions (DATE,QUERY,PAGE,COUNTRY,DEVICE,SEARCH_APPEARANCE,HOUR)" default:"QUERY"`
	Type        string   `name:"type" help:"Search type (WEB,IMAGE,VIDEO,NEWS,DISCOVER,GOOGLE_NEWS)" default:"WEB"`
	Aggregation string   `name:"aggregation" help:"Aggregation type (AUTO,BY_PROPERTY,BY_PAGE,BY_NEWS_SHOWCASE_PANEL)"`
	DataState   string   `name:"data-state" help:"Data state (FINAL,ALL,HOURLY_ALL)"`
	Max         int64    `name:"max" aliases:"limit" help:"Max rows to return (1-25000)" default:"1000"`
	Offset      int64    `name:"offset" aliases:"start-row" help:"Row offset for pagination" default:"0"`
	Filter      []string `name:"filter" help:"Dimension filter, repeatable: dimension:operator:expression"`
	Request     string   `name:"request" help:"SearchAnalyticsQueryRequest JSON spec. Accepts @file, a plain file path, -, or inline JSON."`
	FailEmpty   bool     `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no rows"`
}

func (c *SearchConsoleQueryCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	plan, err := c.plan(stdinReader(ctx))
	if err != nil {
		return err
	}

	svc, err := searchConsoleService(ctx, account)
	if err != nil {
		return err
	}
	resp, err := svc.Searchanalytics.Query(plan.SiteURL, plan.Request).Context(ctx).Do()
	if err != nil {
		return wrapSearchConsoleError(err)
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"site_url":                  plan.SiteURL,
			"from":                      plan.Request.StartDate,
			"to":                        plan.Request.EndDate,
			"type":                      plan.queryType(),
			"dimensions":                plan.Request.Dimensions,
			"response_aggregation_type": resp.ResponseAggregationType,
			"rows":                      resp.Rows,
		}); err != nil {
			return err
		}
		if len(resp.Rows) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(resp.Rows) == 0 {
		u.Err().Println("No Search Console rows")
		return failEmptyExit(c.FailEmpty)
	}

	headers := requestSearchConsoleDimensions(plan.Request, resp.Rows)
	headers = append(headers, "CLICKS", "IMPRESSIONS", "CTR", "POSITION")

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, row := range resp.Rows {
		if row == nil {
			continue
		}
		values := make([]string, 0, len(headers))
		for i := range headers[:len(headers)-4] {
			values = append(values, sanitizeTab(searchConsoleKey(row, i)))
		}
		values = append(values,
			formatSearchConsoleMetric(row.Clicks, 0),
			formatSearchConsoleMetric(row.Impressions, 0),
			formatSearchConsoleMetric(row.Ctr, 4),
			formatSearchConsoleMetric(row.Position, 2),
		)
		fmt.Fprintln(w, strings.Join(values, "\t"))
	}
	return nil
}

type SearchConsoleSitemapsCmd struct {
	List   SearchConsoleSitemapsListCmd   `cmd:"" default:"withargs" aliases:"ls" help:"List sitemaps for a site"`
	Get    SearchConsoleSitemapsGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get a sitemap"`
	Submit SearchConsoleSitemapsSubmitCmd `cmd:"" name:"submit" help:"Submit a sitemap"`
	Delete SearchConsoleSitemapsDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove" help:"Delete a sitemap"`
}

type SearchConsoleSitemapsListCmd struct {
	SiteURL      string `arg:"" name:"siteUrl" help:"Search Console property URL (e.g. https://example.com/ or sc-domain:example.com)"`
	SitemapIndex string `name:"sitemap-index" help:"Filter to a sitemap index URL"`
	FailEmpty    bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *SearchConsoleSitemapsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	siteURL := strings.TrimSpace(c.SiteURL)
	if siteURL == "" {
		return usage("empty siteUrl")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := searchConsoleService(ctx, account)
	if err != nil {
		return err
	}
	call := svc.Sitemaps.List(siteURL).Context(ctx)
	if v := strings.TrimSpace(c.SitemapIndex); v != "" {
		call = call.SitemapIndex(v)
	}
	resp, err := call.Do()
	if err != nil {
		return wrapSearchConsoleError(err)
	}

	rows := resp.Sitemap
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"sitemaps": rows,
		}); err != nil {
			return err
		}
		if len(rows) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(rows) == 0 {
		u.Err().Println("No Search Console sitemaps")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "PATH\tTYPE\tPENDING\tWARNINGS\tERRORS\tLAST_SUBMITTED\tLAST_DOWNLOADED\tCONTENTS")
	for _, item := range rows {
		if item == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%t\t%d\t%d\t%s\t%s\t%s\n",
			sanitizeTab(item.Path),
			sanitizeTab(item.Type),
			item.IsPending,
			item.Warnings,
			item.Errors,
			sanitizeTab(item.LastSubmitted),
			sanitizeTab(item.LastDownloaded),
			sanitizeTab(formatSearchConsoleSitemapContents(item.Contents)),
		)
	}
	return nil
}

type SearchConsoleSitemapsGetCmd struct {
	SiteURL  string `arg:"" name:"siteUrl" help:"Search Console property URL (e.g. https://example.com/ or sc-domain:example.com)"`
	FeedPath string `arg:"" name:"feedpath" help:"Sitemap URL"`
}

func (c *SearchConsoleSitemapsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	siteURL := strings.TrimSpace(c.SiteURL)
	if siteURL == "" {
		return usage("empty siteUrl")
	}
	feedPath := strings.TrimSpace(c.FeedPath)
	if feedPath == "" {
		return usage("empty feedpath")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := searchConsoleService(ctx, account)
	if err != nil {
		return err
	}
	sitemap, err := svc.Sitemaps.Get(siteURL, feedPath).Context(ctx).Do()
	if err != nil {
		return wrapSearchConsoleError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"sitemap": sitemap,
		})
	}

	return writeResult(ctx, u,
		kv("path", sitemap.Path),
		kv("type", sitemap.Type),
		kv("pending", sitemap.IsPending),
		kv("warnings", sitemap.Warnings),
		kv("errors", sitemap.Errors),
		kv("last_submitted", sitemap.LastSubmitted),
		kv("last_downloaded", sitemap.LastDownloaded),
		kv("contents", formatSearchConsoleSitemapContents(sitemap.Contents)),
	)
}

type SearchConsoleSitemapsSubmitCmd struct {
	SiteURL  string `arg:"" name:"siteUrl" help:"Search Console property URL (e.g. https://example.com/ or sc-domain:example.com)"`
	FeedPath string `arg:"" name:"feedpath" help:"Sitemap URL"`
}

func (c *SearchConsoleSitemapsSubmitCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	siteURL := strings.TrimSpace(c.SiteURL)
	if siteURL == "" {
		return usage("empty siteUrl")
	}
	feedPath := strings.TrimSpace(c.FeedPath)
	if feedPath == "" {
		return usage("empty feedpath")
	}
	if err := validateSearchConsoleSitemapURL(feedPath); err != nil {
		return err
	}

	if err := dryRunExit(ctx, flags, "searchconsole.sitemaps.submit", map[string]any{
		"site_url":  siteURL,
		"feed_path": feedPath,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := searchConsoleService(ctx, account)
	if err != nil {
		return err
	}
	if err := svc.Sitemaps.Submit(siteURL, feedPath).Context(ctx).Do(); err != nil {
		return wrapSearchConsoleError(err)
	}

	return writeResult(ctx, u,
		kv("submitted", true),
		kv("site_url", siteURL),
		kv("feed_path", feedPath),
	)
}

type SearchConsoleSitemapsDeleteCmd struct {
	SiteURL  string `arg:"" name:"siteUrl" help:"Search Console property URL (e.g. https://example.com/ or sc-domain:example.com)"`
	FeedPath string `arg:"" name:"feedpath" help:"Sitemap URL"`
}

func (c *SearchConsoleSitemapsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	siteURL := strings.TrimSpace(c.SiteURL)
	if siteURL == "" {
		return usage("empty siteUrl")
	}
	feedPath := strings.TrimSpace(c.FeedPath)
	if feedPath == "" {
		return usage("empty feedpath")
	}
	if err := validateSearchConsoleSitemapURL(feedPath); err != nil {
		return err
	}

	if err := dryRunAndConfirmDestructive(ctx, flags, "searchconsole.sitemaps.delete", map[string]any{
		"site_url":  siteURL,
		"feed_path": feedPath,
	}, fmt.Sprintf("delete sitemap %s", feedPath)); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := searchConsoleService(ctx, account)
	if err != nil {
		return err
	}
	if err := svc.Sitemaps.Delete(siteURL, feedPath).Context(ctx).Do(); err != nil {
		return wrapSearchConsoleError(err)
	}

	return writeResult(ctx, u,
		kv("deleted", true),
		kv("site_url", siteURL),
		kv("feed_path", feedPath),
	)
}

func validateSearchConsoleSitemapURL(raw string) error {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed == nil || parsed.Host == "" {
		return usagef("invalid feedpath %q (expected http(s) sitemap URL)", raw)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return nil
	default:
		return usagef("invalid feedpath %q (expected http(s) sitemap URL)", raw)
	}
}

func requestSearchConsoleDimensions(
	req *searchconsoleapi.SearchAnalyticsQueryRequest,
	rows []*searchconsoleapi.ApiDataRow,
) []string {
	if req != nil && len(req.Dimensions) > 0 {
		out := make([]string, 0, len(req.Dimensions))
		for _, dim := range req.Dimensions {
			out = append(out, strings.ToUpper(strings.TrimSpace(dim)))
		}
		return out
	}

	keyCount := 0
	for _, row := range rows {
		if row != nil && len(row.Keys) > keyCount {
			keyCount = len(row.Keys)
		}
	}

	out := make([]string, 0, keyCount)
	for i := 0; i < keyCount; i++ {
		out = append(out, "KEY_"+strconv.Itoa(i+1))
	}
	return out
}

func searchConsoleKey(row *searchconsoleapi.ApiDataRow, index int) string {
	if row == nil || index < 0 || index >= len(row.Keys) {
		return ""
	}
	return row.Keys[index]
}

func formatSearchConsoleMetric(v float64, decimals int) string {
	if decimals <= 0 {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strconv.FormatFloat(v, 'f', decimals, 64)
}

func formatSearchConsoleSitemapContents(contents []*searchconsoleapi.WmxSitemapContent) string {
	if len(contents) == 0 {
		return ""
	}

	parts := make([]string, 0, len(contents))
	for _, content := range contents {
		if content == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:%d/%d", content.Type, content.Indexed, content.Submitted))
	}
	return strings.Join(parts, ",")
}

func wrapSearchConsoleError(err error) error {
	var apiErr *gapi.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	if apiErr.Code != 403 {
		return err
	}

	message := strings.ToLower(apiErr.Message)
	switch {
	case strings.Contains(message, "accessnotconfigured"), strings.Contains(message, "api has not been used"):
		return fmt.Errorf("search console API is not enabled for this OAuth project. Enable it at https://console.cloud.google.com/apis/library/searchconsole.googleapis.com")
	case strings.Contains(message, "insufficientpermissions"), strings.Contains(message, "insufficient permission"):
		return fmt.Errorf("insufficient permissions for Search Console API. Re-authorize with: gog auth add <email> --services searchconsole")
	default:
		return err
	}
}
