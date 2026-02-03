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
	ids := collectHistoryMessageIDs(resp)
	joined := strings.Join(ids, ",")
	if !strings.Contains(joined, "m1") ||
		!strings.Contains(joined, "m2") ||
		!strings.Contains(joined, "m3") ||
		!strings.Contains(joined, "m4") ||
		!strings.Contains(joined, "m5") ||
		!strings.Contains(joined, "m6") {
		t.Fatalf("unexpected ids: %v", ids)
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
