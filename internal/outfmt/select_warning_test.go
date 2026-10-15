package outfmt

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestWriteJSON_UnmatchedSelectWarnings(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    any
		fields   []string
		want     string
		warnings string
	}{
		{"unknown field", []any{map[string]any{"name": "one"}}, []string{"missing"}, "[{}]", "warning: --select path \"missing\" matched no objects in this result\n"},
		{"mixed", []any{map[string]any{"name": "one"}}, []string{"name", "bogus", "bogus"}, "[{\"name\":\"one\"}]", "warning: --select path \"bogus\" matched no objects in this result\n"},
		{"partial", []any{map[string]any{"name": "one"}, map[string]any{"color": "blue"}}, []string{"name", "color"}, "[{\"name\":\"one\"},{\"color\":\"blue\"}]", ""},
		{"null", map[string]any{"name": nil}, []string{"name"}, "{\"name\":null}", ""},
		{"empty list", []any{}, []string{"name"}, "[]", ""},
		{"scalar", "text", []string{"name"}, "\"text\"", ""},
		{"nested array index", map[string]any{"items": []any{map[string]any{"id": "one"}}}, []string{"items.0.id"}, "{\"items.0.id\":\"one\"}", ""},
		{"no array broadcast", map[string]any{"items": []any{map[string]any{"id": "one"}}}, []string{"items.id"}, "{}", "warning: --select path \"items.id\" matched no objects in this result\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output, warnings bytes.Buffer

			ctx := WithJSONTransform(context.Background(), JSONTransform{Select: tc.fields, Warnings: &warnings})
			if err := WriteJSON(ctx, &output, tc.input); err != nil {
				t.Fatal(err)
			}

			compact := strings.Join(strings.Fields(output.String()), "")
			if compact != tc.want || warnings.String() != tc.warnings {
				t.Fatalf("output=%q warnings=%q; want output=%q warnings=%q", compact, warnings.String(), tc.want, tc.warnings)
			}
		})
	}
}
