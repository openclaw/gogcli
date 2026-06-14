//nolint:wsl_v5 // HTTP fixtures and assertions stay grouped for scanability.
package gmailbackup

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func TestServiceSourceProjectsLabelsListsAndRawMessages(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/labels"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "z", "name": "Zed"},
					{"id": "a", "name": "Alpha", "messagesTotal": 2},
				},
			})
		case strings.HasSuffix(r.URL.Path, "/messages"):
			if r.URL.Query().Get("pageToken") != "next" {
				t.Fatalf("pageToken = %q", r.URL.Query().Get("pageToken"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages":      []any{nil, map[string]string{"id": ""}, map[string]string{"id": "m1"}},
				"nextPageToken": "p2",
			})
		case strings.HasSuffix(r.URL.Path, "/messages/m1"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           "m1",
				"threadId":     "t1",
				"historyId":    "42",
				"internalDate": "1000",
				"labelIds":     []string{"INBOX"},
				"sizeEstimate": 10,
				"raw":          base64.RawURLEncoding.EncodeToString([]byte("Subject: test\r\n\r\nbody")),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	service, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("gmail.NewService: %v", err)
	}
	source, err := NewServiceSource(service)
	if err != nil {
		t.Fatalf("NewServiceSource: %v", err)
	}

	labels, err := source.Labels(context.Background())
	if err != nil {
		t.Fatalf("Labels: %v", err)
	}
	if len(labels) != 2 || labels[0].ID != "a" || labels[1].ID != "z" {
		t.Fatalf("labels = %+v", labels)
	}
	page, err := source.ListMessageIDs(context.Background(), ListRequest{
		Query:            "in:anywhere",
		MaxResults:       10,
		IncludeSpamTrash: true,
		PageToken:        "next",
	})
	if err != nil {
		t.Fatalf("ListMessageIDs: %v", err)
	}
	if len(page.IDs) != 1 || page.IDs[0] != "m1" || page.NextPageToken != "p2" {
		t.Fatalf("page = %+v", page)
	}
	message, err := source.RawMessage(context.Background(), "m1")
	if err != nil {
		t.Fatalf("RawMessage: %v", err)
	}
	if message.ID != "m1" || message.HistoryID != "42" || message.Raw == "" {
		t.Fatalf("message = %+v", message)
	}
}

func TestNewServiceSourceRejectsNil(t *testing.T) {
	t.Parallel()
	if _, err := NewServiceSource(nil); err == nil {
		t.Fatal("NewServiceSource(nil) succeeded")
	}
}
