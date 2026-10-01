package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/config"
)

func TestMCPRuntimeAbandonedNativeExportRetriesForLiveWaiter(t *testing.T) {
	want := []byte{0, 255, 13, 10, 128}
	started := make(chan struct{})
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			close(started)
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "m1", "threadId": "t1", "raw": base64.RawURLEncoding.EncodeToString(want)})
	}))
	defer srv.Close()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx := authclient.WithClientResolver(t.Context(), func(string, string) (string, error) { return "default", nil })
	rt, err := newMCPToolRuntime(ctx, self, &McpCmd{TimeoutSeconds: 30, MaxOutputBytes: 4096}, &RootFlags{Home: t.TempDir(), Account: "fixture@example.invalid", AccessToken: "synthetic-test-token"}, config.MCPPolicy{})
	if err != nil || rt.store == nil {
		t.Fatal(err)
	}
	rt.env = append(rt.env, "GOG_TEST_MCP_RUNTIME_EXPORT=native", "GOG_TEST_MCP_GMAIL_ENDPOINT="+srv.URL+"/")
	firstCtx, cancel := context.WithCancel(t.Context())
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseFetch := func() { releaseOnce.Do(func() { close(release) }) }
	defer func() { cancel(); releaseFetch(); _ = rt.store.Close() }()
	stopped := make(chan error, 1)
	var fetches atomic.Int32
	fetch := func(ctx context.Context) ([]byte, gmailExportMetadata, error) {
		first := fetches.Add(1) == 1
		data, info, fetchErr := rt.fetchExport(ctx, gmailExportKey{Kind: "raw", MessageID: "m1"})
		if first {
			stopped <- fetchErr
			// Hold completion to let the live waiter join the abandoned pending.
			<-release
		}
		return data, info, fetchErr
	}
	key := mcpSnapshotKey{Partition: rt.partition, Object: "raw:m1:"}
	first := make(chan error, 1)
	go func() {
		lease, e := rt.store.Acquire(firstCtx, key, gmailExportMaxBytes, fetch)
		if lease != nil {
			lease.Release()
		}
		first <- e
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("native export child did not start")
	}
	cancel()
	if err = <-first; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = <-stopped; err == nil || errors.Is(err, context.Canceled) {
		t.Fatal("fixture did not exercise redacted child cancellation", err)
	}
	second := make(chan error, 1)
	go func() {
		lease, e := rt.store.Acquire(t.Context(), key, gmailExportMaxBytes, fetch)
		if lease != nil {
			defer lease.Release()
			data := make([]byte, len(want))
			if _, e = lease.ReadAt(data, 0); e == nil && (!bytes.Equal(data, want) || lease.Info.Metadata.MessageID != "m1") {
				e = errors.New("fresh native export integrity mismatch")
			}
		}
		second <- e
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		rt.store.mu.Lock()
		joined := rt.store.pending[key] != nil && rt.store.pending[key].waiters == 1
		rt.store.mu.Unlock()
		if joined || time.Now().After(deadline) {
			releaseFetch()
			if !joined {
				t.Fatal("live waiter did not join abandoned native export")
			}
			break
		}
		runtime.Gosched()
	}
	if err = <-second; err != nil || fetches.Load() != 2 || requests.Load() != 2 {
		t.Fatal("live waiter inherited redacted cancellation", err, fetches.Load(), requests.Load())
	}
}

// This test-binary child receives the real export argv without testing flags.
// Success runs the native CLI against the parent's local Gmail fixture; other
// modes simulate a faulty child's file/metadata contract, never a live service.
func runMCPExportRuntimeFixture(mode string) int {
	if mode == "native" {
		rt := &app.Runtime{IO: app.IO{Out: os.Stdout, Err: os.Stderr}, Services: app.Services{Gmail: func(ctx context.Context, _ string) (*gmail.Service, error) {
			return gmail.NewService(ctx, option.WithHTTPClient(http.DefaultClient), option.WithEndpoint(os.Getenv("GOG_TEST_MCP_GMAIL_ENDPOINT")))
		}}}
		if err := executeWithRuntime(os.Args[1:], rt); err != nil {
			return 1
		}
		return 0
	}
	dest := ""
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "--out=") {
			dest = strings.TrimPrefix(arg, "--out=")
		}
	}
	parent, err := os.Stat(filepath.Dir(dest))
	if dest == "" || err != nil || !parent.IsDir() || (runtime.GOOS != "windows" && parent.Mode().Perm() != 0o700) {
		return 1
	}
	data := []byte{0, 255, 13, 10, 128}
	info := gmailExportMetadata{MessageID: "m1", ThreadID: "t1", Size: int64(len(data))}
	switch mode {
	case "message_mismatch":
		info.MessageID = "other"
	case "attachment_mismatch":
		info.AttachmentID = "other"
	case "oversized":
		info.Size = gmailExportMaxBytes + 1
	case "short_file":
		data = data[:len(data)-1]
	case "long_file":
		data = append(data, 1)
	}
	if mode != "missing_file" {
		if err = os.WriteFile(dest, data, 0o600); err != nil {
			return 1
		}
	}
	switch mode {
	case "truncated_stdout":
		_, err = os.Stdout.Write(bytes.Repeat([]byte("x"), 17<<10))
	case "invalid_json":
		_, err = os.Stdout.WriteString("{invalid")
	default:
		err = json.NewEncoder(os.Stdout).Encode(info)
	}
	if err != nil {
		return 1
	}
	return 0
}

func TestMCPRuntimeExportChildContract(t *testing.T) {
	want := []byte{0, 255, 13, 10, 128}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/attachments/a1") {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": base64.RawURLEncoding.EncodeToString(want), "size": len(want)})
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/messages/m1") || r.URL.Query().Get("format") != "raw" {
			t.Error("unexpected native export request", r.URL)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "m1", "threadId": "t1", "raw": base64.RawURLEncoding.EncodeToString(want)})
	}))
	defer srv.Close()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"native", "native_attachment", "message_mismatch", "attachment_mismatch", "oversized", "short_file", "long_file", "missing_file", "truncated_stdout", "invalid_json"} {
		t.Run(mode, func(t *testing.T) {
			ctx := authclient.WithClientResolver(t.Context(), func(string, string) (string, error) { return "default", nil })
			rt, err := newMCPToolRuntime(ctx, self, &McpCmd{TimeoutSeconds: 30, MaxOutputBytes: 4096}, &RootFlags{Home: t.TempDir(), Account: "fixture@example.invalid", AccessToken: "synthetic-test-token"}, config.MCPPolicy{})
			if err != nil || rt.store == nil {
				t.Fatal(err)
			}
			defer rt.store.Close()
			childMode := mode
			if mode == "native_attachment" {
				childMode = "native"
			}
			rt.env = append(rt.env, "GOG_TEST_MCP_RUNTIME_EXPORT="+childMode, "GOG_TEST_MCP_GMAIL_ENDPOINT="+srv.URL+"/")
			req := mcp.CallToolRequest{}
			req.Params.Name = "gmail_get_raw"
			req.Params.Arguments = map[string]any{"message_id": "m1"}
			tool := mcpExportTools()[0]
			if mode == "native_attachment" {
				req.Params.Name = "gmail_get_attachment"
				req.Params.Arguments.(map[string]any)["attachment_id"] = "a1"
				tool = mcpExportTools()[1]
			}
			result := rt.runSpecial(t.Context(), tool, req)
			if mode == "native" || mode == "native_attachment" {
				if result.IsError {
					t.Fatal("native child export failed", result)
				}
				chunk := result.StructuredContent.(mcpCommandResult).Stdout.(mcpExportChunk)
				data, decodeErr := base64.StdEncoding.Strict().DecodeString(chunk.DataBase64)
				if decodeErr != nil || !bytes.Equal(data, want) || chunk.Size != int64(len(want)) || chunk.MessageID != "m1" {
					t.Fatal("native child integrity failed", chunk, decodeErr)
				}
			} else if !result.IsError || len(rt.store.entries) != 0 || rt.store.bytes != 0 {
				t.Fatal("faulty export was published or leaked a reservation", result)
			}
			files, err := os.ReadDir(rt.store.dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, file := range files {
				if file.IsDir() {
					t.Fatal("child fetch directory retained", file.Name())
				}
			}
		})
	}
}

func TestMCPRuntimeThreadIDsDirect(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "identity_mismatch"}[mismatch], func(t *testing.T) {
			svc, stop := newGoogleTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("format") != "metadata" || strings.Contains(r.URL.Query().Get("fields"), "body") {
					t.Error("not a metadata-only request", r.URL)
				}
				id := "t1"
				if mismatch {
					id = "other"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "messages": []map[string]any{{"id": "m1", "threadId": "t1"}}})
			}), gmail.NewService)
			defer stop()
			ctx := app.WithRuntime(t.Context(), &app.Runtime{Services: app.Services{Gmail: func(context.Context, string) (*gmail.Service, error) { return svc, nil }}})
			ctx = authclient.WithClientResolver(ctx, func(string, string) (string, error) { return "default", nil })
			rt, err := newMCPToolRuntime(ctx, "unused", &McpCmd{TimeoutSeconds: 30, MaxOutputBytes: 4096}, &RootFlags{Account: "fixture@example.invalid"}, config.MCPPolicy{})
			if err != nil || rt.store == nil {
				t.Fatal(err)
			}
			defer rt.store.Close()
			req := mcp.CallToolRequest{}
			req.Params.Name = "gmail_thread_message_ids"
			req.Params.Arguments = map[string]any{"thread_id": "t1"}
			result := rt.runSpecial(t.Context(), mcpGmailThreadIDsTool(), req)
			if mismatch {
				if !result.IsError || len(rt.store.entries) != 0 {
					t.Fatal("mismatched provider thread published", result)
				}
			} else {
				if result.IsError {
					t.Fatal(result)
				}
				page := result.StructuredContent.(mcpCommandResult).Stdout.(mcpThreadIDsPage)
				if page.ThreadID != "t1" || len(page.Messages) != 1 || page.Messages[0].ID != "m1" || !page.Complete {
					t.Fatal("native metadata fetch failed", page)
				}
			}
		})
	}
}
