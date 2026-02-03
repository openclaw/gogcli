package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	datatransfer "google.golang.org/api/admin/datatransfer/v1"
	"google.golang.org/api/option"
)

func TestTransferListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/admin/datatransfer/v1/transfers") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"dataTransfers": []map[string]any{
				{"id": "t1", "oldOwnerUserId": "old@example.com", "newOwnerUserId": "new@example.com", "overallTransferStatusCode": "completed"},
			},
		})
	})
	stubDataTransfer(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &TransferListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "t1") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestTransferCreateCmd(t *testing.T) {
	var gotOld string
	var gotApp string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/admin/datatransfer/v1/transfers") {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			OldOwnerUserId           string `json:"oldOwnerUserId"`
			ApplicationDataTransfers []struct {
				ApplicationId string `json:"applicationId"`
			} `json:"applicationDataTransfers"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotOld = payload.OldOwnerUserId
		if len(payload.ApplicationDataTransfers) > 0 {
			gotApp = payload.ApplicationDataTransfers[0].ApplicationId
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "t2",
		})
	})
	stubDataTransfer(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &TransferCreateCmd{OldOwner: "old@example.com", NewOwner: "new@example.com", Application: "4350700"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotOld != "old@example.com" {
		t.Fatalf("unexpected old owner: %q", gotOld)
	}
	if gotApp != "4350700" {
		t.Fatalf("unexpected app id: %s", gotApp)
	}
	if !strings.Contains(out, "Created transfer") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func stubDataTransfer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	orig := newDataTransferService
	svc, err := datatransfer.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new datatransfer service: %v", err)
	}
	newDataTransferService = func(context.Context, string) (*datatransfer.Service, error) { return svc, nil }
	t.Cleanup(func() {
		newDataTransferService = orig
		srv.Close()
	})
	return srv
}
