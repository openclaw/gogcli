package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/api/gmail/v1"
)

func TestMCPGmailSettingsTypedArgs(t *testing.T) {
	if len(mcpGmailSettingsTools()) != 3 {
		t.Fatal("missing settings tools")
	}
	tool := mcpGmailCreateFilterTool()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"query": "fixture", "add_labels": []any{"comma,back\\slash", "Label_AbC"}, "mark_read": false}
	args, err := tool.BuildArgs(req)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--add-label-entry=comma,back\\slash") || !strings.Contains(joined, "--add-label-entry=Label_AbC") || !strings.Contains(joined, "--mark-read=false") {
		t.Fatal(args)
	}
	for _, arguments := range []map[string]any{{"query": "fixture", "forward": "private@example.invalid"}, {"query": "fixture", "mark_read": "true"}, {"query": "fixture"}, {"mark_read": true}, {"query": "fixture", "body_file": "@file"}} {
		req.Params.Arguments = arguments
		if _, err = tool.BuildArgs(req); err == nil {
			t.Fatal("unsafe or incomplete filter accepted", arguments)
		}
	}
}

func TestMCPGmailSettingsLiteralFilterLabels(t *testing.T) {
	svc, closeServer := newGoogleTestService(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"labels": []any{map[string]string{"id": "Label_AbC", "name": "comma,back\\slash", "type": "user"}}})
	}), gmail.NewService)
	defer closeServer()
	command := &GmailFiltersCreateCmd{Query: "fixture", AddLabels: []string{"comma,back\\slash"}}
	if _, err := command.validate(); err != nil {
		t.Fatal(err)
	}
	filter, err := command.buildFilter(svc, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(filter.Action.AddLabelIds) != 1 || filter.Action.AddLabelIds[0] != "Label_AbC" {
		t.Fatal(filter.Action)
	}
	_ = context.Background()
}
