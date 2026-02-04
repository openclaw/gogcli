package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/cloudchannel/v1"

	"github.com/steipete/gogcli/internal/outfmt"
)

func stubCloudChannelService(t *testing.T, svc *cloudchannel.Service) {
	t.Helper()
	orig := newCloudChannelService
	t.Cleanup(func() { newCloudChannelService = orig })
	newCloudChannelService = func(context.Context, string) (*cloudchannel.Service, error) { return svc, nil }
}

// ChannelCustomersListCmd tests

func TestChannelCustomersListCmd(t *testing.T) {
	svc, closeSrv := newCloudChannelServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/accounts/acc/customers") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"customers": []map[string]any{{
				"name":            "accounts/acc/customers/cust1",
				"domain":          "example.com",
				"cloudIdentityId": "CID",
			}},
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudChannelService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ChannelCustomersListCmd{ChannelAccount: "acc"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestChannelCustomersListCmd_EmptyResults(t *testing.T) {
	svc, closeSrv := newCloudChannelServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/accounts/acc/customers") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"customers": []map[string]any{},
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudChannelService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ChannelCustomersListCmd{ChannelAccount: "acc"}

	// Empty results should not error
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestChannelCustomersListCmd_MissingChannelAccount(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ChannelCustomersListCmd{ChannelAccount: ""}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing channel account")
	}
	if !strings.Contains(err.Error(), "--channel-account is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChannelCustomersListCmd_WithAccountsPrefix(t *testing.T) {
	var gotPath string
	svc, closeSrv := newCloudChannelServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/accounts/acc/customers") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"customers": []map[string]any{{
				"name":   "accounts/acc/customers/cust1",
				"domain": "example.com",
			}},
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudChannelService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ChannelCustomersListCmd{ChannelAccount: "accounts/acc"}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	// Should not double the prefix
	if strings.Contains(gotPath, "accounts/accounts/") {
		t.Fatalf("unexpected double prefix in path: %q", gotPath)
	}
}

func TestChannelCustomersListCmd_JSON(t *testing.T) {
	svc, closeSrv := newCloudChannelServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/accounts/acc/customers") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"customers": []map[string]any{{
				"name":            "accounts/acc/customers/cust1",
				"domain":          "example.com",
				"cloudIdentityId": "CID123",
			}},
			"nextPageToken": "token123",
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudChannelService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ChannelCustomersListCmd{ChannelAccount: "acc"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["nextPageToken"] != "token123" {
		t.Fatalf("unexpected nextPageToken: %v", parsed["nextPageToken"])
	}
}

func TestChannelCustomersListCmd_WithPaging(t *testing.T) {
	var gotPageSize, gotPageToken string

	svc, closeSrv := newCloudChannelServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/accounts/acc/customers") {
			http.NotFound(w, r)
			return
		}
		gotPageSize = r.URL.Query().Get("pageSize")
		gotPageToken = r.URL.Query().Get("pageToken")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"customers": []map[string]any{{
				"name":   "accounts/acc/customers/cust1",
				"domain": "example.com",
			}},
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudChannelService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ChannelCustomersListCmd{ChannelAccount: "acc", Max: 25, Page: "mytoken"}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotPageSize != "25" {
		t.Fatalf("unexpected pageSize: %q", gotPageSize)
	}
	if gotPageToken != "mytoken" {
		t.Fatalf("unexpected pageToken: %q", gotPageToken)
	}
}

// ChannelOffersListCmd tests

func TestChannelOffersListCmd(t *testing.T) {
	svc, closeSrv := newCloudChannelServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/accounts/acc/offers") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"offers": []map[string]any{{
				"name": "accounts/acc/offers/offer1",
				"sku": map[string]any{
					"name": "sku1",
					"product": map[string]any{
						"name": "product1",
					},
				},
			}},
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudChannelService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ChannelOffersListCmd{ChannelAccount: "acc"}

	ctx := testContextJSON(t)
	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var parsed struct {
		Offers []struct {
			Name string `json:"name"`
			Sku  struct {
				Name    string `json:"name"`
				Product struct {
					Name string `json:"name"`
				} `json:"product"`
			} `json:"sku"`
		} `json:"offers"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Offers) != 1 {
		t.Fatalf("expected 1 offer, got %d", len(parsed.Offers))
	}
	if parsed.Offers[0].Name != "accounts/acc/offers/offer1" {
		t.Fatalf("unexpected offer name: %s", parsed.Offers[0].Name)
	}
	if parsed.Offers[0].Sku.Name != "sku1" {
		t.Fatalf("unexpected sku name: %s", parsed.Offers[0].Sku.Name)
	}
	if parsed.Offers[0].Sku.Product.Name != "product1" {
		t.Fatalf("unexpected product name: %s", parsed.Offers[0].Sku.Product.Name)
	}
}

func TestChannelOffersListCmd_EmptyResults(t *testing.T) {
	svc, closeSrv := newCloudChannelServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/accounts/acc/offers") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"offers": []map[string]any{},
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudChannelService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ChannelOffersListCmd{ChannelAccount: "acc"}

	// Empty results should not error
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestChannelOffersListCmd_MissingChannelAccount(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ChannelOffersListCmd{ChannelAccount: ""}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing channel account")
	}
	if !strings.Contains(err.Error(), "--channel-account is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChannelOffersListCmd_WithFilter(t *testing.T) {
	var gotFilter string

	svc, closeSrv := newCloudChannelServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/accounts/acc/offers") {
			http.NotFound(w, r)
			return
		}
		gotFilter = r.URL.Query().Get("filter")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"offers": []map[string]any{{
				"name": "accounts/acc/offers/offer1",
			}},
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudChannelService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ChannelOffersListCmd{ChannelAccount: "acc", Filter: "sku.product.name = 'test'"}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotFilter != "sku.product.name = 'test'" {
		t.Fatalf("unexpected filter: %q", gotFilter)
	}
}

func TestChannelOffersListCmd_WithLanguage(t *testing.T) {
	var gotLanguage string

	svc, closeSrv := newCloudChannelServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/accounts/acc/offers") {
			http.NotFound(w, r)
			return
		}
		gotLanguage = r.URL.Query().Get("languageCode")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"offers": []map[string]any{{
				"name": "accounts/acc/offers/offer1",
			}},
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudChannelService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ChannelOffersListCmd{ChannelAccount: "acc", Language: "de-DE"}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotLanguage != "de-DE" {
		t.Fatalf("unexpected language: %q", gotLanguage)
	}
}

func TestChannelOffersListCmd_WithFutureOffers(t *testing.T) {
	var gotFuture string

	svc, closeSrv := newCloudChannelServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/accounts/acc/offers") {
			http.NotFound(w, r)
			return
		}
		gotFuture = r.URL.Query().Get("showFutureOffers")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"offers": []map[string]any{{
				"name": "accounts/acc/offers/offer1",
			}},
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudChannelService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ChannelOffersListCmd{ChannelAccount: "acc", Future: true}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotFuture != "true" {
		t.Fatalf("unexpected showFutureOffers: %q", gotFuture)
	}
}

func TestChannelOffersListCmd_JSON(t *testing.T) {
	svc, closeSrv := newCloudChannelServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/accounts/acc/offers") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"offers": []map[string]any{{
				"name": "accounts/acc/offers/offer1",
				"sku": map[string]any{
					"name": "sku1",
				},
			}},
			"nextPageToken": "token456",
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudChannelService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ChannelOffersListCmd{ChannelAccount: "acc"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["nextPageToken"] != "token456" {
		t.Fatalf("unexpected nextPageToken: %v", parsed["nextPageToken"])
	}
}

// ChannelEntitlementsListCmd tests

func TestChannelEntitlementsListCmd(t *testing.T) {
	svc, closeSrv := newCloudChannelServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/accounts/acc/customers/cust1/entitlements") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"entitlements": []map[string]any{{
				"name":              "accounts/acc/customers/cust1/entitlements/e1",
				"offer":             "offers/o1",
				"provisioningState": "ACTIVE",
			}},
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudChannelService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ChannelEntitlementsListCmd{ChannelAccount: "acc", Customer: "cust1"}

	ctx := testContextJSON(t)
	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var parsed struct {
		Entitlements []struct {
			Name              string `json:"name"`
			Offer             string `json:"offer"`
			ProvisioningState string `json:"provisioningState"`
		} `json:"entitlements"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Entitlements) != 1 {
		t.Fatalf("expected 1 entitlement, got %d", len(parsed.Entitlements))
	}
	if parsed.Entitlements[0].Name != "accounts/acc/customers/cust1/entitlements/e1" {
		t.Fatalf("unexpected entitlement name: %s", parsed.Entitlements[0].Name)
	}
	if parsed.Entitlements[0].ProvisioningState != "ACTIVE" {
		t.Fatalf("unexpected provisioning state: %s", parsed.Entitlements[0].ProvisioningState)
	}
}

func TestChannelEntitlementsListCmd_EmptyResults(t *testing.T) {
	svc, closeSrv := newCloudChannelServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/accounts/acc/customers/cust1/entitlements") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"entitlements": []map[string]any{},
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudChannelService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ChannelEntitlementsListCmd{ChannelAccount: "acc", Customer: "cust1"}

	// Empty results should not error
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestChannelEntitlementsListCmd_MissingChannelAccount(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ChannelEntitlementsListCmd{ChannelAccount: "", Customer: "cust1"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing channel account")
	}
	if !strings.Contains(err.Error(), "--channel-account is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChannelEntitlementsListCmd_MissingCustomer(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ChannelEntitlementsListCmd{ChannelAccount: "acc", Customer: ""}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing customer")
	}
	if !strings.Contains(err.Error(), "--customer is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChannelEntitlementsListCmd_WithFullCustomerPath(t *testing.T) {
	var gotPath string

	svc, closeSrv := newCloudChannelServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/entitlements") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"entitlements": []map[string]any{{
				"name":              "accounts/acc/customers/cust1/entitlements/e1",
				"offer":             "offers/o1",
				"provisioningState": "ACTIVE",
			}},
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudChannelService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ChannelEntitlementsListCmd{ChannelAccount: "acc", Customer: "accounts/acc/customers/cust1"}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	// When customer already has full path, it should be used directly
	if !strings.Contains(gotPath, "accounts/acc/customers/cust1/entitlements") {
		t.Fatalf("unexpected path: %q", gotPath)
	}
}

func TestChannelEntitlementsListCmd_JSON(t *testing.T) {
	svc, closeSrv := newCloudChannelServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/accounts/acc/customers/cust1/entitlements") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"entitlements": []map[string]any{{
				"name":              "accounts/acc/customers/cust1/entitlements/e1",
				"offer":             "offers/o1",
				"provisioningState": "ACTIVE",
			}},
			"nextPageToken": "token789",
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudChannelService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ChannelEntitlementsListCmd{ChannelAccount: "acc", Customer: "cust1"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["nextPageToken"] != "token789" {
		t.Fatalf("unexpected nextPageToken: %v", parsed["nextPageToken"])
	}
}

func TestChannelEntitlementsListCmd_WithPaging(t *testing.T) {
	var gotPageSize, gotPageToken string

	svc, closeSrv := newCloudChannelServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/accounts/acc/customers/cust1/entitlements") {
			http.NotFound(w, r)
			return
		}
		gotPageSize = r.URL.Query().Get("pageSize")
		gotPageToken = r.URL.Query().Get("pageToken")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"entitlements": []map[string]any{{
				"name":              "accounts/acc/customers/cust1/entitlements/e1",
				"offer":             "offers/o1",
				"provisioningState": "ACTIVE",
			}},
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudChannelService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ChannelEntitlementsListCmd{ChannelAccount: "acc", Customer: "cust1", Max: 50, Page: "pagetoken"}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotPageSize != "50" {
		t.Fatalf("unexpected pageSize: %q", gotPageSize)
	}
	if gotPageToken != "pagetoken" {
		t.Fatalf("unexpected pageToken: %q", gotPageToken)
	}
}

// normalizeChannelAccount tests

func TestNormalizeChannelAccount(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"   ", ""},
		{"acc123", "accounts/acc123"},
		{"accounts/acc123", "accounts/acc123"},
		{"  acc123  ", "accounts/acc123"},
		{"  accounts/acc123  ", "accounts/acc123"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeChannelAccount(tt.input)
			if got != tt.want {
				t.Fatalf("normalizeChannelAccount(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
