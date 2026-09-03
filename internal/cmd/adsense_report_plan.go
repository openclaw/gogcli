package cmd

import (
	"strings"
	"time"
)

type adSenseReportInput struct {
	Account    string
	From       string
	To         string
	DateRange  string
	Dimensions string
	Metrics    string
	Filters    []string
	OrderBy    []string
	Currency   string
	Language   string
	Timezone   string
	Max        int64
}

type adSenseDateParts struct {
	Year  int64
	Month int64
	Day   int64
}

type adSenseReportPlan struct {
	Account    string
	Dimensions []string
	Metrics    []string
	DateRange  string
	Start      adSenseDateParts
	End        adSenseDateParts
	Filters    []string
	OrderBy    []string
	Currency   string
	Language   string
	Timezone   string
	Max        int64
}

func newAdSenseReportPlan(input adSenseReportInput) (adSenseReportPlan, error) {
	account, err := normalizeAdSenseAccount(input.Account)
	if err != nil {
		return adSenseReportPlan{}, err
	}

	dateRangePlan, err := newAdSenseDateRangePlan(input.From, input.To, input.DateRange)
	if err != nil {
		return adSenseReportPlan{}, err
	}

	timezone, err := normalizeAdSenseReportingTimezone(input.Timezone)
	if err != nil {
		return adSenseReportPlan{}, err
	}

	metrics := adSenseUpperList(splitCommaList(input.Metrics))
	if len(metrics) == 0 {
		return adSenseReportPlan{}, usage("empty --metrics")
	}
	if input.Max < 0 {
		return adSenseReportPlan{}, usage("--max must be >= 0")
	}

	return adSenseReportPlan{
		Account:    account,
		DateRange:  dateRangePlan.DateRange,
		Start:      dateRangePlan.Start,
		End:        dateRangePlan.End,
		Dimensions: adSenseUpperList(splitCommaList(input.Dimensions)),
		Metrics:    metrics,
		Filters:    trimmedStrings(input.Filters),
		OrderBy:    trimmedStrings(input.OrderBy),
		Currency:   strings.TrimSpace(input.Currency),
		Language:   strings.TrimSpace(input.Language),
		Timezone:   timezone,
		Max:        input.Max,
	}, nil
}

func normalizeAdSenseReportingTimezone(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	switch value {
	case "", "ACCOUNT_TIME_ZONE", "GOOGLE_TIME_ZONE":
		return value, nil
	default:
		return "", usage("--timezone must be ACCOUNT_TIME_ZONE or GOOGLE_TIME_ZONE (not an IANA timezone name)")
	}
}

type adSenseDateRangePlan struct {
	DateRange string
	Start     adSenseDateParts
	End       adSenseDateParts
}

func newAdSenseDateRangePlan(from, to, dateRangeRaw string) (adSenseDateRangePlan, error) {
	var plan adSenseDateRangePlan

	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	switch {
	case from != "" || to != "":
		if from == "" || to == "" {
			return adSenseDateRangePlan{}, usage("--from and --to must be set together")
		}
		start, startErr := parseAdSenseDate(from, "--from")
		if startErr != nil {
			return adSenseDateRangePlan{}, startErr
		}
		end, endErr := parseAdSenseDate(to, "--to")
		if endErr != nil {
			return adSenseDateRangePlan{}, endErr
		}
		if end.Before(start) {
			return adSenseDateRangePlan{}, usage("--to must be on or after --from")
		}
		plan.Start = adSenseDatePartsFromTime(start)
		plan.End = adSenseDatePartsFromTime(end)
	default:
		dateRange := strings.ToUpper(strings.TrimSpace(dateRangeRaw))
		if dateRange == "" {
			return adSenseDateRangePlan{}, usage("empty --date-range (or set --from/--to)")
		}
		plan.DateRange = dateRange
	}

	return plan, nil
}

func parseAdSenseDate(value, flagName string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, usagef("invalid %s (expected YYYY-MM-DD)", flagName)
	}
	return t, nil
}

func adSenseDatePartsFromTime(t time.Time) adSenseDateParts {
	return adSenseDateParts{Year: int64(t.Year()), Month: int64(t.Month()), Day: int64(t.Day())}
}

func adSenseUpperList(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.ToUpper(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func trimmedStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}
