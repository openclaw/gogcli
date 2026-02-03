package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/outfmt"
)

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

func TestSchemasListCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schemas": []map[string]any{
				{"schemaName": "Custom", "schemaId": "s1"},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SchemasListCmd{}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, `"schemas"`) {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestSchemasListCmd_MultipleSchemas(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schemas": []map[string]any{
				{"schemaName": "Schema1", "schemaId": "s1", "fields": []map[string]any{{"fieldName": "f1"}, {"fieldName": "f2"}}},
				{"schemaName": "Schema2", "schemaId": "s2", "fields": []map[string]any{{"fieldName": "f3"}}},
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

	if !strings.Contains(out, "Schema1") || !strings.Contains(out, "Schema2") {
		t.Fatalf("unexpected output: %s", out)
	}
	// Schema1 has 2 fields
	if !strings.Contains(out, "2") {
		t.Fatalf("expected field count, got: %s", out)
	}
}

func TestSchemasGetCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/schemas/Custom") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schemaName":  "Custom",
			"schemaId":    "s1",
			"displayName": "Custom Schema",
			"fields": []map[string]any{
				{"fieldName": "Department", "fieldType": "STRING"},
				{"fieldName": "EmployeeID", "fieldType": "INT64"},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SchemasGetCmd{Name: "Custom"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Name:") || !strings.Contains(out, "Custom") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "ID:") || !strings.Contains(out, "s1") {
		t.Fatalf("expected ID, got: %s", out)
	}
	if !strings.Contains(out, "Display:") || !strings.Contains(out, "Custom Schema") {
		t.Fatalf("expected display name, got: %s", out)
	}
	if !strings.Contains(out, "Department") || !strings.Contains(out, "STRING") {
		t.Fatalf("expected fields, got: %s", out)
	}
}

func TestSchemasGetCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schemaName": "Custom",
			"schemaId":   "s1",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SchemasGetCmd{Name: "Custom"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, `"schemaName"`) {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestSchemasGetCmd_MissingName(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SchemasGetCmd{Name: "  "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "schema name is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSchemasGetCmd_NoDisplayName(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schemaName": "Custom",
			"schemaId":   "s1",
			"fields":     []map[string]any{},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SchemasGetCmd{Name: "Custom"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	// Should not contain "Display:" when displayName is empty
	if strings.Contains(out, "Display:") {
		t.Fatalf("expected no display line when empty, got: %s", out)
	}
}

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

func TestSchemasCreateCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schemaName": "Custom",
				"schemaId":   "s1",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SchemasCreateCmd{Name: "Custom", Fields: []string{"Department:string"}}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, `"schemaName"`) {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestSchemasCreateCmd_MissingName(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SchemasCreateCmd{Name: "", Fields: []string{"Department:string"}}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "schema name is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSchemasCreateCmd_MissingFields(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SchemasCreateCmd{Name: "Custom", Fields: []string{}}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing fields")
	}
	if !strings.Contains(err.Error(), "--field is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSchemasCreateCmd_MultipleFields(t *testing.T) {
	var gotFields []string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var payload struct {
				Fields []struct {
					FieldName string `json:"fieldName"`
					FieldType string `json:"fieldType"`
				} `json:"fields"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			for _, f := range payload.Fields {
				gotFields = append(gotFields, f.FieldName+":"+f.FieldType)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schemaName": "Custom",
				"schemaId":   "s1",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SchemasCreateCmd{Name: "Custom", Fields: []string{"Department:string", "EmployeeID:int", "Active:bool"}}

	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(gotFields) != 3 {
		t.Fatalf("expected 3 fields, got: %v", gotFields)
	}
	if gotFields[0] != "Department:STRING" {
		t.Errorf("unexpected field: %s", gotFields[0])
	}
	if gotFields[1] != "EmployeeID:INT64" {
		t.Errorf("unexpected field: %s", gotFields[1])
	}
	if gotFields[2] != "Active:BOOL" {
		t.Errorf("unexpected field: %s", gotFields[2])
	}
}

func TestSchemasUpdateCmd(t *testing.T) {
	calls := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/schemas/Custom"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schemaName": "Custom",
				"schemaId":   "s1",
				"fields": []map[string]any{
					{"fieldName": "Department", "fieldType": "STRING"},
				},
			})
			return
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/schemas/Custom"):
			calls++
			var payload struct {
				Fields []struct {
					FieldName string `json:"fieldName"`
					FieldType string `json:"fieldType"`
				} `json:"fields"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schemaName": "Custom",
				"schemaId":   "s1",
			})
			return
		default:
			http.NotFound(w, r)
		}
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SchemasUpdateCmd{Name: "Custom", AddFields: []string{"Title:string"}}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if calls != 1 {
		t.Fatalf("expected 1 update call, got %d", calls)
	}
	if !strings.Contains(out, "Updated schema") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestSchemasUpdateCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schemaName": "Custom",
				"schemaId":   "s1",
				"fields":     []map[string]any{{"fieldName": "Department", "fieldType": "STRING"}},
			})
			return
		}
		if r.Method == http.MethodPut {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schemaName": "Custom",
				"schemaId":   "s1",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SchemasUpdateCmd{Name: "Custom", AddFields: []string{"Title:string"}}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, `"schemaName"`) {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestSchemasUpdateCmd_MissingName(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SchemasUpdateCmd{Name: "  ", AddFields: []string{"Title:string"}}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "schema name is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSchemasUpdateCmd_NoUpdates(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SchemasUpdateCmd{Name: "Custom"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for no updates")
	}
	if !strings.Contains(err.Error(), "no updates specified") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSchemasUpdateCmd_RemoveField(t *testing.T) {
	var updatedFields []string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schemaName": "Custom",
				"schemaId":   "s1",
				"fields": []map[string]any{
					{"fieldName": "Department", "fieldType": "STRING"},
					{"fieldName": "Title", "fieldType": "STRING"},
				},
			})
			return
		}
		if r.Method == http.MethodPut {
			var payload struct {
				Fields []struct {
					FieldName string `json:"fieldName"`
				} `json:"fields"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			for _, f := range payload.Fields {
				updatedFields = append(updatedFields, f.FieldName)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schemaName": "Custom",
				"schemaId":   "s1",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SchemasUpdateCmd{Name: "Custom", RemoveField: []string{"Title"}}

	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(updatedFields) != 1 || updatedFields[0] != "Department" {
		t.Fatalf("expected only Department field, got: %v", updatedFields)
	}
}

func TestSchemasUpdateCmd_FieldAlreadyExists(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schemaName": "Custom",
				"schemaId":   "s1",
				"fields": []map[string]any{
					{"fieldName": "Department", "fieldType": "STRING"},
				},
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SchemasUpdateCmd{Name: "Custom", AddFields: []string{"Department:int"}}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for duplicate field")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSchemasUpdateCmd_FieldNotFound(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schemaName": "Custom",
				"schemaId":   "s1",
				"fields": []map[string]any{
					{"fieldName": "Department", "fieldType": "STRING"},
				},
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SchemasUpdateCmd{Name: "Custom", RemoveField: []string{"NonExistent"}}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for non-existent field")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSchemasDeleteCmd(t *testing.T) {
	var deleteCalled bool
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/schemas/Custom") {
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &SchemasDeleteCmd{Name: "Custom"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !deleteCalled {
		t.Fatal("delete was not called")
	}
	if !strings.Contains(out, "Deleted schema") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestSchemasDeleteCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &SchemasDeleteCmd{Name: "Custom"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, `"deleted"`) || !strings.Contains(out, `true`) {
		t.Fatalf("expected JSON output with deleted: true, got: %s", out)
	}
}

func TestSchemasDeleteCmd_MissingName(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &SchemasDeleteCmd{Name: ""}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "schema name is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSchemasDeleteCmd_RequiresConfirmation(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com", Force: false, NoInput: true}
	cmd := &SchemasDeleteCmd{Name: "Custom"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error when confirmation is required but not provided")
	}
	if !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseSchemaFields(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		wantLen  int
		wantName string
		wantType string
		wantErr  bool
	}{
		{
			name:     "string type",
			input:    []string{"Department:string"},
			wantLen:  1,
			wantName: "Department",
			wantType: "STRING",
		},
		{
			name:     "str alias",
			input:    []string{"Name:str"},
			wantLen:  1,
			wantName: "Name",
			wantType: "STRING",
		},
		{
			name:     "int type",
			input:    []string{"Count:int"},
			wantLen:  1,
			wantName: "Count",
			wantType: "INT64",
		},
		{
			name:     "int64 type",
			input:    []string{"Count:int64"},
			wantLen:  1,
			wantName: "Count",
			wantType: "INT64",
		},
		{
			name:     "bool type",
			input:    []string{"Active:bool"},
			wantLen:  1,
			wantName: "Active",
			wantType: "BOOL",
		},
		{
			name:     "boolean type",
			input:    []string{"Active:boolean"},
			wantLen:  1,
			wantName: "Active",
			wantType: "BOOL",
		},
		{
			name:     "date type",
			input:    []string{"StartDate:date"},
			wantLen:  1,
			wantName: "StartDate",
			wantType: "DATE",
		},
		{
			name:     "double type",
			input:    []string{"Salary:double"},
			wantLen:  1,
			wantName: "Salary",
			wantType: "DOUBLE",
		},
		{
			name:     "email type",
			input:    []string{"Contact:email"},
			wantLen:  1,
			wantName: "Contact",
			wantType: "EMAIL",
		},
		{
			name:     "phone type",
			input:    []string{"Mobile:phone"},
			wantLen:  1,
			wantName: "Mobile",
			wantType: "PHONE",
		},
		{
			name:    "multiple fields",
			input:   []string{"Name:string", "Age:int", "Active:bool"},
			wantLen: 3,
		},
		{
			name:    "empty input",
			input:   []string{},
			wantLen: 0,
		},
		{
			name:    "whitespace only",
			input:   []string{"  ", "\t"},
			wantLen: 0,
		},
		{
			name:    "invalid format - no colon",
			input:   []string{"Department"},
			wantErr: true,
		},
		{
			name:    "invalid type",
			input:   []string{"Department:invalid"},
			wantErr: true,
		},
		{
			name:    "empty name",
			input:   []string{":string"},
			wantErr: true,
		},
		{
			name:     "case insensitive type",
			input:    []string{"Field:STRING"},
			wantLen:  1,
			wantName: "Field",
			wantType: "STRING",
		},
		{
			name:     "type with spaces",
			input:    []string{"Field: string "},
			wantLen:  1,
			wantName: "Field",
			wantType: "STRING",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSchemaFields(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("expected %d fields, got %d", tt.wantLen, len(got))
			}
			if tt.wantLen > 0 && tt.wantName != "" {
				if got[0].FieldName != tt.wantName {
					t.Errorf("expected name %q, got %q", tt.wantName, got[0].FieldName)
				}
				if got[0].FieldType != tt.wantType {
					t.Errorf("expected type %q, got %q", tt.wantType, got[0].FieldType)
				}
			}
		})
	}
}

func TestNormalizeSchemaFieldType(t *testing.T) {
	tests := []struct {
		input    string
		wantType string
		wantOK   bool
	}{
		{"bool", "BOOL", true},
		{"boolean", "BOOL", true},
		{"BOOL", "BOOL", true},
		{"date", "DATE", true},
		{"DATE", "DATE", true},
		{"double", "DOUBLE", true},
		{"DOUBLE", "DOUBLE", true},
		{"email", "EMAIL", true},
		{"EMAIL", "EMAIL", true},
		{"int", "INT64", true},
		{"int64", "INT64", true},
		{"INT64", "INT64", true},
		{"phone", "PHONE", true},
		{"PHONE", "PHONE", true},
		{"string", "STRING", true},
		{"str", "STRING", true},
		{"STRING", "STRING", true},
		{"  string  ", "STRING", true},
		{"invalid", "", false},
		{"", "", false},
		{"text", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotType, gotOK := normalizeSchemaFieldType(tt.input)
			if gotOK != tt.wantOK {
				t.Errorf("normalizeSchemaFieldType(%q) ok = %v, want %v", tt.input, gotOK, tt.wantOK)
			}
			if gotType != tt.wantType {
				t.Errorf("normalizeSchemaFieldType(%q) = %q, want %q", tt.input, gotType, tt.wantType)
			}
		})
	}
}
