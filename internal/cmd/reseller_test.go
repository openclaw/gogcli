package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/reseller/v1"

	"github.com/steipete/gogcli/internal/outfmt"
)

func newResellerServiceStub(t *testing.T, handler http.HandlerFunc) (*reseller.Service, func()) {
	t.Helper()

	srv := httptest.NewServer(handler)
	svc, err := reseller.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		srv.Close()
		t.Fatalf("NewService: %v", err)
	}
	return svc, srv.Close
}

func stubResellerService(t *testing.T, svc *reseller.Service) {
	t.Helper()
	orig := newResellerService
	t.Cleanup(func() { newResellerService = orig })
	newResellerService = func(context.Context, string) (*reseller.Service, error) { return svc, nil }
}

func TestResellerCustomersListCmd(t *testing.T) {
	svc, closeSrv := newResellerServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/apps/reseller/v1/subscriptions") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subscriptions": []map[string]any{{
				"customerId":     "C123",
				"customerDomain": "example.com",
			}},
		})
	}))
	t.Cleanup(closeSrv)
	stubResellerService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerCustomersListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestResellerSubscriptionsCreateCmd(t *testing.T) {
	var gotPlan string
	var gotSeats int64

	svc, closeSrv := newResellerServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/apps/reseller/v1/customers/C1/subscriptions") {
			http.NotFound(w, r)
			return
		}
		var payload reseller.Subscription
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload.Plan != nil {
			gotPlan = payload.Plan.PlanName
		}
		if payload.Seats != nil {
			gotSeats = payload.Seats.NumberOfSeats
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subscriptionId": "sub1",
			"customerId":     "C1",
		})
	}))
	t.Cleanup(closeSrv)
	stubResellerService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerSubscriptionsCreateCmd{
		Customer: "C1",
		Plan:     "ANNUAL_MONTHLY_PAY",
		SKU:      "sku1",
		Seats:    5,
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotPlan != "ANNUAL_MONTHLY_PAY" || gotSeats != 5 {
		t.Fatalf("unexpected payload: plan=%s seats=%d", gotPlan, gotSeats)
	}
	if !strings.Contains(out, "Created subscription") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestResellerCustomersGetCmd(t *testing.T) {
	svc, closeSrv := newResellerServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/apps/reseller/v1/customers/C123") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"customerId":     "C123",
			"customerDomain": "example.com",
			"customerType":   "domain",
			"primaryAdmin":   map[string]any{"primaryEmail": "admin@example.com"},
		})
	}))
	t.Cleanup(closeSrv)
	stubResellerService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerCustomersGetCmd{Customer: "C123"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "C123") {
		t.Fatalf("expected customer ID in output, got: %s", out)
	}
	if !strings.Contains(out, "example.com") {
		t.Fatalf("expected domain in output, got: %s", out)
	}
}

func TestResellerCustomersGetCmd_JSON(t *testing.T) {
	svc, closeSrv := newResellerServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/apps/reseller/v1/customers/C123") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"customerId":     "C123",
			"customerDomain": "example.com",
			"customerType":   "domain",
		})
	}))
	t.Cleanup(closeSrv)
	stubResellerService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerCustomersGetCmd{Customer: "C123"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, `"customerId"`) {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestResellerCustomersGetCmd_EmptyCustomer(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerCustomersGetCmd{Customer: "   "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty customer")
	}
	if !strings.Contains(err.Error(), "customer is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResellerCustomersGetCmd_APIError(t *testing.T) {
	svc, closeSrv := newResellerServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    404,
				"message": "Customer not found",
			},
		})
	}))
	t.Cleanup(closeSrv)
	stubResellerService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerCustomersGetCmd{Customer: "NOTFOUND"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for API failure")
	}
	if !strings.Contains(err.Error(), "get customer") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResellerCustomersGetCmd_WithPrimaryAdmin(t *testing.T) {
	svc, closeSrv := newResellerServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/apps/reseller/v1/customers/C123") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"customerId":     "C123",
			"customerDomain": "example.com",
			"customerType":   "team",
			"primaryAdmin":   map[string]any{"primaryEmail": "admin@example.com"},
		})
	}))
	t.Cleanup(closeSrv)
	stubResellerService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerCustomersGetCmd{Customer: "C123"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Primary Admin:") || !strings.Contains(out, "admin@example.com") {
		t.Fatalf("expected primary admin in output, got: %s", out)
	}
	if !strings.Contains(out, "Type:") || !strings.Contains(out, "team") {
		t.Fatalf("expected type in output, got: %s", out)
	}
}

func TestResellerSubscriptionsListCmd(t *testing.T) {
	svc, closeSrv := newResellerServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/apps/reseller/v1/subscriptions") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subscriptions": []map[string]any{
				{
					"customerId":     "C123",
					"subscriptionId": "sub1",
					"skuId":          "Google-Apps-Unlimited",
					"plan":           map[string]any{"planName": "ANNUAL"},
					"status":         "ACTIVE",
				},
				{
					"customerId":     "C456",
					"subscriptionId": "sub2",
					"skuId":          "Google-Vault",
					"plan":           map[string]any{"planName": "FLEXIBLE"},
					"status":         "ACTIVE",
				},
			},
		})
	}))
	t.Cleanup(closeSrv)
	stubResellerService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerSubscriptionsListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "C123") || !strings.Contains(out, "sub1") {
		t.Fatalf("expected subscription data in output, got: %s", out)
	}
	if !strings.Contains(out, "ANNUAL") {
		t.Fatalf("expected plan name in output, got: %s", out)
	}
}

func TestResellerSubscriptionsListCmd_JSON(t *testing.T) {
	svc, closeSrv := newResellerServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/apps/reseller/v1/subscriptions") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subscriptions": []map[string]any{
				{
					"customerId":     "C123",
					"subscriptionId": "sub1",
					"skuId":          "Google-Apps-Unlimited",
				},
			},
		})
	}))
	t.Cleanup(closeSrv)
	stubResellerService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerSubscriptionsListCmd{}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, `"subscriptions"`) {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestResellerSubscriptionsListCmd_Empty(t *testing.T) {
	svc, closeSrv := newResellerServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/apps/reseller/v1/subscriptions") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subscriptions": []map[string]any{},
		})
	}))
	t.Cleanup(closeSrv)
	stubResellerService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerSubscriptionsListCmd{}

	stderr := captureStderr(t, func() {
		if err := cmd.Run(testContextWithStderr(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(stderr, "No subscriptions found") {
		t.Fatalf("expected 'No subscriptions found' message, got: %s", stderr)
	}
}

func TestResellerSubscriptionsListCmd_WithFilters(t *testing.T) {
	var gotCustomer, gotPrefix string
	svc, closeSrv := newResellerServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/apps/reseller/v1/subscriptions") {
			http.NotFound(w, r)
			return
		}
		gotCustomer = r.URL.Query().Get("customerId")
		gotPrefix = r.URL.Query().Get("customerNamePrefix")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subscriptions": []map[string]any{
				{
					"customerId":     "C123",
					"subscriptionId": "sub1",
					"skuId":          "Google-Apps-Unlimited",
				},
			},
		})
	}))
	t.Cleanup(closeSrv)
	stubResellerService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerSubscriptionsListCmd{Customer: "C123", Prefix: "test"}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotCustomer != "C123" {
		t.Fatalf("expected customerId filter, got: %s", gotCustomer)
	}
	if gotPrefix != "test" {
		t.Fatalf("expected prefix filter, got: %s", gotPrefix)
	}
}

func TestResellerSubscriptionsListCmd_APIError(t *testing.T) {
	svc, closeSrv := newResellerServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    500,
				"message": "Internal error",
			},
		})
	}))
	t.Cleanup(closeSrv)
	stubResellerService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerSubscriptionsListCmd{}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for API failure")
	}
	if !strings.Contains(err.Error(), "list subscriptions") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResellerSubscriptionsGetCmd(t *testing.T) {
	svc, closeSrv := newResellerServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/apps/reseller/v1/customers/C123/subscriptions/sub1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"customerId":     "C123",
			"subscriptionId": "sub1",
			"skuId":          "Google-Apps-Unlimited",
			"plan":           map[string]any{"planName": "ANNUAL"},
			"status":         "ACTIVE",
		})
	}))
	t.Cleanup(closeSrv)
	stubResellerService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerSubscriptionsGetCmd{Customer: "C123", Subscription: "sub1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Customer:") || !strings.Contains(out, "C123") {
		t.Fatalf("expected customer in output, got: %s", out)
	}
	if !strings.Contains(out, "Subscription:") || !strings.Contains(out, "sub1") {
		t.Fatalf("expected subscription in output, got: %s", out)
	}
	if !strings.Contains(out, "Plan:") || !strings.Contains(out, "ANNUAL") {
		t.Fatalf("expected plan in output, got: %s", out)
	}
}

func TestResellerSubscriptionsGetCmd_JSON(t *testing.T) {
	svc, closeSrv := newResellerServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/apps/reseller/v1/customers/C123/subscriptions/sub1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"customerId":     "C123",
			"subscriptionId": "sub1",
			"skuId":          "Google-Apps-Unlimited",
		})
	}))
	t.Cleanup(closeSrv)
	stubResellerService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerSubscriptionsGetCmd{Customer: "C123", Subscription: "sub1"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, `"subscriptionId"`) {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestResellerSubscriptionsGetCmd_EmptyCustomer(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerSubscriptionsGetCmd{Customer: "   ", Subscription: "sub1"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty customer")
	}
	if !strings.Contains(err.Error(), "customer and subscription are required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResellerSubscriptionsGetCmd_EmptySubscription(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerSubscriptionsGetCmd{Customer: "C123", Subscription: ""}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty subscription")
	}
	if !strings.Contains(err.Error(), "customer and subscription are required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResellerSubscriptionsGetCmd_APIError(t *testing.T) {
	svc, closeSrv := newResellerServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    404,
				"message": "Subscription not found",
			},
		})
	}))
	t.Cleanup(closeSrv)
	stubResellerService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerSubscriptionsGetCmd{Customer: "C123", Subscription: "NOTFOUND"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for API failure")
	}
	if !strings.Contains(err.Error(), "get subscription") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResellerSubscriptionsGetCmd_WithStatus(t *testing.T) {
	svc, closeSrv := newResellerServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/apps/reseller/v1/customers/C123/subscriptions/sub1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"customerId":     "C123",
			"subscriptionId": "sub1",
			"skuId":          "Google-Apps-Unlimited",
			"status":         "SUSPENDED",
		})
	}))
	t.Cleanup(closeSrv)
	stubResellerService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerSubscriptionsGetCmd{Customer: "C123", Subscription: "sub1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Status:") || !strings.Contains(out, "SUSPENDED") {
		t.Fatalf("expected status in output, got: %s", out)
	}
}

func TestResellerCustomersListCmd_JSON(t *testing.T) {
	svc, closeSrv := newResellerServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/apps/reseller/v1/subscriptions") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subscriptions": []map[string]any{{
				"customerId":     "C123",
				"customerDomain": "example.com",
			}},
			"nextPageToken": "token123",
		})
	}))
	t.Cleanup(closeSrv)
	stubResellerService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerCustomersListCmd{}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, `"customers"`) {
		t.Fatalf("expected JSON output with customers, got: %s", out)
	}
	if !strings.Contains(out, `"nextPageToken"`) {
		t.Fatalf("expected nextPageToken in JSON output, got: %s", out)
	}
}

func TestResellerCustomersListCmd_Empty(t *testing.T) {
	svc, closeSrv := newResellerServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/apps/reseller/v1/subscriptions") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subscriptions": []map[string]any{},
		})
	}))
	t.Cleanup(closeSrv)
	stubResellerService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerCustomersListCmd{}

	stderr := captureStderr(t, func() {
		if err := cmd.Run(testContextWithStderr(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(stderr, "No customers found") {
		t.Fatalf("expected 'No customers found' message, got: %s", stderr)
	}
}

func TestResellerCustomersListCmd_WithPrefix(t *testing.T) {
	var gotPrefix string
	svc, closeSrv := newResellerServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/apps/reseller/v1/subscriptions") {
			http.NotFound(w, r)
			return
		}
		gotPrefix = r.URL.Query().Get("customerNamePrefix")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subscriptions": []map[string]any{{
				"customerId":     "C123",
				"customerDomain": "example.com",
			}},
		})
	}))
	t.Cleanup(closeSrv)
	stubResellerService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerCustomersListCmd{Prefix: "test"}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotPrefix != "test" {
		t.Fatalf("expected prefix filter, got: %s", gotPrefix)
	}
}

func TestResellerCustomersListCmd_DedupesCustomers(t *testing.T) {
	svc, closeSrv := newResellerServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/apps/reseller/v1/subscriptions") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// Return multiple subscriptions for the same customer
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subscriptions": []map[string]any{
				{"customerId": "C123", "customerDomain": "example.com", "skuId": "sku1"},
				{"customerId": "C123", "customerDomain": "example.com", "skuId": "sku2"},
				{"customerId": "C456", "customerDomain": "other.com", "skuId": "sku3"},
			},
		})
	}))
	t.Cleanup(closeSrv)
	stubResellerService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ResellerCustomersListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	// Should only have 2 customers, not 3 rows
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// Header + 2 customers = 3 lines
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 customers), got %d: %s", len(lines), out)
	}
}
