package cmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestMCPExportStdioHelper(t *testing.T) {
	if os.Getenv("GOG_TEST_MCP_EXPORT_STDIO") != "1" {
		return
	}
	store, err := newMCPSnapshotStore("", mcpSnapshotLimits{Bytes: 64 << 20, Entries: 8, Fetches: 2})
	if err != nil {
		t.Fatal(err)
	}
	rt := &mcpToolRuntime{store: store, partition: "fixture", budget: 102400, calls: make(chan struct{}, 8)}
	rt.fetchExport = func(context.Context, gmailExportKey) ([]byte, gmailExportMetadata, error) {
		return bytes.Repeat([]byte{0, 255, 13, 10, 128}, (20<<20)/5), gmailExportMetadata{MessageID: "m1", ThreadID: "t1"}, nil
	}
	s := newMCPServer()
	tool := mcpExportTools()[0]
	s.AddTool(newMCPTool(tool), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return rt.runSpecial(ctx, tool, req), nil
	})
	err = server.ServeStdio(s)
	closeErr := store.Close()
	if err != nil || closeErr != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestMCPExport20MiBThroughStdio(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	client, err := mcpclient.NewStdioMCPClient(executable, []string{"GOG_TEST_MCP_EXPORT_STDIO=1"}, "-test.run=^TestMCPExportStdioHelper$")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	init := mcp.InitializeRequest{}
	init.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	init.Params.ClientInfo = mcp.Implementation{Name: "export-fixture", Version: "1"}
	if _, err = client.Initialize(ctx, init); err != nil {
		t.Fatal(err)
	}
	var offset int64
	var snapshotID, digest string
	hash := sha256.New()
	for {
		request := mcp.CallToolRequest{}
		request.Params.Name = "gmail_get_raw"
		args := map[string]any{"message_id": "m1", "offset": offset, "length": 262144}
		if snapshotID != "" {
			args["snapshot_id"] = snapshotID
		}
		request.Params.Arguments = args
		result, err := client.CallTool(ctx, request)
		if err != nil || result.IsError {
			t.Fatal(result, err)
		}
		encoded, err := json.Marshal(result)
		if err != nil || len(encoded) > 102400 {
			t.Fatal("unbounded result", len(encoded), err)
		}
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			ExitCode int            `json:"exit_code"`
			Stdout   mcpExportChunk `json:"stdout"`
		}
		if err = json.Unmarshal(raw, &envelope); err != nil {
			t.Fatal(err)
		}
		chunk := envelope.Stdout
		if snapshotID == "" {
			snapshotID, digest = chunk.SnapshotID, chunk.SHA256
		}
		data, err := base64.StdEncoding.Strict().DecodeString(chunk.DataBase64)
		if err != nil || chunk.Offset != offset || chunk.Length != int64(len(data)) || chunk.SnapshotID != snapshotID || chunk.SHA256 != digest || chunk.MessageID != "m1" || chunk.ThreadID != "t1" || envelope.ExitCode != 0 {
			t.Fatal("integrity metadata", chunk, err)
		}
		_, _ = hash.Write(data)
		offset += chunk.Length
		if chunk.Complete {
			break
		}
		if len(data) == 0 {
			t.Fatal("no progress")
		}
	}
	original := bytes.Repeat([]byte{0, 255, 13, 10, 128}, (20<<20)/5)
	expected := sha256.Sum256(original)
	if offset != int64(len(original)) || digest != hex.EncodeToString(expected[:]) || hex.EncodeToString(hash.Sum(nil)) != digest {
		t.Fatal("byte-exact integrity failed")
	}
}

func TestMCPBoundedAdmissionAndCancellation(t *testing.T) {
	rt := &mcpToolRuntime{budget: 4096, calls: make(chan struct{}, 8)}
	started := make(chan struct{}, 8)
	rt.runChild = func(ctx context.Context, tool mcpToolSpec, args []string) (mcpCommandResult, error) {
		started <- struct{}{}
		<-ctx.Done()
		return mcpCommandResult{}, ctx.Err()
	}
	tool := mcpCalendarGetEventTool()
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"event_id": "e1"}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{}, 8)
	for range 8 {
		go func() { rt.runSpecial(ctx, tool, request); done <- struct{}{} }()
	}
	for range 8 {
		<-started
	}
	if result := rt.runSpecial(context.Background(), tool, request); !result.IsError {
		t.Fatal("ninth call admitted")
	}
	cancel()
	for range 8 {
		<-done
	}
	if len(rt.calls) != 0 {
		t.Fatal("cancelled calls retain admission slots")
	}
}
