package cmd

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"
)

func mcpGmailSearchThreadsTool() mcpToolSpec {
	return mcpToolSpec{Name: "gmail_search_threads", Service: "gmail", Risk: mcpRiskRead, Bounded: true, CommandPath: []string{"gmail", "search"}, Description: "Search one compact provider page (20 by default, maximum 100). Follow nextPageToken; count is this page's count. Search is not a mailbox snapshot. Minimum output budget 4096 bytes.", Options: []mcp.ToolOption{mcp.WithString("query", mcp.Required()), mcp.WithInteger("max", mcp.DefaultNumber(20), mcp.Min(1), mcp.Max(100)), mcp.WithString("page")}, Handle: handleMCPThreadSearch}
}

func buildMCPThreadSearchArgs(req mcp.CallToolRequest, maximum int64) ([]string, error) {
	query, err := req.RequireString("query")
	if err != nil || strings.TrimSpace(query) == "" || utf8.RuneCountInString(query) > 4000 {
		return nil, fmt.Errorf("invalid query")
	}
	if _, err = mcpBoundedInt(req, "max", 20, 1, 100); err != nil {
		return nil, err
	}
	args := []string{"gmail", "search", "--compact", "--max=" + strconv.FormatInt(maximum, 10)}
	if _, present := req.GetArguments()["page"]; present {
		page, e := req.RequireString("page")
		if e != nil || len(page) > 4096 {
			return nil, fmt.Errorf("invalid page token")
		}
		if page != "" {
			args = append(args, "--page="+page)
		}
	}
	return append(args, "--", query), nil
}

func handleMCPThreadSearch(ctx context.Context, req mcp.CallToolRequest, rt *mcpToolRuntime) *mcp.CallToolResult {
	tool := mcpGmailSearchThreadsTool()
	maximum, err := mcpBoundedInt(req, "max", 20, 1, 100)
	if err != nil {
		return mcpBoundedError(tool, "invalid_input", rt.budget)
	}
	for {
		args, buildErr := buildMCPThreadSearchArgs(req, maximum)
		if buildErr != nil {
			return mcpBoundedError(tool, "invalid_input", rt.budget)
		}
		result, runErr := rt.runChild(ctx, tool, args)
		if runErr == nil {
			if payload, ok := result.Stdout.(map[string]any); ok {
				token, _ := payload["nextPageToken"].(string)
				payload["complete"] = token == ""
			}
			if output, outputErr := boundedMCPResult(result, rt.budget); outputErr == nil {
				return output
			}
		} else if !errors.Is(runErr, errMCPOutputBudget) {
			return mcpBoundedError(tool, "provider_error", rt.budget)
		}
		if maximum == 1 {
			return mcpBoundedError(tool, "response_too_large", rt.budget)
		}
		maximum = max(1, maximum/2)
	}
}

type mcpThreadIDsPage struct {
	ThreadID   string             `json:"thread_id"`
	Total      int                `json:"total"`
	Count      int                `json:"count"`
	Messages   []gmailThreadIDRow `json:"messages"`
	NextCursor string             `json:"next_cursor,omitempty"`
	Complete   bool               `json:"complete"`
}

func mcpGmailThreadIDsTool() mcpToolSpec {
	return mcpToolSpec{Name: "gmail_thread_message_ids", Service: "gmail", Risk: mcpRiskRead, Bounded: true, NeedsSnapshot: true, CommandPath: []string{"gmail", "thread", "ids"}, Description: "Enumerate ordered metadata-only message IDs, up to 128 per page. Cursor pins a private 900-second snapshot. Minimum output budget 4096 bytes.", Options: []mcp.ToolOption{mcp.WithString("thread_id", mcp.Required()), mcp.WithInteger("max", mcp.DefaultNumber(128), mcp.Min(1), mcp.Max(128)), mcp.WithString("cursor")}, Handle: handleMCPThreadIDs}
}

func (rt *mcpToolRuntime) threadCursor(key mcpSnapshotKey, id string, index int) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(id + ":" + strconv.Itoa(index)))
	mac := hmac.New(sha256.New, []byte(rt.partition))
	_, _ = mac.Write([]byte(key.Object + ":" + payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (rt *mcpToolRuntime) parseThreadCursor(key mcpSnapshotKey, cursor string) (string, int, error) {
	parts := strings.Split(cursor, ".")
	if len(parts) != 2 || len(cursor) > 4096 {
		return "", 0, fmt.Errorf("invalid cursor")
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil {
		return "", 0, err
	}
	fields := strings.Split(string(decoded), ":")
	if len(fields) != 2 || len(fields[0]) != 32 {
		return "", 0, fmt.Errorf("invalid cursor")
	}
	index, err := strconv.Atoi(fields[1])
	if err != nil || index < 0 || index > 10000 {
		return "", 0, fmt.Errorf("invalid cursor")
	}
	expected := rt.threadCursor(key, fields[0], index)
	if !hmac.Equal([]byte(expected), []byte(cursor)) {
		return "", 0, fmt.Errorf("cursor mismatch")
	}
	return fields[0], index, nil
}

func handleMCPThreadIDs(ctx context.Context, req mcp.CallToolRequest, rt *mcpToolRuntime) *mcp.CallToolResult {
	tool := mcpGmailThreadIDsTool()
	id, err := req.RequireString("thread_id")
	if err != nil || !gmailExportIDValid(id) {
		return mcpBoundedError(tool, "invalid_input", rt.budget)
	}
	maximum, err := mcpBoundedInt(req, "max", 128, 1, 128)
	if err != nil {
		return mcpBoundedError(tool, "invalid_input", rt.budget)
	}
	cursor := ""
	if _, ok := req.GetArguments()["cursor"]; ok {
		cursor, err = req.RequireString("cursor")
		if err != nil {
			return mcpBoundedError(tool, "invalid_cursor", rt.budget)
		}
	}
	key := mcpSnapshotKey{Partition: rt.partition, Object: "threadids:" + id}
	index := 0
	var lease *mcpSnapshotLease
	if cursor != "" {
		var snapshotID string
		snapshotID, index, err = rt.parseThreadCursor(key, cursor)
		if err == nil {
			lease, err = rt.store.AcquireID(key, snapshotID)
		}
	} else {
		lease, err = rt.store.Acquire(ctx, key, 8<<20, func(ctx context.Context) ([]byte, gmailExportMetadata, error) { return rt.fetchThreadIDs(ctx, id) })
	}
	if err != nil {
		return mcpBoundedError(tool, mcpSnapshotErrorCode(err), rt.budget)
	}
	defer lease.Release()
	data := make([]byte, lease.Info.Size)
	if len(data) > 0 {
		if _, err = lease.ReadAt(data, 0); err != nil {
			return mcpBoundedError(tool, mcpSnapshotErrorCode(err), rt.budget)
		}
	}
	var snapshot gmailThreadIDsSnapshot
	if err = json.Unmarshal(data, &snapshot); err != nil || snapshot.ThreadID != id || index > len(snapshot.Messages) {
		return mcpBoundedError(tool, "invalid_snapshot", rt.budget)
	}
	return rt.threadIDsPage(ctx, tool, key, lease.Info.ID, snapshot, index, int(maximum))
}

func (rt *mcpToolRuntime) threadIDsPage(ctx context.Context, tool mcpToolSpec, key mcpSnapshotKey, snapshotID string, snapshot gmailThreadIDsSnapshot, index, maximum int) *mcp.CallToolResult {
	count := min(maximum, len(snapshot.Messages)-index)
	rows := make([]gmailThreadIDRow, count)
	for i := range rows {
		if err := ctx.Err(); err != nil {
			return mcpBoundedError(tool, mcpSnapshotErrorCode(err), rt.budget)
		}
		rows[i] = wrapMCPThreadRow(snapshot.Messages[index+i])
	}
	candidate := func(n int) (*mcp.CallToolResult, error) {
		page := mcpThreadIDsPage{ThreadID: snapshot.ThreadID, Total: len(snapshot.Messages), Count: n, Messages: rows[:n], Complete: index+n == len(snapshot.Messages)}
		if !page.Complete {
			page.NextCursor = rt.threadCursor(key, snapshotID, index+n)
		}
		return boundedMCPResult(mcpCommandResult{Tool: tool.Name, Service: "gmail", Risk: "read", Stdout: page}, rt.budget)
	}
	// A complete page omits the cursor and can fit even when its preceding prefix does not.
	if err := ctx.Err(); err != nil {
		return mcpBoundedError(tool, mcpSnapshotErrorCode(err), rt.budget)
	}
	if result, err := candidate(count); err == nil && (count > 0 || index == len(snapshot.Messages)) {
		return result
	}
	var best *mcp.CallToolResult
	for low, high := 1, count-1; low <= high; {
		if err := ctx.Err(); err != nil {
			return mcpBoundedError(tool, mcpSnapshotErrorCode(err), rt.budget)
		}
		middle := low + (high-low)/2
		if result, err := candidate(middle); err == nil {
			best = result
			low = middle + 1
		} else {
			high = middle - 1
		}
	}
	if best != nil {
		return best
	}
	return mcpBoundedError(tool, "response_too_large", rt.budget)
}
