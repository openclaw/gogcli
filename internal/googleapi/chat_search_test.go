package googleapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"
	"google.golang.org/api/chat/v1"
	gapi "google.golang.org/api/googleapi"

	"github.com/openclaw/gogcli/internal/authclient"
)

func TestChatSearchClientDirectTokenReadOnly(t *testing.T) {
	t.Parallel()
	ctx := WithReadOnly(authclient.WithAccessToken(context.Background(), "test-token"), true)

	client, err := NewChatSearchClientForAccount(ctx, "user@example.com")
	if err != nil {
		t.Fatal(err)
	}
	// Replace only the network boundary, retaining readonly, retry, and OAuth.
	transport := client.client.Transport.(*readOnlyTransport).base.(*RetryTransport).Base.(*oauth2.Transport)
	transport.Base = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.String() != "https://chat.googleapis.com/v1/spaces/-/messages:search" || req.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("unexpected authenticated request: method=%s url=%s", req.Method, req.URL)
		}

		if req.GetBody == nil || req.Header.Get("Content-Type") != "application/json" {
			t.Fatal("search request must be replayable JSON")
		}

		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"results":[{}, {"read":false}, {"read":true}],"nextPageToken":"p2"}`))}, nil
	})

	result, err := client.Search(ctx, &chat.SearchMessagesRequest{Filter: "project", PageSize: 3})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Results) != 3 || result.NextPageToken != "p2" || result.Results[0].Read != nil || result.Results[1].Read == nil || *result.Results[1].Read || result.Results[2].Read == nil || !*result.Results[2].Read {
		t.Fatalf("lost read presence: %#v", result)
	}
}

func TestChatSearchClientErrors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		code int
		body string
	}{
		{name: "permission", code: 403, body: `{"error":{"code":403,"message":"permission denied"}}`},
		{name: "malformed JSON", code: 200, body: `{"results":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.code)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			_, err := NewChatSearchClient(srv.Client(), srv.URL).Search(context.Background(), &chat.SearchMessagesRequest{Filter: "project"})
			if err == nil {
				t.Fatal("expected response error")
			}

			if tc.code == 403 {
				var apiErr *gapi.Error
				if !errors.As(err, &apiErr) || apiErr.Code != 403 {
					t.Fatalf("lost structured API error: %v", err)
				}
			}
		})
	}
}
