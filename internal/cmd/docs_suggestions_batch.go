package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/api/docs/v1"
)

type DocsSuggestionsBatchCmd struct {
	DocID    string `arg:"" name:"docId" help:"Doc ID"`
	File     string `name:"file" required:"" help:"JSON array of supported Docs requests (use - for stdin)"`
	Revision string `required:"" help:"Document revision ID from the inline read used to build the requests"`
}

func (c *DocsSuggestionsBatchCmd) Run(ctx context.Context, flags *RootFlags) error {
	id := normalizeGoogleID(strings.TrimSpace(c.DocID))
	if id == "" || strings.TrimSpace(c.Revision) == "" {
		return usage("docId and --revision are required")
	}
	input, err := readDocsSuggestionsBatchInput(ctx, c.File)
	if err != nil {
		return err
	}
	requests, err := parseDocsSuggestionsBatch(input)
	if err != nil {
		return usage(err.Error())
	}
	payload := struct {
		WriteControl *docs.WriteControl `json:"writeControl"`
		Requests     []json.RawMessage  `json:"requests"`
	}{WriteControl: &docs.WriteControl{WriteMode: "SUGGEST", RequiredRevisionId: c.Revision}, Requests: requests}
	if dryRunErr := dryRunExit(ctx, flags, "docs.suggestions.batch", map[string]any{"documentId": id, "body": payload}); dryRunErr != nil {
		return dryRunErr
	}
	return postDocsSuggestions(ctx, flags, id, len(requests), payload)
}

func parseDocsSuggestionsBatch(input []byte) ([]json.RawMessage, error) {
	var rows []json.RawMessage
	if err := json.Unmarshal(input, &rows); err != nil {
		return nil, fmt.Errorf("expected a JSON array of Docs requests: %w", err)
	}
	if len(rows) == 0 || len(rows) > docsBatchUpdateRequestCap {
		return nil, fmt.Errorf("suggestion writes must contain 1 to %d requests in one batch", docsBatchUpdateRequestCap)
	}
	for i, row := range rows {
		if err := validateDocsSuggestionOperation(row); err != nil {
			return nil, fmt.Errorf("request %d: %w", i+1, err)
		}
	}
	return rows, nil
}

func validateDocsSuggestionOperation(row json.RawMessage) error {
	// Validate the exact operation name before decoding into the generated
	// type. Reject duplicate keys and multi-operation objects, rather than
	// accepting a request that means something different after serialization.
	keys := json.NewDecoder(bytes.NewReader(row))
	if delim, err := keys.Token(); err != nil || delim != json.Delim('{') || !keys.More() {
		return errors.New("expected an object containing one suggestion operation")
	}
	field, err := keys.Token()
	if err != nil {
		return err
	}
	name, ok := field.(string)
	if !ok || !isDocsSuggestionBatchOperation(name) {
		return fmt.Errorf("unsupported suggestion operation %q", field)
	}
	var value json.RawMessage
	if err := keys.Decode(&value); err != nil {
		return err
	}
	if keys.More() || len(value) == 0 || value[0] != '{' {
		return errors.New("expected exactly one non-null suggestion operation")
	}

	if err := validateDocsSuggestionJSONKeys(json.NewDecoder(bytes.NewReader(row))); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(row))
	dec.DisallowUnknownFields()
	var req docs.Request
	if err := dec.Decode(&req); err != nil {
		return err
	}
	if req.UpdateDocumentStyle != nil {
		for _, field := range strings.Split(req.UpdateDocumentStyle.Fields, ",") {
			switch strings.TrimSpace(field) {
			case "marginTop", "marginBottom", "marginLeft", "marginRight":
			default:
				return fmt.Errorf("unsupported document style mask for suggestions: %q (only explicit page margins are allowed)", field)
			}
		}
	}

	if req.UpdateSectionStyle != nil {
		for _, field := range strings.Split(req.UpdateSectionStyle.Fields, ",") {
			switch strings.TrimSpace(field) {
			case "columnProperties", "columnSeparatorStyle":
			default:
				return fmt.Errorf("unsupported section style mask for suggestions: %q (only explicit column fields are allowed)", field)
			}
		}
	}

	// Keep the validated original JSON on the wire: the Go types would
	// otherwise omit explicit false, zero, and empty values.
	return nil
}

func isDocsSuggestionBatchOperation(name string) bool {
	switch name {
	case "insertText", "deleteContentRange", "replaceAllText", "updateTextStyle", "updateParagraphStyle",
		"createParagraphBullets", "deleteParagraphBullets", "insertPageBreak", "insertSectionBreak", "insertTable", "createFootnote",
		"insertTableRow", "deleteTableRow", "insertTableColumn", "deleteTableColumn", "mergeTableCells", "updateTableCellStyle",
		"pinTableHeaderRows", "unmergeTableCells", "updateTableRowStyle", "updateSectionStyle", "insertPerson", "insertDate",
		"insertRichLink", "insertInlineImage", "replaceImage", "createHeader", "createFooter", "updateDocumentStyle":
		return true
	default:
		return false
	}
}

const docsSuggestionsBatchMaxBytes = 4 << 20

func readDocsSuggestionsBatchInput(ctx context.Context, path string) ([]byte, error) {
	reader := stdinReader(ctx)
	if path != "-" {
		file, err := os.Open(filepath.Clean(path))
		if err != nil {
			return nil, fmt.Errorf("open suggestion request file: %w", err)
		}
		defer file.Close()
		reader = file
	}
	data, err := io.ReadAll(io.LimitReader(reader, docsSuggestionsBatchMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read suggestion requests: %w", err)
	}
	if len(data) > docsSuggestionsBatchMaxBytes {
		return nil, errors.New("suggestion request JSON exceeds 4 MiB")
	}
	return data, nil
}

func validateDocsSuggestionJSONKeys(dec *json.Decoder) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	switch token {
	case json.Delim('{'):
		seen := make(map[string]bool)
		for dec.More() {
			key, keyErr := dec.Token()
			if keyErr != nil {
				return keyErr
			}
			name, ok := key.(string)
			if !ok || seen[strings.ToLower(name)] {
				return fmt.Errorf("duplicate or invalid JSON field %q", key)
			}
			seen[strings.ToLower(name)] = true
			if valueErr := validateDocsSuggestionJSONKeys(dec); valueErr != nil {
				return valueErr
			}
		}
	case json.Delim('['):
		for dec.More() {
			if valueErr := validateDocsSuggestionJSONKeys(dec); valueErr != nil {
				return valueErr
			}
		}
	default:
		return nil
	}
	_, err = dec.Token()
	return err
}
