package cmd

import (
	"regexp"
	"testing"
)

func TestImageRefPatternEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantNil bool
		desc    string
	}{
		{"zero position", "!(0)", false, "position 0 parses but won't match anything"},
		{"float position", "!(1.5)", true, "floats not supported"},
		{"negative zero", "!(-0)", false, "parses as 0, won't match anything"},
		{"large positive", "!(999)", false, "valid, will just not match"},
		{"large negative", "!(-999)", false, "valid, will just not match"},
		{"empty parens", "!()", true, "empty is invalid"},
		{"space in parens", "!( 1 )", true, "spaces not trimmed"},
		{"alt with spaces", "![my logo]", false, "spaces in alt ok"},
		{"complex regex", `![^fig-\d{2,4}$]`, false, "complex regex ok"},
		{"invalid regex", "![[invalid]", true, "unclosed bracket"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseImageRefPattern(tt.input)
			if tt.wantNil && got != nil {
				t.Errorf("parseImageRefPattern(%q) = %+v, want nil (%s)", tt.input, got, tt.desc)
			}
			if !tt.wantNil && got == nil {
				t.Errorf("parseImageRefPattern(%q) = nil, want non-nil (%s)", tt.input, tt.desc)
			}
		})
	}
}

func TestMatchImagesEdgeCases(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		ref := parseImageRefPattern("!(1)")
		matched := matchImages(nil, ref)
		if len(matched) != 0 {
			t.Errorf("expected no matches for empty list")
		}
	})

	t.Run("single image", func(t *testing.T) {
		images := []DocImage{{ObjectID: "only", Alt: "solo"}}

		ref1 := parseImageRefPattern("!(1)")
		if m := matchImages(images, ref1); len(m) != 1 || m[0].ObjectID != "only" {
			t.Errorf("!(1) should match single image")
		}

		ref2 := parseImageRefPattern("!(-1)")
		if m := matchImages(images, ref2); len(m) != 1 || m[0].ObjectID != "only" {
			t.Errorf("!(-1) should match single image")
		}

		ref3 := parseImageRefPattern("!(2)")
		if m := matchImages(images, ref3); len(m) != 0 {
			t.Errorf("!(2) should not match single image")
		}
	})

	t.Run("empty alt", func(t *testing.T) {
		images := []DocImage{
			{ObjectID: "img1", Alt: ""},
			{ObjectID: "img2", Alt: "has-alt"},
		}

		ref := parseImageRefPattern("![^$]")
		matched := matchImages(images, ref)
		if len(matched) != 1 || matched[0].ObjectID != "img1" {
			t.Errorf("![^$] should match empty alt")
		}
	})

	t.Run("special chars in alt", func(t *testing.T) {
		images := []DocImage{
			{ObjectID: "img1", Alt: "image (1)"},
			{ObjectID: "img2", Alt: "image [2]"},
		}

		ref := parseImageRefPattern(`![image \(1\)]`)
		matched := matchImages(images, ref)
		if len(matched) != 1 || matched[0].ObjectID != "img1" {
			t.Errorf("escaped parens should match")
		}
	})
}

func TestCanUseNativeReplace(t *testing.T) {
	tests := []struct {
		name        string
		replacement string
		want        bool
	}{
		{"plain text", "hello world", true},
		{"with numbers", "item123", true},
		{"with special chars", "foo@bar.com", true},
		{"empty", "", true},
		{"single word", "replaced", true},
		{"path", "/usr/local/bin", true},
		{"url", "https://example.com", true},
		{"bold", "**bold**", false},
		{"italic", "*italic*", false},
		{"bold partial", "some **bold** text", false},
		{"strikethrough", "~~struck~~", false},
		{"code", "`code`", false},
		{"heading 1", "# Heading", false},
		{"heading 2", "## Heading", false},
		{"heading 3", "### Heading", false},
		{"bullet dash", "- item", false},
		{"bullet plus", "+ item", false},
		{"numbered list", "1. first", false},
		{"newline escape", "line1\\nline2", false},
		{"asterisk not format", "5 * 3 = 15", false},
		{"hash in middle", "item #123", true},
		{"dash in middle", "foo-bar", true},
		{"number without dot space", "123", true},
		{"heading no space", "#hashtag", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canUseNativeReplace(tt.replacement)
			if got != tt.want {
				t.Errorf("canUseNativeReplace(%q) = %v, want %v", tt.replacement, got, tt.want)
			}
		})
	}
}

func BenchmarkParseSedExpr(b *testing.B) {
	expr := `s/(\w+)@(\w+)\.(\w+)/\1[at]\2[dot]\3/g`
	for i := 0; i < b.N; i++ {
		_, _, _, _ = parseSedExpr(expr)
	}
}

func BenchmarkParseMarkdownReplacement(b *testing.B) {
	inputs := []string{
		"plain text",
		"**bold text**",
		"- bullet with **bold**",
		"### Heading Three",
	}
	for i := 0; i < b.N; i++ {
		for _, input := range inputs {
			parseMarkdownReplacement(input)
		}
	}
}

func BenchmarkRegexReplace(b *testing.B) {
	re := regexp.MustCompile(`\b(\w+)\b`)
	input := "The quick brown fox jumps over the lazy dog"
	replacement := `"$1"`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		re.ReplaceAllString(input, replacement)
	}
}
