package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/api/cloudidentity/v1"
)

const groupsQueryTestCursor = "opaque+page/2==?"

func supportedGroupsQueryService(t *testing.T, pageSize int64) (*cloudidentity.Service, <-chan string) {
	t.Helper()
	requests := make(chan string, 4)
	svc := newCloudIdentityTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/groups/-/memberships:searchTransitiveGroups") {
			http.NotFound(w, r)
			return
		}
		const query = "member_key_id == 'person@example.com' && 'cloudidentity.googleapis.com/groups.discussion_forum' in labels"
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("query") != query {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
				"code": http.StatusBadRequest, "message": "unsupported group label query", "status": "INVALID_ARGUMENT",
			}})
			return
		}
		if got := r.URL.Query().Get("pageSize"); got != strconv.FormatInt(pageSize, 10) {
			t.Errorf("pageSize = %q, want %d", got, pageSize)
		}
		cursor := r.URL.Query().Get("pageToken")
		requests <- cursor
		switch cursor {
		case "":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"memberships": []map[string]any{{
					"groupKey": map[string]any{"id": "regular@example.com"}, "displayName": "Regular", "relationType": "DIRECT",
				}},
				"nextPageToken": groupsQueryTestCursor,
			})
		case groupsQueryTestCursor:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"memberships": []map[string]any{{
					"groupKey": map[string]any{"id": "dynamic@example.com"}, "displayName": "Dynamic", "relationType": "INDIRECT",
				}},
			})
		default:
			http.Error(w, "unexpected page token", http.StatusBadRequest)
		}
	}))
	return svc, requests
}

func assertGroupsQueryRequests(t *testing.T, requests <-chan string, want []string) {
	t.Helper()
	for _, expected := range want {
		select {
		case got := <-requests:
			if got != expected {
				t.Fatalf("page token = %q, want %q", got, expected)
			}
		default:
			t.Fatalf("missing request for page token %q", expected)
		}
	}
	select {
	case got := <-requests:
		t.Fatalf("unexpected extra request for page token %q", got)
	default:
	}
}

func TestGroupsListSupportedQueryPagination(t *testing.T) {
	for _, tc := range []struct {
		name      string
		args      []string
		wantNames []string
		wantPage  string
		wantCalls []string
	}{
		{name: "first page", wantNames: []string{"regular@example.com"}, wantPage: groupsQueryTestCursor, wantCalls: []string{""}},
		{name: "all pages", args: []string{"--all"}, wantNames: []string{"regular@example.com", "dynamic@example.com"}, wantCalls: []string{"", groupsQueryTestCursor}},
		{name: "cursor", args: []string{"--page", groupsQueryTestCursor}, wantNames: []string{"dynamic@example.com"}, wantCalls: []string{groupsQueryTestCursor}},
		{name: "all from cursor", args: []string{"--all", "--page", groupsQueryTestCursor}, wantNames: []string{"dynamic@example.com"}, wantCalls: []string{groupsQueryTestCursor}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--json", "--account", "person@example.com", "groups", "list", "--max", "1"}, tc.args...)
			svc, requests := supportedGroupsQueryService(t, 1)
			result := executeWithCloudIdentityTestService(t, args, svc)
			if result.err != nil {
				t.Fatalf("list groups: %v; stderr=%q", result.err, result.stderr)
			}
			var output struct {
				Groups []struct {
					GroupName string `json:"groupName"`
				} `json:"groups"`
				NextPageToken string `json:"nextPageToken"`
			}
			if err := json.Unmarshal([]byte(result.stdout), &output); err != nil {
				t.Fatal(err)
			}
			if len(output.Groups) != len(tc.wantNames) || output.NextPageToken != tc.wantPage {
				t.Fatalf("unexpected output: %s", result.stdout)
			}
			for i, name := range tc.wantNames {
				if output.Groups[i].GroupName != name {
					t.Fatalf("group %d = %q, want %q", i, output.Groups[i].GroupName, name)
				}
			}
			assertGroupsQueryRequests(t, requests, tc.wantCalls)
		})
	}
}

func TestBackupGroupsSupportedQuery(t *testing.T) {
	svc, requests := supportedGroupsQueryService(t, 1000)
	groups, err := fetchBackupCloudIdentityGroups(context.Background(), svc, "person@example.com")
	if err != nil {
		t.Fatalf("fetch backup groups: %v", err)
	}
	if len(groups) != 2 || groups[0].GroupKey.Id != "dynamic@example.com" || groups[1].GroupKey.Id != "regular@example.com" {
		t.Fatalf("unexpected groups: %#v", groups)
	}
	assertGroupsQueryRequests(t, requests, []string{"", groupsQueryTestCursor})
}
