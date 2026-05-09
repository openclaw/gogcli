package cmd

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/api/people/v1"
)

func TestParseContactsDedupeMatch(t *testing.T) {
	got, err := parseContactsDedupeMatch("email, phone")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !got.Email || !got.Phone || got.Name {
		t.Fatalf("unexpected match: %#v", got)
	}
	if _, err := parseContactsDedupeMatch("email,bogus"); err == nil {
		t.Fatalf("expected invalid field error")
	}
}

func TestBuildContactsDedupeGroupsTransitive(t *testing.T) {
	contacts := []*people.Person{
		testDedupePerson("people/1", "Ada One", []string{"ada@example.com"}, nil),
		testDedupePerson("people/2", "Ada Two", []string{"ADA@example.com"}, []string{"+1 (555) 0100"}),
		testDedupePerson("people/3", "Ada Three", nil, []string{"15550100"}),
		testDedupePerson("people/4", "Grace", []string{"grace@example.com"}, nil),
	}
	groups := buildContactsDedupeGroups(contacts, contactsDedupeMatch{Email: true, Phone: true})
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1: %#v", len(groups), groups)
	}
	if got := len(groups[0].Members); got != 3 {
		t.Fatalf("members = %d, want 3", got)
	}
	if !reflect.DeepEqual(groups[0].MatchedOn, []string{"email:ada@example.com", "phone:15550100"}) {
		t.Fatalf("matched_on = %#v", groups[0].MatchedOn)
	}
	if groups[0].Primary.ResourceName != "people/2" {
		t.Fatalf("primary = %s, want people/2", groups[0].Primary.ResourceName)
	}
}

func TestBuildContactsDedupeGroupsNameOptIn(t *testing.T) {
	contacts := []*people.Person{
		testDedupePerson("people/1", "Ada Lovelace", nil, nil),
		testDedupePerson("people/2", " ada   lovelace ", nil, nil),
	}
	if groups := buildContactsDedupeGroups(contacts, contactsDedupeMatch{Email: true, Phone: true}); len(groups) != 0 {
		t.Fatalf("default match should ignore name-only duplicates: %#v", groups)
	}
	if groups := buildContactsDedupeGroups(contacts, contactsDedupeMatch{Name: true}); len(groups) != 1 {
		t.Fatalf("name match should find one group, got %d", len(groups))
	}
}

func TestContactsDedupeExecuteJSON(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/people/me/connections" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("personFields"); !strings.Contains(got, "emailAddresses") {
			t.Fatalf("missing personFields: %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"connections": []map[string]any{
				{
					"resourceName":   "people/1",
					"names":          []map[string]any{{"displayName": "Ada One"}},
					"emailAddresses": []map[string]any{{"value": "ada@example.com"}},
				},
				{
					"resourceName":   "people/2",
					"names":          []map[string]any{{"displayName": "Ada Two"}},
					"emailAddresses": []map[string]any{{"value": "ADA@example.com"}},
				},
			},
		})
	}))
	defer closeSrv()
	stubPeopleServices(t, svc)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@example.com", "contacts", "dedupe"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	var parsed struct {
		Scanned int `json:"scanned"`
		Groups  []struct {
			MatchedOn []string `json:"matched_on"`
			Members   []struct {
				Resource string `json:"resource"`
			} `json:"members"`
		} `json:"groups"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\n%s", err, out)
	}
	if parsed.Scanned != 2 || len(parsed.Groups) != 1 || len(parsed.Groups[0].Members) != 2 {
		t.Fatalf("unexpected payload: %#v", parsed)
	}
	if !reflect.DeepEqual(parsed.Groups[0].MatchedOn, []string{"email:ada@example.com"}) {
		t.Fatalf("matched_on = %#v", parsed.Groups[0].MatchedOn)
	}
}

func testDedupePerson(resource, name string, emails, phones []string) *people.Person {
	p := &people.Person{ResourceName: resource}
	if name != "" {
		p.Names = []*people.Name{{DisplayName: name}}
	}
	for _, email := range emails {
		p.EmailAddresses = append(p.EmailAddresses, &people.EmailAddress{Value: email})
	}
	for _, phone := range phones {
		p.PhoneNumbers = append(p.PhoneNumbers, &people.PhoneNumber{Value: phone})
	}
	return p
}
