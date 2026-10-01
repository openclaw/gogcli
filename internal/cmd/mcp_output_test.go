package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestMCPExportRangeContract(t *testing.T) {
	store, err := newMCPSnapshotStore(t.TempDir(), mcpSnapshotLimits{Bytes: 1 << 20, Entries: 8, Fetches: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key := mcpSnapshotKey{Partition: "account/client", Object: "raw:m1"}
	original := bytes.Repeat([]byte{0, 255, 13, 10, 10}, 10000)
	lease, err := store.Acquire(context.Background(), key, int64(len(original)), func(context.Context) ([]byte, gmailExportMetadata, error) {
		return original, gmailExportMetadata{MessageID: "m1", ThreadID: "t1"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	for _, budget := range []int{4096, 65536, 2097152} {
		var collected []byte
		for offset := int64(0); ; {
			result, rangeErr := mcpExportRange(mcpToolSpec{Name: "gmail_get_raw", Service: "gmail", Risk: mcpRiskRead}, lease, offset, 256<<10, budget)
			if rangeErr != nil {
				t.Fatal(rangeErr)
			}
			encoded, encodeErr := json.Marshal(result)
			if encodeErr != nil {
				t.Fatal(encodeErr)
			}
			if len(encoded) > min(budget, 1<<20) {
				t.Fatalf("budget %d: got %d", budget, len(encoded))
			}
			envelope := result.StructuredContent.(mcpCommandResult)
			payload := envelope.Stdout.(mcpExportChunk)
			data, decodeErr := base64.StdEncoding.Strict().DecodeString(payload.DataBase64)
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if payload.Offset != offset || payload.Length != int64(len(data)) || payload.SnapshotID != lease.Info.ID || payload.SHA256 != lease.Info.SHA256 || payload.ThreadID != "t1" {
				t.Fatal("integrity metadata changed")
			}
			collected = append(collected, data...)
			offset += int64(len(data))
			if payload.Complete {
				break
			}
			if len(data) == 0 {
				t.Fatal("no progress")
			}
		}
		if !bytes.Equal(collected, original) {
			t.Fatal("bytes altered")
		}
	}
	eof, err := mcpExportRange(mcpToolSpec{}, lease, int64(len(original)), 1, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if !eof.StructuredContent.(mcpCommandResult).Stdout.(mcpExportChunk).Complete {
		t.Fatal("EOF incomplete")
	}
	for _, offset := range []int64{-1, int64(len(original)) + 1, 9223372036854775807} {
		if _, err = mcpExportRange(mcpToolSpec{}, lease, offset, 100, 4096); err == nil {
			t.Fatal("invalid range accepted")
		}
	}
}

func TestMCPExportIntegerNegatives(t *testing.T) {
	for _, value := range []any{true, 1.5, json.Number("9223372036854775808"), "1", json.Number("1.0")} {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]any{"offset": value}
		if _, err := mcpBoundedInt(req, "offset", 0, 0, 9223372036854775807); err == nil {
			t.Fatalf("accepted %v", value)
		}
	}
}

func TestMCPBoundedResultError(t *testing.T) {
	result := mcpBoundedError(mcpToolSpec{Name: "gmail_get_raw", Service: "gmail", Risk: mcpRiskRead}, "invalid_input", 4096)
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > 4096 || !result.IsError {
		t.Fatal(string(encoded), err)
	}
	envelope := result.StructuredContent.(mcpCommandResult)
	if envelope.ExitCode != 1 {
		t.Fatal(envelope)
	}
}

func TestMCPExportRangeRejectsTruncatedSnapshot(t *testing.T) {
	store, err := newMCPSnapshotStore(t.TempDir(), mcpSnapshotLimits{Bytes: 1024, Entries: 2, Fetches: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	lease, err := store.Acquire(context.Background(), mcpSnapshotKey{Object: "raw:m1"}, 10, func(context.Context) ([]byte, gmailExportMetadata, error) {
		return []byte("0123456789"), gmailExportMetadata{MessageID: "m1"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	if err := lease.entry.file.Truncate(3); err != nil {
		t.Fatal(err)
	}
	if _, err := mcpExportRange(mcpToolSpec{}, lease, 0, 10, 4096); err == nil {
		t.Fatal("silently exported zero-filled bytes from a short snapshot read")
	}
}

func TestMCPWriteCaptureFailureRetainsUnknownReceipt(t *testing.T) {
	rt := &mcpToolRuntime{budget: 4096}
	rt.runChild = func(context.Context, mcpToolSpec, []string) (mcpCommandResult, error) {
		return mcpCommandResult{}, errMCPOutputBudget
	}
	tool := mcpGmailCreateFilterTool()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"from": "sender@example.com", "add_labels": []string{"Label_A"}}
	result := rt.runTyped(context.Background(), tool, req)
	encoded, err := json.Marshal(result)
	if err != nil || !result.IsError || !bytes.Contains(encoded, []byte(`"outcome":"outcome_unknown"`)) || !bytes.Contains(encoded, []byte(`"retry_safe":false`)) {
		t.Fatalf("write uncertainty lost: %s (%v)", encoded, err)
	}
}

func TestMCPResultWireRoundTripBudget(t *testing.T) {
	data := bytes.Repeat([]byte{0, 255, 13, 10}, 10000)
	result := mcpCommandResult{Tool: "gmail_get_raw", Service: "gmail", Risk: "read", Stdout: map[string]any{"data_base64": base64.StdEncoding.EncodeToString(data)}}
	output, err := boundedMCPResult(result, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	var decoded mcp.CallToolResult
	if err = json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(&decoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) > len(encoded) {
		t.Fatalf("SDK roundtrip expands by %d (type %q meta %#v)", len(second)-len(encoded), decoded.ResultType, decoded.Meta)
	}
}

func FuzzMCPExportRanges(f *testing.F) {
	f.Add([]byte{0, 255, 13, 10}, uint64(0), uint32(32), uint32(4096))
	f.Add(bytes.Repeat([]byte{0, 255}, 8192), uint64(13), uint32(262144), uint32(4096))
	f.Fuzz(func(t *testing.T, data []byte, offset uint64, length, budget uint32) {
		if len(data) > 16384 {
			data = data[:16384]
		}
		store, err := newMCPSnapshotStore(t.TempDir(), mcpSnapshotLimits{Bytes: 32768, Entries: 2, Fetches: 1})
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		lease, err := store.Acquire(context.Background(), mcpSnapshotKey{Object: "raw:m1"}, 32768, func(context.Context) ([]byte, gmailExportMetadata, error) {
			return data, gmailExportMetadata{MessageID: "m1"}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		defer lease.Release()
		position := int64(offset % uint64(len(data)+2))
		maximum := int64(length%262144) + 1
		limit := int(budget%65536) + 4096
		result, err := mcpExportRange(mcpToolSpec{Name: "gmail_get_raw", Service: "gmail", Risk: mcpRiskRead}, lease, position, maximum, limit)
		if position > int64(len(data)) {
			if err == nil {
				t.Fatal("invalid offset accepted")
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		chunk := result.StructuredContent.(mcpCommandResult).Stdout.(mcpExportChunk)
		decoded, err := base64.StdEncoding.Strict().DecodeString(chunk.DataBase64)
		if err != nil || chunk.Length != int64(len(decoded)) || chunk.Length > maximum || !bytes.Equal(decoded, data[position:position+chunk.Length]) || chunk.Complete != (position+chunk.Length == int64(len(data))) {
			t.Fatal("range bytes or progress changed", err)
		}
		if len(decoded) == 0 && !chunk.Complete {
			t.Fatal("no progress")
		}
		encoded, err := json.Marshal(result)
		if err != nil || len(encoded) > limit {
			t.Fatal("range result exceeds budget", err)
		}
	})
}
