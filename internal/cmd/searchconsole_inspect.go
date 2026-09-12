package cmd

import (
	"context"
	"fmt"
	"strings"

	searchconsoleapi "google.golang.org/api/searchconsole/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
)

type SearchConsoleInspectCmd struct {
	SiteURL       string `arg:"" name:"siteUrl" help:"Search Console property URL (e.g. https://example.com/ or sc-domain:example.com)"`
	InspectionURL string `arg:"" name:"url" help:"URL to inspect; must live under the property"`

	Language string `name:"language" help:"BCP-47 language for issue messages (e.g. zh-TW)" default:"en-US"`
}

func (c *SearchConsoleInspectCmd) Run(ctx context.Context, flags *RootFlags) error {
	siteURL := strings.TrimSpace(c.SiteURL)
	if siteURL == "" {
		return usage("empty siteUrl")
	}
	inspectionURL := strings.TrimSpace(c.InspectionURL)
	if inspectionURL == "" {
		return usage("empty url")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := searchConsoleService(ctx, account)
	if err != nil {
		return err
	}

	req := &searchconsoleapi.InspectUrlIndexRequest{
		SiteUrl:       siteURL,
		InspectionUrl: inspectionURL,
		LanguageCode:  strings.TrimSpace(c.Language),
	}
	resp, err := svc.UrlInspection.Index.Inspect(req).Context(ctx).Do()
	if err != nil {
		return wrapSearchConsoleError(err)
	}

	res := resp.InspectionResult
	if res == nil {
		return fmt.Errorf("empty inspection result")
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"site_url":         siteURL,
			"inspection_url":   inspectionURL,
			"inspectionResult": res,
		})
	}

	idx := res.IndexStatusResult
	if idx == nil {
		idx = &searchconsoleapi.IndexStatusInspectionResult{}
	}
	canonical := sanitizeTab(idx.GoogleCanonical)
	if idx.UserCanonical != "" && idx.UserCanonical != idx.GoogleCanonical {
		canonical = fmt.Sprintf("%s (user: %s)", canonical, sanitizeTab(idx.UserCanonical))
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "FIELD\tVALUE")
	fmt.Fprintf(w, "url\t%s\n", sanitizeTab(inspectionURL))
	fmt.Fprintf(w, "verdict\t%s\n", sanitizeTab(idx.Verdict))
	fmt.Fprintf(w, "coverage_state\t%s\n", sanitizeTab(idx.CoverageState))
	fmt.Fprintf(w, "indexing_state\t%s\n", sanitizeTab(idx.IndexingState))
	fmt.Fprintf(w, "page_fetch_state\t%s\n", sanitizeTab(idx.PageFetchState))
	fmt.Fprintf(w, "robots_txt_state\t%s\n", sanitizeTab(idx.RobotsTxtState))
	fmt.Fprintf(w, "crawled_as\t%s\n", sanitizeTab(idx.CrawledAs))
	fmt.Fprintf(w, "last_crawl_time\t%s\n", sanitizeTab(idx.LastCrawlTime))
	fmt.Fprintf(w, "canonical\t%s\n", canonical)
	if n := len(idx.Sitemap); n > 0 {
		fmt.Fprintf(w, "sitemap\t%s\n", sanitizeTab(strings.Join(idx.Sitemap, ", ")))
	}
	if n := len(idx.ReferringUrls); n > 0 {
		fmt.Fprintf(w, "referring_urls\t%d\n", n)
	}
	return nil
}
