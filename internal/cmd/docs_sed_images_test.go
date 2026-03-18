package cmd

import "testing"

func TestParseImageSyntax(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantNil     bool
		wantURL     string
		wantAlt     string
		wantCaption string
		wantWidth   int
		wantHeight  int
	}{
		{
			name:    "basic image",
			input:   "![](https://example.com/image.png)",
			wantURL: "https://example.com/image.png",
		},
		{
			name:    "with alt text",
			input:   "![Company Logo](https://example.com/logo.png)",
			wantURL: "https://example.com/logo.png",
			wantAlt: "Company Logo",
		},
		{
			name:        "with title as caption",
			input:       `![Logo](https://example.com/logo.png "Our Company Logo")`,
			wantURL:     "https://example.com/logo.png",
			wantAlt:     "Logo",
			wantCaption: "Our Company Logo",
		},
		{
			name:      "with width",
			input:     "![](https://example.com/img.png){width=300}",
			wantURL:   "https://example.com/img.png",
			wantWidth: 300,
		},
		{
			name:       "with height",
			input:      "![](https://example.com/img.png){height=200}",
			wantURL:    "https://example.com/img.png",
			wantHeight: 200,
		},
		{
			name:       "with both dimensions",
			input:      "![](https://example.com/img.png){width=300 height=200}",
			wantURL:    "https://example.com/img.png",
			wantWidth:  300,
			wantHeight: 200,
		},
		{
			name:       "short dimension syntax",
			input:      "![](https://example.com/img.png){w=300 h=200}",
			wantURL:    "https://example.com/img.png",
			wantWidth:  300,
			wantHeight: 200,
		},
		{
			name:      "with px suffix",
			input:     "![](https://example.com/img.png){width=300px}",
			wantURL:   "https://example.com/img.png",
			wantWidth: 300,
		},
		{
			name:        "full syntax",
			input:       `![Logo](https://example.com/logo.png "Figure 1"){width=400 height=300}`,
			wantURL:     "https://example.com/logo.png",
			wantAlt:     "Logo",
			wantCaption: "Figure 1",
			wantWidth:   400,
			wantHeight:  300,
		},
		{
			name:    "url with query params",
			input:   "![](https://example.com/img.png?size=large&format=webp)",
			wantURL: "https://example.com/img.png?size=large&format=webp",
		},
		{
			name:    "alt with special chars",
			input:   "![A cool image!](https://example.com/img.png)",
			wantURL: "https://example.com/img.png",
			wantAlt: "A cool image!",
		},
		{
			name:    "plain text",
			input:   "hello world",
			wantNil: true,
		},
		{
			name:    "bold text",
			input:   "**bold**",
			wantNil: true,
		},
		{
			name:    "starts with ! but not image",
			input:   "!important",
			wantNil: true,
		},
		{
			name:    "no closing bracket",
			input:   "![alt text(url)",
			wantNil: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseImageSyntax(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Errorf("parseImageSyntax(%q) = %+v, want nil", tt.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseImageSyntax(%q) = nil, want non-nil", tt.input)
			}
			if got.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", got.URL, tt.wantURL)
			}
			if got.Alt != tt.wantAlt {
				t.Errorf("Alt = %q, want %q", got.Alt, tt.wantAlt)
			}
			if got.Caption != tt.wantCaption {
				t.Errorf("Caption = %q, want %q", got.Caption, tt.wantCaption)
			}
			if got.Width != tt.wantWidth {
				t.Errorf("Width = %d, want %d", got.Width, tt.wantWidth)
			}
			if got.Height != tt.wantHeight {
				t.Errorf("Height = %d, want %d", got.Height, tt.wantHeight)
			}
		})
	}
}

func TestParseImageRefPattern(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantNil      bool
		wantPosition bool
		wantPos      int
		wantAll      bool
		wantByAlt    bool
		wantRegex    string
	}{
		{"first image", "!(1)", false, true, 1, false, false, ""},
		{"second image", "!(2)", false, true, 2, false, false, ""},
		{"last image", "!(-1)", false, true, -1, false, false, ""},
		{"second to last", "!(-2)", false, true, -2, false, false, ""},
		{"all images", "!(*)", false, true, 0, true, false, ""},
		{"first image alt syntax", "![](1)", false, true, 1, false, false, ""},
		{"all images alt syntax", "![](*)", false, true, 0, true, false, ""},
		{"exact alt match", "![logo]", false, false, 0, false, true, "logo"},
		{"alt starts with", "![fig-.*]", false, false, 0, false, true, "fig-.*"},
		{"alt contains", "![.*draft.*]", false, false, 0, false, true, ".*draft.*"},
		{"alt with digits", `![img-\d+]`, false, false, 0, false, true, `img-\d+`},
		{"case insensitive", "![(?i)logo]", false, false, 0, false, true, "(?i)logo"},
		{"actual image insert", "!(https://example.com/img.png)", true, false, 0, false, false, ""},
		{"full image syntax", "![alt](https://example.com/img.png)", true, false, 0, false, false, ""},
		{"plain text", "hello world", true, false, 0, false, false, ""},
		{"empty brackets", "![]", true, false, 0, false, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseImageRefPattern(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Errorf("parseImageRefPattern(%q) = %+v, want nil", tt.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseImageRefPattern(%q) = nil, want non-nil", tt.input)
			}
			if got.ByPosition != tt.wantPosition {
				t.Errorf("ByPosition = %v, want %v", got.ByPosition, tt.wantPosition)
			}
			if got.Position != tt.wantPos {
				t.Errorf("Position = %d, want %d", got.Position, tt.wantPos)
			}
			if got.AllImages != tt.wantAll {
				t.Errorf("AllImages = %v, want %v", got.AllImages, tt.wantAll)
			}
			if got.ByAlt != tt.wantByAlt {
				t.Errorf("ByAlt = %v, want %v", got.ByAlt, tt.wantByAlt)
			}
			if tt.wantByAlt && got.AltRegex != nil && got.AltRegex.String() != tt.wantRegex {
				t.Errorf("AltRegex = %q, want %q", got.AltRegex.String(), tt.wantRegex)
			}
		})
	}
}

func TestMatchImages(t *testing.T) {
	images := []DocImage{
		{ObjectID: "img1", Index: 10, Alt: "logo"},
		{ObjectID: "img2", Index: 20, Alt: "fig-1"},
		{ObjectID: "img3", Index: 30, Alt: "fig-2"},
		{ObjectID: "img4", Index: 40, Alt: "header-draft"},
		{ObjectID: "img5", Index: 50, Alt: "footer"},
	}

	tests := []struct {
		name    string
		pattern string
		wantIDs []string
	}{
		{"first image", "!(1)", []string{"img1"}},
		{"second image", "!(2)", []string{"img2"}},
		{"last image", "!(-1)", []string{"img5"}},
		{"second to last", "!(-2)", []string{"img4"}},
		{"all images", "!(*)", []string{"img1", "img2", "img3", "img4", "img5"}},
		{"exact alt", "![logo]", []string{"img1"}},
		{"alt starts with fig", "![fig-.*]", []string{"img2", "img3"}},
		{"alt contains draft", "![.*draft.*]", []string{"img4"}},
		{"no match", "![nonexistent]", nil},
		{"out of range positive", "!(10)", nil},
		{"out of range negative", "!(-10)", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := parseImageRefPattern(tt.pattern)
			if ref == nil {
				t.Fatalf("parseImageRefPattern(%q) = nil", tt.pattern)
			}
			matched := matchImages(images, ref)
			gotIDs := make([]string, 0, len(matched))
			for _, img := range matched {
				gotIDs = append(gotIDs, img.ObjectID)
			}
			if len(gotIDs) != len(tt.wantIDs) {
				t.Errorf("matched %v, want %v", gotIDs, tt.wantIDs)
				return
			}
			for i, id := range gotIDs {
				if id != tt.wantIDs[i] {
					t.Errorf("matched[%d] = %q, want %q", i, id, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestCanUseNativeReplace_Image(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"![](https://example.com/img.png)", false},
		{"![alt](url)", false},
		{"![](url){width=100}", false},
		{"!(https://example.com/img.png)", false},
		{"plain text", true},
	}

	for _, tt := range tests {
		got := canUseNativeReplace(tt.input)
		if got != tt.want {
			t.Errorf("canUseNativeReplace(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestImageShorthandSyntax(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantURL string
		wantNil bool
	}{
		{"shorthand http", "!(http://example.com/img.png)", "http://example.com/img.png", false},
		{"shorthand https", "!(https://example.com/img.png)", "https://example.com/img.png", false},
		{"shorthand with query", "!(https://example.com/img.png?w=100)", "https://example.com/img.png?w=100", false},
		{"positional ref not url", "!(1)", "", true},
		{"all images ref", "!(*)", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := parseImageRefPattern(tt.input)
			if ref != nil && !tt.wantNil {
				t.Errorf("parseImageRefPattern(%q) matched as reference, expected URL", tt.input)
				return
			}

			img := parseImageSyntax(tt.input)
			if tt.wantNil {
				if img != nil {
					t.Errorf("parseImageSyntax(%q) = %+v, want nil", tt.input, img)
				}
				return
			}
			if ref != nil {
				t.Errorf("!(url) should not be parsed as reference")
			}
		})
	}
}
