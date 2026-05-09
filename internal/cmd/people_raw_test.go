package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/people/v1"
)

func newPeopleRawTestServer(t *testing.T, status int, body map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/people/") || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if status != 0 {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": status, "message": "mock error"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func installMockPeopleContactsService(t *testing.T, srv *httptest.Server) {
	t.Helper()
	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", people.NewService)
	stubGoogleTestService(t, &newPeopleContactsService, svc)
}

func fullPersonResponse(id string) map[string]any {
	return map[string]any{
		"resourceName": id,
		"etag":         "abc",
		"names": []map[string]any{
			{"displayName": "Ada Lovelace", "givenName": "Ada", "familyName": "Lovelace"},
		},
		"emailAddresses": []map[string]any{
			{"value": "ada@example.com"},
		},
	}
}

func TestPeopleRaw_HappyPath(t *testing.T) {
	srv := newPeopleRawTestServer(t, 0, fullPersonResponse("people/c1"))
	defer srv.Close()
	installMockPeopleContactsService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		if err := runKong(t, &PeopleRawCmd{}, []string{"people/c1"}, ctx, flags); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	if got["resourceName"] != "people/c1" {
		t.Fatalf("expected resourceName=people/c1, got: %v", got["resourceName"])
	}
	if _, ok := got["names"]; !ok {
		t.Fatalf("expected names in raw output")
	}
}

func TestContactsRaw_HappyPath(t *testing.T) {
	srv := newPeopleRawTestServer(t, 0, fullPersonResponse("people/c1"))
	defer srv.Close()
	installMockPeopleContactsService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		if err := runKong(t, &ContactsRawCmd{}, []string{"people/c1"}, ctx, flags); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	if got["resourceName"] != "people/c1" {
		t.Fatalf("expected resourceName=people/c1, got: %v", got["resourceName"])
	}
}

func TestPeopleRaw_APIError(t *testing.T) {
	srv := newPeopleRawTestServer(t, http.StatusInternalServerError, nil)
	defer srv.Close()
	installMockPeopleContactsService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	_ = captureStdout(t, func() {
		if err := runKong(t, &PeopleRawCmd{}, []string{"people/c1"}, ctx, flags); err == nil {
			t.Fatalf("expected error on 500")
		}
	})
}

func TestPeopleRaw_EmptyID(t *testing.T) {
	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	if err := (&PeopleRawCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected error on empty id")
	}
}

func TestContactsRaw_EmptyID(t *testing.T) {
	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	if err := (&ContactsRawCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected error on empty id")
	}
}
