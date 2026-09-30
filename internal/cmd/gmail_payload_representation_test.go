package cmd

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
)

func TestGmailReadPreservesFullPayloadBodySizeMismatch(t *testing.T) {
	// Model a 128-byte line-ending discrepancy without assuming that it explains
	// any particular Gmail response. Neither read may repair the supplied size.
	body := strings.Repeat("synthetic line\r\n", 128)
	data := base64.RawURLEncoding.EncodeToString([]byte(body))
	size := int64(len(body) - 128)
	message := &gmail.Message{
		Id: "m1", ThreadId: "t1",
		Payload: &gmail.MessagePart{
			MimeType: "multipart/alternative",
			Parts: []*gmail.MessagePart{{
				MimeType: "text/plain",
				Headers:  []*gmail.MessagePartHeader{{Name: "Content-Transfer-Encoding", Value: "quoted-printable"}},
				Body:     &gmail.MessagePartBody{Data: data, Size: size},
			}},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Query().Get("format") != "full" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/gmail/v1/users/me/messages/m1":
			_ = json.NewEncoder(w).Encode(message)
		case "/gmail/v1/users/me/threads/t1":
			_ = json.NewEncoder(w).Encode(&gmail.Thread{Id: "t1", Messages: []*gmail.Message{message}})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	for _, command := range [][]string{{"gmail", "get", "m1"}, {"gmail", "thread", "get", "t1"}} {
		t.Run(strings.Join(command, " "), func(t *testing.T) {
			args := append([]string{"--readonly", "--json", "--account", "reader@example.com"}, command...)
			result := executeWithGmailTestService(t, args, newGmailServiceFromServer(t, srv))
			if result.err != nil {
				t.Fatalf("execute: %v\nstderr=%s", result.err, result.stderr)
			}
			var output struct {
				Message *gmail.Message `json:"message"`
				Thread  *gmail.Thread  `json:"thread"`
				Body    string         `json:"body"`
			}
			if err := json.Unmarshal([]byte(result.stdout), &output); err != nil {
				t.Fatal(err)
			}
			got := output.Message
			if command[1] == "thread" {
				if output.Thread == nil || len(output.Thread.Messages) != 1 {
					t.Fatalf("unexpected thread: %+v", output.Thread)
				}
				got = output.Thread.Messages[0]
			} else if output.Body != body {
				t.Fatalf("display body changed: %q", output.Body)
			}
			if got == nil || got.Payload == nil || len(got.Payload.Parts) != 1 {
				t.Fatalf("unexpected message: %+v", got)
			}
			partBody := got.Payload.Parts[0].Body
			if partBody == nil || partBody.Data != data || partBody.Size != size {
				t.Fatalf("API body changed: %+v", partBody)
			}
		})
	}
}
