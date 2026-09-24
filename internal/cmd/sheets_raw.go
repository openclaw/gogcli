package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	gapi "google.golang.org/api/googleapi"

	"github.com/openclaw/gogcli/internal/ui"
)

var sheetsRawBaseURL = "https://sheets.googleapis.com/v4"

// SheetsRawCmd preserves the full API response; typed SDK decoding loses unknown
// fields and re-marshaling omits explicit zero, false, and empty values.
type SheetsRawCmd struct {
	SpreadsheetID   string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Sheet           string `name:"sheet" help:"Return only this sheet (exact tab title); spreadsheet-level metadata remains included"`
	IncludeGridData bool   `name:"include-grid-data" help:"Include cell-level grid data in the response (off by default; payloads can be large and may contain secrets in formulas)"`
	Pretty          bool   `name:"pretty" help:"Pretty-print JSON (default: compact single-line)"`
}

func (c *SheetsRawCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	client, err := sheetsHTTPClient(ctx, account)
	if err != nil {
		return err
	}
	query := url.Values{"alt": {"json"}, "prettyPrint": {"false"}}
	if c.Sheet != "" {
		// Quote exact titles so A1-like names cannot resolve to cell ranges.
		query.Set("ranges", "'"+strings.ReplaceAll(c.Sheet, "'", "''")+"'")
	}
	if c.IncludeGridData {
		query.Set("includeGridData", "true")
		u.Err().Println("warning: --include-grid-data may expose cell-level formulas that contain API keys or hardcoded secrets")
	}
	endpoint := sheetsRawBaseURL + "/spreadsheets/" + url.PathEscape(spreadsheetID) + "?" + query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create Sheets read request: %w", err)
	}
	readClient := *client
	readClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := readClient.Do(request)
	if err != nil {
		return fmt.Errorf("get spreadsheet: %w", err)
	}
	defer response.Body.Close()
	if checkErr := gapi.CheckResponse(response); checkErr != nil {
		return checkErr
	}
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read spreadsheet response: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("decode spreadsheet response: %w", err)
	}
	if fields == nil {
		return errors.New("spreadsheet not found")
	}
	if metadata, ok := fields["developerMetadata"]; ok {
		var entries []json.RawMessage
		if err := json.Unmarshal(metadata, &entries); err != nil {
			return fmt.Errorf("decode spreadsheet developer metadata: %w", err)
		}
		if len(entries) > 0 {
			u.Err().Println("warning: response contains developerMetadata which may hold third-party app secrets")
		}
	}
	return writeRawJSON(ctx, json.RawMessage(raw), c.Pretty)
}
