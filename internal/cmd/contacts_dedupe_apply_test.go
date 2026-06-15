package cmd

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"

	"google.golang.org/api/people/v1"
)

func TestBuildContactsDedupeApplyPlanMergesFields(t *testing.T) {
	primary := testDedupeApplyPerson("people/1", "etag-1", "Ada", "ada@example.com", "+1 555 0100")
	primary.Memberships = []*people.Membership{{
		ContactGroupMembership: &people.ContactGroupMembership{ContactGroupResourceName: "contactGroups/friends"},
	}}
	duplicate := testDedupeApplyPerson("people/2", "etag-2", "Ada", "ADA@example.com", "+1 555 0200")
	duplicate.Memberships = []*people.Membership{{
		ContactGroupMembership: &people.ContactGroupMembership{ContactGroupResourceName: "contactGroups/work"},
	}}

	group := buildContactsDedupeGroups(
		[]*people.Person{primary, duplicate},
		contactsDedupeMatch{Email: true, Phone: true},
	)[0]
	plan, err := buildContactsDedupeApplyPlan(group)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if contactsDedupeResource(plan.Merged) != "people/1" {
		t.Fatalf("primary = %q, want people/1", contactsDedupeResource(plan.Merged))
	}
	if got := phoneValues(plan.Merged.PhoneNumbers); !reflect.DeepEqual(got, []string{"+1 555 0100", "+1 555 0200"}) {
		t.Fatalf("phones = %#v", got)
	}
	if len(plan.Merged.EmailAddresses) != 1 {
		t.Fatalf("emails = %#v, want one normalized address", plan.Merged.EmailAddresses)
	}
	if len(plan.Merged.Memberships) != 2 {
		t.Fatalf("memberships = %#v", plan.Merged.Memberships)
	}
	if len(plan.Delete) != 1 || contactsDedupeResource(plan.Delete[0]) != "people/2" {
		t.Fatalf("delete = %#v", plan.Delete)
	}
	for _, email := range plan.Merged.EmailAddresses {
		if email.Metadata != nil {
			t.Fatalf("merged email retained source metadata: %#v", email.Metadata)
		}
	}
}

func TestBuildContactsDedupeApplyPlanRejectsUnsafeFields(t *testing.T) {
	t.Run("conflicting singleton", func(t *testing.T) {
		primary := testDedupeApplyPerson("people/1", "etag-1", "Ada One", "ada@example.com", "")
		duplicate := testDedupeApplyPerson("people/2", "etag-2", "Ada Two", "ADA@example.com", "")
		group := contactsDedupeGroup{
			Primary: primary,
			Members: []*people.Person{primary, duplicate},
		}
		_, err := buildContactsDedupeApplyPlan(group)
		if err == nil || !strings.Contains(err.Error(), "conflicting names") {
			t.Fatalf("error = %v, want conflicting names", err)
		}
	})

	t.Run("secondary photo", func(t *testing.T) {
		primary := testDedupeApplyPerson("people/1", "etag-1", "Ada", "ada@example.com", "")
		duplicate := testDedupeApplyPerson("people/2", "etag-2", "Ada", "ADA@example.com", "")
		duplicate.Photos = []*people.Photo{{Url: "https://example.com/photo.jpg"}}
		group := contactsDedupeGroup{
			Primary: primary,
			Members: []*people.Person{primary, duplicate},
		}
		_, err := buildContactsDedupeApplyPlan(group)
		if err == nil || !strings.Contains(err.Error(), "photo") {
			t.Fatalf("error = %v, want photo refusal", err)
		}
	})
}

func TestContactsDedupeApplyExecuteJSON(t *testing.T) {
	var mu sync.Mutex
	var updateBody people.Person
	var updateMask string
	var deleted []string

	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/people/me/connections":
			if got := r.URL.Query()["sources"]; !reflect.DeepEqual(got, []string{contactsDedupeContactSource}) {
				t.Fatalf("connections sources = %#v", got)
			}
			writeDedupeConnections(t, w)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/people/1":
			writeDedupePerson(t, w, testDedupeApplyPerson("people/1", "etag-1", "Ada", "ada@example.com", "+1 555 0100"))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/people/2" && r.URL.Query().Get("personFields") != "metadata":
			writeDedupePerson(t, w, testDedupeApplyPerson("people/2", "etag-2", "Ada", "ADA@example.com", "+1 555 0200"))
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/people/1:updateContact":
			updateMask = r.URL.Query().Get("updatePersonFields")
			if err := json.NewDecoder(r.Body).Decode(&updateBody); err != nil {
				t.Fatalf("decode update: %v", err)
			}
			writeDedupePerson(t, w, &people.Person{ResourceName: "people/1", Metadata: contactMetadata("etag-updated")})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/people/2":
			writeDedupePerson(t, w, &people.Person{ResourceName: "people/2", Metadata: contactMetadata("etag-2")})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/people/2:deleteContact":
			mu.Lock()
			deleted = append(deleted, "people/2")
			mu.Unlock()
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer closeSrv()

	result := executeWithPeopleTestServices(
		t,
		[]string{"--json", "--account", "a@example.com", "--force", "contacts", "dedupe", "--apply"},
		peopleTestServices{Contacts: fixedPeopleTestService(svc)},
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	var payload struct {
		Applied         bool `json:"applied"`
		GroupsMerged    int  `json:"groups_merged"`
		ContactsDeleted int  `json:"contacts_deleted"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("decode output: %v\n%s", err, result.stdout)
	}
	if !payload.Applied || payload.GroupsMerged != 1 || payload.ContactsDeleted != 1 {
		t.Fatalf("output = %#v", payload)
	}
	if !strings.Contains(updateMask, "emailAddresses") || !strings.Contains(updateMask, "phoneNumbers") {
		t.Fatalf("update mask = %q", updateMask)
	}
	if got := phoneValues(updateBody.PhoneNumbers); !reflect.DeepEqual(got, []string{"+1 555 0100", "+1 555 0200"}) {
		t.Fatalf("updated phones = %#v", got)
	}
	if contactSourceETag(&updateBody) != "etag-1" {
		t.Fatalf("update etag = %q", contactSourceETag(&updateBody))
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(deleted, []string{"people/2"}) {
		t.Fatalf("deleted = %#v", deleted)
	}
}

func TestContactsDedupeApplyDryRunSkipsMutations(t *testing.T) {
	mutated := false
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/people/me/connections":
			writeDedupeConnections(t, w)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/people/1":
			writeDedupePerson(t, w, testDedupeApplyPerson("people/1", "etag-1", "Ada", "ada@example.com", "+1 555 0100"))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/people/2":
			writeDedupePerson(t, w, testDedupeApplyPerson("people/2", "etag-2", "Ada", "ADA@example.com", "+1 555 0200"))
		default:
			mutated = true
			http.NotFound(w, r)
		}
	}))
	defer closeSrv()

	result := executeWithPeopleTestServices(
		t,
		[]string{"--json", "--account", "a@example.com", "--dry-run", "contacts", "dedupe", "--apply"},
		peopleTestServices{Contacts: fixedPeopleTestService(svc)},
	)
	if ExitCode(result.err) != 0 {
		t.Fatalf("Execute: %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	var payload struct {
		DryRun  bool   `json:"dry_run"`
		Op      string `json:"op"`
		Request struct {
			GroupsMerged    int `json:"groups_merged"`
			ContactsDeleted int `json:"contacts_deleted"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("decode output: %v\n%s", err, result.stdout)
	}
	if !payload.DryRun || payload.Op != "contacts.dedupe.apply" ||
		payload.Request.GroupsMerged != 1 || payload.Request.ContactsDeleted != 1 {
		t.Fatalf("output = %#v", payload)
	}
	if mutated {
		t.Fatal("dry-run sent a mutation request")
	}
}

func TestContactsDedupeApplyRetainsChangedContact(t *testing.T) {
	deleted := false
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/people/me/connections":
			writeDedupeConnections(t, w)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/people/1":
			writeDedupePerson(t, w, testDedupeApplyPerson("people/1", "etag-1", "Ada", "ada@example.com", "+1 555 0100"))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/people/2" && r.URL.Query().Get("personFields") != "metadata":
			writeDedupePerson(t, w, testDedupeApplyPerson("people/2", "etag-2", "Ada", "ADA@example.com", "+1 555 0200"))
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/people/1:updateContact":
			writeDedupePerson(t, w, &people.Person{ResourceName: "people/1", Metadata: contactMetadata("etag-updated")})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/people/2":
			writeDedupePerson(t, w, &people.Person{ResourceName: "people/2", Metadata: contactMetadata("etag-changed")})
		case r.Method == http.MethodDelete:
			deleted = true
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer closeSrv()

	result := executeWithPeopleTestServices(
		t,
		[]string{"--account", "a@example.com", "--force", "contacts", "dedupe", "--apply"},
		peopleTestServices{Contacts: fixedPeopleTestService(svc)},
	)
	if result.err == nil || !strings.Contains(result.err.Error(), "changed after preview and was not deleted") {
		t.Fatalf("error = %v", result.err)
	}
	if deleted {
		t.Fatal("changed contact was deleted")
	}
}

func testDedupeApplyPerson(resource, etag, name, email, phone string) *people.Person {
	person := &people.Person{
		ResourceName: resource,
		Metadata:     contactMetadata(etag),
		Memberships: []*people.Membership{{
			ContactGroupMembership: &people.ContactGroupMembership{ContactGroupResourceName: "contactGroups/myContacts"},
		}},
	}
	if name != "" {
		person.Names = []*people.Name{{GivenName: name, DisplayName: name}}
	}
	if email != "" {
		person.EmailAddresses = []*people.EmailAddress{{
			Value:    email,
			Metadata: &people.FieldMetadata{Source: &people.Source{Type: "CONTACT", Id: resource}},
		}}
	}
	if phone != "" {
		person.PhoneNumbers = []*people.PhoneNumber{{
			Value:    phone,
			Metadata: &people.FieldMetadata{Source: &people.Source{Type: "CONTACT", Id: resource}},
		}}
	}
	return person
}

func contactMetadata(etag string) *people.PersonMetadata {
	return &people.PersonMetadata{Sources: []*people.Source{{Type: "CONTACT", Etag: etag}}}
}

func writeDedupeConnections(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"connections": []map[string]any{
			{
				"resourceName":   "people/1",
				"names":          []map[string]any{{"givenName": "Ada", "displayName": "Ada"}},
				"emailAddresses": []map[string]any{{"value": "ada@example.com"}},
				"phoneNumbers":   []map[string]any{{"value": "+1 555 0100"}},
			},
			{
				"resourceName":   "people/2",
				"names":          []map[string]any{{"givenName": "Ada", "displayName": "Ada"}},
				"emailAddresses": []map[string]any{{"value": "ADA@example.com"}},
				"phoneNumbers":   []map[string]any{{"value": "+1 555 0200"}},
			},
		},
	})
}

func writeDedupePerson(t *testing.T, w http.ResponseWriter, person *people.Person) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(person); err != nil {
		t.Fatalf("encode person: %v", err)
	}
}

func phoneValues(phones []*people.PhoneNumber) []string {
	values := make([]string, 0, len(phones))
	for _, phone := range phones {
		if phone != nil {
			values = append(values, phone.Value)
		}
	}
	return values
}
