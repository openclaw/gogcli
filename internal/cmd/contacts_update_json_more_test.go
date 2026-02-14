package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/ui"
)

func TestContactsUpdate_FromFile_JSON_CanClearFields(t *testing.T) {
	var gotUpdateFields string
	var gotURLsPresent bool
	var gotURLsLen int
	var gotBioPresent bool

	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "people/c1") && r.Method == http.MethodGet && !strings.Contains(r.URL.Path, ":"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resourceName": "people/c1",
				"metadata": map[string]any{
					"sources": []map[string]any{
						{"type": "CONTACT", "etag": "etag-cur"},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, ":updateContact") && (r.Method == http.MethodPatch || r.Method == http.MethodPost):
			gotUpdateFields = r.URL.Query().Get("updatePersonFields")
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if urls, ok := body["urls"]; ok {
				gotURLsPresent = true
				if arr, ok := urls.([]any); ok {
					gotURLsLen = len(arr)
				}
			}
			if _, ok := body["biographies"]; ok {
				gotBioPresent = true
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceName": "people/c1"})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	withStdin(t, `{"resourceName":"people/c1","etag":"etag-cur","urls":[],"biographies":null}`, func() {
		if err := runKong(t, &ContactsUpdateCmd{}, []string{"people/c1", "--from-file", "-"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("runKong: %v", err)
		}
	})

	// Sorted mask.
	if gotUpdateFields != "biographies,urls" {
		t.Fatalf("unexpected updatePersonFields: %q", gotUpdateFields)
	}
	if !gotURLsPresent || gotURLsLen != 0 {
		t.Fatalf("expected urls present as empty list, present=%v len=%d", gotURLsPresent, gotURLsLen)
	}
	if !gotBioPresent {
		t.Fatalf("expected biographies present (clear)")
	}
}

func TestContactsUpdate_FromFile_JSON_UnsupportedFieldErrors(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	withStdin(t, `{"resourceName":"people/c1","photos":[]}`, func() {
		err := runKong(t, &ContactsUpdateCmd{}, []string{"people/c1", "--from-file", "-"}, context.Background(), &RootFlags{Account: "a@b.com"})
		if err == nil || !strings.Contains(err.Error(), "photos") {
			t.Fatalf("expected unsupported field error mentioning photos, got %v", err)
		}
	})
}

func TestContactsUpdate_FromFile_JSON_ETagMismatch(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "people/c1") && r.Method == http.MethodGet && !strings.Contains(r.URL.Path, ":"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resourceName": "people/c1",
				"metadata": map[string]any{
					"sources": []map[string]any{
						{"type": "CONTACT", "etag": "etag-cur"},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	withStdin(t, `{"resourceName":"people/c1","etag":"etag-old","urls":[{"value":"https://example.com"}]}`, func() {
		err := runKong(t, &ContactsUpdateCmd{}, []string{"people/c1", "--from-file", "-"}, context.Background(), &RootFlags{Account: "a@b.com"})
		if err == nil || !strings.Contains(err.Error(), "etag mismatch") {
			t.Fatalf("expected etag mismatch error, got %v", err)
		}
	})
}

func TestContactsUpdate_FromFile_JSON_IgnoreETagAllowsUpdate(t *testing.T) {
	var gotETag string

	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "people/c1") && r.Method == http.MethodGet && !strings.Contains(r.URL.Path, ":"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resourceName": "people/c1",
				"metadata": map[string]any{
					"sources": []map[string]any{
						{"type": "CONTACT", "etag": "etag-cur"},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, ":updateContact") && (r.Method == http.MethodPatch || r.Method == http.MethodPost):
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			gotETag, _ = body["etag"].(string)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceName": "people/c1"})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	withStdin(t, `{"resourceName":"people/c1","etag":"etag-old","urls":[{"value":"https://example.com"}]}`, func() {
		if err := runKong(t, &ContactsUpdateCmd{}, []string{"people/c1", "--from-file", "-", "--ignore-etag"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("runKong: %v", err)
		}
	})

	if gotETag != "etag-cur" {
		t.Fatalf("expected request to use current etag, got %q", gotETag)
	}
}

func TestContactsUpdate_FromFile_CantCombineWithFlags(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	tmp, err := os.CreateTemp(t.TempDir(), "contact-*.json")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, writeErr := tmp.WriteString(`{"resourceName":"people/c1","urls":[]}`); writeErr != nil {
		t.Fatalf("write temp: %v", writeErr)
	}
	_ = tmp.Close()

	err = runKong(t, &ContactsUpdateCmd{}, []string{"people/c1", "--from-file", tmp.Name(), "--email", "x@example.com"}, context.Background(), &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "can't combine --from-file") {
		t.Fatalf("expected combine error, got %v", err)
	}
}
