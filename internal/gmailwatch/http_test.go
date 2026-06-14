package gmailwatch

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPHandlerProcessesPush(t *testing.T) {
	t.Parallel()

	handler := &HTTPHandler{
		Config: HTTPConfig{
			Path:      "/hook",
			Account:   "a@example.com",
			BodyLimit: 1024,
			HasHook:   true,
		},
		Authorize: func(*http.Request) bool {
			return true
		},
		Process: func(_ context.Context, notification Notification) (*ProcessedPayload, error) {
			if notification.HistoryID != "200" || notification.MessageID != "push-1" {
				t.Fatalf("notification = %#v", notification)
			}

			return &ProcessedPayload{Payload: &Payload{HistoryID: "200"}}, nil
		},
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, pushRequest(t, "a@example.com", "200", "push-1"))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestHTTPHandlerMapsNoMessagesAndRateLimit(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0)
	processErr := error(ErrNoNewMessages)
	handler := &HTTPHandler{
		Config: HTTPConfig{Path: "/hook", BodyLimit: 1024, HasHook: true},
		Process: func(context.Context, Notification) (*ProcessedPayload, error) {
			return nil, processErr
		},
		Now: func() time.Time {
			return now
		},
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, pushRequest(t, "", "200", ""))

	if response.Code != http.StatusAccepted {
		t.Fatalf("no-message status = %d", response.Code)
	}

	processErr = &RateLimitError{Until: now.Add(30 * time.Second)}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, pushRequest(t, "", "201", ""))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("rate-limit status = %d", response.Code)
	}

	if response.Header().Get("Retry-After") != "30" {
		t.Fatalf("Retry-After = %q", response.Header().Get("Retry-After"))
	}
}

func TestHTTPHandlerReturnsPayloadWithoutHook(t *testing.T) {
	t.Parallel()

	handler := &HTTPHandler{
		Config: HTTPConfig{
			Path:        "/hook",
			BodyLimit:   1024,
			AllowNoHook: true,
		},
		Process: func(context.Context, Notification) (*ProcessedPayload, error) {
			return &ProcessedPayload{Payload: &Payload{Source: "gmail", HistoryID: "200"}}, nil
		},
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, pushRequest(t, "", "200", ""))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}

	var payload Payload
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	if payload.HistoryID != "200" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestParsePushRejectsLargeAndUnreadableBodies(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", bytes.NewReader(bytes.Repeat([]byte("a"), 11)))
	if _, err := ParsePush(request, 10); !errors.Is(err, ErrPushBodyTooLarge) {
		t.Fatalf("large body error = %v", err)
	}

	request = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", errorReader{})
	if _, err := ParsePush(request, 10); err == nil {
		t.Fatal("ParsePush accepted unreadable body")
	}
}

func TestDecodePushPayloadAcceptsNumericHistoryID(t *testing.T) {
	t.Parallel()

	envelope := &PushEnvelope{}
	envelope.Message.Data = base64.StdEncoding.EncodeToString([]byte(`{"emailAddress":"a@example.com","historyId":123}`))

	payload, err := DecodePushPayload(envelope)
	if err != nil {
		t.Fatalf("DecodePushPayload: %v", err)
	}

	if payload.HistoryID != "123" {
		t.Fatalf("history ID = %q", payload.HistoryID)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed") //nolint:err113 // Test-only reader failure.
}

func (errorReader) Close() error {
	return nil
}

func pushRequest(t *testing.T, account, historyID, messageID string) *http.Request {
	t.Helper()

	payload, err := json.Marshal(PushPayload{EmailAddress: account, HistoryID: historyID})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	envelope := PushEnvelope{}
	envelope.Message.Data = base64.StdEncoding.EncodeToString(payload)
	envelope.Message.MessageID = messageID

	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	return httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/hook", bytes.NewReader(body))
}
