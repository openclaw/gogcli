package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const pollTestStartToken = "start"

func TestDriveChangesListAllStopsAtNewStartPageToken(t *testing.T) {
	var tokens []string
	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/changes" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		token := r.URL.Query().Get("pageToken")
		tokens = append(tokens, token)
		switch token {
		case pollTestStartToken:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"nextPageToken": "page-2",
				"changes":       []map[string]any{{"fileId": "file-1"}},
			})
		case "page-2":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"newStartPageToken": "next-start",
				"changes":           []map[string]any{{"fileId": "file-2"}},
			})
		default:
			t.Fatalf("unexpected page token %q", token)
		}
	}))
	defer closeSrv()

	changes, next, err := loadDriveChanges(context.Background(), svc, pollTestStartToken, driveChangesLoadOptions{
		max:            10,
		includeRemoved: true,
		all:            true,
	})
	if err != nil {
		t.Fatalf("loadDriveChanges: %v", err)
	}
	if next != "next-start" {
		t.Fatalf("next = %q, want next-start", next)
	}
	if len(changes) != 2 {
		t.Fatalf("changes = %d, want 2", len(changes))
	}
	if got := strings.Join(tokens, ","); got != "start,page-2" {
		t.Fatalf("tokens = %q", got)
	}
}

func TestDriveChangesPollPersistsFilteredBatch(t *testing.T) {
	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/changes/startPageToken":
			_ = json.NewEncoder(w).Encode(map[string]any{"startPageToken": pollTestStartToken})
		case "/changes":
			requireQuery(t, r, "pageToken", pollTestStartToken)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"newStartPageToken": "next",
				"changes": []map[string]any{
					{"fileId": "wanted", "type": "file", "file": map[string]any{"name": "Wanted"}},
					{"fileId": "other", "type": "file", "file": map[string]any{"name": "Other"}},
				},
			})
		default:
			t.Fatalf("path = %q", r.URL.Path)
		}
	}))
	defer closeSrv()
	stubDriveServiceForTest(t, svc)

	statePath := filepath.Join(t.TempDir(), "nested", "drive-state.json")
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	var hooked driveChangesPollEvent
	hookCalls := 0
	cmd := DriveChangesPollCmd{
		StateFile:      statePath,
		Interval:       time.Second,
		FilterFile:     "wanted",
		MaxIterations:  1,
		Max:            10,
		IncludeRemoved: true,
	}
	out := captureStdout(t, func() {
		err := cmd.run(
			newCmdJSONOutputContext(t, io.Discard, io.Discard),
			&RootFlags{Account: "a@example.com"},
			pollRuntime{
				now: func() time.Time { return now },
				runHook: func(_ context.Context, _ string, payload any) error {
					hookCalls++
					hooked = payload.(driveChangesPollEvent)
					return nil
				},
			},
		)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	if hookCalls != 1 || len(hooked.Changes) != 1 || hooked.Changes[0].FileId != "wanted" {
		t.Fatalf("hooked = %#v, calls=%d", hooked, hookCalls)
	}
	if strings.Count(out, "\n") != 1 || !json.Valid([]byte(out)) {
		t.Fatalf("expected one NDJSON event, got %q", out)
	}
	state, exists, err := readDriveChangesPollState(statePath)
	if err != nil || !exists {
		t.Fatalf("read state: exists=%t err=%v", exists, err)
	}
	if state.PageToken != "next" {
		t.Fatalf("page token = %q, want next", state.PageToken)
	}
	if os.PathSeparator != '\\' {
		info, statErr := os.Stat(statePath)
		if statErr != nil {
			t.Fatalf("stat state: %v", statErr)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("state mode = %o, want 600", info.Mode().Perm())
		}
	}
	leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(statePath), ".drive-state.json.*.tmp"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("temporary state files remain: %v", leftovers)
	}
}

func TestDriveChangesPollHookFailureRetainsToken(t *testing.T) {
	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/changes" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"newStartPageToken": "next",
			"changes":           []map[string]any{{"fileId": "file-1", "type": "file"}},
		})
	}))
	defer closeSrv()
	stubDriveServiceForTest(t, svc)

	statePath := filepath.Join(t.TempDir(), "drive-state.json")
	initial := driveChangesPollState{
		Version:   pollStateVersion,
		PageToken: pollTestStartToken,
		UpdatedAt: "2026-06-11T10:00:00Z",
	}
	if err := writePollState(statePath, initial); err != nil {
		t.Fatalf("write state: %v", err)
	}
	cmd := DriveChangesPollCmd{
		StateFile:      statePath,
		Interval:       time.Second,
		MaxIterations:  1,
		Max:            10,
		IncludeRemoved: true,
	}
	hookErr := errors.New("hook failed")
	_ = captureStdout(t, func() {
		err := cmd.run(
			newCmdJSONOutputContext(t, io.Discard, io.Discard),
			&RootFlags{Account: "a@example.com"},
			pollRuntime{runHook: func(context.Context, string, any) error { return hookErr }},
		)
		if !errors.Is(err, hookErr) {
			t.Fatalf("run error = %v, want hook error", err)
		}
	})
	state, _, err := readDriveChangesPollState(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if state.PageToken != pollTestStartToken {
		t.Fatalf("page token = %q, want start", state.PageToken)
	}
}

func TestDocsCommentsPollPersistsWatermarkAndSeenIDs(t *testing.T) {
	const baseline = "2026-06-11T10:00:00Z"
	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/files/doc-1/comments" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		requireQuery(t, r, "startModifiedTime", baseline)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"comments": []map[string]any{
				{"id": "c2", "content": "Second", "modifiedTime": "2026-06-11T10:00:01Z"},
				{"id": "c1", "content": "First", "modifiedTime": "2026-06-11T10:00:01Z"},
			},
		})
	}))
	defer closeSrv()
	stubDriveServiceForTest(t, svc)

	statePath := filepath.Join(t.TempDir(), "comments-state.json")
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	var hooked []string
	cmd := DocsCommentsPollCmd{
		DocID:         "doc-1",
		StateFile:     statePath,
		Interval:      time.Second,
		MaxIterations: 1,
		Max:           10,
	}
	out := captureStdout(t, func() {
		err := cmd.run(
			newCmdJSONOutputContext(t, io.Discard, io.Discard),
			&RootFlags{Account: "a@example.com"},
			pollRuntime{
				now: func() time.Time { return now },
				runHook: func(_ context.Context, _ string, payload any) error {
					hooked = append(hooked, payload.(docsCommentPollEvent).Comment.Id)
					return nil
				},
			},
		)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	if got := strings.Join(hooked, ","); got != "c1,c2" {
		t.Fatalf("hook order = %q, want c1,c2", got)
	}
	if strings.Count(out, "\n") != 2 {
		t.Fatalf("expected two NDJSON events, got %q", out)
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if !json.Valid([]byte(line)) {
			t.Fatalf("invalid NDJSON line %q", line)
		}
	}
	state, exists, err := readDocsCommentsPollState(statePath)
	if err != nil || !exists {
		t.Fatalf("read state: exists=%t err=%v", exists, err)
	}
	if state.Watermark != "2026-06-11T10:00:01Z" {
		t.Fatalf("watermark = %q", state.Watermark)
	}
	if got := strings.Join(state.SeenIDs, ","); got != "c1,c2" {
		t.Fatalf("seen IDs = %q", got)
	}
}

func TestDocsCommentsPollSkipsSeenAtInclusiveWatermark(t *testing.T) {
	const watermark = "2026-06-11T10:00:01Z"
	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireQuery(t, r, "startModifiedTime", watermark)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"comments": []map[string]any{
				{"id": "seen", "modifiedTime": watermark},
				{"id": "new-same-time", "modifiedTime": watermark},
				{"id": "new-later", "modifiedTime": "2026-06-11T10:00:02Z"},
			},
		})
	}))
	defer closeSrv()
	stubDriveServiceForTest(t, svc)

	statePath := filepath.Join(t.TempDir(), "comments-state.json")
	if err := writePollState(statePath, docsCommentsPollState{
		Version:   pollStateVersion,
		DocID:     "doc-1",
		Watermark: watermark,
		SeenIDs:   []string{"seen"},
		UpdatedAt: watermark,
	}); err != nil {
		t.Fatalf("write state: %v", err)
	}

	var hooked []string
	cmd := DocsCommentsPollCmd{
		DocID:         "doc-1",
		StateFile:     statePath,
		Interval:      time.Second,
		MaxIterations: 1,
		Max:           10,
	}
	_ = captureStdout(t, func() {
		err := cmd.run(
			newCmdJSONOutputContext(t, io.Discard, io.Discard),
			&RootFlags{Account: "a@example.com"},
			pollRuntime{
				runHook: func(_ context.Context, _ string, payload any) error {
					hooked = append(hooked, payload.(docsCommentPollEvent).Comment.Id)
					return nil
				},
			},
		)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	if got := strings.Join(hooked, ","); got != "new-same-time,new-later" {
		t.Fatalf("hooked = %q", got)
	}
	state, _, err := readDocsCommentsPollState(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if state.Watermark != "2026-06-11T10:00:02Z" || strings.Join(state.SeenIDs, ",") != "new-later" {
		t.Fatalf("unexpected state: %#v", state)
	}
}

func TestDocsCommentsPollAdvancesPastExcludedResolvedComments(t *testing.T) {
	const watermark = "2026-06-11T10:00:00Z"
	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"comments": []map[string]any{{
				"id":           "resolved",
				"modifiedTime": "2026-06-11T10:00:01Z",
				"resolved":     true,
			}},
		})
	}))
	defer closeSrv()
	stubDriveServiceForTest(t, svc)

	statePath := filepath.Join(t.TempDir(), "comments-state.json")
	if err := writePollState(statePath, docsCommentsPollState{
		Version:   pollStateVersion,
		DocID:     "doc-1",
		Watermark: watermark,
		UpdatedAt: watermark,
	}); err != nil {
		t.Fatalf("write state: %v", err)
	}
	hookCalls := 0
	cmd := DocsCommentsPollCmd{
		DocID:         "doc-1",
		StateFile:     statePath,
		Interval:      time.Second,
		MaxIterations: 1,
		Max:           10,
	}
	out := captureStdout(t, func() {
		err := cmd.run(
			newCmdJSONOutputContext(t, io.Discard, io.Discard),
			&RootFlags{Account: "a@example.com"},
			pollRuntime{
				runHook: func(context.Context, string, any) error {
					hookCalls++
					return nil
				},
			},
		)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	if out != "" || hookCalls != 0 {
		t.Fatalf("excluded comment emitted output=%q hooks=%d", out, hookCalls)
	}
	state, _, err := readDocsCommentsPollState(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if state.Watermark != "2026-06-11T10:00:01Z" || strings.Join(state.SeenIDs, ",") != "resolved" {
		t.Fatalf("unexpected state: %#v", state)
	}
}

func TestDocsCommentsPollHookFailureRetainsWatermark(t *testing.T) {
	const watermark = "2026-06-11T10:00:00Z"
	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"comments": []map[string]any{{"id": "c1", "modifiedTime": "2026-06-11T10:00:01Z"}},
		})
	}))
	defer closeSrv()
	stubDriveServiceForTest(t, svc)

	statePath := filepath.Join(t.TempDir(), "comments-state.json")
	initial := docsCommentsPollState{
		Version:   pollStateVersion,
		DocID:     "doc-1",
		Watermark: watermark,
		UpdatedAt: watermark,
	}
	if err := writePollState(statePath, initial); err != nil {
		t.Fatalf("write state: %v", err)
	}
	hookErr := errors.New("hook failed")
	cmd := DocsCommentsPollCmd{
		DocID:         "doc-1",
		StateFile:     statePath,
		Interval:      time.Second,
		MaxIterations: 1,
		Max:           10,
	}
	_ = captureStdout(t, func() {
		err := cmd.run(
			newCmdJSONOutputContext(t, io.Discard, io.Discard),
			&RootFlags{Account: "a@example.com"},
			pollRuntime{runHook: func(context.Context, string, any) error { return hookErr }},
		)
		if !errors.Is(err, hookErr) {
			t.Fatalf("run error = %v, want hook error", err)
		}
	})
	state, _, err := readDocsCommentsPollState(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if state.Watermark != watermark {
		t.Fatalf("watermark = %q, want %q", state.Watermark, watermark)
	}
}

func TestWaitForPollIntervalCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitForPollInterval(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}
