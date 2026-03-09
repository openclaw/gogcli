package cmd

import "testing"

func TestParseSedExpr_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		expr        string
		wantPattern string
		wantReplace string
		wantGlobal  bool
		wantErr     bool
	}{
		{
			name:        "multiple backrefs",
			expr:        `s/(\w+)\s+(\w+)/\2 \1/g`,
			wantPattern: `(\w+)\s+(\w+)`,
			wantReplace: "${2} ${1}",
			wantGlobal:  true,
		},
		{
			name:        "replacement with slashes using alternate delim",
			expr:        `s#/usr/local#/opt/homebrew#g`,
			wantPattern: "/usr/local",
			wantReplace: "/opt/homebrew",
			wantGlobal:  true,
		},
		{
			name:        "empty pattern",
			expr:        "s//replacement/",
			wantPattern: "",
			wantReplace: "replacement",
			wantGlobal:  false,
		},
		{
			name:        "special regex chars in pattern",
			expr:        `s/\$\d+\.\d{2}/PRICE/g`,
			wantPattern: `\$\d+\.\d{2}`,
			wantReplace: "PRICE",
			wantGlobal:  true,
		},
		{
			name:        "newline escape preserved",
			expr:        `s/;/;\n/g`,
			wantPattern: ";",
			wantReplace: ";\\n",
			wantGlobal:  true,
		},
		{
			name:        "tab escape preserved",
			expr:        `s/,/\t/g`,
			wantPattern: ",",
			wantReplace: "\\t",
			wantGlobal:  true,
		},
		{
			name:    "just s",
			expr:    "s",
			wantErr: true,
		},
		{
			name:    "s with delimiter only",
			expr:    "s/",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern, replacement, global, err := parseSedExpr(tt.expr)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pattern != tt.wantPattern {
				t.Errorf("pattern = %q, want %q", pattern, tt.wantPattern)
			}
			if replacement != tt.wantReplace {
				t.Errorf("replacement = %q, want %q", replacement, tt.wantReplace)
			}
			if global != tt.wantGlobal {
				t.Errorf("global = %v, want %v", global, tt.wantGlobal)
			}
		})
	}
}

func TestParseMarkdownReplacement_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantText    string
		wantFormats []string
	}{
		{
			name:        "multiple newlines",
			input:       "line1\\nline2\\nline3",
			wantText:    "line1\nline2\nline3",
			wantFormats: nil,
		},
		{
			name:        "heading with trailing space",
			input:       "## Title ",
			wantText:    "Title ",
			wantFormats: []string{"heading2"},
		},
		{
			name:        "bold with spaces inside",
			input:       "**bold with spaces**",
			wantText:    "bold with spaces",
			wantFormats: []string{"bold"},
		},
		{
			name:        "not bold - unmatched asterisks",
			input:       "**not bold",
			wantText:    "**not bold",
			wantFormats: nil,
		},
		{
			name:        "not italic - single asterisk",
			input:       "*",
			wantText:    "*",
			wantFormats: nil,
		},
		{
			name:        "numbered list double digit",
			input:       "12. twelfth item",
			wantText:    "12. twelfth item",
			wantFormats: nil,
		},
		{
			name:        "code with special chars",
			input:       "`func main() {}`",
			wantText:    "func main() {}",
			wantFormats: []string{"code"},
		},
		{
			name:        "strikethrough with emoji",
			input:       "~~old value 🎉~~",
			wantText:    "old value 🎉",
			wantFormats: []string{"strikethrough"},
		},
		{
			name:        "heading 4",
			input:       "#### H4 Title",
			wantText:    "H4 Title",
			wantFormats: []string{"heading4"},
		},
		{
			name:        "heading 5",
			input:       "##### H5 Title",
			wantText:    "H5 Title",
			wantFormats: []string{"heading5"},
		},
		{
			name:        "four asterisks parsed as italic",
			input:       "****",
			wantText:    "**",
			wantFormats: []string{"italic"},
		},
		{
			name:        "empty code",
			input:       "``",
			wantText:    "``",
			wantFormats: nil,
		},
		{
			name:        "bullet then italic",
			input:       "- *italic item*",
			wantText:    "italic item",
			wantFormats: []string{"bullet", "italic"},
		},
		{
			name:        "numbered then bold",
			input:       "1. **important**",
			wantText:    "important",
			wantFormats: []string{"numbered", "bold"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, formats := parseMarkdownReplacement(tt.input)
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			if len(formats) != len(tt.wantFormats) {
				t.Errorf("formats = %v, want %v", formats, tt.wantFormats)
				return
			}
			for i, f := range formats {
				if f != tt.wantFormats[i] {
					t.Errorf("formats[%d] = %q, want %q", i, f, tt.wantFormats[i])
				}
			}
		})
	}
}
