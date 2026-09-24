package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/outfmt"
)

func withRawTestHTTP(t *testing.T, ctx context.Context, endpoint string) context.Context {
	t.Helper()
	base, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		request = request.Clone(request.Context())
		request.URL.Scheme = base.Scheme
		request.URL.Host = base.Host
		return http.DefaultTransport.RoundTrip(request)
	})}
	factory := app.HTTPClientFactory(func(context.Context, string) (*http.Client, error) { return client, nil })
	return withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Services.CalendarHTTP = factory
		runtime.Services.DocsHTTP = app.DocsHTTPClientFactory(factory)
		runtime.Services.DriveHTTP = factory
		runtime.Services.FormsHTTP = factory
		runtime.Services.GmailHTTP = factory
		runtime.Services.PeopleContactsHTTP = factory
		runtime.Services.SheetsHTTP = app.SheetsHTTPClientFactory(factory)
		runtime.Services.SlidesHTTP = factory
		runtime.Services.TasksHTTP = factory
	})
}

func TestRawCommandsPreserveJSON(t *testing.T) {
	cases := []struct {
		name    string
		command func() any
		args    []string
		path    string
		fields  string
	}{
		{"slides", func() any { return &SlidesRawCmd{} }, []string{"p1"}, "/v1/presentations/p1", `"presentationId":"p1","slides":[{"pageElements":[{"shape":{"text":{"textElements":[{"startIndex":0,"textRun":{"style":{"bold":false,"italic":false}}}]}}}]}]`},
		{"docs", func() any { return &DocsRawCmd{} }, []string{"d1"}, "/v1/documents/d1", `"documentId":"d1","body":{"content":[{"startIndex":0,"paragraph":{"elements":[{"textRun":{"textStyle":{"bold":false}}}]}}]}`},
		{"drive", func() any { return &DriveRawCmd{} }, []string{"f1"}, "/drive/v3/files/f1", `"id":"f1","size":"0","trashed":false,"capabilities":{"canShare":false}`},
		{"gmail", func() any { return &GmailRawCmd{} }, []string{"m1"}, "/gmail/v1/users/me/messages/m1", `"id":"m1","payload":{"partId":"","body":{"size":0}}`},
		{"calendar", func() any { return &CalendarRawCmd{} }, []string{"primary", "e1"}, "/calendar/v3/calendars/primary/events/e1", `"id":"e1","sequence":0,"attendees":[]`},
		{"forms", func() any { return &FormsRawCmd{} }, []string{"f1"}, "/v1/forms/f1", `"formId":"f1","items":[{"questionItem":{"question":{"required":false}}}]`},
		{"tasks", func() any { return &TasksRawCmd{} }, []string{"@default", "t1"}, "/tasks/v1/lists/@default/tasks/t1", `"id":"t1","hidden":false,"deleted":false`},
		{"people", func() any { return &PeopleRawCmd{} }, []string{"people/c1"}, "/v1/people/c1", `"resourceName":"people/c1","names":[{"metadata":{"primary":false}}]`},
		{"contacts", func() any { return &ContactsRawCmd{} }, []string{"people/c1"}, "/v1/people/c1", `"resourceName":"people/c1","names":[{"metadata":{"primary":false}}]`},
		{"sheets", func() any { return &SheetsRawCmd{} }, []string{"s1"}, "/v4/spreadsheets/s1", `"spreadsheetId":"s1","sheets":[{"properties":{"sheetId":0,"hidden":false}}]`},
	}
	for _, tc := range cases {
		for _, mode := range []string{"compact", "pretty", "wrapped"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				payload := `{` + tc.fields + `,"title":"Raw title","url":"https://example.com/?x=1&y=2","future":{"integer":9007199254740993,"null":null,"empty":"","array":[],"object":{},"false":false}}`
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.Method != http.MethodGet || r.URL.EscapedPath() != tc.path || r.URL.Query().Get("alt") != "json" || r.URL.Query().Get("prettyPrint") != "false" {
						t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					}
					_, _ = io.WriteString(w, payload)
				}))
				t.Cleanup(server.Close)
				var output bytes.Buffer
				ctx := withRawTestHTTP(t, newCmdRuntimeOutputContext(t, &output, io.Discard), server.URL)
				ctx = withCalendarTestService(ctx, newMockCalendarService(t, server))
				ctx = withTasksTestService(ctx, newTasksServiceFromServer(t, server))
				ctx = withMockPeopleContactsService(t, ctx, server)
				ctx = outfmt.WithJSONTransform(ctx, outfmt.JSONTransform{Select: []string{"title"}})
				args := append([]string(nil), tc.args...)
				if mode == "pretty" {
					args = append(args, "--pretty")
				}
				if mode == "wrapped" {
					ctx = outfmt.WithUntrustedWrapper(ctx, outfmt.UntrustedWrapOptions{Enabled: true})
				}
				if err := runKong(t, tc.command(), args, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
					t.Fatal(err)
				}
				assertRawJSONPreserved(t, output.String(), payload, mode == "wrapped")
				if calls != 1 || !strings.HasSuffix(output.String(), "\n") || (mode != "pretty" && strings.Count(output.String(), "\n") != 1) {
					t.Fatalf("calls=%d, output=%q", calls, output.String())
				}
			})
		}
	}
}

func assertRawJSONPreserved(t *testing.T, output, payload string, wrapped bool) {
	t.Helper()
	decode := func(raw string) any {
		var value any
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	var compare func(any, any)
	compare = func(got, want any) {
		t.Helper()
		switch want := want.(type) {
		case map[string]any:
			object, ok := got.(map[string]any)
			if !ok || len(object) != len(want) {
				t.Fatalf("object changed: got %v, want %v", got, want)
			}
			for key, value := range want {
				actual, exists := object[key]
				if !exists {
					t.Fatalf("missing field %q", key)
				}
				compare(actual, value)
			}
		case []any:
			array, ok := got.([]any)
			if !ok || len(array) != len(want) {
				t.Fatalf("array changed: got %v, want %v", got, want)
			}
			for i, value := range want {
				compare(array[i], value)
			}
		case string:
			if got == want {
				return
			}
			text, ok := got.(string)
			if !wrapped || !ok || !strings.Contains(text, "EXTERNAL_UNTRUSTED_CONTENT") || !strings.Contains(text, want) {
				t.Fatalf("string changed: got %v, want %q", got, want)
			}
		default:
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("value changed: got %#v, want %#v", got, want)
			}
		}
	}
	got, want := decode(output), decode(payload)
	if wrapped {
		object := got.(map[string]any)
		metadata, ok := object["externalContent"].(map[string]any)
		if !ok || metadata["untrusted"] != true || metadata["wrapped"] != true {
			t.Fatalf("missing wrapping metadata: %v", object)
		}
		delete(object, "externalContent")
		if title, ok := object["title"].(string); !ok || !strings.Contains(title, "EXTERNAL_UNTRUSTED_CONTENT") {
			t.Fatalf("title not wrapped: %v", object["title"])
		}
		for _, key := range []string{"id", "documentId", "presentationId", "spreadsheetId", "formId", "resourceName", "url"} {
			if !reflect.DeepEqual(object[key], want.(map[string]any)[key]) {
				t.Fatalf("identity or URL changed: %s", key)
			}
		}
	}
	compare(got, want)
}

func TestReadRawObjectErrors(t *testing.T) {
	for _, body := range []string{"", "null", "[]", "true", "not json", "{} {}"} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) }))
			t.Cleanup(server.Close)
			if raw, err := readRawObject(t.Context(), server.Client(), server.URL, nil, "object"); err == nil || raw != nil {
				t.Fatalf("raw=%v, err=%v", raw, err)
			}
		})
	}
	for code, want := range map[int]int{403: 6, 404: 5, 429: 7, 503: 8} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }))
			t.Cleanup(server.Close)
			raw, err := readRawObject(t.Context(), server.Client(), server.URL, nil, "object")
			if got := ExitCode(stableExitCode(err)); got != want || raw != nil {
				t.Fatalf("raw=%v, err=%v, exit=%d; want %d", raw, err, got, want)
			}
		})
	}
}

func TestReadRawObjectHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("canceled request reached server") }))
	t.Cleanup(server.Close)
	_, err := readRawObject(ctx, server.Client(), server.URL, nil, "object")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v; want canceled", err)
	}
}

func TestDocsRawProjectionPreservesSelectedValues(t *testing.T) {
	const payload = `{"documentId":"d1","title":"","revisionId":null,"body":{"firstTab":true},"headers":{"firstTab":{}},"futureRoot":{"integer":9007199254740993},"tabs":[null,{"tabProperties":{"tabId":"first","title":"First","index":0},"documentTab":{"body":{}},"childTabs":[{"tabProperties":{"tabId":"nested","title":"Notes","index":0},"documentTab":{"documentId":"must-not-override","title":"must-not-override","suggestionsViewMode":"must-not-add","body":null,"lists":{},"futureTab":{"zero":0,"false":false,"empty":"","array":[],"integer":9007199254740993}}}]}]}`
	const projected = `{"documentId":"d1","title":"","revisionId":null,"body":null,"lists":{},"futureRoot":{"integer":9007199254740993},"futureTab":{"zero":0,"false":false,"empty":"","array":[],"integer":9007199254740993}}`
	for _, selector := range []string{"nested", " nOtEs ", "all"} {
		t.Run(selector, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("includeTabsContent") != "true" {
					t.Error("missing includeTabsContent")
				}
				_, _ = io.WriteString(w, payload)
			}))
			t.Cleanup(server.Close)
			var output bytes.Buffer
			ctx := withRawTestHTTP(t, newCmdRuntimeOutputContext(t, &output, io.Discard), server.URL)
			command := &DocsRawCmd{DocID: "d1", Tab: selector}
			want := projected
			if selector == "all" {
				command.Tab, command.AllTabs, want = "", true, payload
			}
			if err := command.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
				t.Fatal(err)
			}
			assertRawJSONPreserved(t, output.String(), want, false)
		})
	}
}

func TestDriveRawRedactionPreservesUnknownNestedValues(t *testing.T) {
	const payload = `{"id":"f1","trashed":false,"size":"0","thumbnailLink":"https://example.com/capability","properties":{"synthetic":"private"},"contentHints":{"future":null,"thumbnail":{"image":"synthetic-bytes","mimeType":"image/png","future":{"integer":9007199254740993,"false":false,"empty":[]}}}}`
	const redacted = `{"id":"f1","trashed":false,"size":"0","contentHints":{"future":null,"thumbnail":{"mimeType":"image/png","future":{"integer":9007199254740993,"false":false,"empty":[]}}}}`
	for _, fields := range []string{"", " \t", "*", "id,thumbnailLink"} {
		t.Run(fields, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wantMask := fields
				if strings.TrimSpace(fields) == "" {
					wantMask = "*"
				}
				if r.URL.Query().Get("fields") != wantMask || r.URL.Query().Get("supportsAllDrives") != "true" {
					t.Errorf("unexpected query: %s", r.URL.RawQuery)
				}
				_, _ = io.WriteString(w, payload)
			}))
			t.Cleanup(server.Close)
			var output bytes.Buffer
			ctx := withRawTestHTTP(t, newCmdRuntimeOutputContext(t, &output, io.Discard), server.URL)
			if err := (&DriveRawCmd{FileID: "f1", Fields: fields}).Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
				t.Fatal(err)
			}
			want := payload
			if strings.TrimSpace(fields) == "" {
				want = redacted
			}
			assertRawJSONPreserved(t, output.String(), want, false)
		})
	}
}

func TestDriveRawRedactionPreservesNullAndEmptyParents(t *testing.T) {
	for _, payload := range []string{`{}`, `{"contentHints":null}`, `{"contentHints":{}}`, `{"contentHints":{"thumbnail":null}}`, `{"contentHints":{"thumbnail":{}}}`} {
		t.Run(payload, func(t *testing.T) {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal([]byte(payload), &fields); err != nil {
				t.Fatal(err)
			}
			if err := redactDriveRawThumbnail(fields); err != nil {
				t.Fatal(err)
			}
			output, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			assertRawJSONPreserved(t, string(output), payload, false)
		})
	}
}
