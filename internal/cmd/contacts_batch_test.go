package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/api/people/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
)

func contactsBatchTestFile(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "contacts.json")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeContactsBatchGetTestResponse(t *testing.T, w http.ResponseWriter, contacts []*people.Person) {
	t.Helper()
	response := &people.GetPeopleResponse{}
	for _, contact := range contacts {
		response.Responses = append(response.Responses, &people.PersonResponse{RequestedResourceName: contact.ResourceName, Person: contact})
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		t.Error(err)
	}
}

func contactsBatchTestNames(count int) []string {
	names := make([]string, count)
	for i := range names {
		names[i] = fmt.Sprintf("people/c%04d", i)
	}
	return names
}

func decodeContactsBatchTestResult(t *testing.T, output string) contactsBatchResult {
	t.Helper()
	var result contactsBatchResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("result JSON: %v\n%s", err, output)
	}
	return result
}

func TestContactsBatchGetChunksAndMatchesRequestedResource(t *testing.T) {
	var sizes []int
	svc, closeServer := newPeopleService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/people:batchGet" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if !strings.Contains(r.URL.Query().Get("personFields"), "metadata") || r.URL.Query().Get("sources") != contactsDedupeContactSource {
			t.Errorf("unsafe read query: %s", r.URL.RawQuery)
		}
		names := r.URL.Query()["resourceNames"]
		sizes = append(sizes, len(names))
		response := &people.GetPeopleResponse{}
		for i := len(names) - 1; i >= 0; i-- {
			response.Responses = append(response.Responses, &people.PersonResponse{
				RequestedResourceName: names[i], Person: &people.Person{ResourceName: names[i] + "_linked", Metadata: contactMetadata("etag")},
			})
		}
		_ = json.NewEncoder(w).Encode(response)
	})
	defer closeServer()
	args := append([]string{"--json", "--results-only", "--readonly", "--account", "a@example.com", "contacts", "batch", "get"}, contactsBatchTestNames(201)...)
	executed := executeWithPeopleContactsTestService(t, args, svc)
	if executed.err != nil {
		t.Fatal(executed.err)
	}
	result := decodeContactsBatchTestResult(t, executed.stdout)
	if result.Requested != 201 || result.Completed != 201 || !reflect.DeepEqual(sizes, []int{200, 1}) {
		t.Fatalf("result=%+v sizes=%v", result, sizes)
	}
}

func TestContactsBatchGetRejectsIncompleteResponses(t *testing.T) {
	valid := &people.PersonResponse{RequestedResourceName: "people/c1", Person: &people.Person{ResourceName: "people/c1"}}
	for name, responses := range map[string][]*people.PersonResponse{
		"missing":        nil,
		"null":           {nil},
		"status":         {{RequestedResourceName: "people/c1", Status: &people.Status{Code: 5, Message: "not found"}}},
		"wrong resource": {{RequestedResourceName: "people/c2", Person: valid.Person}},
		"duplicate":      {valid, valid},
	} {
		t.Run(name, func(t *testing.T) {
			names := []string{"people/c1"}
			if name == "duplicate" {
				names = append(names, "people/c2")
			}
			if _, err := validateContactsBatchGet(names, &people.GetPeopleResponse{Responses: responses}); err == nil {
				t.Fatal("incomplete response accepted")
			}
		})
	}
}

func TestContactsBatchCreateRetainsPartialProgressWithoutRetry(t *testing.T) {
	inputs := make([]map[string]any, 401)
	for i := range inputs {
		inputs[i] = map[string]any{"names": []map[string]string{{"givenName": fmt.Sprintf("Fixture %d", i)}}}
	}
	data, err := json.Marshal(inputs)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/v1/people:batchCreateContacts" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var request people.BatchCreateContactsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if len(request.Contacts) != 200 || request.ReadMask == "" {
			t.Errorf("invalid create request: count=%d mask=%q", len(request.Contacts), request.ReadMask)
		}
		if calls == 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":{"code":503,"message":"response lost"}}`)
			return
		}
		response := &people.BatchCreateContactsResponse{}
		for i := range request.Contacts {
			response.CreatedPeople = append(response.CreatedPeople, &people.PersonResponse{Person: &people.Person{ResourceName: fmt.Sprintf("people/c%d", i)}})
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	client := &http.Client{Transport: googleapi.NewRetryTransport(http.DefaultTransport)}
	svc := newGoogleTestServiceWithEndpoint(t, client, server.URL+"/", people.NewService)
	executed := executeWithPeopleContactsTestService(t, []string{"--json", "--account", "a@example.com", "contacts", "batch", "create", "--from-file", contactsBatchTestFile(t, string(data))}, svc)
	if ExitCode(executed.err) != 1 || calls != 2 || !strings.Contains(executed.err.Error(), "inspect") {
		t.Fatalf("error=%v calls=%d", executed.err, calls)
	}
	result := decodeContactsBatchTestResult(t, executed.stdout)
	if result.Completed != 200 || len(result.Batches) != 3 || result.Batches[1].State != contactsBatchUnconfirmed || result.Batches[2].State != contactsBatchPending {
		t.Fatalf("incorrect partial progress: %+v", result)
	}
	if !reflect.DeepEqual(result.Batches[2].InputIndexes, []int{400}) {
		t.Fatalf("unattempted inputs = %v", result.Batches[2].InputIndexes)
	}
}

func TestContactsBatchUpdateSeparatesMasksAndPreservesETags(t *testing.T) {
	var masks []string
	calls := 0
	svc, closeServer := newPeopleService(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/v1/people:batchUpdateContacts" {
			t.Errorf("unexpected request %s %s (must not refresh etags)", r.Method, r.URL.Path)
		}
		var raw struct {
			Contacts   map[string]json.RawMessage `json:"contacts"`
			UpdateMask string                     `json:"updateMask"`
		}
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Error(err)
		}
		masks = append(masks, raw.UpdateMask)
		response := &people.BatchUpdateContactsResponse{UpdateResult: map[string]people.PersonResponse{}}
		for name, data := range raw.Contacts {
			var person people.Person
			if err := json.Unmarshal(data, &person); err != nil {
				t.Error(err)
			}
			if source := contactsBatchContactSource(&person); source == nil || source.Etag != "snapshot-etag" {
				t.Errorf("caller etag lost for %s", name)
			}
			if raw.UpdateMask == "urls" && !strings.Contains(string(data), `"urls":[]`) {
				t.Errorf("explicit clear missing: %s", data)
			}
			if name == "people/c2" && raw.UpdateMask != "emailAddresses" || name != "people/c2" && raw.UpdateMask != "urls" {
				t.Errorf("mask %q erases unrelated data on %s", raw.UpdateMask, name)
			}
			response.UpdateResult[name] = people.PersonResponse{Person: &person}
		}
		_ = json.NewEncoder(w).Encode(response)
	})
	defer closeServer()
	metadata := `"metadata":{"sources":[{"type":"CONTACT","etag":"snapshot-etag"},{"type":"PROFILE","etag":"other-etag"}]}`
	input := `{"people/c1":{` + metadata + `,"urls":null},"people/c2":{` + metadata + `,"emailAddresses":[{"value":"fixture@example.com"}]},"people/c3":{` + metadata + `,"urls":[]}}`
	executed := executeWithPeopleContactsTestService(t, []string{"--json", "--account", "a@example.com", "contacts", "batch", "update", "--from-file", contactsBatchTestFile(t, input)}, svc)
	if executed.err != nil || calls != 2 || !reflect.DeepEqual(masks, []string{"urls", "emailAddresses"}) {
		t.Fatalf("err=%v calls=%d masks=%v", executed.err, calls, masks)
	}
	if result := decodeContactsBatchTestResult(t, executed.stdout); result.Completed != 3 {
		t.Fatalf("result=%+v", result)
	}
}

func TestContactsBatchDeleteChunksAndPlainOutput(t *testing.T) {
	var sizes []int
	svc, closeServer := newPeopleService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/people:batchDeleteContacts" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var request people.BatchDeleteContactsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		sizes = append(sizes, len(request.ResourceNames))
		_, _ = io.WriteString(w, `{}`)
	})
	defer closeServer()
	args := append([]string{"--plain", "--force", "--account", "a@example.com", "contacts", "batch", "delete"}, contactsBatchTestNames(501)...)
	executed := executeWithPeopleContactsTestService(t, args, svc)
	if executed.err != nil || !reflect.DeepEqual(sizes, []int{500, 1}) {
		t.Fatalf("error=%v sizes=%v", executed.err, sizes)
	}
	if !strings.Contains(executed.stdout, "1\tcompleted\t500\t") || !strings.Contains(executed.stdout, "2\tcompleted\t1\tpeople/c0500") {
		t.Fatalf("plain output=%s", executed.stdout)
	}
}

func TestContactsBatchUpdateRetainsAPIErrorDetails(t *testing.T) {
	calls := 0
	svc, closeServer := newPeopleService(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"code":400,"message":"stale contact","details":[{"@type":"type.googleapis.com/google.people.v1.BatchUpdateContactsErrorDetails","contactErrors":{"people/c1":{"code":9,"message":"etag mismatch"}}}]}}`)
	})
	defer closeServer()
	input := `{"people/c1":{"metadata":{"sources":[{"type":"CONTACT","etag":"stale"}]},"urls":[]}}`
	executed := executeWithPeopleContactsTestService(t, []string{"--json", "--account", "a@example.com", "contacts", "batch", "update", "--from-file", contactsBatchTestFile(t, input)}, svc)
	if executed.err == nil || calls != 1 {
		t.Fatalf("error=%v calls=%d", executed.err, calls)
	}
	result := decodeContactsBatchTestResult(t, executed.stdout)
	if result.Completed != 0 || result.Batches[0].State != contactsBatchUnconfirmed || len(result.Batches[0].Error.Details) != 1 || !strings.Contains(executed.stdout, "etag mismatch") {
		t.Fatalf("error details or unconfirmed state lost: %s", executed.stdout)
	}
}

func TestContactsBatchMutationRejectsMissingResult(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			svc, closeServer := newPeopleService(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `{}`)
			})
			defer closeServer()
			input := `[{"names":[{"givenName":"Fixture"}]}]`
			if operation == "update" {
				input = `{"people/c1":{"metadata":{"sources":[{"type":"CONTACT","etag":"snapshot"}]},"urls":[]}}`
			}
			executed := executeWithPeopleContactsTestService(t, []string{"--json", "--account", "a@example.com", "contacts", "batch", operation, "--from-file", contactsBatchTestFile(t, input)}, svc)
			if ExitCode(executed.err) != 1 {
				t.Fatalf("missing result accepted: %v", executed.err)
			}
			result := decodeContactsBatchTestResult(t, executed.stdout)
			if result.Completed != 0 || result.Batches[0].State != contactsBatchUnconfirmed {
				t.Fatalf("incorrect result: %+v", result)
			}
		})
	}
}

func TestContactsBatchUpdateChunksHomogeneousFields(t *testing.T) {
	contacts := map[string]any{}
	for _, name := range contactsBatchTestNames(201) {
		contacts[name] = map[string]any{"metadata": contactMetadata("snapshot"), "urls": []any{}}
	}
	data, err := json.Marshal(contacts)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := parseContactsBatchUpdates(data)
	if err != nil || len(requests) != 2 || len(requests[0].Contacts) != 200 || len(requests[1].Contacts) != 1 {
		t.Fatalf("requests=%v error=%v", requests, err)
	}
}

func TestContactsBatchBakedMutationDenial(t *testing.T) {
	withBakedSafetyProfile(t, "name: contacts-read\nallow: [contacts]\ndeny: [contacts.delete]\n")
	result := executeWithPeopleTestServices(t, []string{"--dry-run", "--force", "contacts", "batch", "delete", "people/c1"}, peopleTestServices{})
	if ExitCode(result.err) != 2 || !strings.Contains(result.err.Error(), "contacts.delete") {
		t.Fatalf("baked deletion denial bypassed: %v", result.err)
	}
}

func TestContactsBatchValidationAndSafetyBeforeAuth(t *testing.T) {
	valid := contactsBatchTestFile(t, `[{"names":[{"givenName":"Fixture"}]}]`)
	for _, args := range [][]string{
		{"--readonly", "contacts", "batch", "create", "--from-file", valid},
		{"--readonly", "--force", "contacts", "batch", "delete", "people/c1"},
		{"--no-input", "contacts", "batch", "delete", "people/c1"},
		{"--disable-commands", "contacts.create", "contacts", "batch", "create", "--from-file", valid},
		{"--enable-commands", "contacts", "--disable-commands", "contacts.delete", "--force", "contacts", "batch", "delete", "people/c1"},
		{"--enable-commands", "contacts.create", "contacts", "batch", "create", "--from-file", valid},
		{"contacts", "batch", "delete", "people/c1", "people/c1"},
		{"contacts", "batch", "get", "someone@example.com"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			executed := executeWithPeopleTestServices(t, args, peopleTestServices{Contacts: func(context.Context, string) (*people.Service, error) {
				t.Fatal("unsafe command reached auth/service")
				return nil, errUnexpectedContactsServiceCall
			}})
			if executed.err == nil {
				t.Fatal("unsafe command succeeded")
			}
		})
	}
	for _, input := range []string{
		`[]`, `null`, `[null]`, `[{"names":[{"givenName":"Fixture"}]},{"typo":[]}]`,
		`[{"names":[null]}]`, `[{"names":[{},{}]}]`, `[{"names":[{"unknown":"x"}]}]`,
		`[{"names":[],"names":[{}]}]`, `[{"memberships":[]}]`, `[{"resourceName":"people/c1","names":[{}]}]`,
	} {
		t.Run(input, func(t *testing.T) {
			executed := executeWithPeopleTestServices(t, []string{"--dry-run", "contacts", "batch", "create", "--from-file", contactsBatchTestFile(t, input)}, peopleTestServices{})
			if ExitCode(executed.err) != 2 {
				t.Fatalf("invalid input accepted: %v", executed.err)
			}
		})
	}
	for _, input := range []string{
		`{"people/c1":{"urls":[]}}`,
		`{"people/c1":{"urls":[],"metadata":{"sources":[{"type":"PROFILE","etag":"x"}]}}}`,
		`{"people/c1":{"urls":[],"metadata":{"sources":[{"type":"CONTACT","etag":"x"}]},"resourceName":"people/c2"}}`,
		`{"people/c1":{"urls":[]},"people/c1":{"urls":[]}}`,
	} {
		executed := executeWithPeopleTestServices(t, []string{"--dry-run", "contacts", "batch", "update", "--from-file", contactsBatchTestFile(t, input)}, peopleTestServices{})
		if ExitCode(executed.err) != 2 {
			t.Fatalf("invalid update %s accepted: %v", input, executed.err)
		}
	}
}

func TestContactsBatchDryRunFromStdin(t *testing.T) {
	setTestConfigHome(t)
	var output bytes.Buffer
	ctx := newCmdRuntimeIOContext(t, strings.NewReader(`[{"names":[{"givenName":"Fixture"}]}]`), &output, io.Discard)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	err := (&ContactsBatchCreateCmd{FromFile: "-"}).Run(ctx, &RootFlags{DryRun: true})
	if ExitCode(err) != 0 || !strings.Contains(output.String(), `"contactPerson"`) || !strings.Contains(output.String(), `"dry_run": true`) {
		t.Fatalf("error=%v output=%s", err, output.String())
	}
}
