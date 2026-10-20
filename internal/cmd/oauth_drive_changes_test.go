package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	gogapi "github.com/openclaw/gogcli/internal/googleapi"
)

func newDriveChangesOAuthService(t *testing.T, allowWatch bool) (*drive.Service, *atomic.Int32) {
	t.Helper()
	var tokenCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/token":
			tokenCalls.Add(1)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, `{"error":"invalid_grant","error_description":%q}`, oauthOutputSecret)
		case allowWatch && r.Method == http.MethodPost && r.URL.Path == "/changes/watch":
			var channel drive.Channel
			if err := json.NewDecoder(r.Body).Decode(&channel); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": channel.Id, "resourceId": "synthetic-new-resource",
				"expiration": fmt.Sprint(time.Now().Add(defaultDriveChangesChannelTTL).UnixMilli()),
			})
		default:
			t.Errorf("unexpected API request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	cfg := &oauth2.Config{Endpoint: oauth2.Endpoint{TokenURL: srv.URL + "/token", AuthStyle: oauth2.AuthStyleInParams}}
	refresh := cfg.TokenSource(t.Context(), &oauth2.Token{RefreshToken: "synthetic-refresh-token"})
	first := true
	source := oauthOutputTokenSourceFunc(func() (*oauth2.Token, error) {
		if first && allowWatch {
			first = false
			return &oauth2.Token{AccessToken: "synthetic-access-token"}, nil
		}
		return refresh.Token()
	})
	client := &http.Client{Transport: gogapi.NewRetryTransport(&oauth2.Transport{Source: source})}
	svc, err := drive.NewService(t.Context(), option.WithHTTPClient(client), option.WithEndpoint(srv.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	return svc, &tokenCalls
}

func TestDriveChangesOAuthNotificationWarning(t *testing.T) {
	svc, tokenCalls := newDriveChangesOAuthService(t, false)
	server := newDriveChangesTestReceiver(t, svc, driveChangesServeState{
		Version: pollStateVersion, PageToken: pollTestStartToken,
		Channel: &driveChangesServeChannelState{ID: "channel-1", ResourceID: "resource-1"},
	})
	server.autoRenew = true
	var warnings []string
	server.warnf = func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) }
	for range 2 {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, newDriveChangesNotificationRequest(t, driveChangesTestChannelToken, "change", 2))
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", response.Code)
		}
	}
	if tokenCalls.Load() != 2 || len(warnings) != 2 || server.state.PageToken != pollTestStartToken || len(server.state.LastMessageNumbers) != 0 {
		t.Fatal("failed notifications must remain retryable without advancing state")
	}
	for _, warning := range warnings {
		if strings.Contains(warning, oauthOutputSecret) || !strings.Contains(warning, "invalid_grant") {
			t.Fatalf("unsafe or missing notification diagnostic: %s", warning)
		}
	}
}

func TestDriveChangesOAuthRenewalWarning(t *testing.T) {
	svc, tokenCalls := newDriveChangesOAuthService(t, false)
	server := newDriveChangesTestReceiver(t, svc, driveChangesServeState{Version: pollStateVersion, PageToken: pollTestStartToken})
	server.webhookURL = "https://example.invalid/changes"
	var waits []time.Duration
	server.runtime.wait = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		if len(waits) > 1 {
			return context.Canceled
		}
		return nil
	}
	var warning string
	server.warnf = func(format string, args ...any) { warning = fmt.Sprintf(format, args...) }
	server.runRenewLoop(t.Context(), time.Second)
	if tokenCalls.Load() != 1 || len(waits) != 2 || waits[1] != driveChangesRenewRetry || server.pendingChannel != "" || server.state.Channel != nil {
		t.Fatal("failed renewal must retain the existing retry/channel behavior")
	}
	if strings.Contains(warning, oauthOutputSecret) || !strings.Contains(warning, "invalid_grant") {
		t.Fatalf("unsafe or missing renewal diagnostic: %s", warning)
	}
}

func TestDriveChangesOAuthCleanupWarning(t *testing.T) {
	for _, afterRenewal := range []bool{false, true} {
		t.Run(fmt.Sprint(afterRenewal), func(t *testing.T) {
			svc, tokenCalls := newDriveChangesOAuthService(t, afterRenewal)
			state := driveChangesServeState{
				Version: pollStateVersion, PageToken: pollTestStartToken,
				Channel: &driveChangesServeChannelState{
					ID: "synthetic-old-channel", ResourceID: "synthetic-old-resource",
					Expiration: time.Now().Add(defaultDriveChangesChannelTTL).UnixMilli(),
					WebhookURL: "https://example.invalid/changes", TokenHash: channelTokenHash(driveChangesTestChannelToken),
				},
			}
			if afterRenewal {
				state.Channel.Expiration = time.Now().Add(-time.Hour).UnixMilli()
			} else {
				state.PreviousChannel = &driveChangesServeChannelState{ID: "synthetic-previous-channel", ResourceID: "synthetic-previous-resource"}
			}
			server := newDriveChangesTestReceiver(t, svc, state)
			server.statePath = filepath.Join(t.TempDir(), "state.json")
			server.webhookURL = "https://example.invalid/changes"
			var warning string
			server.warnf = func(format string, args ...any) { warning = fmt.Sprintf(format, args...) }
			delay, err := server.ensureChannel(t.Context())
			if err != nil || delay <= 0 || tokenCalls.Load() != 1 || server.state.PreviousChannel == nil || server.state.PageToken != pollTestStartToken {
				t.Fatalf("cleanup must remain pending: delay=%s, error=%v", delay, err)
			}
			if !afterRenewal && delay != driveChangesRenewRetry {
				t.Fatalf("cleanup retry delay = %s", delay)
			}
			if strings.Contains(warning, oauthOutputSecret) || !strings.Contains(warning, "invalid_grant") {
				t.Fatalf("unsafe or missing cleanup diagnostic: %s", warning)
			}
		})
	}
}
