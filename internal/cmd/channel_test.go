package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/cloudchannel/v1"
	"google.golang.org/api/option"
)

func newCloudChannelServiceStub(t *testing.T, handler http.HandlerFunc) (*cloudchannel.Service, func()) {
	t.Helper()

	srv := httptest.NewServer(handler)
	svc, err := cloudchannel.NewService(context.Background(),
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

func stubCloudChannelService(t *testing.T, svc *cloudchannel.Service) {
	t.Helper()
	orig := newCloudChannelService
	t.Cleanup(func() { newCloudChannelService = orig })
	newCloudChannelService = func(context.Context, string) (*cloudchannel.Service, error) { return svc, nil }
}

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

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "entitlements/e1") {
		t.Fatalf("unexpected output: %s", out)
	}
}
