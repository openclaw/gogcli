package csv

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var errCallbackFailed = errors.New("callback failed")

// Helper to create temp CSV files for testing
func createTempCSV(t *testing.T, content string) string {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "test-*.csv")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	return f.Name()
}

// ─────────────────────────────────────────────────────────────────────────────
// SubstituteArgs Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestSubstituteArgs_SimpleSubstitution(t *testing.T) {
	row := Row{
		Index: 1,
		Values: map[string]string{
			"email":     "user@example.com",
			"firstname": "John",
			"lastname":  "Doe",
		},
	}

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "single field",
			args: []string{"~email"},
			want: []string{"user@example.com"},
		},
		{
			name: "multiple fields",
			args: []string{"~firstname", "~lastname"},
			want: []string{"John", "Doe"},
		},
		{
			name: "mixed literal and field",
			args: []string{"--name", "~firstname", "--email", "~email"},
			want: []string{"--name", "John", "--email", "user@example.com"},
		},
		{
			name: "field not in row returns empty",
			args: []string{"~missing"},
			want: []string{""},
		},
		{
			name: "case insensitive field name",
			args: []string{"~EMAIL", "~FirstName"},
			want: []string{"user@example.com", "John"},
		},
		{
			name: "empty tilde returns empty",
			args: []string{"~"},
			want: []string{""},
		},
		{
			name: "tilde with spaces normalizes",
			args: []string{"~ email "},
			want: []string{"user@example.com"},
		},
		{
			name: "no substitution for literal",
			args: []string{"literal", "text"},
			want: []string{"literal", "text"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SubstituteArgs(tt.args, row)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %d args, want %d", len(got), len(tt.want))
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("arg[%d]: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSubstituteArgs_AdvancedSubstitution(t *testing.T) {
	row := Row{
		Index: 1,
		Values: map[string]string{
			"email":     "user@example.com",
			"firstname": "John",
			"lastname":  "Doe",
		},
	}

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "double tilde substitution",
			args: []string{"Hello ~~firstname~~!"},
			want: []string{"Hello John!"},
		},
		{
			name: "multiple double tilde in single arg",
			args: []string{"~~firstname~~ ~~lastname~~"},
			want: []string{"John Doe"},
		},
		{
			name: "double tilde in quoted string",
			args: []string{`"Name: ~~firstname~~ ~~lastname~~"`},
			want: []string{`"Name: John Doe"`},
		},
		{
			name: "double tilde missing field returns empty",
			args: []string{"Hello ~~missing~~!"},
			want: []string{"Hello !"},
		},
		{
			name: "unclosed double tilde left as is",
			args: []string{"Hello ~~firstname"},
			want: []string{"Hello ~~firstname"},
		},
		{
			name: "single tilde inside double tilde context",
			args: []string{"prefix ~~email~~ suffix"},
			want: []string{"prefix user@example.com suffix"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SubstituteArgs(tt.args, row)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %d args, want %d", len(got), len(tt.want))
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("arg[%d]: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSubstituteArgs_RegexReplacement(t *testing.T) {
	row := Row{
		Index: 1,
		Values: map[string]string{
			"email":   "user@example.com",
			"phone":   "+1-555-123-4567",
			"name":    "John Doe",
			"country": "USA",
		},
	}

	tests := []struct {
		name    string
		args    []string
		want    []string
		wantErr bool
	}{
		{
			name: "regex with replacement string",
			args: []string{"~~email~!~@example.com~!~@company.org~~"},
			want: []string{"user@company.org"},
		},
		{
			name: "capture group replacement",
			args: []string{"~~name~!~(\\w+) (\\w+)~!~$2, $1~~"},
			want: []string{"Doe, John"},
		},
		{
			name: "regex no match leaves unchanged",
			args: []string{"~~country~!~XXX~!~YYY~~"},
			want: []string{"USA"},
		},
		{
			name: "regex replace with single char",
			args: []string{"~~phone~!~[^0-9]~!~_~~"},
			want: []string{"_1_555_123_4567"},
		},
		{
			name: "extract username with capture group",
			args: []string{"~~email~!~(.*)@.*~!~$1~~"},
			want: []string{"user"},
		},
		{
			name: "multiple regex replacements in one arg",
			args: []string{"~~name~!~ ~!~_~~ at ~~email~!~@~!~ AT ~~"},
			want: []string{"John_Doe at user AT example.com"},
		},
		{
			name:    "invalid regex pattern",
			args:    []string{"~~email~!~[invalid~!~x~~"},
			wantErr: true,
		},
		{
			name:    "malformed replacement token (too few parts)",
			args:    []string{"~~email~!~pattern~~"},
			wantErr: true,
		},
		{
			name:    "malformed replacement token (too many parts)",
			args:    []string{"~~email~!~a~!~b~!~c~~"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SubstituteArgs(tt.args, row)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %d args, want %d", len(got), len(tt.want))
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("arg[%d]: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSubstituteArgs_EmptyArgs(t *testing.T) {
	row := Row{Index: 1, Values: map[string]string{"a": "b"}}

	got, err := SubstituteArgs([]string{}, row)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ParseFieldFilters Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestParseFieldFilters_Valid(t *testing.T) {
	tests := []struct {
		name   string
		inputs []string
		want   []struct {
			field   string
			pattern string
		}
	}{
		{
			name:   "single filter",
			inputs: []string{"status:active"},
			want:   []struct{ field, pattern string }{{field: "status", pattern: "active"}},
		},
		{
			name:   "multiple filters",
			inputs: []string{"status:active", "role:admin"},
			want: []struct{ field, pattern string }{
				{field: "status", pattern: "active"},
				{field: "role", pattern: "admin"},
			},
		},
		{
			name:   "regex pattern",
			inputs: []string{"email:.*@example\\.com$"},
			want:   []struct{ field, pattern string }{{field: "email", pattern: ".*@example\\.com$"}},
		},
		{
			name:   "field name normalized to lowercase",
			inputs: []string{"Status:active", "EMAIL:test"},
			want: []struct{ field, pattern string }{
				{field: "status", pattern: "active"},
				{field: "email", pattern: "test"},
			},
		},
		{
			name:   "empty input list",
			inputs: []string{},
			want:   []struct{ field, pattern string }{},
		},
		{
			name:   "outer whitespace trimmed field normalized",
			inputs: []string{"  status :active"},
			want:   []struct{ field, pattern string }{{field: "status", pattern: "active"}},
		},
		{
			name:   "empty string skipped",
			inputs: []string{"", "  ", "status:ok"},
			want:   []struct{ field, pattern string }{{field: "status", pattern: "ok"}},
		},
		{
			name:   "colon in regex pattern",
			inputs: []string{"time:^\\d{2}:\\d{2}$"},
			want:   []struct{ field, pattern string }{{field: "time", pattern: "^\\d{2}:\\d{2}$"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFieldFilters(tt.inputs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %d filters, want %d", len(got), len(tt.want))
			}

			for i, wf := range tt.want {
				if got[i].Field != wf.field {
					t.Errorf("filter[%d].Field: got %q, want %q", i, got[i].Field, wf.field)
				}

				if got[i].Regex.String() != wf.pattern {
					t.Errorf("filter[%d].Regex: got %q, want %q", i, got[i].Regex.String(), wf.pattern)
				}
			}
		})
	}
}

func TestParseFieldFilters_Errors(t *testing.T) {
	tests := []struct {
		name    string
		inputs  []string
		wantErr string
	}{
		{
			name:    "missing colon",
			inputs:  []string{"statusactive"},
			wantErr: "invalid filter",
		},
		{
			name:    "missing field name",
			inputs:  []string{":active"},
			wantErr: "missing field",
		},
		{
			name:    "invalid regex",
			inputs:  []string{"status:[invalid"},
			wantErr: "invalid filter regex",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseFieldFilters(tt.inputs)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Process Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestProcess_BasicCSV(t *testing.T) {
	csv := `email,firstName,lastName
alice@example.com,Alice,Smith
bob@example.com,Bob,Jones
charlie@example.com,Charlie,Brown
`
	path := createTempCSV(t, csv)

	var rows []Row

	err := Process(path, Options{}, func(row Row) error {
		rows = append(rows, row)
		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// Verify first row
	if rows[0].Index != 1 {
		t.Errorf("row[0].Index: got %d, want 1", rows[0].Index)
	}

	if rows[0].Values["email"] != "alice@example.com" {
		t.Errorf("row[0].email: got %q", rows[0].Values["email"])
	}

	if rows[0].Values["firstname"] != "Alice" {
		t.Errorf("row[0].firstname: got %q", rows[0].Values["firstname"])
	}
}

func TestProcess_HeaderNormalization(t *testing.T) {
	csv := `  Email  ,  First Name  , LAST_NAME
test@test.com,Test,User
`
	path := createTempCSV(t, csv)

	var row Row

	err := Process(path, Options{}, func(r Row) error {
		row = r
		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	// Headers should be normalized to lowercase and trimmed
	if row.Values["email"] != "test@test.com" {
		t.Errorf("email field not found or wrong: %v", row.Values)
	}

	if row.Values["first name"] != "Test" {
		t.Errorf("first name field not found or wrong: %v", row.Values)
	}

	if row.Values["last_name"] != "User" {
		t.Errorf("last_name field not found or wrong: %v", row.Values)
	}
}

func TestProcess_FieldSelection(t *testing.T) {
	csv := `email,name,role,status
user@test.com,User,admin,active
`
	path := createTempCSV(t, csv)

	var row Row

	err := Process(path, Options{Fields: []string{"email", "role"}}, func(r Row) error {
		row = r
		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	// Only selected fields should be present
	if _, ok := row.Values["email"]; !ok {
		t.Error("email field should be present")
	}

	if _, ok := row.Values["role"]; !ok {
		t.Error("role field should be present")
	}

	if _, ok := row.Values["name"]; ok {
		t.Error("name field should NOT be present")
	}

	if _, ok := row.Values["status"]; ok {
		t.Error("status field should NOT be present")
	}
}

func TestProcess_MatchFilter(t *testing.T) {
	csv := `email,status,role
alice@test.com,active,admin
bob@test.com,inactive,user
charlie@test.com,active,user
`
	path := createTempCSV(t, csv)

	// Use anchored regex ^active$ to match exactly "active", not "inactive"
	matchFilters, _ := ParseFieldFilters([]string{"status:^active$"})

	var emails []string

	err := Process(path, Options{Match: matchFilters}, func(row Row) error {
		emails = append(emails, row.Values["email"])
		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	if len(emails) != 2 {
		t.Fatalf("expected 2 matching rows, got %d", len(emails))
	}

	if emails[0] != "alice@test.com" || emails[1] != "charlie@test.com" {
		t.Errorf("wrong emails matched: %v", emails)
	}
}

func TestProcess_SkipFilter(t *testing.T) {
	csv := `email,status,role
alice@test.com,active,admin
bob@test.com,inactive,user
charlie@test.com,active,user
`
	path := createTempCSV(t, csv)

	skipFilters, _ := ParseFieldFilters([]string{"status:inactive"})

	var emails []string

	err := Process(path, Options{Skip: skipFilters}, func(row Row) error {
		emails = append(emails, row.Values["email"])
		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	if len(emails) != 2 {
		t.Fatalf("expected 2 rows (skipping inactive), got %d", len(emails))
	}

	for _, e := range emails {
		if e == "bob@test.com" {
			t.Error("bob@test.com should have been skipped")
		}
	}
}

func TestProcess_MatchAndSkipCombined(t *testing.T) {
	csv := `email,status,role
alice@test.com,active,admin
bob@test.com,active,user
charlie@test.com,active,guest
dave@test.com,inactive,admin
`
	path := createTempCSV(t, csv)

	// Use anchored regex ^active$ to avoid matching "inactive"
	matchFilters, _ := ParseFieldFilters([]string{"status:^active$"})
	skipFilters, _ := ParseFieldFilters([]string{"role:^guest$"})

	var emails []string

	err := Process(path, Options{Match: matchFilters, Skip: skipFilters}, func(row Row) error {
		emails = append(emails, row.Values["email"])
		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	// Should match active users, but skip guests
	if len(emails) != 2 {
		t.Fatalf("expected 2 rows, got %d: %v", len(emails), emails)
	}

	expected := map[string]bool{"alice@test.com": true, "bob@test.com": true}
	for _, e := range emails {
		if !expected[e] {
			t.Errorf("unexpected email: %s", e)
		}
	}
}

func TestProcess_MatchWithRegex(t *testing.T) {
	csv := `email,domain
alice@example.com,example.com
bob@test.org,test.org
charlie@example.net,example.net
`
	path := createTempCSV(t, csv)

	matchFilters, _ := ParseFieldFilters([]string{"email:@example\\."})

	var emails []string

	err := Process(path, Options{Match: matchFilters}, func(row Row) error {
		emails = append(emails, row.Values["email"])
		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	if len(emails) != 2 {
		t.Fatalf("expected 2 rows matching @example., got %d", len(emails))
	}
}

func TestProcess_SkipRows(t *testing.T) {
	csv := `email,name
row1@test.com,Row1
row2@test.com,Row2
row3@test.com,Row3
row4@test.com,Row4
`
	path := createTempCSV(t, csv)

	var emails []string

	err := Process(path, Options{SkipRows: 2}, func(row Row) error {
		emails = append(emails, row.Values["email"])
		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	if len(emails) != 2 {
		t.Fatalf("expected 2 rows after skipping 2, got %d", len(emails))
	}

	if emails[0] != "row3@test.com" {
		t.Errorf("first email should be row3@test.com, got %s", emails[0])
	}
}

func TestProcess_MaxRows(t *testing.T) {
	csv := `email,name
row1@test.com,Row1
row2@test.com,Row2
row3@test.com,Row3
row4@test.com,Row4
`
	path := createTempCSV(t, csv)

	var count int

	err := Process(path, Options{MaxRows: 2}, func(row Row) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 rows with MaxRows=2, got %d", count)
	}
}

func TestProcess_SkipAndMaxRows(t *testing.T) {
	csv := `email,name
row1@test.com,Row1
row2@test.com,Row2
row3@test.com,Row3
row4@test.com,Row4
row5@test.com,Row5
`
	path := createTempCSV(t, csv)

	var emails []string

	err := Process(path, Options{SkipRows: 2, MaxRows: 2}, func(row Row) error {
		emails = append(emails, row.Values["email"])
		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	if len(emails) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(emails))
	}

	if emails[0] != "row3@test.com" || emails[1] != "row4@test.com" {
		t.Errorf("wrong emails: %v", emails)
	}
}

func TestProcess_CallbackError(t *testing.T) {
	csv := `email,name
alice@test.com,Alice
bob@test.com,Bob
`
	path := createTempCSV(t, csv)

	var count int
	err := Process(path, Options{}, func(row Row) error {
		count++
		if count == 1 {
			return errCallbackFailed
		}

		return nil
	})

	if !errors.Is(err, errCallbackFailed) {
		t.Errorf("expected callback error, got: %v", err)
	}

	if count != 1 {
		t.Errorf("callback should have been called once, got %d", count)
	}
}

func TestProcess_EmptyCSV(t *testing.T) {
	path := createTempCSV(t, "")

	err := Process(path, Options{}, func(row Row) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "empty csv") {
		t.Errorf("expected empty csv error, got: %v", err)
	}
}

func TestProcess_HeaderOnly(t *testing.T) {
	csv := `email,name,status
`
	path := createTempCSV(t, csv)

	var count int

	err := Process(path, Options{}, func(row Row) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 rows (header only), got %d", count)
	}
}

func TestProcess_FileNotFound(t *testing.T) {
	err := Process("/nonexistent/path.csv", Options{}, func(row Row) error {
		return nil
	})
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestProcess_EmptyPath(t *testing.T) {
	err := Process("", Options{}, func(row Row) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "file is required") {
		t.Errorf("expected 'file is required' error, got: %v", err)
	}
}

func TestProcess_WhitespacePath(t *testing.T) {
	err := Process("   ", Options{}, func(row Row) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "file is required") {
		t.Errorf("expected 'file is required' error, got: %v", err)
	}
}

func TestProcess_ShortRowFails(t *testing.T) {
	// Go's CSV reader by default requires consistent field counts
	// Row with fewer columns than header should fail
	csv := `email,name,role
alice@test.com,Alice
`
	path := createTempCSV(t, csv)

	err := Process(path, Options{}, func(r Row) error {
		return nil
	})

	// Should fail because CSV reader enforces consistent field counts
	if err == nil {
		t.Error("expected error for short row")
	}

	if !strings.Contains(err.Error(), "wrong number of fields") {
		t.Errorf("expected 'wrong number of fields' error, got: %v", err)
	}
}

func TestProcess_ValueTrimming(t *testing.T) {
	csv := `email,name
  alice@test.com  ,  Alice
`
	path := createTempCSV(t, csv)

	var row Row

	err := Process(path, Options{}, func(r Row) error {
		row = r
		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	// Values should be trimmed
	if row.Values["email"] != "alice@test.com" {
		t.Errorf("email not trimmed: %q", row.Values["email"])
	}

	if row.Values["name"] != "Alice" {
		t.Errorf("name not trimmed: %q", row.Values["name"])
	}
}

func TestProcess_EmptyHeaderColumn(t *testing.T) {
	// Empty column header should be skipped
	csv := `email,,name
alice@test.com,ignored,Alice
`
	path := createTempCSV(t, csv)

	var row Row

	err := Process(path, Options{}, func(r Row) error {
		row = r
		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	if _, ok := row.Values[""]; ok {
		t.Error("empty header column should not create a value")
	}

	if row.Values["email"] != "alice@test.com" {
		t.Errorf("wrong email: %q", row.Values["email"])
	}

	if row.Values["name"] != "Alice" {
		t.Errorf("wrong name: %q", row.Values["name"])
	}
}

func TestProcess_RowIndexing(t *testing.T) {
	csv := `email
row1@test.com
row2@test.com
row3@test.com
`
	path := createTempCSV(t, csv)

	var indices []int

	err := Process(path, Options{}, func(row Row) error {
		indices = append(indices, row.Index)
		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	// Row indices should be 1, 2, 3 (1-based, after header)
	expected := []int{1, 2, 3}
	for i, want := range expected {
		if indices[i] != want {
			t.Errorf("indices[%d]: got %d, want %d", i, indices[i], want)
		}
	}
}

func TestProcess_MalformedCSV(t *testing.T) {
	// Create a file with malformed CSV (unmatched quotes)
	dir := t.TempDir()
	path := filepath.Join(dir, "malformed.csv")
	// Unbalanced quotes cause csv.ReadAll to fail
	err := os.WriteFile(path, []byte(`email,name
"alice@test.com,"Alice
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	err = Process(path, Options{}, func(row Row) error {
		return nil
	})
	if err == nil {
		t.Error("expected error for malformed CSV")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Integration: SubstituteArgs with Process
// ─────────────────────────────────────────────────────────────────────────────

func TestIntegration_SubstituteWithProcess(t *testing.T) {
	csv := `email,firstName,lastName,dept
alice@example.com,Alice,Smith,Engineering
bob@example.com,Bob,Jones,Sales
`
	path := createTempCSV(t, csv)

	template := []string{
		"--email", "~email",
		"--name", "~~firstName~~ ~~lastName~~",
		"--dept", "~~dept~!~Engineering~!~Eng~~",
	}

	var results [][]string

	err := Process(path, Options{}, func(row Row) error {
		args, err := SubstituteArgs(template, row)
		if err != nil {
			return err
		}

		results = append(results, args)

		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First row
	expected1 := []string{"--email", "alice@example.com", "--name", "Alice Smith", "--dept", "Eng"}
	for i, want := range expected1 {
		if results[0][i] != want {
			t.Errorf("results[0][%d]: got %q, want %q", i, results[0][i], want)
		}
	}

	// Second row (Sales stays unchanged)
	if results[1][5] != "Sales" {
		t.Errorf("results[1][5]: got %q, want %q", results[1][5], "Sales")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Additional Edge Case Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestProcess_MultipleMatchFilters(t *testing.T) {
	csv := `email,status,role
alice@test.com,active,admin
bob@test.com,active,user
charlie@test.com,inactive,admin
dave@test.com,active,guest
`
	path := createTempCSV(t, csv)

	// Both filters must match (AND logic)
	matchFilters, _ := ParseFieldFilters([]string{"status:^active$", "role:^admin$"})

	var emails []string

	err := Process(path, Options{Match: matchFilters}, func(row Row) error {
		emails = append(emails, row.Values["email"])
		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	// Only alice matches both active AND admin
	if len(emails) != 1 || emails[0] != "alice@test.com" {
		t.Errorf("expected only alice, got: %v", emails)
	}
}

func TestProcess_MultipleSkipFilters(t *testing.T) {
	csv := `email,status,role
alice@test.com,active,admin
bob@test.com,inactive,user
charlie@test.com,suspended,admin
dave@test.com,active,guest
`
	path := createTempCSV(t, csv)

	// Skip if any filter matches (OR logic)
	skipFilters, _ := ParseFieldFilters([]string{"status:inactive", "status:suspended"})

	var emails []string

	err := Process(path, Options{Skip: skipFilters}, func(row Row) error {
		emails = append(emails, row.Values["email"])
		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	// alice and dave should remain (bob=inactive, charlie=suspended skipped)
	if len(emails) != 2 {
		t.Fatalf("expected 2 rows, got %d: %v", len(emails), emails)
	}
}

func TestProcess_NilRegexInFilter(t *testing.T) {
	// Test that nil Regex in filter is handled gracefully
	csv := `email,status
alice@test.com,active
`
	path := createTempCSV(t, csv)

	// Create filter with nil regex manually
	nilFilter := FieldFilter{Field: "status", Regex: nil}

	var count int

	err := Process(path, Options{Match: []FieldFilter{nilFilter}}, func(row Row) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	// With nil regex, filter should be skipped (passes by default)
	if count != 1 {
		t.Errorf("expected 1 row (nil filter ignored), got %d", count)
	}
}

func TestSubstituteArgs_NestedTildes(t *testing.T) {
	row := Row{
		Index: 1,
		Values: map[string]string{
			"path": "/home/~user/files",
		},
	}

	// Value contains literal tilde - should be preserved
	args, err := SubstituteArgs([]string{"~~path~~"}, row)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if args[0] != "/home/~user/files" {
		t.Errorf("tilde in value should be preserved, got: %s", args[0])
	}
}

func TestSubstituteArgs_EmptyRow(t *testing.T) {
	row := Row{Index: 1, Values: map[string]string{}}

	args, err := SubstituteArgs([]string{"~missing", "~~also_missing~~"}, row)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if args[0] != "" || args[1] != "" {
		t.Errorf("missing fields should return empty strings, got: %v", args)
	}
}

func TestParseFieldFilters_ComplexRegex(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		pattern string
	}{
		{
			name:    "email regex with escaped dots",
			input:   "email:^[a-z0-9._%+-]+@[a-z0-9.-]+\\.[a-z]{2,}$",
			pattern: "^[a-z0-9._%+-]+@[a-z0-9.-]+\\.[a-z]{2,}$",
		},
		{
			name:    "phone regex",
			input:   "phone:^\\+?[0-9]{10,14}$",
			pattern: "^\\+?[0-9]{10,14}$",
		},
		{
			name:    "date regex",
			input:   "date:^\\d{4}-\\d{2}-\\d{2}$",
			pattern: "^\\d{4}-\\d{2}-\\d{2}$",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filters, err := ParseFieldFilters([]string{tt.input})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(filters) != 1 {
				t.Fatalf("expected 1 filter, got %d", len(filters))
			}

			if filters[0].Regex.String() != tt.pattern {
				t.Errorf("pattern mismatch: got %q, want %q", filters[0].Regex.String(), tt.pattern)
			}
		})
	}
}

func TestProcess_SpecialCharactersInCSV(t *testing.T) {
	// CSV with special characters that need proper handling
	csv := `email,note,tags
alice@test.com,"Contains, commas",tag1|tag2
bob@test.com,"Has ""quotes""",tag3
charlie@test.com,Normal text,tag4|tag5|tag6
`
	path := createTempCSV(t, csv)

	var rows []Row

	err := Process(path, Options{}, func(row Row) error {
		rows = append(rows, row)
		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// Check that commas inside quotes are preserved
	if rows[0].Values["note"] != "Contains, commas" {
		t.Errorf("row[0].note: got %q, want %q", rows[0].Values["note"], "Contains, commas")
	}

	// Check that escaped quotes are unescaped
	if rows[1].Values["note"] != `Has "quotes"` {
		t.Errorf("row[1].note: got %q, want %q", rows[1].Values["note"], `Has "quotes"`)
	}
}

func TestProcess_UnicodeInCSV(t *testing.T) {
	csv := `name,city,emoji
日本語,東京,🎌
Español,México,🇲🇽
Français,Paris,🇫🇷
`
	path := createTempCSV(t, csv)

	var rows []Row

	err := Process(path, Options{}, func(row Row) error {
		rows = append(rows, row)
		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	if rows[0].Values["name"] != "日本語" || rows[0].Values["city"] != "東京" {
		t.Errorf("Unicode not preserved in row[0]: %v", rows[0].Values)
	}

	if rows[0].Values["emoji"] != "🎌" {
		t.Errorf("Emoji not preserved: got %q", rows[0].Values["emoji"])
	}
}

func TestProcess_ReadFromStdin(t *testing.T) {
	csv := `email,name,status
alice@test.com,Alice,active
bob@test.com,Bob,inactive
`

	// Create a pipe to simulate stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	defer r.Close()

	// Write CSV data to the pipe
	go func() {
		defer w.Close()

		if _, writeErr := w.WriteString(csv); writeErr != nil {
			t.Errorf("write to pipe: %v", writeErr)
		}
	}()

	// Temporarily replace stdin
	oldStdin := os.Stdin
	os.Stdin = r

	defer func() { os.Stdin = oldStdin }()

	var rows []Row

	err = Process("-", Options{}, func(row Row) error {
		rows = append(rows, row)
		return nil
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if rows[0].Values["email"] != "alice@test.com" {
		t.Errorf("row[0].email: got %q, want %q", rows[0].Values["email"], "alice@test.com")
	}

	if rows[1].Values["name"] != "Bob" {
		t.Errorf("row[1].name: got %q, want %q", rows[1].Values["name"], "Bob")
	}
}
