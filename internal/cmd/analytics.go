package cmd

import (
	"context"
	"fmt"
	"strings"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
	analyticsdata "google.golang.org/api/analyticsdata/v1beta"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type AnalyticsCmd struct {
	Accounts AnalyticsAccountsCmd `cmd:"" name:"accounts" aliases:"list,ls" default:"withargs" help:"List GA4 account summaries"`
	Report   AnalyticsReportCmd   `cmd:"" name:"report" help:"Run a GA4 report (Analytics Data API)"`
}

type AnalyticsAccountsCmd struct {
	Max       int64  `name:"max" aliases:"limit" help:"Max account summaries per page (API max 200)" default:"50"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *AnalyticsAccountsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if c.Max <= 0 {
		return usage("--max must be > 0")
	}

	svc, err := analyticsAdminService(ctx, account)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*analyticsadmin.GoogleAnalyticsAdminV1betaAccountSummary, string, error) {
		call := svc.AccountSummaries.List().PageSize(c.Max).Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return nil, "", callErr
		}
		return resp.AccountSummaries, resp.NextPageToken, nil
	}

	var items []*analyticsadmin.GoogleAnalyticsAdminV1betaAccountSummary
	nextPageToken := ""
	if c.All {
		all, collectErr := collectAllPages(c.Page, fetch)
		if collectErr != nil {
			return collectErr
		}
		items = all
	} else {
		items, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"account_summaries": items,
			"nextPageToken":     nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(items) == 0 {
		u.Err().Println("No Analytics accounts")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ACCOUNT\tDISPLAY_NAME\tPROPERTIES")
	for _, item := range items {
		if item == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%d\n",
			sanitizeTab(analyticsResourceID(item.Account)),
			sanitizeTab(item.DisplayName),
			len(item.PropertySummaries),
		)
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}

type AnalyticsReportCmd struct {
	Property   string `arg:"" name:"property" help:"GA4 property ID or resource (e.g. 123456789 or properties/123456789)"`
	From       string `name:"from" help:"Start date (YYYY-MM-DD or GA relative date like 7daysAgo)" default:"7daysAgo"`
	To         string `name:"to" help:"End date (YYYY-MM-DD or GA relative date like today)" default:"today"`
	Dimensions string `name:"dimensions" help:"Comma-separated dimensions (e.g. date,country)" default:"date"`
	Metrics    string `name:"metrics" help:"Comma-separated metrics (e.g. activeUsers,sessions)" default:"activeUsers"`
	Max        int64  `name:"max" aliases:"limit" help:"Max rows to return (1-250000)" default:"100"`
	Offset     int64  `name:"offset" help:"Row offset for pagination" default:"0"`
	FailEmpty  bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no rows"`
}

func (c *AnalyticsReportCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	plan, err := newAnalyticsReportPlan(analyticsReportInput{
		Property:   c.Property,
		From:       c.From,
		To:         c.To,
		Dimensions: c.Dimensions,
		Metrics:    c.Metrics,
		Max:        c.Max,
		Offset:     c.Offset,
	})
	if err != nil {
		return err
	}

	svc, err := analyticsDataService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Properties.RunReport(plan.Property, plan.Request).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"property":         plan.Property,
			"from":             plan.From,
			"to":               plan.To,
			"dimensions":       plan.Dimensions,
			"metrics":          plan.Metrics,
			"row_count":        resp.RowCount,
			"dimensionHeaders": resp.DimensionHeaders,
			"metricHeaders":    resp.MetricHeaders,
			"rows":             resp.Rows,
		}); err != nil {
			return err
		}
		if len(resp.Rows) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(resp.Rows) == 0 {
		u.Err().Println("No analytics rows")
		return failEmptyExit(c.FailEmpty)
	}

	headers := make([]string, 0, len(plan.Dimensions)+len(plan.Metrics))
	for _, d := range plan.Dimensions {
		headers = append(headers, strings.ToUpper(d))
	}
	for _, m := range plan.Metrics {
		headers = append(headers, strings.ToUpper(m))
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, row := range resp.Rows {
		values := make([]string, 0, len(plan.Dimensions)+len(plan.Metrics))
		for i := range plan.Dimensions {
			values = append(values, sanitizeTab(analyticsDimensionValue(row, i)))
		}
		for i := range plan.Metrics {
			values = append(values, sanitizeTab(analyticsMetricValue(row, i)))
		}
		fmt.Fprintln(w, strings.Join(values, "\t"))
	}
	return nil
}

func analyticsDimensionValue(row *analyticsdata.Row, index int) string {
	if row == nil || index < 0 || index >= len(row.DimensionValues) || row.DimensionValues[index] == nil {
		return ""
	}
	return row.DimensionValues[index].Value
}

func analyticsMetricValue(row *analyticsdata.Row, index int) string {
	if row == nil || index < 0 || index >= len(row.MetricValues) || row.MetricValues[index] == nil {
		return ""
	}
	return row.MetricValues[index].Value
}

func analyticsResourceID(resource string) string {
	resource = strings.TrimSpace(resource)
	if resource == "" {
		return ""
	}
	if i := strings.LastIndex(resource, "/"); i >= 0 && i+1 < len(resource) {
		return resource[i+1:]
	}
	return resource
}
