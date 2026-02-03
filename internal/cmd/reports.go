package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	reports "google.golang.org/api/admin/reports/v1"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newReportsService = googleapi.NewReports

type ReportsCmd struct {
	User     ReportsUserCmd     `cmd:"" name:"user" help:"User activity reports"`
	Admin    ReportsAdminCmd    `cmd:"" name:"admin" help:"Admin activity reports"`
	Login    ReportsLoginCmd    `cmd:"" name:"login" help:"Login activity reports"`
	Drive    ReportsDriveCmd    `cmd:"" name:"drive" help:"Drive activity reports"`
	Usage    ReportsUsageCmd    `cmd:"" name:"usage" help:"Customer usage reports"`
	Accounts ReportsAccountsCmd `cmd:"" name:"accounts" help:"Account usage reports"`
	EmailLog ReportsEmailLogCmd `cmd:"" name:"email-log" help:"Email log search"`
}

type ReportsUserCmd struct {
	Date    string `name:"date" help:"Report date (YYYY-MM-DD)"`
	User    string `name:"user" help:"User email or ID (default: all)"`
	Filters string `name:"filters" help:"Filters query"`
	Max     int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page    string `name:"page" help:"Page token"`
}

func (c *ReportsUserCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runActivityReport(ctx, flags, activityReportOptions{
		Application: "user",
		Date:        c.Date,
		User:        c.User,
		Filters:     c.Filters,
		Max:         c.Max,
		Page:        c.Page,
	})
}

type ReportsAdminCmd struct {
	Date    string `name:"date" help:"Report date (YYYY-MM-DD)"`
	Event   string `name:"event" help:"Event name filter"`
	Filters string `name:"filters" help:"Filters query"`
	Max     int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page    string `name:"page" help:"Page token"`
}

func (c *ReportsAdminCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runActivityReport(ctx, flags, activityReportOptions{
		Application: "admin",
		Date:        c.Date,
		Event:       c.Event,
		Filters:     c.Filters,
		Max:         c.Max,
		Page:        c.Page,
	})
}

type ReportsLoginCmd struct {
	Date    string `name:"date" help:"Report date (YYYY-MM-DD)"`
	User    string `name:"user" help:"User email or ID (default: all)"`
	Filters string `name:"filters" help:"Filters query"`
	Max     int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page    string `name:"page" help:"Page token"`
}

func (c *ReportsLoginCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runActivityReport(ctx, flags, activityReportOptions{
		Application: "login",
		Date:        c.Date,
		User:        c.User,
		Filters:     c.Filters,
		Max:         c.Max,
		Page:        c.Page,
	})
}

type ReportsDriveCmd struct {
	Date    string `name:"date" help:"Report date (YYYY-MM-DD)"`
	User    string `name:"user" help:"User email or ID (default: all)"`
	Filters string `name:"filters" help:"Filters query"`
	Max     int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page    string `name:"page" help:"Page token"`
}

func (c *ReportsDriveCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runActivityReport(ctx, flags, activityReportOptions{
		Application: "drive",
		Date:        c.Date,
		User:        c.User,
		Filters:     c.Filters,
		Max:         c.Max,
		Page:        c.Page,
	})
}

type ReportsUsageCmd struct {
	Application string `arg:"" name:"application" help:"Application name"`
	Date        string `name:"date" help:"Report date (YYYY-MM-DD)"`
	Parameters  string `name:"parameters" help:"Comma-separated parameters"`
	Page        string `name:"page" help:"Page token"`
}

func (c *ReportsUsageCmd) Run(ctx context.Context, flags *RootFlags) error {
	if strings.TrimSpace(c.Application) == "" {
		return usage("application required")
	}
	return runUsageReport(ctx, flags, c.Application, c.Date, c.Parameters, c.Page)
}

type ReportsAccountsCmd struct {
	Date string `name:"date" help:"Report date (YYYY-MM-DD)"`
	Page string `name:"page" help:"Page token"`
}

func (c *ReportsAccountsCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runUsageReport(ctx, flags, "accounts", c.Date, "", c.Page)
}

type ReportsEmailLogCmd struct {
	Date      string `name:"date" help:"Report date (YYYY-MM-DD)"`
	Recipient string `name:"recipient" help:"Recipient email filter"`
	Filters   string `name:"filters" help:"Filters query"`
	Max       int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page      string `name:"page" help:"Page token"`
}

func (c *ReportsEmailLogCmd) Run(ctx context.Context, flags *RootFlags) error {
	filters := strings.TrimSpace(c.Filters)
	if c.Recipient != "" {
		recipientFilter := fmt.Sprintf("recipient==%s", c.Recipient)
		if filters == "" {
			filters = recipientFilter
		} else {
			filters = filters + "," + recipientFilter
		}
	}
	return runActivityReport(ctx, flags, activityReportOptions{
		Application: "email",
		Date:        c.Date,
		User:        "all",
		Filters:     filters,
		Max:         c.Max,
		Page:        c.Page,
	})
}

type activityReportOptions struct {
	Application string
	Date        string
	User        string
	Event       string
	Filters     string
	Max         int64
	Page        string
}

func runActivityReport(ctx context.Context, flags *RootFlags, opts activityReportOptions) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newReportsService(ctx, account)
	if err != nil {
		return err
	}

	userKey := strings.TrimSpace(opts.User)
	if userKey == "" {
		userKey = "all"
	}

	call := svc.Activities.List(userKey, opts.Application)
	if date := reportDate(opts.Date); date != "" {
		start, end := reportDateRange(date)
		call = call.StartTime(start).EndTime(end)
	}
	if opts.Event != "" {
		call = call.EventName(opts.Event)
	}
	if opts.Filters != "" {
		call = call.Filters(opts.Filters)
	}
	if opts.Max > 0 {
		call = call.MaxResults(opts.Max)
	}
	if opts.Page != "" {
		call = call.PageToken(opts.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("fetch %s report: %w", opts.Application, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Items) == 0 {
		u.Err().Println("No events found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "TIME\tACTOR\tIP\tEVENTS")
	for _, item := range resp.Items {
		if item == nil {
			continue
		}
		timeStr := formatActivityTime(item.Id)
		actor := ""
		if item.Actor != nil {
			actor = item.Actor.Email
		}
		events := activityEventNames(item.Events)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(timeStr),
			sanitizeTab(actor),
			sanitizeTab(item.IpAddress),
			sanitizeTab(events),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

func runUsageReport(ctx context.Context, flags *RootFlags, application, date, parameters, page string) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newReportsService(ctx, account)
	if err != nil {
		return err
	}

	date = reportDate(date)

	call := svc.CustomerUsageReports.Get(date).CustomerId(adminCustomerID)
	params := strings.TrimSpace(parameters)
	if params == "" {
		params = application
	} else if !strings.Contains(params, ":") {
		params = fmt.Sprintf("%s:%s", application, params)
	}
	if params != "" {
		call = call.Parameters(params)
	}
	if page != "" {
		call = call.PageToken(page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("fetch usage report: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.UsageReports) == 0 {
		u.Err().Println("No usage reports found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "DATE\tENTITY\tPARAMETERS")
	for _, report := range resp.UsageReports {
		if report == nil {
			continue
		}
		entity := ""
		if report.Entity != nil {
			entity = report.Entity.Type
			if report.Entity.EntityId != "" {
				entity = entity + ":" + report.Entity.EntityId
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(report.Date),
			sanitizeTab(entity),
			sanitizeTab(formatUsageParameters(report.Parameters)),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

func reportDate(date string) string {
	date = strings.TrimSpace(date)
	if date != "" {
		return date
	}
	return time.Now().UTC().Format("2006-01-02")
}

func reportDateRange(date string) (string, string) {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date, date
	}
	start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	end := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, time.UTC)
	return start.Format(time.RFC3339), end.Format(time.RFC3339)
}

func formatActivityTime(id *reports.ActivityId) string {
	if id == nil || id.Time == "" {
		return ""
	}
	sec, err := strconv.ParseInt(id.Time, 10, 64)
	if err != nil {
		return id.Time
	}
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}

func activityEventNames(events []*reports.ActivityEvents) string {
	if len(events) == 0 {
		return ""
	}
	out := make([]string, 0, len(events))
	for _, ev := range events {
		if ev == nil || ev.Name == "" {
			continue
		}
		out = append(out, ev.Name)
	}
	return strings.Join(out, ",")
}

func formatUsageParameters(params []*reports.UsageReportParameters) string {
	if len(params) == 0 {
		return ""
	}
	out := make([]string, 0, len(params))
	for _, p := range params {
		if p == nil {
			continue
		}
		value := ""
		switch {
		case p.StringValue != "":
			value = p.StringValue
		case p.DatetimeValue != "":
			value = p.DatetimeValue
		default:
			if p.IntValue != 0 {
				value = strconv.FormatInt(p.IntValue, 10)
			} else if p.BoolValue {
				value = strconv.FormatBool(p.BoolValue)
			}
		}
		if p.Name != "" {
			if value != "" {
				out = append(out, fmt.Sprintf("%s=%s", p.Name, value))
			} else {
				out = append(out, p.Name)
			}
		}
	}
	return strings.Join(out, ",")
}
