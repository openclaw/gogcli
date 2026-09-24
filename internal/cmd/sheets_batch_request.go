package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	gapi "google.golang.org/api/googleapi"

	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

var sheetsBatchRequestBaseURL = "https://sheets.googleapis.com/v4"

type SheetsBatchRequestCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	RequestsJSON  string `name:"requests-json" required:"" help:"Structural requests as a JSON array, or @file/@-"`
}

type sheetsBatchRequestBody struct {
	Requests []json.RawMessage `json:"requests"`
}

func (c *SheetsBatchRequestCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := enforceExplicitCommandPermission(flags, []string{"sheets", "batch-request"}); err != nil {
		return err
	}
	id := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if id == "" {
		return usage("empty spreadsheetId")
	}
	requests, err := parseSheetsBatchRequests(c.RequestsJSON, stdinReader(ctx))
	if err != nil {
		return err
	}
	body := sheetsBatchRequestBody{Requests: requests}
	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "sheets.batch-request", map[string]any{
		"spreadsheet_id": id,
		"requests":       requests,
	}, fmt.Sprintf("apply %d structural requests to spreadsheet %s (may delete data)", len(requests), id)); confirmErr != nil {
		return confirmErr
	}
	if googleapi.ReadOnly(ctx) {
		return googleapi.ErrReadOnly
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	client, err := sheetsHTTPClient(ctx, account)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode Sheets batch: %w", err)
	}
	// A lost response can follow a successful insert/delete. Never replay a raw batch.
	request, err := http.NewRequestWithContext(googleapi.WithoutRetries(ctx), http.MethodPost,
		sheetsBatchRequestBaseURL+"/spreadsheets/"+url.PathEscape(id)+":batchUpdate", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create Sheets batch request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	// Redirects can replay the POST and move the authenticated request to another host.
	batchClient := *client
	batchClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := batchClient.Do(request)
	if err != nil {
		return fmt.Errorf("submit Sheets batch: %w", err)
	}
	defer response.Body.Close()
	if err := gapi.CheckResponse(response); err != nil {
		return err
	}
	var result json.RawMessage
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode Sheets batch response: %w", err)
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), result)
	}
	ui.FromContext(ctx).Out().Linef("Applied %d structural requests to %s", len(requests), id)
	return nil
}

func parseSheetsBatchRequests(source string, input io.Reader) ([]json.RawMessage, error) {
	raw, err := resolveInlineOrFileBytes(source, input)
	if err != nil {
		return nil, usagef("read --requests-json: %v", err)
	}
	// RawMessage preserves explicit zero, false, null, and large integer values.
	var requests []json.RawMessage
	if err := json.Unmarshal(raw, &requests); err != nil {
		return nil, usagef("invalid --requests-json array: %v", err)
	}
	if len(requests) == 0 {
		return nil, usage("--requests-json must contain at least one request")
	}
	for i, request := range requests {
		if !bytes.HasPrefix(bytes.TrimSpace(request), []byte("{")) {
			return nil, usagef("--requests-json request %d must be an object", i+1)
		}
	}
	return requests, nil
}
