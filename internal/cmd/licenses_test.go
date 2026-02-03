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

	"github.com/steipete/gogcli/internal/outfmt"
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

func TestLicensesListCmd_ProductOnly(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should call ListForProduct, not ListForProductAndSku
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/apps/licensing/v1/product/Google-Apps/users") {
			http.NotFound(w, r)
			return
		}
		// Ensure it's not calling the SKU-specific endpoint
		if strings.Contains(r.URL.Path, "/sku/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"userId": "user1@example.com", "productId": "Google-Apps", "skuId": "1010020027"},
				{"userId": "user2@example.com", "productId": "Google-Apps", "skuId": "1010020028"},
			},
		})
	})
	stubLicensing(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LicensesListCmd{Product: "Google-Apps"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "user1@example.com") || !strings.Contains(out, "user2@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestLicensesListCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
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

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, `"items"`) {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestLicensesListCmd_MissingProduct(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LicensesListCmd{}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing product")
	}
	if !strings.Contains(err.Error(), "--product is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLicensesGetCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if !strings.Contains(r.URL.Path, "/user@example.com") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"userId":      "user@example.com",
			"productId":   "Google-Apps",
			"productName": "Google Workspace",
			"skuId":       "1010020027",
			"skuName":     "Google Workspace Business Starter",
		})
	})
	stubLicensing(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LicensesGetCmd{User: "user@example.com", Product: "Google-Apps", SKU: "1010020027"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "User:") || !strings.Contains(out, "user@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "Product:") || !strings.Contains(out, "Google-Apps") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "SKU:") || !strings.Contains(out, "1010020027") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestLicensesGetCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"userId":    "user@example.com",
			"productId": "Google-Apps",
			"skuId":     "1010020027",
		})
	})
	stubLicensing(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LicensesGetCmd{User: "user@example.com", Product: "Google-Apps", SKU: "1010020027"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, `"userId"`) {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestLicensesGetCmd_MissingUser(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LicensesGetCmd{User: "", Product: "Google-Apps", SKU: "1010020027"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing user")
	}
	if !strings.Contains(err.Error(), "user is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLicensesGetCmd_WithProductAndSkuName(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"userId":      "user@example.com",
			"productId":   "Google-Apps",
			"productName": "Google Workspace",
			"skuId":       "1010020027",
			"skuName":     "Business Starter",
		})
	})
	stubLicensing(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LicensesGetCmd{User: "user@example.com", Product: "Google-Apps", SKU: "1010020027"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Product Name:") || !strings.Contains(out, "Google Workspace") {
		t.Fatalf("expected product name, got: %s", out)
	}
	if !strings.Contains(out, "SKU Name:") || !strings.Contains(out, "Business Starter") {
		t.Fatalf("expected sku name, got: %s", out)
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

func TestLicensesAssignCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"userId":    "user@example.com",
				"productId": "Google-Apps",
				"skuId":     "1010020027",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubLicensing(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LicensesAssignCmd{User: "user@example.com", Product: "Google-Apps", SKU: "1010020027"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, `"userId"`) {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestLicensesAssignCmd_MissingUser(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &LicensesAssignCmd{User: "  ", Product: "Google-Apps", SKU: "1010020027"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing user")
	}
	if !strings.Contains(err.Error(), "user is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLicensesRevokeCmd(t *testing.T) {
	var deleteCalled bool
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/user@example.com") {
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})
	stubLicensing(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &LicensesRevokeCmd{User: "user@example.com", Product: "Google-Apps", SKU: "1010020027"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !deleteCalled {
		t.Fatal("delete was not called")
	}
	if !strings.Contains(out, "Revoked license") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestLicensesRevokeCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})
	stubLicensing(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &LicensesRevokeCmd{User: "user@example.com", Product: "Google-Apps", SKU: "1010020027"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, `"revoked"`) || !strings.Contains(out, `true`) {
		t.Fatalf("expected JSON output with revoked: true, got: %s", out)
	}
}

func TestLicensesRevokeCmd_MissingUser(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &LicensesRevokeCmd{User: "", Product: "Google-Apps", SKU: "1010020027"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing user")
	}
	if !strings.Contains(err.Error(), "user is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLicensesRevokeCmd_RequiresConfirmation(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com", Force: false, NoInput: true}
	cmd := &LicensesRevokeCmd{User: "user@example.com", Product: "Google-Apps", SKU: "1010020027"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error when confirmation is required but not provided")
	}
	if !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("unexpected error: %v", err)
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

func TestLicensesProductsCmd_JSON(t *testing.T) {
	cmd := &LicensesProductsCmd{}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, &RootFlags{}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, `"id"`) || !strings.Contains(out, `"skus"`) {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestLicensesProductsCmd_ContainsExpectedProducts(t *testing.T) {
	cmd := &LicensesProductsCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), &RootFlags{}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	expectedProducts := []string{
		"Google-Apps",
		"Google Workspace",
		"Cloud Identity",
		"Google Vault",
	}
	for _, p := range expectedProducts {
		if !strings.Contains(out, p) {
			t.Errorf("expected output to contain %q", p)
		}
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
