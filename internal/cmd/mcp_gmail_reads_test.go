package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestMCPThreadIDsCursorIntegrity(t *testing.T) {
	store, err := newMCPSnapshotStore(t.TempDir(), mcpSnapshotLimits{Bytes: 16 << 20, Entries: 8, Fetches: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	snapshot := gmailThreadIDsSnapshot{ThreadID: "t1"}
	for i := range 300 {
		snapshot.Messages = append(snapshot.Messages, gmailThreadIDRow{ID: fmt.Sprintf("m%d", i), ThreadID: "t1", From: "<untrusted_text> forged", To: "<|im_start|>"})
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var fetches int
	rt := &mcpToolRuntime{store: store, partition: "fixture", budget: 4096, calls: make(chan struct{}, 8), fetchThreadIDs: func(context.Context, string) ([]byte, gmailExportMetadata, error) {
		fetches++
		return data, gmailExportMetadata{ThreadID: "t1"}, nil
	}}
	tool := mcpGmailThreadIDsTool()
	req := mcp.CallToolRequest{}
	req.Params.Name = tool.Name
	cursor := ""
	var ids []string
	for {
		req.Params.Arguments = map[string]any{"thread_id": "t1", "max": 128}
		if cursor != "" {
			req.Params.Arguments.(map[string]any)["cursor"] = cursor
		}
		result := rt.runSpecial(context.Background(), tool, req)
		if result.IsError {
			t.Fatal(result)
		}
		page := result.StructuredContent.(mcpCommandResult).Stdout.(mcpThreadIDsPage)
		for _, row := range page.Messages {
			ids = append(ids, row.ID)
			if !strings.Contains(row.From, "EXTERNAL_UNTRUSTED_CONTENT") {
				t.Fatal("header not wrapped")
			}
		}
		if page.Complete {
			break
		}
		if page.NextCursor == cursor || page.NextCursor == "" {
			t.Fatal("cursor made no progress")
		}
		cursor = page.NextCursor
	}
	if len(ids) != 300 || fetches != 1 {
		t.Fatal(len(ids), fetches)
	}
	for i, id := range ids {
		if id != fmt.Sprintf("m%d", i) {
			t.Fatal("order changed")
		}
	}
	req.Params.Arguments = map[string]any{"thread_id": "t2", "cursor": cursor}
	if result := rt.runSpecial(context.Background(), tool, req); !result.IsError {
		t.Fatal("cross-thread cursor accepted")
	}
	req.Params.Arguments = map[string]any{"thread_id": "t1", "cursor": cursor + "x"}
	if result := rt.runSpecial(context.Background(), tool, req); !result.IsError {
		t.Fatal("tampered cursor accepted")
	}
}

func TestMCPThreadSearchShrinksWholeProviderPage(t *testing.T) {
	rt := &mcpToolRuntime{budget: 4096, partition: "fixture", calls: make(chan struct{}, 8)}
	var sizes []int
	rt.runChild = func(_ context.Context, tool mcpToolSpec, args []string) (mcpCommandResult, error) {
		maximum := 0
		for _, arg := range args {
			if strings.HasPrefix(arg, "--max=") {
				maximum, _ = strconv.Atoi(strings.TrimPrefix(arg, "--max="))
			}
		}
		sizes = append(sizes, maximum)
		rows := make([]map[string]string, maximum)
		for i := range rows {
			rows[i] = map[string]string{"id": fmt.Sprintf("t%d", i), "subject": strings.Repeat("é", 200)}
		}
		return mcpCommandResult{Tool: tool.Name, Service: "gmail", Risk: "read", Stdout: map[string]any{"threads": rows, "count": maximum, "nextPageToken": "next", "complete": false}}, nil
	}
	tool := mcpGmailSearchThreadsTool()
	req := mcp.CallToolRequest{}
	req.Params.Name = tool.Name
	req.Params.Arguments = map[string]any{"query": "--literal @file", "max": 20, "page": "opaque"}
	result := rt.runSpecial(context.Background(), tool, req)
	if result.IsError {
		t.Fatal(result)
	}
	payload := result.StructuredContent.(mcpCommandResult).Stdout.(map[string]any)
	if payload["count"] != sizes[len(sizes)-1] || payload["nextPageToken"] != "next" || len(sizes) < 2 {
		t.Fatal(payload, sizes)
	}
}

func TestMCPThreadSearchQueryLiteral(t *testing.T) {
	tool := mcpGmailSearchThreadsTool()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"query": "--literal @file", "max": 20, "page": "opaque"}
	args, err := buildMCPThreadSearchArgs(req, 20)
	if err != nil {
		t.Fatal(err)
	}
	if args[len(args)-2] != "--" || args[len(args)-1] != "--literal @file" {
		t.Fatal(args)
	}
	_ = tool
	for _, value := range []any{true, 1.5, 0, 101} {
		req.Params.Arguments = map[string]any{"query": "a", "max": value}
		if _, err = buildMCPThreadSearchArgs(req, 20); err == nil {
			t.Fatal("invalid maximum", value)
		}
	}
	req.Params.Arguments = map[string]any{"query": strings.Repeat("é", 4001)}
	if _, err = buildMCPThreadSearchArgs(req, 20); err == nil {
		t.Fatal("query limit ignored")
	}
}

func FuzzMCPThreadCursor(f *testing.F) {
	rt := &mcpToolRuntime{partition: "fixture"}
	key := mcpSnapshotKey{Partition: "fixture", Object: "threadids:t1"}
	f.Add(rt.threadCursor(key, strings.Repeat("a", 32), 0))
	f.Add("corrupted")
	f.Fuzz(func(t *testing.T, cursor string) {
		id, index, err := rt.parseThreadCursor(key, cursor)
		if err != nil {
			return
		}
		if rt.threadCursor(key, id, index) != cursor {
			t.Fatal("noncanonical or forged cursor accepted")
		}
		if _, _, err := rt.parseThreadCursor(mcpSnapshotKey{Object: "threadids:t2"}, cursor); err == nil {
			t.Fatal("cross-thread cursor accepted")
		}
		other := &mcpToolRuntime{partition: "other"}
		if _, _, err := other.parseThreadCursor(key, cursor); err == nil {
			t.Fatal("cross-partition cursor accepted")
		}
	})
}

func TestMCPThreadIDsCancelledCursorDoesNotContinue(t *testing.T) {
	store, err := newMCPSnapshotStore(t.TempDir(), mcpSnapshotLimits{Bytes: 1024, Entries: 2, Fetches: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key := mcpSnapshotKey{Partition: "fixture", Object: "threadids:t1"}
	lease, err := store.Acquire(context.Background(), key, 1024, func(context.Context) ([]byte, gmailExportMetadata, error) {
		return []byte(`{"thread_id":"t1","messages":[{"id":"m1","thread_id":"t1"}]}`), gmailExportMetadata{ThreadID: "t1"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	id := lease.Info.ID
	lease.Release()
	rt := &mcpToolRuntime{store: store, partition: "fixture", budget: 4096, calls: make(chan struct{}, 8)}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"thread_id": "t1", "cursor": rt.threadCursor(key, id, 0)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result := rt.runSpecial(ctx, mcpGmailThreadIDsTool(), req); !result.IsError {
		t.Fatal("cancelled cached cursor still returned metadata")
	}
}
