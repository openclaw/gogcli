package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/idtoken"
)

func TestParsePubSubPush(t *testing.T) {
	payload := pubsubPushEnvelope{}
	payload.Message.Data = "Zm9v"
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	env, err := parsePubSubPush(req)
	if err != nil {
		t.Fatalf("parsePubSubPush: %v", err)
	}
	if env.Message.Data != "Zm9v" {
		t.Fatalf("unexpected data")
	}

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"message":{}}`)))
	if _, err := parsePubSubPush(req); err == nil {
		t.Fatalf("expected missing data error")
	}

	oversize := bytes.Repeat([]byte("a"), defaultPushBodyLimitBytes+1)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(oversize))
	if _, err := parsePubSubPush(req); err == nil {
		t.Fatalf("expected size error")
	}
}

func TestCollectHistoryMessageIDs(t *testing.T) {
	resp := &gmail.ListHistoryResponse{
		History: []*gmail.History{
			{
				MessagesAdded: []*gmail.HistoryMessageAdded{
					{Message: &gmail.Message{Id: "m1"}},
					{Message: &gmail.Message{Id: "m1"}},
					nil,
				},
				MessagesDeleted: []*gmail.HistoryMessageDeleted{
					{Message: &gmail.Message{Id: "m4"}},
				},
				LabelsAdded: []*gmail.HistoryLabelAdded{
					{Message: &gmail.Message{Id: "m5"}},
				},
				LabelsRemoved: []*gmail.HistoryLabelRemoved{
					{Message: &gmail.Message{Id: "m6"}},
				},
				Messages: []*gmail.Message{
					{Id: "m2"},
					{Id: ""},
				},
			},
			{
				Messages: []*gmail.Message{{Id: "m3"}},
			},
		},
	}
	result := collectHistoryMessageIDs(resp)

	// Check fetch IDs contain added, labels, and general messages
	fetchJoined := strings.Join(result.FetchIDs, ",")
	if !strings.Contains(fetchJoined, "m1") ||
		!strings.Contains(fetchJoined, "m2") ||
		!strings.Contains(fetchJoined, "m3") ||
		!strings.Contains(fetchJoined, "m5") ||
		!strings.Contains(fetchJoined, "m6") {
		t.Fatalf("unexpected fetch ids: %v", result.FetchIDs)
	}

	// Deleted message m4 should NOT be in fetch IDs
	if strings.Contains(fetchJoined, "m4") {
		t.Fatalf("deleted message m4 should not be in fetch ids: %v", result.FetchIDs)
	}

	// Verify exact count: 5 unique fetch IDs (m1, m2, m3, m5, m6)
	if len(result.FetchIDs) != 5 {
		t.Fatalf("expected 5 unique fetch ids, got %d: %v", len(result.FetchIDs), result.FetchIDs)
	}

	// Deleted message m4 should be in deleted IDs
	if len(result.DeletedIDs) != 1 || result.DeletedIDs[0] != "m4" {
		t.Fatalf("expected deleted ids [m4], got: %v", result.DeletedIDs)
	}
}

func TestCollectHistoryMessageIDs_DeletedRemovesFromFetch(t *testing.T) {
	// Test that if a message is added then deleted, it ends up only in DeletedIDs
	resp := &gmail.ListHistoryResponse{
		History: []*gmail.History{
			{
				MessagesAdded: []*gmail.HistoryMessageAdded{
					{Message: &gmail.Message{Id: "m1"}},
				},
			},
			{
				MessagesDeleted: []*gmail.HistoryMessageDeleted{
					{Message: &gmail.Message{Id: "m1"}}, // Same message deleted later
				},
			},
		},
	}
	result := collectHistoryMessageIDs(resp)

	// m1 should not be in fetch IDs since it was deleted
	for _, id := range result.FetchIDs {
		if id == "m1" {
			t.Fatalf("deleted message m1 should not be in fetch ids: %v", result.FetchIDs)
		}
	}

	// m1 should be in deleted IDs
	found := false
	for _, id := range result.DeletedIDs {
		if id == "m1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("deleted message m1 should be in deleted ids: %v", result.DeletedIDs)
	}
}

func TestCollectHistoryMessageIDs_EmptyResponse(t *testing.T) {
	result := collectHistoryMessageIDs(nil)
	if len(result.FetchIDs) != 0 || len(result.DeletedIDs) != 0 {
		t.Fatalf("expected empty result for nil response")
	}

	result = collectHistoryMessageIDs(&gmail.ListHistoryResponse{})
	if len(result.FetchIDs) != 0 || len(result.DeletedIDs) != 0 {
		t.Fatalf("expected empty result for empty response")
	}
}

func TestParseHistoryTypes(t *testing.T) {
	got, err := parseHistoryTypes([]string{"messageAdded,labelRemoved", "labelsAdded", "messagesDeleted", "messageAdded"})
	if err != nil {
		t.Fatalf("parseHistoryTypes: %v", err)
	}
	want := []string{"messageAdded", "labelRemoved", "labelAdded", "messageDeleted"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected types: %v", got)
	}
	if _, err := parseHistoryTypes([]string{"nope"}); err == nil {
		t.Fatalf("expected error for invalid history type")
	} else if err.Error() != "--history-types must be one of "+gmailHistoryTypesHelp {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseHistoryTypes_CanonicalizationAndDeduplication(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "mixed case and spaces are normalized",
			input: []string{" MessageAdded ", " LABELADDED ", "messagesdeleted "},
			want:  []string{"messageAdded", "labelAdded", "messageDeleted"},
		},
		{
			name:  "duplicates collapsed while preserving order",
			input: []string{"labelsAdded", "LabelAdded", "labelsadded", "messageadded"},
			want:  []string{"labelAdded", "messageAdded"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseHistoryTypes(tt.input)
			if err != nil {
				t.Fatalf("parseHistoryTypes(%v): %v", tt.input, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("unexpected parsed types: %v", got)
			}
		})
	}
}

func TestParseHistoryTypes_DefaultsToMessageAdded(t *testing.T) {
	// When no history types are provided, default to messageAdded for backward compatibility.
	got, err := parseHistoryTypes(nil)
	if err != nil {
		t.Fatalf("parseHistoryTypes(nil): %v", err)
	}
	want := []string{"messageAdded"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected default %v, got %v", want, got)
	}

	// Empty slice should also default to messageAdded.
	got, err = parseHistoryTypes([]string{})
	if err != nil {
		t.Fatalf("parseHistoryTypes([]string{}): %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected default %v, got %v", want, got)
	}
}

func TestParseHistoryTypes_EmptyStringsInInput(t *testing.T) {
	// Test that empty strings between commas are handled correctly.
	got, err := parseHistoryTypes([]string{"messageAdded,,labelAdded"})
	if err != nil {
		t.Fatalf("parseHistoryTypes with empty strings: %v", err)
	}
	want := []string{"messageAdded", "labelAdded"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}

	// Test leading/trailing empty parts.
	got, err = parseHistoryTypes([]string{",messageAdded,", ",labelRemoved,"})
	if err != nil {
		t.Fatalf("parseHistoryTypes with leading/trailing empty: %v", err)
	}
	want = []string{"messageAdded", "labelRemoved"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}

	// Test whitespace-only parts.
	got, err = parseHistoryTypes([]string{"messageAdded, ,labelAdded"})
	if err != nil {
		t.Fatalf("parseHistoryTypes with whitespace: %v", err)
	}
	want = []string{"messageAdded", "labelAdded"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestParseHistoryTypes_RejectsEmptyOnlyInput(t *testing.T) {
	expect := "--history-types must include at least one value"
	if _, err := parseHistoryTypes([]string{","}); err == nil {
		t.Fatalf("expected error for empty-only history types")
	} else if err.Error() != expect {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := parseHistoryTypes([]string{" , "}); err == nil {
		t.Fatalf("expected error for whitespace-only history types")
	} else if err.Error() != expect {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := parseHistoryTypes([]string{", ,"}); err == nil {
		t.Fatalf("expected error for comma/whitespace-only history types")
	} else if err.Error() != expect {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecodeGmailPushPayload(t *testing.T) {
	payload := `{"emailAddress":"a@b.com","historyId":"123"}`
	env := &pubsubPushEnvelope{}
	env.Message.Data = base64.StdEncoding.EncodeToString([]byte(payload))

	got, err := decodeGmailPushPayload(env)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.EmailAddress != "a@b.com" || got.HistoryID != "123" {
		t.Fatalf("unexpected payload: %#v", got)
	}

	env.Message.Data = base64.RawStdEncoding.EncodeToString([]byte(payload))
	if _, err := decodeGmailPushPayload(env); err != nil {
		t.Fatalf("decode raw: %v", err)
	}
}

func TestSharedTokenAndBearerEdgeCases(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/hook?token=query", nil)
	if sharedTokenMatches(r, "") {
		t.Fatalf("expected false for empty expected token")
	}
	if sharedTokenMatches(r, "nope") {
		t.Fatalf("expected false for mismatch")
	}
	if !sharedTokenMatches(r, "query") {
		t.Fatalf("expected query token match")
	}

	if got := bearerToken(&http.Request{}); got != "" {
		t.Fatalf("expected empty bearer")
	}
	if got := bearerToken(&http.Request{Header: http.Header{"Authorization": []string{"token abc"}}}); got != "" {
		t.Fatalf("expected empty bearer for non-bearer scheme")
	}
	if got := bearerToken(&http.Request{Header: http.Header{"Authorization": []string{"Bearer"}}}); got != "" {
		t.Fatalf("expected empty bearer for missing token")
	}
}

func TestIsStaleHistoryError_MoreCases(t *testing.T) {
	if !isStaleHistoryError(&googleapi.Error{Code: http.StatusBadRequest, Message: "History too old"}) {
		t.Fatalf("expected stale history error")
	}
	if !isStaleHistoryError(&googleapi.Error{Code: http.StatusNotFound, Message: "Requested entity was not found."}) {
		t.Fatalf("expected stale history error for not found")
	}
	if !isStaleHistoryError(errors.New("missing history")) {
		t.Fatalf("expected stale history error from message")
	}
	if isStaleHistoryError(errors.New("other")) {
		t.Fatalf("expected non-stale history error")
	}
}

func TestIsNotFoundAPIError(t *testing.T) {
	if !isNotFoundAPIError(&googleapi.Error{Code: http.StatusNotFound}) {
		t.Fatalf("expected not found error")
	}
	if isNotFoundAPIError(&googleapi.Error{
		Code: http.StatusForbidden,
		Errors: []googleapi.ErrorItem{
			{Reason: "notFound"},
		},
	}) {
		t.Fatalf("expected non-notfound for forbidden")
	}
}

func TestVerifyOIDCToken_NoValidator_Error(t *testing.T) {
	ok, err := verifyOIDCToken(context.Background(), nil, "tok", "aud", "")
	if err == nil || ok {
		t.Fatalf("expected error without validator")
	}
}

func TestVerifyOIDCToken_InvalidToken(t *testing.T) {
	validator, err := idtoken.NewValidator(context.Background())
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	ok, err := verifyOIDCToken(context.Background(), validator, "not-a-token", "aud", "")
	if ok || err == nil {
		t.Fatalf("expected error, got ok=%v err=%v", ok, err)
	}
}

func TestAuthorizeVariants(t *testing.T) {
	s := &gmailWatchServer{
		cfg:   gmailWatchServeConfig{},
		warnf: func(string, ...any) {},
	}
	req := httptest.NewRequest(http.MethodPost, "/hook", nil)
	if !s.authorize(req) {
		t.Fatalf("expected authorize when no shared token")
	}

	s = &gmailWatchServer{
		cfg:   gmailWatchServeConfig{SharedToken: "tok"},
		warnf: func(string, ...any) {},
	}
	req = httptest.NewRequest(http.MethodPost, "/hook?token=bad", nil)
	if s.authorize(req) {
		t.Fatalf("expected shared token mismatch")
	}

	s = &gmailWatchServer{
		cfg:   gmailWatchServeConfig{VerifyOIDC: true, SharedToken: "tok"},
		warnf: func(string, ...any) {},
	}
	req = httptest.NewRequest(http.MethodPost, "/hook?token=tok", nil)
	req.Header.Set("Authorization", "Bearer abc")
	if !s.authorize(req) {
		t.Fatalf("expected shared token fallback with oidc")
	}

	s = &gmailWatchServer{
		cfg:   gmailWatchServeConfig{VerifyOIDC: true},
		warnf: func(string, ...any) {},
	}
	req = httptest.NewRequest(http.MethodPost, "/hook", nil)
	if s.authorize(req) {
		t.Fatalf("expected oidc authorization failure without token")
	}
}
