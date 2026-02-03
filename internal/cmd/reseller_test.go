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
