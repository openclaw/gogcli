package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	adsenseapi "google.golang.org/api/adsense/v2"
	gapi "google.golang.org/api/googleapi"
)

func TestNormalizeAdSenseAccount(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"pub-123", "accounts/pub-123", false},
		{"accounts/pub-123", "accounts/pub-123", false},
		{"  pub-123  ", "accounts/pub-123", false},
		{"", "", true},
	}
	for _, tt := range tests {
		got, err := normalizeAdSenseAccount(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("normalizeAdSenseAccount(%q): expected error", tt.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("normalizeAdSenseAccount(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("normalizeAdSenseAccount(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestExecute_AdSensePreservesPermissionErrors(t *testing.T) {
	for _, message := range []string{"AdSense Management API has not been used", "insufficientPermissions"} {
		t.Run(message, func(t *testing.T) {
			svc := newAdSenseTestService(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": 403, "message": message}})
			}))
			result := executeWithAdSenseTestService(t, []string{"--account", "a@b.com", "adsense", "accounts", "list"}, svc)
			var apiErr *gapi.Error
			if !errors.As(result.err, &apiErr) || apiErr.Code != 403 || ExitCode(stableExitCode(result.err)) != 6 {
				t.Fatalf("lost permission error identity/classification: %v", result.err)
			}
		})
	}
}

func TestNewAdSenseReportPlan_CustomRange(t *testing.T) {
	plan, err := newAdSenseReportPlan(adSenseReportInput{
		Account:    "pub-123",
		From:       "2026-02-01",
		To:         "2026-02-07",
		Dimensions: "date,country_name",
		Metrics:    "estimated_earnings,clicks",
		Filters:    []string{" COUNTRY_NAME==United States "},
		OrderBy:    []string{"-DATE"},
		Max:        10,
	})
	if err != nil {
		t.Fatalf("newAdSenseReportPlan: %v", err)
	}
	if plan.Account != "accounts/pub-123" {
		t.Fatalf("unexpected account: %q", plan.Account)
	}
	if plan.DateRange != "" {
		t.Fatalf("expected no named date range, got %q", plan.DateRange)
	}
	if plan.Start != (adSenseDateParts{Year: 2026, Month: 2, Day: 1}) {
		t.Fatalf("unexpected start: %#v", plan.Start)
	}
	if plan.End != (adSenseDateParts{Year: 2026, Month: 2, Day: 7}) {
		t.Fatalf("unexpected end: %#v", plan.End)
	}
	if len(plan.Dimensions) != 2 || plan.Dimensions[0] != "DATE" || plan.Dimensions[1] != "COUNTRY_NAME" {
		t.Fatalf("unexpected dimensions: %#v", plan.Dimensions)
	}
	if len(plan.Metrics) != 2 || plan.Metrics[0] != "ESTIMATED_EARNINGS" || plan.Metrics[1] != "CLICKS" {
		t.Fatalf("unexpected metrics: %#v", plan.Metrics)
	}
	if len(plan.Filters) != 1 || plan.Filters[0] != "COUNTRY_NAME==United States" {
		t.Fatalf("unexpected filters: %#v", plan.Filters)
	}
	if len(plan.OrderBy) != 1 || plan.OrderBy[0] != "-DATE" {
		t.Fatalf("unexpected orderBy: %#v", plan.OrderBy)
	}
}

func TestNewAdSenseReportPlan_NamedDateRangeDefault(t *testing.T) {
	plan, err := newAdSenseReportPlan(adSenseReportInput{
		Account:   "pub-123",
		DateRange: "last_7_days",
		Metrics:   "CLICKS",
	})
	if err != nil {
		t.Fatalf("newAdSenseReportPlan: %v", err)
	}
	if plan.DateRange != "LAST_7_DAYS" {
		t.Fatalf("unexpected date range: %q", plan.DateRange)
	}
	if plan.Start != (adSenseDateParts{}) || plan.End != (adSenseDateParts{}) {
		t.Fatalf("expected empty explicit dates, got start=%#v end=%#v", plan.Start, plan.End)
	}
}

func TestNewAdSenseReportPlan_RequiresBothFromAndTo(t *testing.T) {
	_, err := newAdSenseReportPlan(adSenseReportInput{
		Account: "pub-123",
		From:    "2026-02-01",
		Metrics: "CLICKS",
	})
	if err == nil {
		t.Fatal("expected error when only --from is set")
	}
}

func TestNewAdSenseReportPlan_RequiresMetrics(t *testing.T) {
	_, err := newAdSenseReportPlan(adSenseReportInput{
		Account:   "pub-123",
		DateRange: "TODAY",
	})
	if err == nil {
		t.Fatal("expected error for empty metrics")
	}
}

func TestExecute_AdSenseAccountsList_JSON(t *testing.T) {
	svc := newAdSenseTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/v2/accounts")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accounts": []map[string]any{
				{
					"name":        "accounts/pub-123",
					"displayName": "My Site",
					"state":       "READY",
					"premium":     false,
				},
			},
		})
	}))
	result := executeWithAdSenseTestService(t, []string{
		"--json",
		"--account", "a@b.com",
		"adsense", "accounts", "list",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed struct {
		Accounts []struct {
			Name string `json:"name"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Accounts) != 1 || parsed.Accounts[0].Name != "accounts/pub-123" {
		t.Fatalf("unexpected payload: %#v", parsed)
	}
}

func TestExecute_AdSenseAccountsGet_JSON(t *testing.T) {
	svc := newAdSenseTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v2/accounts/pub-123")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "accounts/pub-123",
			"displayName": "My Site",
			"state":       "READY",
		})
	}))
	result := executeWithAdSenseTestService(t, []string{
		"--json",
		"--account", "a@b.com",
		"adsense", "accounts", "get", "pub-123",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed struct {
		Account struct {
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"account"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Account.Name != "accounts/pub-123" || parsed.Account.State != "READY" {
		t.Fatalf("unexpected payload: %#v", parsed)
	}
}

func TestExecute_AdSenseAdClientsList_JSON(t *testing.T) {
	svc := newAdSenseTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/v2/accounts/pub-123/adclients")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"adClients": []map[string]any{
				{
					"name":        "accounts/pub-123/adclients/ca-pub-123",
					"productCode": "AFC",
					"state":       "READY",
				},
			},
		})
	}))
	result := executeWithAdSenseTestService(t, []string{
		"--json",
		"--account", "a@b.com",
		"adsense", "adclients", "list", "pub-123",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed struct {
		AdClients []struct {
			Name string `json:"name"`
		} `json:"adClients"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.AdClients) != 1 || parsed.AdClients[0].Name != "accounts/pub-123/adclients/ca-pub-123" {
		t.Fatalf("unexpected payload: %#v", parsed)
	}
}

func TestExecute_AdSenseReportsQuery_JSON(t *testing.T) {
	svc := newAdSenseTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/reports:generate")) {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("dateRange") != "LAST_7_DAYS" {
			t.Fatalf("unexpected dateRange: %q", q.Get("dateRange"))
		}
		if got := q["dimensions"]; len(got) != 1 || got[0] != "DATE" {
			t.Fatalf("unexpected dimensions: %#v", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"headers": []map[string]any{
				{"name": "DATE", "type": "DIMENSION"},
				{"name": "ESTIMATED_EARNINGS", "type": "METRIC_CURRENCY"},
			},
			"rows": []map[string]any{
				{"cells": []map[string]any{{"value": "2026-02-01"}, {"value": "1.23"}}},
			},
			"totalMatchedRows": "1",
		})
	}))
	result := executeWithAdSenseTestService(t, []string{
		"--json",
		"--account", "a@b.com",
		"adsense", "reports", "query", "pub-123",
		"--metrics", "estimated_earnings",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed struct {
		Rows []struct {
			Cells []struct {
				Value string `json:"value"`
			} `json:"cells"`
		} `json:"rows"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Rows) != 1 || len(parsed.Rows[0].Cells) != 2 || parsed.Rows[0].Cells[0].Value != "2026-02-01" {
		t.Fatalf("unexpected payload: %#v", parsed)
	}
}

func TestExecute_AdSenseReportsQuery_ValidatesFlagsBeforeServiceCall(t *testing.T) {
	result := executeWithAdSenseTestServiceFactory(
		t,
		[]string{
			"--json", "--account", "a@b.com",
			"adsense", "reports", "query", "pub-123",
			"--from", "2026-02-01",
			"--metrics", "CLICKS",
		},
		unexpectedAdSenseTestService(t, "expected validation to fail before creating adsense service"),
	)
	if result.err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(result.err.Error(), "--from and --to must be set together") {
		t.Fatalf("unexpected error: %v", result.err)
	}
}

func TestExecute_AdSenseAccountsList_ServiceError(t *testing.T) {
	result := executeWithAdSenseTestServiceFactory(
		t,
		[]string{"--account", "a@b.com", "adsense", "accounts", "list"},
		func(context.Context, string) (*adsenseapi.Service, error) {
			return nil, errors.New("adsense service down")
		},
	)
	if result.err == nil || !strings.Contains(result.err.Error(), "adsense service down") {
		t.Fatalf("unexpected err: %v", result.err)
	}
}

func TestExecute_AdSenseReportTimezones(t *testing.T) {
	for _, saved := range []bool{false, true} {
		for _, timezone := range []string{"", "ACCOUNT_TIME_ZONE", "google_time_zone", "America/Los_Angeles"} {
			args := []string{"--json", "--account", "a@b.com", "adsense", "reports"}
			if saved {
				args = append(args, "saved", "query", "accounts/pub-123/reports/report-1")
			} else {
				args = append(args, "query", "pub-123")
			}
			args = append(args, "--timezone", timezone)
			if timezone == "America/Los_Angeles" {
				result := executeWithAdSenseTestServiceFactory(t, args, unexpectedAdSenseTestService(t, "invalid timezone must fail before auth"))
				if ExitCode(result.err) != 2 || !strings.Contains(result.err.Error(), "--timezone must be") {
					t.Fatalf("expected timezone usage error, got %v", result.err)
				}
				continue
			}
			svc := newAdSenseTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "generate") || r.URL.Query().Get("reportingTimeZone") != strings.ToUpper(timezone) {
					t.Errorf("unexpected report request: %s %s", r.Method, r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"rows":[]}`))
			}))
			if result := executeWithAdSenseTestService(t, args, svc); result.err != nil {
				t.Fatalf("report timezone %q (saved=%t): %v", timezone, saved, result.err)
			}
		}
	}
}
