package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	reports "google.golang.org/api/admin/reports/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
)

func TestReportsUserCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "sam@example.com"},
					"ipAddress": "1.2.3.4",
					"events": []map[string]any{
						{"name": "login"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUserCmd{Date: "2026-01-02"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "sam@example.com") || !strings.Contains(out, "login") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsUserCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "json@example.com"},
					"ipAddress": "5.6.7.8",
					"events": []map[string]any{
						{"name": "user_event"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUserCmd{Date: "2026-01-02"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "json@example.com") || !strings.Contains(out, "items") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestReportsUserCmd_Empty(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUserCmd{Date: "2026-01-02"}

	// No error expected, just "no events" message
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestReportsUserCmd_WithFilters(t *testing.T) {
	var gotFilters string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		gotFilters = r.URL.Query().Get("filters")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "filtered@example.com"},
					"ipAddress": "1.1.1.1",
					"events": []map[string]any{
						{"name": "filtered_event"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUserCmd{Date: "2026-01-02", Filters: "event_name==login"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotFilters != "event_name==login" {
		t.Fatalf("expected filters event_name==login, got %q", gotFilters)
	}
	if !strings.Contains(out, "filtered@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsAdminCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		// Verify application is admin
		if !strings.Contains(r.URL.Path, "/admin") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "admin@example.com"},
					"ipAddress": "10.0.0.1",
					"events": []map[string]any{
						{"name": "CREATE_USER"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsAdminCmd{Date: "2026-01-02"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "admin@example.com") || !strings.Contains(out, "CREATE_USER") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsAdminCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "admin-json@example.com"},
					"ipAddress": "10.0.0.2",
					"events": []map[string]any{
						{"name": "DELETE_USER"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsAdminCmd{Date: "2026-01-02"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "admin-json@example.com") || !strings.Contains(out, "items") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestReportsAdminCmd_WithEvent(t *testing.T) {
	var gotEventName string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		gotEventName = r.URL.Query().Get("eventName")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "admin@example.com"},
					"ipAddress": "10.0.0.1",
					"events": []map[string]any{
						{"name": "CREATE_USER"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsAdminCmd{Date: "2026-01-02", Event: "CREATE_USER"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotEventName != "CREATE_USER" {
		t.Fatalf("expected event name CREATE_USER, got %q", gotEventName)
	}
	if !strings.Contains(out, "CREATE_USER") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsAdminCmd_Empty(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsAdminCmd{Date: "2026-01-02"}

	// No error expected, just "no events" message
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestReportsLoginCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		// Verify application is login
		if !strings.Contains(r.URL.Path, "/login") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "user@example.com"},
					"ipAddress": "192.168.1.1",
					"events": []map[string]any{
						{"name": "login_success"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsLoginCmd{Date: "2026-01-02"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "user@example.com") || !strings.Contains(out, "login_success") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsLoginCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "login-json@example.com"},
					"ipAddress": "192.168.1.2",
					"events": []map[string]any{
						{"name": "login_failure"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsLoginCmd{Date: "2026-01-02"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "login-json@example.com") || !strings.Contains(out, "items") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestReportsLoginCmd_WithUser(t *testing.T) {
	var gotUserKey string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		// Extract user key from path
		parts := strings.Split(r.URL.Path, "/")
		for i, part := range parts {
			if part == "users" && i+1 < len(parts) {
				gotUserKey = parts[i+1]
				break
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "specific@example.com"},
					"ipAddress": "192.168.1.3",
					"events": []map[string]any{
						{"name": "login_success"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsLoginCmd{Date: "2026-01-02", User: "specific@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotUserKey != "specific@example.com" {
		t.Fatalf("expected user key specific@example.com, got %q", gotUserKey)
	}
	if !strings.Contains(out, "specific@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsLoginCmd_Empty(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsLoginCmd{Date: "2026-01-02"}

	// No error expected, just "no events" message
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestReportsDriveCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		// Verify application is drive
		if !strings.Contains(r.URL.Path, "/drive") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "drive@example.com"},
					"ipAddress": "172.16.0.1",
					"events": []map[string]any{
						{"name": "view"},
						{"name": "download"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsDriveCmd{Date: "2026-01-02"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "drive@example.com") || !strings.Contains(out, "view") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsDriveCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "drive-json@example.com"},
					"ipAddress": "172.16.0.2",
					"events": []map[string]any{
						{"name": "edit"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsDriveCmd{Date: "2026-01-02"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "drive-json@example.com") || !strings.Contains(out, "items") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestReportsDriveCmd_WithUser(t *testing.T) {
	var gotUserKey string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		// Extract user key from path
		parts := strings.Split(r.URL.Path, "/")
		for i, part := range parts {
			if part == "users" && i+1 < len(parts) {
				gotUserKey = parts[i+1]
				break
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "driveuser@example.com"},
					"ipAddress": "172.16.0.3",
					"events": []map[string]any{
						{"name": "create"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsDriveCmd{Date: "2026-01-02", User: "driveuser@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotUserKey != "driveuser@example.com" {
		t.Fatalf("expected user key driveuser@example.com, got %q", gotUserKey)
	}
	if !strings.Contains(out, "driveuser@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsDriveCmd_Empty(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsDriveCmd{Date: "2026-01-02"}

	// No error expected, just "no events" message
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestReportsUsageCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/usage/dates/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"usageReports": []map[string]any{
				{
					"date": "2026-01-02",
					"entity": map[string]any{
						"type":       "CUSTOMER",
						"customerId": "my_customer",
					},
					"parameters": []map[string]any{
						{"name": "num_users", "intValue": "42"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUsageCmd{Application: "gmail", Date: "2026-01-02", Parameters: "num_users"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "num_users=42") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsUsageCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/usage/dates/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"usageReports": []map[string]any{
				{
					"date": "2026-01-02",
					"entity": map[string]any{
						"type": "CUSTOMER",
					},
					"parameters": []map[string]any{
						{"name": "storage_used", "intValue": "1024"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUsageCmd{Application: "drive", Date: "2026-01-02"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "usageReports") || !strings.Contains(out, "storage_used") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestReportsUsageCmd_RequiresApplication(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUsageCmd{Application: "", Date: "2026-01-02"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error when Application is empty")
	}
}

func TestReportsUsageCmd_Empty(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/usage/dates/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"usageReports": []map[string]any{},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUsageCmd{Application: "gmail", Date: "2026-01-02"}

	// No error expected, just "no reports" message
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestReportsAccountsCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/usage/dates/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"usageReports": []map[string]any{
				{
					"date": "2026-01-02",
					"entity": map[string]any{
						"type":     "USER",
						"entityId": "user123",
					},
					"parameters": []map[string]any{
						{"name": "accounts:total_accounts", "intValue": "100"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsAccountsCmd{Date: "2026-01-02"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "2026-01-02") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsAccountsCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/usage/dates/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"usageReports": []map[string]any{
				{
					"date": "2026-01-02",
					"entity": map[string]any{
						"type": "USER",
					},
					"parameters": []map[string]any{
						{"name": "accounts:active_accounts", "intValue": "85"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsAccountsCmd{Date: "2026-01-02"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "usageReports") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestReportsAccountsCmd_Empty(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/usage/dates/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"usageReports": []map[string]any{},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsAccountsCmd{Date: "2026-01-02"}

	// No error expected, just "no reports" message
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestReportsEmailLogCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		// Verify application is email
		if !strings.Contains(r.URL.Path, "/email") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "sender@example.com"},
					"ipAddress": "203.0.113.1",
					"events": []map[string]any{
						{"name": "email_sent"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsEmailLogCmd{Date: "2026-01-02"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "sender@example.com") || !strings.Contains(out, "email_sent") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsEmailLogCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "email-json@example.com"},
					"ipAddress": "203.0.113.2",
					"events": []map[string]any{
						{"name": "email_received"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsEmailLogCmd{Date: "2026-01-02"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "email-json@example.com") || !strings.Contains(out, "items") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestReportsEmailLogCmd_WithRecipient(t *testing.T) {
	var gotFilters string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		gotFilters = r.URL.Query().Get("filters")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "sender@example.com"},
					"ipAddress": "203.0.113.3",
					"events": []map[string]any{
						{"name": "email_sent"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsEmailLogCmd{Date: "2026-01-02", Recipient: "recipient@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(gotFilters, "recipient==recipient@example.com") {
		t.Fatalf("expected recipient filter, got %q", gotFilters)
	}
	if !strings.Contains(out, "sender@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsEmailLogCmd_WithFiltersAndRecipient(t *testing.T) {
	var gotFilters string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		gotFilters = r.URL.Query().Get("filters")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "sender@example.com"},
					"ipAddress": "203.0.113.4",
					"events": []map[string]any{
						{"name": "email_sent"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsEmailLogCmd{
		Date:      "2026-01-02",
		Recipient: "recipient@example.com",
		Filters:   "message_size>1000",
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	// Should contain both filters combined
	if !strings.Contains(gotFilters, "message_size>1000") {
		t.Fatalf("expected message_size filter, got %q", gotFilters)
	}
	if !strings.Contains(gotFilters, "recipient==recipient@example.com") {
		t.Fatalf("expected recipient filter, got %q", gotFilters)
	}
	if !strings.Contains(out, "sender@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsEmailLogCmd_Empty(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsEmailLogCmd{Date: "2026-01-02"}

	// No error expected, just "no events" message
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestReportsActivityCmd_Pagination(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "page@example.com"},
					"ipAddress": "1.1.1.1",
					"events": []map[string]any{
						{"name": "event1"},
					},
				},
			},
			"nextPageToken": "next_page_token_123",
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUserCmd{Date: "2026-01-02"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "page@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsActivityCmd_MaxResults(t *testing.T) {
	var gotMaxResults string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		gotMaxResults = r.URL.Query().Get("maxResults")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "max@example.com"},
					"ipAddress": "1.1.1.1",
					"events": []map[string]any{
						{"name": "event1"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUserCmd{Date: "2026-01-02", Max: 50}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotMaxResults != "50" {
		t.Fatalf("expected maxResults 50, got %q", gotMaxResults)
	}
	if !strings.Contains(out, "max@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsActivityCmd_PageToken(t *testing.T) {
	var gotPageToken string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		gotPageToken = r.URL.Query().Get("pageToken")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "paged@example.com"},
					"ipAddress": "1.1.1.1",
					"events": []map[string]any{
						{"name": "event1"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUserCmd{Date: "2026-01-02", Page: "my_page_token"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotPageToken != "my_page_token" {
		t.Fatalf("expected pageToken my_page_token, got %q", gotPageToken)
	}
	if !strings.Contains(out, "paged@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsActivityCmd_APIError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    500,
				"message": "Internal server error",
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUserCmd{Date: "2026-01-02"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error from API")
	}
	if !strings.Contains(err.Error(), "fetch user report") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReportsUsageCmd_APIError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    403,
				"message": "Access denied",
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUsageCmd{Application: "gmail", Date: "2026-01-02"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error from API")
	}
	if !strings.Contains(err.Error(), "fetch usage report") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReportsActivityCmd_NilActor(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"ipAddress": "1.1.1.1",
					"events": []map[string]any{
						{"name": "system_event"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUserCmd{Date: "2026-01-02"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "system_event") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsActivityCmd_MultipleEvents(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "multi@example.com"},
					"ipAddress": "1.1.1.1",
					"events": []map[string]any{
						{"name": "event1"},
						{"name": "event2"},
						{"name": "event3"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUserCmd{Date: "2026-01-02"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	// Events should be comma-separated
	if !strings.Contains(out, "event1,event2,event3") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsUsageCmd_WithParameters(t *testing.T) {
	var gotParams string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/usage/dates/") {
			http.NotFound(w, r)
			return
		}
		gotParams = r.URL.Query().Get("parameters")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"usageReports": []map[string]any{
				{
					"date": "2026-01-02",
					"entity": map[string]any{
						"type": "CUSTOMER",
					},
					"parameters": []map[string]any{
						{"name": "accounts:num_users", "intValue": "100"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUsageCmd{Application: "accounts", Date: "2026-01-02", Parameters: "num_users"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	// Parameters should be prefixed with application
	if gotParams != "accounts:num_users" {
		t.Fatalf("expected parameters accounts:num_users, got %q", gotParams)
	}
	if !strings.Contains(out, "num_users=100") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsUsageCmd_FullyQualifiedParameters(t *testing.T) {
	var gotParams string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/usage/dates/") {
			http.NotFound(w, r)
			return
		}
		gotParams = r.URL.Query().Get("parameters")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"usageReports": []map[string]any{
				{
					"date": "2026-01-02",
					"entity": map[string]any{
						"type": "CUSTOMER",
					},
					"parameters": []map[string]any{
						{"name": "drive:total_storage", "intValue": "5000"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUsageCmd{Application: "accounts", Date: "2026-01-02", Parameters: "drive:total_storage"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	// Fully qualified parameters should not be prefixed again
	if gotParams != "drive:total_storage" {
		t.Fatalf("expected parameters drive:total_storage, got %q", gotParams)
	}
	if !strings.Contains(out, "total_storage=5000") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsUsageCmd_StringParameter(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/usage/dates/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"usageReports": []map[string]any{
				{
					"date": "2026-01-02",
					"entity": map[string]any{
						"type": "CUSTOMER",
					},
					"parameters": []map[string]any{
						{"name": "version", "stringValue": "v2.0"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUsageCmd{Application: "gmail", Date: "2026-01-02"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "version=v2.0") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsUsageCmd_DatetimeParameter(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/usage/dates/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"usageReports": []map[string]any{
				{
					"date": "2026-01-02",
					"entity": map[string]any{
						"type": "CUSTOMER",
					},
					"parameters": []map[string]any{
						{"name": "last_sync", "datetimeValue": "2026-01-02T10:00:00Z"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUsageCmd{Application: "gmail", Date: "2026-01-02"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "last_sync=2026-01-02T10:00:00Z") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsUsageCmd_BoolParameter(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/usage/dates/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"usageReports": []map[string]any{
				{
					"date": "2026-01-02",
					"entity": map[string]any{
						"type": "CUSTOMER",
					},
					"parameters": []map[string]any{
						{"name": "enabled", "boolValue": true},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUsageCmd{Application: "gmail", Date: "2026-01-02"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "enabled=true") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportDate(t *testing.T) {
	// Test with empty date - should return today's date
	got := reportDate("")
	if got == "" {
		t.Fatal("expected non-empty date")
	}

	// Test with explicit date
	got = reportDate("2026-05-15")
	if got != "2026-05-15" {
		t.Fatalf("expected 2026-05-15, got %s", got)
	}

	// Test with whitespace
	got = reportDate("  2026-05-15  ")
	if got != "2026-05-15" {
		t.Fatalf("expected 2026-05-15, got %s", got)
	}
}

func TestReportDateRange(t *testing.T) {
	start, end := reportDateRange("2026-05-15")
	if start != "2026-05-15T00:00:00Z" {
		t.Fatalf("expected start 2026-05-15T00:00:00Z, got %s", start)
	}
	if end != "2026-05-15T23:59:59Z" {
		t.Fatalf("expected end 2026-05-15T23:59:59Z, got %s", end)
	}

	// Test with invalid date
	start, end = reportDateRange("invalid")
	if start != "invalid" || end != "invalid" {
		t.Fatalf("expected invalid for both, got start=%s end=%s", start, end)
	}
}

func TestFormatActivityTime(t *testing.T) {
	// Test with nil
	got := formatActivityTime(nil)
	if got != "" {
		t.Fatalf("expected empty string, got %s", got)
	}

	// Test with empty time
	got = formatActivityTime(&reports.ActivityId{Time: ""})
	if got != "" {
		t.Fatalf("expected empty string, got %s", got)
	}

	// Test with valid unix timestamp
	got = formatActivityTime(&reports.ActivityId{Time: "1700000000"})
	if got == "" || got == "1700000000" {
		t.Fatalf("expected formatted time, got %s", got)
	}

	// Test with invalid timestamp
	got = formatActivityTime(&reports.ActivityId{Time: "not_a_number"})
	if got != "not_a_number" {
		t.Fatalf("expected original value, got %s", got)
	}
}

func TestActivityEventNames(t *testing.T) {
	// Test with nil
	got := activityEventNames(nil)
	if got != "" {
		t.Fatalf("expected empty string, got %s", got)
	}

	// Test with empty slice
	got = activityEventNames([]*reports.ActivityEvents{})
	if got != "" {
		t.Fatalf("expected empty string, got %s", got)
	}

	// Test with single event
	got = activityEventNames([]*reports.ActivityEvents{
		{Name: "event1"},
	})
	if got != "event1" {
		t.Fatalf("expected event1, got %s", got)
	}

	// Test with multiple events
	got = activityEventNames([]*reports.ActivityEvents{
		{Name: "event1"},
		{Name: "event2"},
		{Name: "event3"},
	})
	if got != "event1,event2,event3" {
		t.Fatalf("expected event1,event2,event3, got %s", got)
	}

	// Test with nil event in slice
	got = activityEventNames([]*reports.ActivityEvents{
		{Name: "event1"},
		nil,
		{Name: "event3"},
	})
	if got != "event1,event3" {
		t.Fatalf("expected event1,event3, got %s", got)
	}

	// Test with empty name
	got = activityEventNames([]*reports.ActivityEvents{
		{Name: "event1"},
		{Name: ""},
		{Name: "event3"},
	})
	if got != "event1,event3" {
		t.Fatalf("expected event1,event3, got %s", got)
	}
}

func TestFormatUsageParameters(t *testing.T) {
	// Test with nil
	got := formatUsageParameters(nil)
	if got != "" {
		t.Fatalf("expected empty string, got %s", got)
	}

	// Test with empty slice
	got = formatUsageParameters([]*reports.UsageReportParameters{})
	if got != "" {
		t.Fatalf("expected empty string, got %s", got)
	}

	// Test with string value
	got = formatUsageParameters([]*reports.UsageReportParameters{
		{Name: "version", StringValue: "v1.0"},
	})
	if got != "version=v1.0" {
		t.Fatalf("expected version=v1.0, got %s", got)
	}

	// Test with int value
	got = formatUsageParameters([]*reports.UsageReportParameters{
		{Name: "count", IntValue: 42},
	})
	if got != "count=42" {
		t.Fatalf("expected count=42, got %s", got)
	}

	// Test with bool value
	got = formatUsageParameters([]*reports.UsageReportParameters{
		{Name: "enabled", BoolValue: true},
	})
	if got != "enabled=true" {
		t.Fatalf("expected enabled=true, got %s", got)
	}

	// Test with datetime value
	got = formatUsageParameters([]*reports.UsageReportParameters{
		{Name: "timestamp", DatetimeValue: "2026-01-01T00:00:00Z"},
	})
	if got != "timestamp=2026-01-01T00:00:00Z" {
		t.Fatalf("expected timestamp=2026-01-01T00:00:00Z, got %s", got)
	}

	// Test with multiple parameters
	got = formatUsageParameters([]*reports.UsageReportParameters{
		{Name: "param1", StringValue: "value1"},
		{Name: "param2", IntValue: 100},
	})
	if got != "param1=value1,param2=100" {
		t.Fatalf("expected param1=value1,param2=100, got %s", got)
	}

	// Test with nil parameter in slice
	got = formatUsageParameters([]*reports.UsageReportParameters{
		{Name: "param1", StringValue: "value1"},
		nil,
		{Name: "param3", IntValue: 300},
	})
	if got != "param1=value1,param3=300" {
		t.Fatalf("expected param1=value1,param3=300, got %s", got)
	}

	// Test with name only (no value)
	got = formatUsageParameters([]*reports.UsageReportParameters{
		{Name: "feature_enabled"},
	})
	if got != "feature_enabled" {
		t.Fatalf("expected feature_enabled, got %s", got)
	}
}

func stubReports(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	orig := newReportsService
	svc, err := reports.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new reports service: %v", err)
	}
	newReportsService = func(context.Context, string) (*reports.Service, error) { return svc, nil }
	t.Cleanup(func() {
		newReportsService = orig
		srv.Close()
	})
	return srv
}
