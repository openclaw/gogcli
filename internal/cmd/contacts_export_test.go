package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/people/v1"
)

func TestContactsExport_AllVCF_PaginatesAndIncludesCategories(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "people/me/connections") && r.Method == http.MethodGet:
			if got := r.URL.Query().Get("personFields"); !strings.Contains(got, "memberships") || !strings.Contains(got, "birthdays") {
				t.Fatalf("missing export fields: %q", got)
			}
			if r.URL.Query().Get("pageToken") == "" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"connections": []map[string]any{{
						"resourceName": "people/c1",
						"names":        []map[string]any{{"givenName": "Ada", "familyName": "Lovelace", "displayName": "Ada Lovelace"}},
						"emailAddresses": []map[string]any{
							{"value": "ada@example.com", "type": "work"},
						},
						"memberships": []map[string]any{{
							"contactGroupMembership": map[string]any{"contactGroupResourceName": "contactGroups/friends"},
						}},
					}},
					"nextPageToken": "next",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"connections": []map[string]any{{
					"resourceName": "people/c2",
					"names":        []map[string]any{{"givenName": "Grace", "familyName": "Hopper", "displayName": "Grace Hopper"}},
					"birthdays":    []map[string]any{{"date": map[string]any{"month": 12, "day": 9}}},
				}},
			})
		case strings.Contains(r.URL.Path, "contactGroups") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"contactGroups": []map[string]any{
					{"resourceName": "contactGroups/friends", "name": "Friends, Work", "groupType": "USER_CONTACT_GROUP"},
					{"resourceName": "contactGroups/myContacts", "name": "Contacts", "groupType": "SYSTEM_CONTACT_GROUP"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			err := Execute([]string{"--account", "a@b.com", "contacts", "export", "--all", "--page-size", "1", "--out", "-"})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	for _, want := range []string{
		"BEGIN:VCARD\r\nVERSION:4.0\r\n",
		"FN:Ada Lovelace\r\n",
		"N:Lovelace;Ada;;;\r\n",
		"EMAIL;TYPE=work:ada@example.com\r\n",
		"CATEGORIES:Friends\\, Work\r\n",
		"FN:Grace Hopper\r\n",
		"BDAY:--1209\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in VCF:\n%s", want, out)
		}
	}
	if got := strings.Count(out, "BEGIN:VCARD"); got != 2 {
		t.Fatalf("expected 2 cards, got %d:\n%s", got, out)
	}
}

func TestContactsExport_SelectorEmailExactMatch(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "people:searchContacts") || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("query") == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"person": map[string]any{
					"resourceName":   "people/c1",
					"names":          []map[string]any{{"displayName": "Wrong Ada"}},
					"emailAddresses": []map[string]any{{"value": "wrong@example.com"}},
					"phoneNumbers":   []map[string]any{{"value": "+1"}},
				}},
				{"person": map[string]any{
					"resourceName": "people/c2",
					"names":        []map[string]any{{"displayName": "Right Ada"}},
					"emailAddresses": []map[string]any{
						{"value": "other@example.com"},
						{"value": "ada@example.com"},
					},
				}},
			},
		})
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			err := Execute([]string{"--account", "a@b.com", "contacts", "export", "ada@example.com", "--out", "-"})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "FN:Right Ada\r\n") || strings.Contains(out, "Wrong Ada") {
		t.Fatalf("unexpected VCF:\n%s", out)
	}
}

func TestContactsExport_AmbiguousSelectorErrors(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "people:searchContacts") || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("query") == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"person": map[string]any{"resourceName": "people/c1", "names": []map[string]any{{"displayName": "Ada One"}}}},
				{"person": map[string]any{"resourceName": "people/c2", "names": []map[string]any{{"displayName": "Ada Two"}}}},
			},
		})
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	err := runKong(t, &ContactsExportCmd{}, []string{"Ada"}, context.Background(), &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "ambiguous contact selector") {
		t.Fatalf("expected ambiguous selector error, got %v", err)
	}
}

func TestWriteContactVCard_EscapesStructuredFieldsAndFolds(t *testing.T) {
	var b strings.Builder
	p := &people.Person{
		ResourceName: "people/c1",
		Names:        []*people.Name{{GivenName: "Ada", FamilyName: "Love;lace", DisplayName: "Ada Love;lace"}},
		Nicknames:    []*people.Nickname{{Value: "Countess, friend"}},
		PhoneNumbers: []*people.PhoneNumber{{Value: "+1 555", Type: "mobile"}},
		Addresses: []*people.Address{{
			Type:            "home",
			PoBox:           "Box 1",
			ExtendedAddress: "Suite 2",
			StreetAddress:   "1 Main; Street",
			City:            "London",
			Region:          "London",
			PostalCode:      "SW1A",
			Country:         "UK",
		}},
		Biographies: []*people.Biography{{Value: strings.Repeat("long note ", 12)}},
	}
	if err := writeContactVCard(&b, p, nil); err != nil {
		t.Fatalf("writeContactVCard: %v", err)
	}
	out := b.String()
	for _, want := range []string{
		"VERSION:4.0\r\n",
		"FN:Ada Love\\;lace\r\n",
		"N:Love\\;lace;Ada;;;\r\n",
		"NICKNAME:Countess\\, friend\r\n",
		"TEL;TYPE=cell:+1 555\r\n",
		"ADR;TYPE=home:Box 1;Suite 2;1 Main\\; Street;London;London;SW1A;UK\r\n",
		"\r\n ",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in VCF:\n%s", want, out)
		}
	}
}
