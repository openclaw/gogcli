package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGmailWatchServer_ServeHTTP_Errors(t *testing.T) {
	s := &gmailWatchServer{
		cfg:   gmailWatchServeConfig{Path: "/hook", SharedToken: "tok"},
		logf:  func(string, ...any) {},
		warnf: func(string, ...any) {},
	}

	t.Run("not found", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/other", nil)
		s.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status: %d", rr.Code)
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/hook", nil)
		s.ServeHTTP(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status: %d", rr.Code)
		}
	})

	t.Run("unauthorized", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/hook", nil)
		s.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status: %d", rr.Code)
		}
	})
}

func TestGmailWatchServer_ServeHTTP_InvalidPayload(t *testing.T) {
	s := &gmailWatchServer{
		cfg:   gmailWatchServeConfig{Path: "/hook"},
		logf:  func(string, ...any) {},
		warnf: func(string, ...any) {},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/hook", bytes.NewReader([]byte("nope")))
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: %d", rr.Code)
	}
}

func TestGmailWatchServer_ServeHTTP_EmptyHistoryID(t *testing.T) {
	s := &gmailWatchServer{
		cfg:   gmailWatchServeConfig{Path: "/hook", Account: "a@b.com"},
		store: &gmailWatchStore{},
		logf:  func(string, ...any) {},
		warnf: func(string, ...any) {},
	}

	push := pubsubPushEnvelope{}
	push.Message.Data = base64.StdEncoding.EncodeToString([]byte(`{"emailAddress":"a@b.com","historyId":""}`))
	body, _ := json.Marshal(push)

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/hook", bytes.NewReader(body))
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status: %d", rr.Code)
	}
}

func TestGmailWatchServer_ServeHTTP_EmailMismatch(t *testing.T) {
	s := &gmailWatchServer{
		cfg:   gmailWatchServeConfig{Path: "/hook", Account: "a@b.com"},
		store: &gmailWatchStore{},
		logf:  func(string, ...any) {},
		warnf: func(string, ...any) {},
	}

	push := pubsubPushEnvelope{}
	push.Message.Data = base64.StdEncoding.EncodeToString([]byte(`{"emailAddress":"b@b.com","historyId":"10"}`))
	body, _ := json.Marshal(push)

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/hook", bytes.NewReader(body))
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status: %d", rr.Code)
	}
}

func TestGmailWatchServer_ServeHTTP_InvalidBase64(t *testing.T) {
	s := &gmailWatchServer{
		cfg:   gmailWatchServeConfig{Path: "/hook"},
		logf:  func(string, ...any) {},
		warnf: func(string, ...any) {},
	}

	push := pubsubPushEnvelope{}
	push.Message.Data = "!!!"
	body, _ := json.Marshal(push)

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/hook", bytes.NewReader(body))
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: %d", rr.Code)
	}
}
