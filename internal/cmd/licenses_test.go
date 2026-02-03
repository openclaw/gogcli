package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/licensing/v1"
	"google.golang.org/api/option"
)

func TestLicensesListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/apps/licensing/v1/product/Google-Apps/sku/1010020027/users") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"userId": "user@example.com", "productId": "Google-Apps", "skuId": "1010020027"},
			},
		})
	})
	stubLicensing(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LicensesListCmd{Product: "Google-Apps", SKU: "1010020027"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "user@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestLicensesAssignCmd(t *testing.T) {
	var gotUser string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/apps/licensing/v1/product/Google-Apps/sku/1010020027/user"):
			var payload struct {
				UserId string `json:"userId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			gotUser = payload.UserId
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"userId":    payload.UserId,
				"productId": "Google-Apps",
				"skuId":     "1010020027",
			})
			return
		default:
			http.NotFound(w, r)
		}
	})
	stubLicensing(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LicensesAssignCmd{User: "user@example.com", Product: "Google-Apps", SKU: "1010020027"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotUser != "user@example.com" {
		t.Fatalf("unexpected user: %q", gotUser)
	}
	if !strings.Contains(out, "Assigned license") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestLicensesProductsCmd(t *testing.T) {
	cmd := &LicensesProductsCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), &RootFlags{}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Google-Apps") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func stubLicensing(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	orig := newLicensingService
	svc, err := licensing.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new licensing service: %v", err)
	}
	newLicensingService = func(context.Context, string) (*licensing.Service, error) { return svc, nil }
	t.Cleanup(func() {
		newLicensingService = orig
		srv.Close()
	})
	return srv
}
