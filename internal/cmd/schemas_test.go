package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSchemasCreateCmd(t *testing.T) {
	var gotName string
	var gotType string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/schemas"):
			var payload struct {
				SchemaName string `json:"schemaName"`
				Fields     []struct {
					FieldName string `json:"fieldName"`
					FieldType string `json:"fieldType"`
				} `json:"fields"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			gotName = payload.SchemaName
			if len(payload.Fields) > 0 {
				gotType = payload.Fields[0].FieldType
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schemaName": payload.SchemaName,
				"schemaId":   "s1",
			})
			return
		default:
			http.NotFound(w, r)
		}
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SchemasCreateCmd{Name: "Custom", Fields: []string{"Department:string"}}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotName != "Custom" {
		t.Fatalf("unexpected name: %q", gotName)
	}
	if gotType != "STRING" {
		t.Fatalf("unexpected type: %q", gotType)
	}
	if !strings.Contains(out, "Created schema") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestSchemasListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/schemas") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schemas": []map[string]any{
				{"schemaName": "Custom", "schemaId": "s1", "fields": []map[string]any{{"fieldName": "Department"}}},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SchemasListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Custom") {
		t.Fatalf("unexpected output: %s", out)
	}
}
