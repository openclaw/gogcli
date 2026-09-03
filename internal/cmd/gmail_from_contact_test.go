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

func TestBuildGmailFromEmailsQuery(t *testing.T) {
	if got := buildGmailFromEmailsQuery([]string{"a@example.com"}); got != "from:a@example.com" {
		t.Fatalf("single = %q", got)
	}
	if got := buildGmailFromEmailsQuery([]string{"a@example.com", "b@example.com"}); got != "from:(a@example.com OR b@example.com)" {
		t.Fatalf("multi = %q", got)
	}
}

func TestSelectGmailFromContactPeoplePrefersExactMatch(t *testing.T) {
	resp := &people.SearchResponse{Results: []*people.SearchResult{
		{Person: &people.Person{Names: []*people.Name{{DisplayName: "Alice A"}}, EmailAddresses: []*people.EmailAddress{{Value: "alice@example.com"}}}},
		{Person: &people.Person{Names: []*people.Name{{DisplayName: "Alice B"}}, EmailAddresses: []*people.EmailAddress{{Value: "b@example.com"}}}},
	}}
	got := selectGmailFromContactPeople("alice@example.com", resp)
	if len(got) != 1 || primaryName(got[0]) != "Alice A" {
		t.Fatalf("unexpected selection: %#v", got)
	}
}

func TestAllContactEmailsDedupes(t *testing.T) {
	got := allContactEmails(&people.Person{EmailAddresses: []*people.EmailAddress{
		{Value: "A@example.com"},
		{Value: "a@example.com"},
		{Value: "b@example.com"},
	}})
	if len(got) != 2 || got[0] != "A@example.com" || got[1] != "b@example.com" {
		t.Fatalf("emails = %#v", got)
	}
}

func TestGmailFromContactQuery_WarmsContactsSearchCache(t *testing.T) {
	var queries []string
	svc := newPeopleSearchTestService(t, "people:searchContacts", "people/c1", "Alice", "alice@example.com", &queries)

	got, err := gmailFromContactQuery(withPeopleContactsTestService(context.Background(), svc), "a@b.com", "Alice")
	if err != nil {
		t.Fatalf("gmailFromContactQuery: %v", err)
	}
	if got != "from:alice@example.com" {
		t.Fatalf("query = %q", got)
	}
	if got, want := strings.Join(queries, ","), ",Alice"; got != want {
		t.Fatalf("search queries = %q, want %q", got, want)
	}
}

func TestGmailFromContactFallbackRejectsRepeatedPageToken(t *testing.T) {
	var listCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "people:searchContacts") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{}})
		case strings.Contains(r.URL.Path, "/people/me/connections") && r.Method == http.MethodGet:
			if listCalls.Add(1) > 2 {
				http.Error(w, "too many fallback list requests", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"connections": []map[string]any{
					{
						"resourceName":   "people/c1",
						"names":          []map[string]any{{"displayName": "Ada"}},
						"emailAddresses": []map[string]any{{"value": "ada@example.com"}},
					},
				},
				"nextPageToken": "stuck",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc := newPeopleServiceFromServer(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := gmailFromContactQuery(withPeopleContactsTestService(ctx, svc), "a@b.com", "Ada")
	if err == nil || !strings.Contains(err.Error(), "repeated page token") {
		t.Fatalf("err = %v after %d list calls", err, listCalls.Load())
	}
	if got := listCalls.Load(); got != 2 {
		t.Fatalf("list calls = %d, want 2", got)
	}
}

func TestGmailFromContactFallbackMatchesAcrossPages(t *testing.T) {
	pages := map[string]*people.ListConnectionsResponse{
		"": {
			Connections: []*people.Person{
				nil,
				{
					Names:          []*people.Name{{DisplayName: "Ada Lovelace"}},
					EmailAddresses: []*people.EmailAddress{{Value: "ada.lovelace@example.com"}},
				},
				{
					Names:          []*people.Name{{DisplayName: "Grace"}},
					EmailAddresses: []*people.EmailAddress{{Value: "grace@example.com"}},
				},
			},
			NextPageToken: "next",
		},
		"next": {
			Connections: []*people.Person{
				{
					Names: []*people.Name{{DisplayName: "Ada"}},
					EmailAddresses: []*people.EmailAddress{
						{Value: "ada@example.com"},
						{Value: " ada.work@example.com "},
					},
				},
				{
					Names:          []*people.Name{{DisplayName: "GRACE"}},
					EmailAddresses: []*people.EmailAddress{{Value: "grace.work@example.com"}},
				},
			},
		},
	}
	cases := []struct {
		name      string
		selector  string
		wantQuery string
		wantErr   string
	}{
		{
			name:      "exact name on later page",
			selector:  " aDa ",
			wantQuery: "from:(ada@example.com OR ada.work@example.com)",
		},
		{
			name:      "secondary email on later page",
			selector:  " ADA.WORK@EXAMPLE.COM ",
			wantQuery: "from:(ada@example.com OR ada.work@example.com)",
		},
		{
			name:     "ambiguity across pages",
			selector: "Grace",
			wantErr:  "matched multiple contacts (Grace, GRACE)",
		},
		{
			name:     "no exact match",
			selector: "Ad",
			wantErr:  "no contact found",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var listCalls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.Contains(r.URL.Path, "people:searchContacts") && r.Method == http.MethodGet:
					_ = json.NewEncoder(w).Encode(&people.SearchResponse{})
				case strings.Contains(r.URL.Path, "/people/me/connections") && r.Method == http.MethodGet:
					if listCalls.Add(1) > 2 {
						http.Error(w, "too many fallback list requests", http.StatusBadRequest)
						return
					}
					page, ok := pages[r.URL.Query().Get("pageToken")]
					if !ok {
						http.Error(w, "unexpected fallback page token", http.StatusBadRequest)
						return
					}
					_ = json.NewEncoder(w).Encode(page)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			svc := newPeopleServiceFromServer(t, srv)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			got, err := gmailFromContactQuery(withPeopleContactsTestService(ctx, svc), "a@b.com", tc.selector)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
			} else if err != nil || got != tc.wantQuery {
				t.Fatalf("query = %q, err = %v, want %q", got, err, tc.wantQuery)
			}
			if got := listCalls.Load(); got != 2 {
				t.Fatalf("list calls = %d, want 2", got)
			}
		})
	}
}
