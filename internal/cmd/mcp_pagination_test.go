package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/app"
)

func TestMCPReadPagination(t *testing.T) {
	for _, name := range []string{"gmail_search", "calendar_events"} {
		t.Run(name, func(t *testing.T) {
			var tokens []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/messages"), strings.HasSuffix(r.URL.Path, "/events"):
					token := r.URL.Query().Get("pageToken")
					tokens = append(tokens, token)
					if q := r.URL.Query().Get("q"); q != "synthetic" {
						t.Errorf("query=%q", q)
					}
					if maxResults := r.URL.Query().Get("maxResults"); maxResults != "1" {
						t.Errorf("max=%q", maxResults)
					}
					next := `,"nextPageToken":"--opaque+/="`
					id := "first"
					if token == "--opaque+/=" {
						next = ""
						id = "second"
					}
					if name == "gmail_search" {
						fmt.Fprintf(w, `{"messages":[{"id":%q,"threadId":"thread"}]%s}`, id, next)
					} else {
						fmt.Fprintf(w, `{"items":[{"id":%q,"summary":"Synthetic","start":{"dateTime":"2026-10-27T12:00:00Z"},"end":{"dateTime":"2026-10-27T13:00:00Z"}}]%s}`, id, next)
					}
				case r.URL.Path == "/users/me/calendarList/primary", r.URL.Path == "/calendars/primary":
					fmt.Fprint(w, `{"id":"primary","summary":"Synthetic","timeZone":"UTC"}`)
				case strings.HasSuffix(r.URL.Path, "/labels"):
					fmt.Fprint(w, `{"labels":[]}`)
				case strings.Contains(r.URL.Path, "/messages/"):
					id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
					fmt.Fprintf(w, `{"id":%q,"threadId":"thread","payload":{"headers":[]}}`, id)
				default:
					t.Errorf("unexpected request %s", r.URL.String())
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			gmailSvc := newGmailServiceFromServer(t, srv)
			calendarSvc, err := calendar.NewService(t.Context(), option.WithoutAuthentication(), option.WithHTTPClient(srv.Client()), option.WithEndpoint(srv.URL+"/"))
			if err != nil {
				t.Fatal(err)
			}
			runtime := &app.Runtime{Services: app.Services{
				Gmail:    func(context.Context, string) (*gmail.Service, error) { return gmailSvc, nil },
				Calendar: func(context.Context, string) (*calendar.Service, error) { return calendarSvc, nil },
			}}
			tool := findMCPTool(t, name)
			s := newMCPServer()
			s.AddTool(newMCPTool(tool), func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				args, err := tool.BuildArgs(req)
				if err != nil {
					return nil, err
				}
				root := mcpParentRootArgs(&RootFlags{Home: t.TempDir(), Account: "me@example.com"})
				result := executeWithTestRuntime(t, append(root, args...), runtime) //nolint:contextcheck
				if result.err != nil {
					return nil, result.err
				}
				return mcp.NewToolResultText(result.stdout), nil
			})
			client := newMCPGmailTestClient(t, s)
			args := map[string]any{"query": "synthetic", "max": 1}
			if name == "calendar_events" {
				args["calendar_id"] = "primary"
				args["from"] = "2026-10-27"
				args["to"] = "2026-10-28"
			}
			for _, page := range []string{"", "--opaque+/="} {
				if page != "" {
					args["page"] = page
				}
				result, err := client.CallTool(t.Context(), mcpGmailRequest(name, args))
				if err != nil || result.IsError {
					t.Fatalf("page=%q result=%v err=%v", page, result, err)
				}
				want := "first"
				if page != "" {
					want = "second"
				}
				if !strings.Contains(mcpResultText(result), want) {
					t.Fatalf("missing %s: %v", want, result)
				}
				var payload struct {
					NextPageToken string `json:"nextPageToken"`
				}
				if err := json.Unmarshal([]byte(mcpResultText(result)), &payload); err != nil {
					t.Fatal(err)
				}
				if page == "" && payload.NextPageToken != "--opaque+/=" {
					t.Fatalf("continuation token=%q", payload.NextPageToken)
				}
				if page != "" && payload.NextPageToken != "" {
					t.Fatalf("unexpected continuation token=%q", payload.NextPageToken)
				}
			}
			if len(tokens) != 2 || tokens[0] != "" || tokens[1] != "--opaque+/=" {
				t.Fatalf("page tokens=%q", tokens)
			}
		})
	}
}
