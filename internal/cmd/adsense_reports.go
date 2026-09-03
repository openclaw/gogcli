package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	adsenseapi "google.golang.org/api/adsense/v2"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type AdSenseReportsCmd struct {
	Query AdSenseReportsQueryCmd `cmd:"" name:"query" default:"withargs" aliases:"run,generate" help:"Run an AdSense report"`
	Saved AdSenseReportsSavedCmd `cmd:"" name:"saved" help:"Saved report operations"`
}

type AdSenseReportsQueryCmd struct {
	Account    string   `arg:"" name:"account" help:"AdSense account (e.g. pub-... or accounts/pub-...)"`
	From       string   `name:"from" aliases:"start" help:"Start date (YYYY-MM-DD); combine with --to for a custom range"`
	To         string   `name:"to" aliases:"end" help:"End date (YYYY-MM-DD); combine with --from for a custom range"`
	DateRange  string   `name:"date-range" help:"Named date range (TODAY,YESTERDAY,MONTH_TO_DATE,YEAR_TO_DATE,LAST_7_DAYS,LAST_30_DAYS); ignored if --from/--to set" default:"LAST_7_DAYS"`
	Dimensions string   `name:"dimensions" help:"Comma-separated report dimensions (e.g. DATE,COUNTRY_NAME)" default:"DATE"`
	Metrics    string   `name:"metrics" help:"Comma-separated report metrics (e.g. ESTIMATED_EARNINGS,CLICKS,IMPRESSIONS)" default:"ESTIMATED_EARNINGS,CLICKS,IMPRESSIONS"`
	Filter     []string `name:"filter" help:"Report filter, repeatable, passed through to the API (e.g. COUNTRY_NAME==United States)"`
	OrderBy    []string `name:"order-by" help:"Sort order, repeatable (e.g. -DATE for descending)"`
	Currency   string   `name:"currency" help:"Currency code override (e.g. USD)"`
	Language   string   `name:"language" help:"Language code for headers (e.g. en-US)"`
	Timezone   string   `name:"timezone" help:"Reporting timezone: ACCOUNT_TIME_ZONE (default) or GOOGLE_TIME_ZONE (America/Los_Angeles)"`
	Max        int64    `name:"max" aliases:"limit" help:"Max rows to return"`
	FailEmpty  bool     `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no rows"`
}

func (c *AdSenseReportsQueryCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	plan, err := newAdSenseReportPlan(adSenseReportInput{
		Account:    c.Account,
		From:       c.From,
		To:         c.To,
		DateRange:  c.DateRange,
		Dimensions: c.Dimensions,
		Metrics:    c.Metrics,
		Filters:    c.Filter,
		OrderBy:    c.OrderBy,
		Currency:   c.Currency,
		Language:   c.Language,
		Timezone:   c.Timezone,
		Max:        c.Max,
	})
	if err != nil {
		return err
	}

	svc, err := adSenseService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Accounts.Reports.Generate(plan.Account).Context(ctx).Metrics(plan.Metrics...)
	if len(plan.Dimensions) > 0 {
		call = call.Dimensions(plan.Dimensions...)
	}
	call = applyAdSenseReportDateRange(call, plan)
	if len(plan.Filters) > 0 {
		call = call.Filters(plan.Filters...)
	}
	if len(plan.OrderBy) > 0 {
		call = call.OrderBy(plan.OrderBy...)
	}
	if plan.Currency != "" {
		call = call.CurrencyCode(plan.Currency)
	}
	if plan.Language != "" {
		call = call.LanguageCode(plan.Language)
	}
	if plan.Timezone != "" {
		call = call.ReportingTimeZone(plan.Timezone)
	}
	if plan.Max > 0 {
		call = call.Limit(plan.Max)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	return writeAdSenseReportResult(ctx, u, plan.Account, plan.Dimensions, plan.Metrics, resp, c.FailEmpty)
}

func applyAdSenseReportDateRange(call *adsenseapi.AccountsReportsGenerateCall, plan adSenseReportPlan) *adsenseapi.AccountsReportsGenerateCall {
	if plan.DateRange != "" {
		return call.DateRange(plan.DateRange)
	}
	return call.
		StartDateYear(plan.Start.Year).StartDateMonth(plan.Start.Month).StartDateDay(plan.Start.Day).
		EndDateYear(plan.End.Year).EndDateMonth(plan.End.Month).EndDateDay(plan.End.Day)
}

func writeAdSenseReportResult(
	ctx context.Context,
	u *ui.UI,
	account string,
	dimensions []string,
	metrics []string,
	resp *adsenseapi.ReportResult,
	failEmpty bool,
) error {
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"account":            account,
			"dimensions":         dimensions,
			"metrics":            metrics,
			"total_matched_rows": resp.TotalMatchedRows,
			"headers":            resp.Headers,
			"rows":               resp.Rows,
			"totals":             resp.Totals,
			"averages":           resp.Averages,
			"warnings":           resp.Warnings,
		}); err != nil {
			return err
		}
		if len(resp.Rows) == 0 {
			return failEmptyExit(failEmpty)
		}
		return nil
	}

	if len(resp.Rows) == 0 {
		u.Err().Println("No AdSense report rows")
		return failEmptyExit(failEmpty)
	}

	headers := adSenseReportHeaderNames(resp.Headers, dimensions, metrics)

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, row := range resp.Rows {
		if row == nil {
			continue
		}
		values := make([]string, 0, len(headers))
		for i := range headers {
			values = append(values, sanitizeTab(adSenseCellValue(row, i)))
		}
		fmt.Fprintln(w, strings.Join(values, "\t"))
	}
	return nil
}

func adSenseReportHeaderNames(headers []*adsenseapi.Header, dimensions, metrics []string) []string {
	if len(headers) > 0 {
		out := make([]string, 0, len(headers))
		for _, h := range headers {
			if h == nil {
				continue
			}
			out = append(out, h.Name)
		}
		return out
	}
	out := make([]string, 0, len(dimensions)+len(metrics))
	out = append(out, dimensions...)
	out = append(out, metrics...)
	return out
}

func adSenseCellValue(row *adsenseapi.Row, index int) string {
	if row == nil || index < 0 || index >= len(row.Cells) || row.Cells[index] == nil {
		return ""
	}
	return row.Cells[index].Value
}

// Saved reports

type AdSenseReportsSavedCmd struct {
	List  AdSenseReportsSavedListCmd  `cmd:"" default:"withargs" aliases:"ls" help:"List saved reports for an account"`
	Get   AdSenseReportsSavedGetCmd   `cmd:"" name:"get" aliases:"info,show" help:"Get saved report metadata"`
	Query AdSenseReportsSavedQueryCmd `cmd:"" name:"query" aliases:"run,generate" help:"Run a saved report"`
}

type AdSenseReportsSavedListCmd struct {
	Account   string `arg:"" name:"account" help:"AdSense account (e.g. pub-... or accounts/pub-...)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max saved reports per page" default:"50"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

//nolint:dupl // shares the runAdSenseList/adSenseFetchPage shape with the AdSense sub-resource list commands in adsense.go by design
func (c *AdSenseReportsSavedListCmd) Run(ctx context.Context, flags *RootFlags) error {
	parent, err := normalizeAdSenseAccount(c.Account)
	if err != nil {
		return err
	}
	return runAdSenseList(ctx, adSenseListPage[*adsenseapi.SavedReport]{
		flags: flags, max: c.Max, page: c.Page, all: c.All, failEmpty: c.FailEmpty,
		jsonKey:  "savedReports",
		emptyMsg: "No saved reports",
		header:   "REPORT\tTITLE",
		fetch: func(svc *adsenseapi.Service, pageSize int64, pageToken string) ([]*adsenseapi.SavedReport, string, error) {
			resp, err := adSenseFetchPage[*adsenseapi.AccountsReportsSavedListCall, adsenseapi.ListSavedReportsResponse](
				svc.Accounts.Reports.Saved.List(parent).Context(ctx), pageSize, pageToken)
			if err != nil {
				return nil, "", err
			}
			return resp.SavedReports, resp.NextPageToken, nil
		},
		printRow: func(w io.Writer, item *adsenseapi.SavedReport) {
			if item == nil {
				return
			}
			fmt.Fprintf(w, "%s\t%s\n",
				sanitizeTab(adSenseResourceID(item.Name)),
				sanitizeTab(item.Title),
			)
		},
	})
}

type AdSenseReportsSavedGetCmd struct {
	Name string `arg:"" name:"name" help:"Full saved report resource name (e.g. accounts/pub-.../reports/...)"`
}

func (c *AdSenseReportsSavedGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runAdSenseGet(ctx, flags, c.Name, requireAdSenseResourceArg,
		func(svc *adsenseapi.Service, name string) (*adsenseapi.SavedReport, error) {
			return svc.Accounts.Reports.GetSaved(name).Context(ctx).Do()
		},
		"savedReport",
		func(report *adsenseapi.SavedReport) []resultKV {
			return []resultKV{
				kv("name", report.Name),
				kv("title", report.Title),
			}
		},
	)
}

type AdSenseReportsSavedQueryCmd struct {
	Name      string `arg:"" name:"name" help:"Full saved report resource name (e.g. accounts/pub-.../reports/...)"`
	From      string `name:"from" aliases:"start" help:"Start date (YYYY-MM-DD); combine with --to for a custom range"`
	To        string `name:"to" aliases:"end" help:"End date (YYYY-MM-DD); combine with --from for a custom range"`
	DateRange string `name:"date-range" help:"Named date range (TODAY,YESTERDAY,MONTH_TO_DATE,YEAR_TO_DATE,LAST_7_DAYS,LAST_30_DAYS); ignored if --from/--to set" default:"LAST_7_DAYS"`
	Currency  string `name:"currency" help:"Currency code override (e.g. USD)"`
	Language  string `name:"language" help:"Language code for headers (e.g. en-US)"`
	Timezone  string `name:"timezone" help:"Reporting timezone: ACCOUNT_TIME_ZONE (default) or GOOGLE_TIME_ZONE (America/Los_Angeles)"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no rows"`
}

func (c *AdSenseReportsSavedQueryCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	name, err := requireAdSenseResource(c.Name, "name")
	if err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	dateRangePlan, err := newAdSenseDateRangePlan(c.From, c.To, c.DateRange)
	if err != nil {
		return err
	}

	timezone, err := normalizeAdSenseReportingTimezone(c.Timezone)
	if err != nil {
		return err
	}

	svc, err := adSenseService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Accounts.Reports.Saved.Generate(name).Context(ctx)
	if dateRangePlan.DateRange != "" {
		call = call.DateRange(dateRangePlan.DateRange)
	} else {
		call = call.
			StartDateYear(dateRangePlan.Start.Year).StartDateMonth(dateRangePlan.Start.Month).StartDateDay(dateRangePlan.Start.Day).
			EndDateYear(dateRangePlan.End.Year).EndDateMonth(dateRangePlan.End.Month).EndDateDay(dateRangePlan.End.Day)
	}
	if v := strings.TrimSpace(c.Currency); v != "" {
		call = call.CurrencyCode(v)
	}
	if v := strings.TrimSpace(c.Language); v != "" {
		call = call.LanguageCode(v)
	}
	if timezone != "" {
		call = call.ReportingTimeZone(timezone)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	return writeAdSenseReportResult(ctx, u, name, nil, nil, resp, c.FailEmpty)
}
