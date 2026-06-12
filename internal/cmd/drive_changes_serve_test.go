package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/api/drive/v3"
)

const (
	driveChangesTestChannelToken = "channel-secret"
	driveChangesTestStatePath    = "/drive-changes"
)

func TestDriveChangesServeRejectsWrongTokenBeforeAPI(t *testing.T) {
	server := newDriveChangesTestReceiver(t, nil, driveChangesServeState{
		Version:   pollStateVersion,
		PageToken: pollTestStartToken,
	})
	var hookCalls atomic.Int32
	server.runtime.runHook = func(context.Context, string, any) error {
		hookCalls.Add(1)
		return nil
	}

	request := newDriveChangesNotificationRequest(t, "wrong", "change", 2)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	if hookCalls.Load() != 0 {
		t.Fatalf("hook calls = %d, want 0", hookCalls.Load())
	}
}

func TestParseDriveChangesNotificationRejectsOversizedOptionalHeaders(t *testing.T) {
	for _, tc := range []struct {
		name   string
		header string
		value  string
	}{
		{name: "changed", header: "X-Goog-Changed", value: strings.Repeat("x", 1025)},
		{name: "expiration", header: "X-Goog-Channel-Expiration", value: strings.Repeat("x", 257)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := newDriveChangesNotificationRequest(t, driveChangesTestChannelToken, "change", 2)
			request.Header.Set(tc.header, tc.value)
			if _, err := parseDriveChangesNotification(request); err == nil || !strings.Contains(err.Error(), "too long") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestDriveChangesServeRejectsUntrackedChannel(t *testing.T) {
	state := driveChangesServeState{
		Version:   pollStateVersion,
		PageToken: pollTestStartToken,
		Channel: &driveChangesServeChannelState{
			ID:         "expected-channel",
			ResourceID: "expected-resource",
		},
	}
	server := newDriveChangesTestReceiver(t, nil, state)
	server.autoRenew = true
	var hookCalls atomic.Int32
	server.onChange = "./handle-change"
	server.runtime.runHook = func(context.Context, string, any) error {
		hookCalls.Add(1)
		return nil
	}

	request := newDriveChangesNotificationRequest(t, driveChangesTestChannelToken, "change", 2)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	if hookCalls.Load() != 0 {
		t.Fatalf("hook calls = %d, want 0", hookCalls.Load())
	}
	if server.state.PageToken != pollTestStartToken || len(server.state.LastMessageNumbers) != 0 {
		t.Fatalf("state changed: %#v", server.state)
	}
}

func TestDriveChangesServeAcceptsPreviousAndPendingChannels(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	state := driveChangesServeState{
		Version:   pollStateVersion,
		PageToken: pollTestStartToken,
		Channel: &driveChangesServeChannelState{
			ID:         "current-channel",
			ResourceID: "current-resource",
		},
		PreviousChannel: &driveChangesServeChannelState{
			ID:         "channel-1",
			ResourceID: "resource-1",
		},
	}
	if err := writePollState(statePath, state); err != nil {
		t.Fatalf("write state: %v", err)
	}
	server := newDriveChangesTestReceiver(t, nil, state)
	server.autoRenew = true
	server.statePath = statePath

	previous := newDriveChangesNotificationRequest(t, driveChangesTestChannelToken, "sync", 1)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, previous)
	if response.Code != http.StatusNoContent {
		t.Fatalf("previous channel status = %d, want 204", response.Code)
	}

	server.mu.Lock()
	server.pendingChannel = "pending-channel"
	server.mu.Unlock()
	pending := newDriveChangesNotificationRequest(t, driveChangesTestChannelToken, "sync", 1)
	pending.Header.Set("X-Goog-Channel-ID", "pending-channel")
	pending.Header.Set("X-Goog-Resource-ID", "pending-resource")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, pending)
	if response.Code != http.StatusNoContent {
		t.Fatalf("pending channel status = %d, want 204", response.Code)
	}
}

func TestDriveChangesServeSyncDoesNotLowerMessageNumber(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	state := driveChangesServeState{
		Version:   pollStateVersion,
		PageToken: pollTestStartToken,
		Channel: &driveChangesServeChannelState{
			ID:         "channel-1",
			ResourceID: "resource-1",
		},
		LastMessageNumbers: map[string]uint64{
			driveChangesMessageKey("channel-1", "resource-1"): 99,
		},
	}
	if err := writePollState(statePath, state); err != nil {
		t.Fatalf("write state: %v", err)
	}
	server := newDriveChangesTestReceiver(t, nil, state)
	server.statePath = statePath

	response := httptest.NewRecorder()
	server.ServeHTTP(response, newDriveChangesNotificationRequest(t, driveChangesTestChannelToken, "sync", 1))
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
	persisted, _, err := readDriveChangesServeState(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if got := persisted.LastMessageNumbers[driveChangesMessageKey("channel-1", "resource-1")]; got != 99 {
		t.Fatalf("message number = %d, want 99", got)
	}
}

func TestDriveChangesServeAcknowledgesSyncAndDuplicates(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	state := driveChangesServeState{
		Version:   pollStateVersion,
		PageToken: pollTestStartToken,
		UpdatedAt: "2026-06-11T10:00:00Z",
	}
	if err := writePollState(statePath, state); err != nil {
		t.Fatalf("write state: %v", err)
	}
	server := newDriveChangesTestReceiver(t, nil, state)
	server.statePath = statePath

	for _, tc := range []struct {
		resourceState string
		messageNumber uint64
	}{
		{resourceState: "sync", messageNumber: 1},
		{resourceState: "sync", messageNumber: 1},
	} {
		request := newDriveChangesNotificationRequest(t, driveChangesTestChannelToken, tc.resourceState, tc.messageNumber)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d, want 204", tc.resourceState, response.Code)
		}
	}

	persisted, _, err := readDriveChangesServeState(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if persisted.PageToken != pollTestStartToken {
		t.Fatalf("page token = %q", persisted.PageToken)
	}
	messageKey := driveChangesMessageKey("channel-1", "resource-1")
	if persisted.LastMessageNumbers[messageKey] != 1 {
		t.Fatalf("message number = %d, want 1", persisted.LastMessageNumbers[messageKey])
	}
}

func TestDriveChangesServeProcessesEveryNonSyncResourceState(t *testing.T) {
	for _, resourceState := range []string{"add", "remove", "update", "trash", "untrash", "change", "future-state"} {
		t.Run(resourceState, func(t *testing.T) {
			svc, closeDrive := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"newStartPageToken": "next-token",
					"changes":           []map[string]any{{"fileId": "file-1"}},
				})
			}))
			defer closeDrive()

			statePath := filepath.Join(t.TempDir(), "state.json")
			state := driveChangesServeState{
				Version:   pollStateVersion,
				PageToken: pollTestStartToken,
			}
			if err := writePollState(statePath, state); err != nil {
				t.Fatalf("write state: %v", err)
			}
			server := newDriveChangesTestReceiver(t, svc, state)
			server.statePath = statePath

			response := httptest.NewRecorder()
			server.ServeHTTP(response, newDriveChangesNotificationRequest(t, driveChangesTestChannelToken, resourceState, 2))
			if response.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want 204", response.Code)
			}
			persisted, _, err := readDriveChangesServeState(statePath)
			if err != nil {
				t.Fatalf("read state: %v", err)
			}
			if persisted.PageToken != "next-token" {
				t.Fatalf("page token = %q, want next-token", persisted.PageToken)
			}
		})
	}
}

func TestDriveChangesServeProcessesNotificationOverHTTP(t *testing.T) {
	svc, closeDrive := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/changes" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		requireQuery(t, r, "pageToken", pollTestStartToken)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"newStartPageToken": "next-token",
			"changes": []map[string]any{{
				"fileId": "file-1",
				"type":   "file",
				"file":   map[string]any{"name": "Doc"},
			}},
		})
	}))
	defer closeDrive()

	statePath := filepath.Join(t.TempDir(), "state.json")
	state := driveChangesServeState{
		Version:   pollStateVersion,
		PageToken: pollTestStartToken,
		UpdatedAt: "2026-06-11T10:00:00Z",
	}
	if err := writePollState(statePath, state); err != nil {
		t.Fatalf("write state: %v", err)
	}
	server := newDriveChangesTestReceiver(t, svc, state)
	server.statePath = statePath
	server.onChange = "./handle-change"
	var event driveChangesServeEvent
	server.runtime.runHook = func(_ context.Context, command string, payload any) error {
		if command != "./handle-change" {
			t.Fatalf("command = %q", command)
		}
		event = payload.(driveChangesServeEvent)
		return nil
	}

	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	request := newDriveChangesNotificationRequest(t, driveChangesTestChannelToken, "change", 7)
	request.URL.Scheme = "http"
	request.URL.Host = strings.TrimPrefix(httpServer.URL, "http://")
	request.RequestURI = ""
	response, err := httpServer.Client().Do(request)
	if err != nil {
		t.Fatalf("notification request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.StatusCode)
	}
	if event.MessageNumber != 7 || event.PageToken != pollTestStartToken || event.NextPageToken != "next-token" {
		t.Fatalf("unexpected event: %#v", event)
	}
	if len(event.Changes) != 1 || event.Changes[0].FileId != "file-1" {
		t.Fatalf("unexpected changes: %#v", event.Changes)
	}
	persisted, _, err := readDriveChangesServeState(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if persisted.PageToken != "next-token" ||
		persisted.LastMessageNumbers[driveChangesMessageKey("channel-1", "resource-1")] != 7 {
		t.Fatalf("unexpected state: %#v", persisted)
	}
}

func TestDriveChangesServeProcessingSurvivesRequestCancellation(t *testing.T) {
	svc, closeDrive := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"newStartPageToken": "next-token",
			"changes":           []map[string]any{{"fileId": "file-1"}},
		})
	}))
	defer closeDrive()

	statePath := filepath.Join(t.TempDir(), "state.json")
	state := driveChangesServeState{
		Version:   pollStateVersion,
		PageToken: pollTestStartToken,
	}
	if err := writePollState(statePath, state); err != nil {
		t.Fatalf("write state: %v", err)
	}
	server := newDriveChangesTestReceiver(t, svc, state)
	server.statePath = statePath
	server.onChange = "./handle-change"
	server.runtime.runHook = func(ctx context.Context, _ string, _ any) error {
		if err := ctx.Err(); err != nil {
			t.Fatalf("hook context canceled with request: %v", err)
		}
		return nil
	}

	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()
	request := newDriveChangesNotificationRequest(t, driveChangesTestChannelToken, "change", 8).WithContext(requestCtx)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
	persisted, _, err := readDriveChangesServeState(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if persisted.PageToken != "next-token" ||
		persisted.LastMessageNumbers[driveChangesMessageKey("channel-1", "resource-1")] != 8 {
		t.Fatalf("unexpected state: %#v", persisted)
	}
}

func TestDriveChangesServeProcessingStopsOnCommandCancellation(t *testing.T) {
	runDone := make(chan struct{})
	server := newDriveChangesTestReceiver(t, nil, driveChangesServeState{})
	server.runDone = runDone
	ctx, cancel := server.notificationContext(context.Background())
	defer cancel()

	close(runDone)
	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("context error = %v", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("processing context did not stop with command context")
	}
}

func TestDriveChangesServeNotificationGateHonorsTimeout(t *testing.T) {
	server := newDriveChangesTestReceiver(t, nil, driveChangesServeState{})
	if err := server.acquireNotification(context.Background()); err != nil {
		t.Fatalf("acquire first notification: %v", err)
	}
	defer server.releaseNotification()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := server.acquireNotification(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued notification error = %v, want deadline exceeded", err)
	}
}

func TestDriveChangesServeFilterSkipsHookButAdvancesState(t *testing.T) {
	svc, closeDrive := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"newStartPageToken": "next-token",
			"changes":           []map[string]any{{"fileId": "other"}},
		})
	}))
	defer closeDrive()

	statePath := filepath.Join(t.TempDir(), "state.json")
	state := driveChangesServeState{
		Version:   pollStateVersion,
		PageToken: pollTestStartToken,
	}
	if err := writePollState(statePath, state); err != nil {
		t.Fatalf("write state: %v", err)
	}
	server := newDriveChangesTestReceiver(t, svc, state)
	server.statePath = statePath
	server.filterFile = "wanted"
	server.onChange = "./handle-change"
	var hookCalls atomic.Int32
	server.runtime.runHook = func(context.Context, string, any) error {
		hookCalls.Add(1)
		return nil
	}

	response := httptest.NewRecorder()
	server.ServeHTTP(response, newDriveChangesNotificationRequest(t, driveChangesTestChannelToken, "change", 3))
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
	if hookCalls.Load() != 0 {
		t.Fatalf("hook calls = %d, want 0", hookCalls.Load())
	}
	persisted, _, err := readDriveChangesServeState(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if persisted.PageToken != "next-token" ||
		persisted.LastMessageNumbers[driveChangesMessageKey("channel-1", "resource-1")] != 3 {
		t.Fatalf("unexpected state: %#v", persisted)
	}
}

func TestDriveChangesServeHookFailureRetainsStateForRetry(t *testing.T) {
	svc, closeDrive := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"newStartPageToken": "next-token",
			"changes":           []map[string]any{{"fileId": "file-1"}},
		})
	}))
	defer closeDrive()

	statePath := filepath.Join(t.TempDir(), "state.json")
	state := driveChangesServeState{
		Version:   pollStateVersion,
		PageToken: pollTestStartToken,
	}
	if err := writePollState(statePath, state); err != nil {
		t.Fatalf("write state: %v", err)
	}
	server := newDriveChangesTestReceiver(t, svc, state)
	server.statePath = statePath
	server.onChange = "./handle-change"
	hookErr := errors.New("hook failed")
	server.runtime.runHook = func(context.Context, string, any) error { return hookErr }

	response := httptest.NewRecorder()
	server.ServeHTTP(response, newDriveChangesNotificationRequest(t, driveChangesTestChannelToken, "change", 4))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
	persisted, _, err := readDriveChangesServeState(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if persisted.PageToken != pollTestStartToken ||
		persisted.LastMessageNumbers[driveChangesMessageKey("channel-1", "resource-1")] != 0 {
		t.Fatalf("unexpected state: %#v", persisted)
	}
}

func TestDriveChangesServeHookDoesNotBlockStateMutex(t *testing.T) {
	svc, closeDrive := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"newStartPageToken": "next-token",
			"changes":           []map[string]any{{"fileId": "file-1"}},
		})
	}))
	defer closeDrive()

	statePath := filepath.Join(t.TempDir(), "state.json")
	state := driveChangesServeState{
		Version:   pollStateVersion,
		PageToken: pollTestStartToken,
	}
	if err := writePollState(statePath, state); err != nil {
		t.Fatalf("write state: %v", err)
	}
	server := newDriveChangesTestReceiver(t, svc, state)
	server.statePath = statePath
	server.onChange = "./handle-change"
	hookStarted := make(chan struct{})
	releaseHook := make(chan struct{})
	server.runtime.runHook = func(context.Context, string, any) error {
		close(hookStarted)
		<-releaseHook
		return nil
	}

	done := make(chan int, 1)
	go func() {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, newDriveChangesNotificationRequest(t, driveChangesTestChannelToken, "change", 5))
		done <- response.Code
	}()
	<-hookStarted

	stateLockAcquired := make(chan struct{})
	go func() {
		server.mu.Lock()
		_ = server.state.PageToken
		server.mu.Unlock()
		close(stateLockAcquired)
	}()
	select {
	case <-stateLockAcquired:
	case <-time.After(time.Second):
		t.Fatal("state mutex remained blocked while hook was running")
	}

	close(releaseHook)
	if code := <-done; code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", code)
	}
}

func TestDriveChangesServeChannelStopDoesNotBlockStateMutex(t *testing.T) {
	stopStarted := make(chan struct{})
	releaseStop := make(chan struct{})
	svc, closeDrive := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/channels/stop" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		close(stopStarted)
		<-releaseStop
		w.WriteHeader(http.StatusNoContent)
	}))
	defer closeDrive()

	statePath := filepath.Join(t.TempDir(), "state.json")
	state := driveChangesServeState{
		Version:   pollStateVersion,
		PageToken: pollTestStartToken,
		PreviousChannel: &driveChangesServeChannelState{
			ID:         "channel-old",
			ResourceID: "resource-old",
		},
	}
	if err := writePollState(statePath, state); err != nil {
		t.Fatalf("write state: %v", err)
	}
	server := newDriveChangesTestReceiver(t, svc, state)
	server.statePath = statePath

	done := make(chan error, 1)
	go func() {
		done <- server.stopPreviousChannel(context.Background())
	}()
	<-stopStarted

	stateLockAcquired := make(chan struct{})
	go func() {
		server.mu.Lock()
		_ = server.state.PageToken
		server.mu.Unlock()
		close(stateLockAcquired)
	}()
	select {
	case <-stateLockAcquired:
	case <-time.After(time.Second):
		t.Fatal("state mutex remained blocked during channel stop")
	}

	close(releaseStop)
	if err := <-done; err != nil {
		t.Fatalf("stop previous channel: %v", err)
	}
}

func TestDriveChangesServeSerializesConcurrentNotifications(t *testing.T) {
	firstAPIStarted := make(chan struct{})
	releaseFirstAPI := make(chan struct{})
	var apiCalls atomic.Int32
	svc, closeDrive := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := apiCalls.Add(1)
		if call == 1 {
			close(firstAPIStarted)
			<-releaseFirstAPI
			requireQuery(t, r, "pageToken", pollTestStartToken)
			_ = json.NewEncoder(w).Encode(map[string]any{"newStartPageToken": "next-1"})
			return
		}
		requireQuery(t, r, "pageToken", "next-1")
		_ = json.NewEncoder(w).Encode(map[string]any{"newStartPageToken": "next-2"})
	}))
	defer closeDrive()

	statePath := filepath.Join(t.TempDir(), "state.json")
	state := driveChangesServeState{
		Version:   pollStateVersion,
		PageToken: pollTestStartToken,
	}
	if err := writePollState(statePath, state); err != nil {
		t.Fatalf("write state: %v", err)
	}
	server := newDriveChangesTestReceiver(t, svc, state)
	server.statePath = statePath

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, newDriveChangesNotificationRequest(t, driveChangesTestChannelToken, "change", 2))
		if response.Code != http.StatusNoContent {
			t.Errorf("first status = %d", response.Code)
		}
	}()
	<-firstAPIStarted

	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, newDriveChangesNotificationRequest(t, driveChangesTestChannelToken, "change", 3))
		if response.Code != http.StatusNoContent {
			t.Errorf("second status = %d", response.Code)
		}
	}()
	time.Sleep(50 * time.Millisecond)
	if apiCalls.Load() != 1 {
		t.Fatalf("API calls before releasing first = %d, want 1", apiCalls.Load())
	}
	close(releaseFirstAPI)
	<-firstDone
	<-secondDone

	persisted, _, err := readDriveChangesServeState(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if persisted.PageToken != "next-2" ||
		persisted.LastMessageNumbers[driveChangesMessageKey("channel-1", "resource-1")] != 3 {
		t.Fatalf("unexpected state: %#v", persisted)
	}
}

func TestDriveChangesServeRenewsAndStopsPreviousChannel(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	var watched drive.Channel
	var stopped drive.Channel
	svc, closeDrive := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/changes/watch":
			requireQuery(t, r, "pageToken", pollTestStartToken)
			if err := json.NewDecoder(r.Body).Decode(&watched); err != nil {
				t.Fatalf("decode watch: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          watched.Id,
				"resourceId":  "resource-new",
				"resourceUri": "https://www.googleapis.com/drive/v3/changes",
				"expiration":  strconv.FormatInt(now.Add(defaultDriveChangesChannelTTL).UnixMilli(), 10),
			})
		case "/channels/stop":
			if err := json.NewDecoder(r.Body).Decode(&stopped); err != nil {
				t.Fatalf("decode stop: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("path = %q", r.URL.Path)
		}
	}))
	defer closeDrive()

	statePath := filepath.Join(t.TempDir(), "state.json")
	state := driveChangesServeState{
		Version:   pollStateVersion,
		PageToken: pollTestStartToken,
		Channel: &driveChangesServeChannelState{
			ID:         "channel-old",
			ResourceID: "resource-old",
			Expiration: now.Add(time.Minute).UnixMilli(),
			WebhookURL: "https://old.example/hook",
			TokenHash:  channelTokenHash(driveChangesTestChannelToken),
		},
	}
	if err := writePollState(statePath, state); err != nil {
		t.Fatalf("write state: %v", err)
	}
	server := newDriveChangesTestReceiver(t, svc, state)
	server.statePath = statePath
	server.autoRenew = true
	server.webhookURL = "https://example.com/hook"
	server.channelTTL = defaultDriveChangesChannelTTL
	server.renewBefore = 10 * time.Minute
	server.runtime.now = func() time.Time { return now }

	delay, err := server.ensureChannel(context.Background())
	if err != nil {
		t.Fatalf("ensureChannel: %v", err)
	}
	if delay != defaultDriveChangesChannelTTL-10*time.Minute {
		t.Fatalf("delay = %v", delay)
	}
	if watched.Address != server.webhookURL || watched.Token != driveChangesTestChannelToken {
		t.Fatalf("unexpected watch request: %#v", watched)
	}
	if watched.Expiration != now.Add(defaultDriveChangesChannelTTL).UnixMilli() {
		t.Fatalf("expiration = %d", watched.Expiration)
	}
	if stopped.Id != "channel-old" || stopped.ResourceId != "resource-old" {
		t.Fatalf("unexpected stopped channel: %#v", stopped)
	}
	persisted, _, err := readDriveChangesServeState(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if persisted.Channel == nil || persisted.Channel.ResourceID != "resource-new" || persisted.PreviousChannel != nil {
		t.Fatalf("unexpected channel state: %#v", persisted)
	}
	if persisted.Channel.TokenHash == driveChangesTestChannelToken || persisted.Channel.TokenHash == "" {
		t.Fatalf("channel token was not hashed")
	}
}

func TestDriveChangesServeBacksOffWhenGrantedExpirationIsInsideRenewalWindow(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	svc, closeDrive := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/changes/watch":
			var watched drive.Channel
			if err := json.NewDecoder(r.Body).Decode(&watched); err != nil {
				t.Fatalf("decode watch: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         watched.Id,
				"resourceId": "resource-new",
				"expiration": strconv.FormatInt(now.Add(5*time.Minute).UnixMilli(), 10),
			})
		default:
			t.Fatalf("path = %q", r.URL.Path)
		}
	}))
	defer closeDrive()

	statePath := filepath.Join(t.TempDir(), "state.json")
	state := driveChangesServeState{
		Version:   pollStateVersion,
		PageToken: pollTestStartToken,
	}
	if err := writePollState(statePath, state); err != nil {
		t.Fatalf("write state: %v", err)
	}
	server := newDriveChangesTestReceiver(t, svc, state)
	server.statePath = statePath
	server.autoRenew = true
	server.webhookURL = "https://example.com/hook"
	server.channelTTL = defaultDriveChangesChannelTTL
	server.renewBefore = 10 * time.Minute
	server.runtime.now = func() time.Time { return now }

	delay, err := server.ensureChannel(context.Background())
	if err != nil {
		t.Fatalf("ensureChannel: %v", err)
	}
	if delay != driveChangesRenewRetry {
		t.Fatalf("delay = %v, want %v", delay, driveChangesRenewRetry)
	}
}

func TestDriveChangesServeRunRetriesPendingCleanupAtStartup(t *testing.T) {
	svc, closeDrive := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/channels/stop" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		http.Error(w, "temporary failure", http.StatusInternalServerError)
	}))
	defer closeDrive()

	statePath := filepath.Join(t.TempDir(), "state.json")
	if err := writePollState(statePath, driveChangesServeState{
		Version:   pollStateVersion,
		PageToken: pollTestStartToken,
		PreviousChannel: &driveChangesServeChannelState{
			ID:         "channel-old",
			ResourceID: "resource-old",
		},
	}); err != nil {
		t.Fatalf("write state: %v", err)
	}

	ctx := withDriveTestService(newCmdOutputContext(t, io.Discard, io.Discard), svc)
	ctx, cancel := context.WithCancel(ctx)
	retryScheduled := make(chan struct{})
	cmd := DriveChangesServeCmd{
		Listen:              "127.0.0.1:0",
		Path:                driveChangesTestStatePath,
		ChannelToken:        driveChangesTestChannelToken,
		StateFile:           statePath,
		Max:                 100,
		IncludeRemoved:      true,
		AutoRenew:           true,
		WebhookURL:          "https://example.com/drive-changes",
		ChannelTTL:          defaultDriveChangesChannelTTL,
		RenewBefore:         10 * time.Minute,
		NotificationTimeout: defaultDriveChangesNotificationTimeout,
	}
	runtime := defaultDriveChangesServeRuntime()
	runtime.wait = func(ctx context.Context, delay time.Duration) error {
		if delay != driveChangesRenewRetry {
			t.Errorf("retry delay = %v, want %v", delay, driveChangesRenewRetry)
		}
		close(retryScheduled)
		<-ctx.Done()
		return ctx.Err()
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.run(ctx, &RootFlags{Account: "a@example.com"}, runtime)
	}()
	<-retryScheduled
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context canceled", err)
	}
}

func TestDriveChangesServeValidation(t *testing.T) {
	t.Setenv("GOG_DRIVE_CHANNEL_TOKEN", "")
	valid := DriveChangesServeCmd{
		Listen:              "127.0.0.1:8443",
		Path:                driveChangesTestStatePath,
		ChannelToken:        driveChangesTestChannelToken,
		StateFile:           "state.json",
		Max:                 100,
		IncludeRemoved:      true,
		ChannelTTL:          defaultDriveChangesChannelTTL,
		RenewBefore:         10 * time.Minute,
		NotificationTimeout: defaultDriveChangesNotificationTimeout,
	}
	cases := []struct {
		name   string
		mutate func(*DriveChangesServeCmd)
		want   string
	}{
		{name: "missing token", mutate: func(c *DriveChangesServeCmd) { c.ChannelToken = "" }, want: "channel-token"},
		{name: "cert without key", mutate: func(c *DriveChangesServeCmd) { c.Cert = "cert.pem" }, want: "--cert and --key"},
		{name: "bad path", mutate: func(c *DriveChangesServeCmd) { c.Path = "hook" }, want: "--path"},
		{name: "renew without url", mutate: func(c *DriveChangesServeCmd) { c.AutoRenew = true }, want: "--webhook-url"},
		{name: "url without renew", mutate: func(c *DriveChangesServeCmd) { c.WebhookURL = "https://example.com" }, want: "requires --auto-renew"},
		{
			name: "ttl too long",
			mutate: func(c *DriveChangesServeCmd) {
				c.AutoRenew = true
				c.WebhookURL = "https://example.com/hook"
				c.ChannelTTL = maxDriveChangesChannelTTL + time.Second
			},
			want: "--channel-ttl",
		},
		{
			name: "renew after expiration",
			mutate: func(c *DriveChangesServeCmd) {
				c.AutoRenew = true
				c.WebhookURL = "https://example.com/hook"
				c.RenewBefore = c.ChannelTTL
			},
			want: "--renew-before",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := valid
			tc.mutate(&cmd)
			token, err := cmd.resolveChannelToken()
			if err == nil {
				err = cmd.validate(token)
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestDriveChangesServeResolvesChannelTokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "channel-token")
	if err := os.WriteFile(path, []byte(" file-secret \n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	t.Setenv("GOG_DRIVE_CHANNEL_TOKEN", "ambient-secret")
	cmd := DriveChangesServeCmd{ChannelTokenFile: path}
	token, err := cmd.resolveChannelToken()
	if err != nil {
		t.Fatalf("resolve channel token: %v", err)
	}
	if token != "file-secret" {
		t.Fatalf("token = %q", token)
	}

	cmd.ChannelToken = "direct-secret"
	if _, err := cmd.resolveChannelToken(); err == nil || !strings.Contains(err.Error(), "only one") {
		t.Fatalf("combined source error = %v", err)
	}
}

func TestDriveChangesStateKindsRejectCrossUse(t *testing.T) {
	dir := t.TempDir()
	servePath := filepath.Join(dir, "serve.json")
	if err := writePollState(servePath, driveChangesServeState{
		Version:   pollStateVersion,
		Kind:      driveChangesServeStateKind,
		PageToken: pollTestStartToken,
	}); err != nil {
		t.Fatalf("write serve state: %v", err)
	}
	if _, _, err := readDriveChangesPollState(servePath); err == nil || !strings.Contains(err.Error(), "belongs to drive changes serve") {
		t.Fatalf("poll reader error = %v", err)
	}

	pollPath := filepath.Join(dir, "poll.json")
	if err := writePollState(pollPath, driveChangesPollState{
		Version:   pollStateVersion,
		Kind:      driveChangesPollStateKind,
		PageToken: pollTestStartToken,
	}); err != nil {
		t.Fatalf("write poll state: %v", err)
	}
	if _, _, err := readDriveChangesServeState(pollPath); err == nil || !strings.Contains(err.Error(), "belongs to drive changes poll") {
		t.Fatalf("serve reader error = %v", err)
	}
}

func TestExpandDriveChangesTLSPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	certPath, keyPath, err := expandDriveChangesTLSPaths("~/tls/cert.pem", "~/tls/key.pem")
	if err != nil {
		t.Fatalf("expand TLS paths: %v", err)
	}
	if certPath != filepath.Join(home, "tls", "cert.pem") {
		t.Fatalf("cert path = %q", certPath)
	}
	if keyPath != filepath.Join(home, "tls", "key.pem") {
		t.Fatalf("key path = %q", keyPath)
	}
}

func TestDriveChangesServeRejectsInvalidTLSKeyPair(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, []byte("not a certificate"), 0o600); err != nil {
		t.Fatalf("write certificate: %v", err)
	}
	if err := os.WriteFile(keyPath, []byte("not a private key"), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}
	cmd := DriveChangesServeCmd{
		Listen:              "127.0.0.1:0",
		Path:                driveChangesTestStatePath,
		Cert:                certPath,
		Key:                 keyPath,
		ChannelToken:        driveChangesTestChannelToken,
		StateFile:           filepath.Join(dir, "state.json"),
		Max:                 100,
		IncludeRemoved:      true,
		ChannelTTL:          defaultDriveChangesChannelTTL,
		RenewBefore:         10 * time.Minute,
		NotificationTimeout: defaultDriveChangesNotificationTimeout,
	}
	err := cmd.run(
		newCmdOutputContext(t, io.Discard, io.Discard),
		&RootFlags{Account: "a@example.com"},
		defaultDriveChangesServeRuntime(),
	)
	if err == nil || !strings.Contains(err.Error(), "load TLS certificate") {
		t.Fatalf("error = %v", err)
	}
}

func TestDriveChangesServeRunShutsDownOnContextCancel(t *testing.T) {
	svc, closeDrive := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer closeDrive()

	ctx := withDriveTestService(newCmdOutputContext(t, io.Discard, io.Discard), svc)
	ctx, cancel := context.WithCancel(ctx)
	listening := make(chan struct{})
	cmd := DriveChangesServeCmd{
		Listen:              "127.0.0.1:0",
		Path:                driveChangesTestStatePath,
		ChannelToken:        driveChangesTestChannelToken,
		StateFile:           filepath.Join(t.TempDir(), "state.json"),
		Token:               pollTestStartToken,
		Max:                 100,
		IncludeRemoved:      true,
		ChannelTTL:          defaultDriveChangesChannelTTL,
		RenewBefore:         10 * time.Minute,
		NotificationTimeout: defaultDriveChangesNotificationTimeout,
	}
	runtime := defaultDriveChangesServeRuntime()
	runtime.listen = func(ctx context.Context, network string, address string) (net.Listener, error) {
		var listenConfig net.ListenConfig
		listener, err := listenConfig.Listen(ctx, network, address)
		if err == nil {
			close(listening)
		}
		return listener, err
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.run(ctx, &RootFlags{Account: "a@example.com"}, runtime)
	}()
	<-listening
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context canceled", err)
	}
}

func newDriveChangesTestReceiver(t *testing.T, svc *drive.Service, state driveChangesServeState) *driveChangesServer {
	t.Helper()
	return &driveChangesServer{
		state:               state,
		service:             svc,
		path:                driveChangesTestStatePath,
		channelToken:        driveChangesTestChannelToken,
		max:                 100,
		includeRemoved:      true,
		channelTTL:          defaultDriveChangesChannelTTL,
		renewBefore:         10 * time.Minute,
		notificationTimeout: defaultDriveChangesNotificationTimeout,
		runtime:             defaultDriveChangesServeRuntime(),
		logf:                func(string, ...any) {},
		warnf:               func(string, ...any) {},
	}
}

func newDriveChangesNotificationRequest(t *testing.T, token string, resourceState string, messageNumber uint64) *http.Request {
	t.Helper()
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, driveChangesTestStatePath, nil)
	request.Header.Set("X-Goog-Channel-ID", "channel-1")
	request.Header.Set("X-Goog-Channel-Token", token)
	request.Header.Set("X-Goog-Resource-ID", "resource-1")
	request.Header.Set("X-Goog-Resource-State", resourceState)
	request.Header.Set("X-Goog-Resource-URI", "https://www.googleapis.com/drive/v3/changes")
	request.Header.Set("X-Goog-Message-Number", strconv.FormatUint(messageNumber, 10))
	return request
}
