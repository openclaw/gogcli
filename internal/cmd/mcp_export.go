package cmd

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func mcpExportTools() []mcpToolSpec {
	tools := make([]mcpToolSpec, 0, 2)
	for _, kind := range []string{"raw", "attachment"} {
		tool := mcpToolSpec{Name: "gmail_get_" + kind, Service: "gmail", Risk: mcpRiskRead, Bounded: true, NeedsSnapshot: true, CommandPath: []string{"gmail", "export", kind}, Description: "Read exact bytes in bounded base64 chunks. Repeat the same object IDs and snapshot_id for later offsets; expires 900 seconds after publication.", Handle: handleMCPExport}
		tool.Options = []mcp.ToolOption{mcp.WithString("message_id", mcp.Required()), mcp.WithString("snapshot_id"), mcp.WithInteger("offset", mcp.DefaultNumber(0), mcp.Min(0)), mcp.WithInteger("length", mcp.DefaultNumber(32768), mcp.Min(1), mcp.Max(262144))}
		if kind == "attachment" {
			tool.Options = append(tool.Options, mcp.WithString("attachment_id", mcp.Required()))
		}
		tools = append(tools, tool)
	}
	return tools
}

func handleMCPExport(ctx context.Context, req mcp.CallToolRequest, rt *mcpToolRuntime) *mcp.CallToolResult {
	tool := mcpExportTools()[0]
	kind := "raw"
	if req.Params.Name == "gmail_get_attachment" {
		kind = "attachment"
		tool = mcpExportTools()[1]
	}
	key := gmailExportKey{Kind: kind}
	var err error
	key.MessageID, err = req.RequireString("message_id")
	if err != nil {
		return mcpBoundedError(tool, "invalid_input", rt.budget)
	}
	if kind == "attachment" {
		key.AttachmentID, err = req.RequireString("attachment_id")
		if err != nil {
			return mcpBoundedError(tool, "invalid_input", rt.budget)
		}
	}
	if err = validateGmailExport(key, gmailExportMaxBytes); err != nil {
		return mcpBoundedError(tool, "invalid_input", rt.budget)
	}
	offset, err := mcpBoundedInt(req, "offset", 0, 0, 9223372036854775807)
	if err != nil {
		return mcpBoundedError(tool, "invalid_range", rt.budget)
	}
	length, err := mcpBoundedInt(req, "length", 32768, 1, 262144)
	if err != nil {
		return mcpBoundedError(tool, "invalid_range", rt.budget)
	}
	id := ""
	if _, present := req.GetArguments()["snapshot_id"]; present {
		id, err = req.RequireString("snapshot_id")
		if err != nil || len(id) != 32 {
			return mcpBoundedError(tool, "invalid_snapshot", rt.budget)
		}
	}
	if offset != 0 && id == "" {
		return mcpBoundedError(tool, "snapshot_required", rt.budget)
	}
	// Reject impossible metadata budgets before starting a provider read.
	_, err = boundedMCPResult(mcpCommandResult{Tool: tool.Name, Service: "gmail", Risk: "read", Stdout: mcpExportChunk{MessageID: key.MessageID, AttachmentID: key.AttachmentID, SnapshotID: strings.Repeat("0", 32), SHA256: strings.Repeat("0", 64), ExpiresAt: time.Now()}}, rt.budget)
	if err != nil {
		return mcpBoundedError(tool, "output_budget_exceeded", rt.budget)
	}
	snapshotKey := mcpSnapshotKey{Partition: rt.partition, Object: kind + ":" + key.MessageID + ":" + key.AttachmentID}
	var lease *mcpSnapshotLease
	if id != "" {
		lease, err = rt.store.AcquireID(snapshotKey, id)
	} else {
		lease, err = rt.store.Acquire(ctx, snapshotKey, gmailExportMaxBytes, func(ctx context.Context) ([]byte, gmailExportMetadata, error) { return rt.fetchExport(ctx, key) })
	}
	if err != nil {
		return mcpBoundedError(tool, mcpSnapshotErrorCode(err), rt.budget)
	}
	defer lease.Release()
	if offset > lease.Info.Size {
		return mcpBoundedError(tool, "invalid_range", rt.budget)
	}
	result, err := mcpExportRange(tool, lease, offset, length, rt.budget)
	if err != nil {
		if errors.Is(err, errMCPOutputBudget) {
			return mcpBoundedError(tool, "response_too_large", rt.budget)
		}
		return mcpBoundedError(tool, "snapshot_unavailable", rt.budget)
	}
	return result
}

type mcpExportChunk struct {
	SnapshotID   string    `json:"snapshot_id"`
	MessageID    string    `json:"message_id"`
	AttachmentID string    `json:"attachment_id,omitempty"`
	ThreadID     string    `json:"thread_id,omitempty"`
	Size         int64     `json:"size"`
	SHA256       string    `json:"sha256"`
	ExpiresAt    time.Time `json:"expires_at"`
	Offset       int64     `json:"offset"`
	Length       int64     `json:"length"`
	Complete     bool      `json:"complete"`
	DataBase64   string    `json:"data_base64"`
}

func mcpExportRange(tool mcpToolSpec, lease *mcpSnapshotLease, offset, length int64, budget int) (*mcp.CallToolResult, error) {
	if offset < 0 || offset > lease.Info.Size || length <= 0 || length > 256<<10 {
		return nil, fmt.Errorf("invalid export range")
	}
	count := min(length, lease.Info.Size-offset)
	data := make([]byte, count)
	if count > 0 {
		if n, err := lease.ReadAt(data, offset); err != nil || int64(n) != count {
			return nil, fmt.Errorf("snapshot read incomplete: %w", err)
		}
	}
	payload := mcpExportChunk{SnapshotID: lease.Info.ID, MessageID: lease.Info.Metadata.MessageID, AttachmentID: lease.Info.Metadata.AttachmentID, ThreadID: lease.Info.Metadata.ThreadID, Size: lease.Info.Size, SHA256: lease.Info.SHA256, ExpiresAt: lease.Info.ExpiresAt, Offset: offset}
	makeResult := func(size int64) (*mcp.CallToolResult, error) {
		payload.Length = size
		payload.Complete = offset+size == payload.Size
		payload.DataBase64 = base64.StdEncoding.EncodeToString(data[:size])
		return boundedMCPResult(mcpCommandResult{Tool: tool.Name, Service: tool.Service, Risk: string(tool.Risk), Stdout: payload}, budget)
	}
	if result, err := makeResult(count); err == nil {
		return result, nil
	}
	// Measure the complete MCP encoding, including its text copy. Find the
	// largest fitting range; never clip JSON or integrity metadata.
	lower, upper := int64(0), count
	for lower < upper {
		middle := lower + (upper-lower+1)/2
		if _, err := makeResult(middle); err == nil {
			lower = middle
		} else {
			upper = middle - 1
		}
	}
	if lower == 0 && count > 0 {
		return nil, fmt.Errorf("%w: cannot fit export metadata and data", errMCPOutputBudget)
	}
	return makeResult(lower)
}

func mcpSnapshotErrorCode(err error) string {
	switch {
	case errors.Is(err, errMCPSnapshotCapacity):
		return "resource_exhausted"
	case errors.Is(err, errMCPSnapshotExpired), errors.Is(err, errMCPSnapshotClosed):
		return "snapshot_expired"
	case errors.Is(err, errMCPSnapshotMismatch):
		return "snapshot_mismatch"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "cancelled"
	default:
		return "snapshot_unavailable"
	}
}
