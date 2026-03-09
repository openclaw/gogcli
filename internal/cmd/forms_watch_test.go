package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	formsapi "google.golang.org/api/forms/v1"
)

func TestFormsWatchCommands(t *testing.T) {
	origNew := newFormsService
	t.Cleanup(func() { newFormsService = origNew })

	var created formsapi.CreateWatchRequest
	deleteCalls := 0
	renewCalls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/v1/forms/form1/watches") && !strings.Contains(r.URL.Path, ":"):
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Fatalf("decode create watch: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "watch1",
				"eventType":  "RESPONSES",
				"state":      "ACTIVE",
				"expireTime": "2026-03-15T00:00:00Z",
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1/forms/form1/watches"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"watches": []map[string]any{
					{"id": "watch1", "eventType": "RESPONSES", "state": "ACTIVE", "expireTime": "2026-03-15T00:00:00Z"},
				},
			})
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/v1/forms/form1/watches/watch1"):
			deleteCalls++
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/v1/forms/form1/watches/watch1:renew"):
			renewCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "watch1",
				"expireTime": "2026-03-22T00:00:00Z",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	newFormsService = func(ctx context.Context, account string) (*formsapi.Service, error) {
		return newFormsTestService(t, ctx, srv), nil
	}

	ctx := newQuietUIContext(t)
	flags := &RootFlags{Account: "a@b.com"}

	if err := runKong(t, &FormsWatchCreateCmd{}, []string{"form1", "--topic", "projects/p/topics/t1"}, ctx, flags); err != nil {
		t.Fatalf("create watch: %v", err)
	}
	if created.Watch == nil || created.Watch.Target == nil || created.Watch.Target.Topic == nil {
		t.Fatalf("unexpected create watch request: %#v", created)
	}
	if created.Watch.Target.Topic.TopicName != "projects/p/topics/t1" {
		t.Fatalf("unexpected topic: %#v", created.Watch.Target.Topic)
	}

	if err := runKong(t, &FormsWatchListCmd{}, []string{"form1"}, ctx, flags); err != nil {
		t.Fatalf("list watches: %v", err)
	}

	if err := runKong(t, &FormsWatchRenewCmd{}, []string{"form1", "watch1"}, ctx, flags); err != nil {
		t.Fatalf("renew watch: %v", err)
	}
	if renewCalls != 1 {
		t.Fatalf("expected one renew call, got %d", renewCalls)
	}

	if err := runKong(t, &FormsWatchDeleteCmd{}, []string{"form1", "watch1"}, ctx, flags); err != nil {
		t.Fatalf("delete watch: %v", err)
	}
	if deleteCalls != 1 {
		t.Fatalf("expected one delete call, got %d", deleteCalls)
	}
}
