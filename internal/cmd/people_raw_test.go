package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

func withMockPeopleContactsService(t *testing.T, ctx context.Context, srv *httptest.Server) context.Context {
	t.Helper()
	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", people.NewService)
	return withPeopleContactsTestService(ctx, svc)
}

func fullPersonResponse() map[string]any {
	return map[string]any{
		"resourceName": "people/c1",
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
	srv := newPeopleRawTestServer(t, 0, fullPersonResponse())
	defer srv.Close()

	ctx := withMockPeopleContactsService(t, rawTestContext(t), srv)
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
	srv := newPeopleRawTestServer(t, 0, fullPersonResponse())
	defer srv.Close()

	ctx := withMockPeopleContactsService(t, rawTestContext(t), srv)
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

func TestContactsRaw_EmailResolvesContactResource(t *testing.T) {
	var gotGet bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/people/me/connections") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"connections": []map[string]any{
					{
						"resourceName":   "people/c1",
						"emailAddresses": []map[string]any{{"value": "ada@example.com"}},
					},
				},
			})
		case strings.Contains(r.URL.Path, "/people/c1") && r.Method == http.MethodGet:
			gotGet = true
			_ = json.NewEncoder(w).Encode(fullPersonResponse())
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx := withMockPeopleContactsService(t, rawTestContext(t), srv)
	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		if err := runKong(t, &ContactsRawCmd{}, []string{"ada@example.com"}, ctx, flags); err != nil {
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
	if !gotGet {
		t.Fatalf("expected People.Get for resolved contact resource")
	}
}

func TestContactsRaw_EmailAmbiguousContactsFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/people/me/connections") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"connections": []map[string]any{
					{
						"resourceName":   "people/c1",
						"emailAddresses": []map[string]any{{"value": "ada@example.com"}},
					},
					{
						"resourceName":   "people/c2",
						"emailAddresses": []map[string]any{{"value": "ada@example.com"}},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx := withMockPeopleContactsService(t, rawTestContext(t), srv)
	flags := &RootFlags{Account: "a@b.com"}
	_ = captureStdout(t, func() {
		if err := runKong(t, &ContactsRawCmd{}, []string{"ada@example.com"}, ctx, flags); err != nil {
			if !strings.Contains(err.Error(), "matched multiple contacts") {
				t.Fatalf("unexpected error: %v", err)
			}
			return
		}
		t.Fatalf("expected ambiguous contact error")
	})
}

func TestPeopleRaw_EmailResolveRejectsRepeatedPageToken(t *testing.T) {
	var listCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/people/me/connections") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		if listCalls.Add(1) > 2 {
			http.Error(w, "unexpected extra page request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"connections": []map[string]any{
				{
					"resourceName":   "people/c1",
					"emailAddresses": []map[string]any{{"value": "ada@example.com"}},
				},
			},
			"nextPageToken": "stuck",
		})
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(rawTestContext(t), 10*time.Second)
	defer cancel()
	ctx = withMockPeopleContactsService(t, ctx, srv)
	flags := &RootFlags{Account: "a@b.com"}
	var err error
	out := captureStdout(t, func() {
		err = runKong(t, &PeopleRawCmd{}, []string{"ada@example.com"}, ctx, flags)
	})
	if err == nil || !strings.Contains(err.Error(), "repeated page token") {
		t.Fatalf("err = %v after %d list calls", err, listCalls.Load())
	}
	if got := listCalls.Load(); got != 2 {
		t.Fatalf("list calls = %d, want 2", got)
	}
	if out != "" {
		t.Fatalf("unexpected partial output: %q", out)
	}
	t.Logf("err = %v after %d list calls", err, listCalls.Load())
}

func TestPeopleRaw_EmailResolveAcrossPages(t *testing.T) {
	match := &people.Person{
		ResourceName:   "people/c1",
		EmailAddresses: []*people.EmailAddress{{Value: " ada@EXAMPLE.com "}},
	}
	otherMatch := &people.Person{
		ResourceName:   "people/c2",
		EmailAddresses: match.EmailAddresses,
	}
	tests := []struct {
		name         string
		pages        []people.ListConnectionsResponse
		errorOnPage  int
		wantErr      string
		wantGetCalls int32
	}{
		{
			name: "nonmatching page then duplicate matching resource",
			pages: []people.ListConnectionsResponse{
				{
					Connections: []*people.Person{
						nil,
						{EmailAddresses: match.EmailAddresses},
						{ResourceName: "people/c1", EmailAddresses: []*people.EmailAddress{{Value: "other@example.com"}}},
					},
					NextPageToken: "page-2",
				},
				{Connections: []*people.Person{match}, NextPageToken: "page-3"},
				{Connections: []*people.Person{match}},
			},
			wantGetCalls: 1,
		},
		{
			name: "distinct matching resources are ambiguous",
			pages: []people.ListConnectionsResponse{
				{Connections: []*people.Person{match}, NextPageToken: "page-2"},
				{Connections: []*people.Person{otherMatch}},
			},
			wantErr: "matched multiple contacts",
		},
		{
			name: "later page error after a match",
			pages: []people.ListConnectionsResponse{
				{Connections: []*people.Person{match}, NextPageToken: "page-2"},
				{},
			},
			errorOnPage: 2,
			wantErr:     "later page failed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var listCalls, getCalls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.Contains(r.URL.Path, "/people/me/connections") && r.Method == http.MethodGet:
					pageIndex := int(listCalls.Add(1)) - 1
					if pageIndex >= len(tc.pages) {
						http.Error(w, "unexpected extra page request", http.StatusBadRequest)
						return
					}
					wantToken := ""
					if pageIndex > 0 {
						wantToken = tc.pages[pageIndex-1].NextPageToken
					}
					if r.URL.Query().Get("pageToken") != wantToken {
						http.Error(w, "unexpected page token", http.StatusBadRequest)
						return
					}
					if tc.errorOnPage == pageIndex+1 {
						http.Error(w, "later page failed", http.StatusBadRequest)
						return
					}
					_ = json.NewEncoder(w).Encode(tc.pages[pageIndex])
				case strings.Contains(r.URL.Path, "/people/c1") && r.Method == http.MethodGet:
					getCalls.Add(1)
					_ = json.NewEncoder(w).Encode(fullPersonResponse())
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			ctx, cancel := context.WithTimeout(rawTestContext(t), 10*time.Second)
			defer cancel()
			ctx = withMockPeopleContactsService(t, ctx, srv)
			var runErr error
			out := captureStdout(t, func() {
				runErr = runKong(t, &PeopleRawCmd{}, []string{"ada@example.com"}, ctx, &RootFlags{Account: "a@b.com"})
			})
			if got := listCalls.Load(); got != int32(len(tc.pages)) {
				t.Errorf("list calls = %d, want %d", got, len(tc.pages))
			}
			if got := getCalls.Load(); got != tc.wantGetCalls {
				t.Errorf("People.Get calls = %d, want %d", got, tc.wantGetCalls)
			}
			if tc.wantErr != "" {
				if runErr == nil || !strings.Contains(runErr.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", runErr, tc.wantErr)
				}
				if out != "" {
					t.Fatalf("unexpected partial output: %q", out)
				}
				return
			}
			if runErr != nil {
				t.Fatalf("run: %v", runErr)
			}
			var person people.Person
			if err := json.Unmarshal([]byte(out), &person); err != nil {
				t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
			}
			if person.ResourceName != "people/c1" {
				t.Fatalf("resource = %q, want people/c1", person.ResourceName)
			}
		})
	}
}

func TestPeopleRaw_APIError(t *testing.T) {
	srv := newPeopleRawTestServer(t, http.StatusInternalServerError, nil)
	defer srv.Close()

	ctx := withMockPeopleContactsService(t, rawTestContext(t), srv)
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
