package slidesmarkdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseInlines_PlainText(t *testing.T) {
	got := parseInlines("hello world", "solid")
	assert.Equal(t, []Inline{TextRun{Text: "hello world"}}, got)
}

func TestParseInlines_Emphasis(t *testing.T) {
	got := parseInlines("plain **bold** _ital_ `code` end", "solid")
	assert.Equal(t, []Inline{
		TextRun{Text: "plain "},
		TextRun{Text: "bold", Bold: true},
		TextRun{Text: " "},
		TextRun{Text: "ital", Italic: true},
		TextRun{Text: " "},
		TextRun{Text: "code", Code: true},
		TextRun{Text: " end"},
	}, got)
}

func TestParseInlines_FAShortcodes(t *testing.T) {
	got := parseInlines("Welcome :fa-truck-fast: to :fab-github: here", "solid")
	assert.Equal(t, []Inline{
		TextRun{Text: "Welcome "},
		IconRef{Style: "solid", Name: "truck-fast"},
		TextRun{Text: " to "},
		IconRef{Style: "brands", Name: "github"},
		TextRun{Text: " here"},
	}, got)
}

func TestParseInlines_FAStyleDerivation(t *testing.T) {
	cases := []struct {
		shortcode     string
		defaultStyle  string
		expectedStyle string
		expectedName  string
	}{
		{":fa-database:", "solid", "solid", "database"},
		{":fas-headset:", "solid", "solid", "headset"},
		{":far-clock:", "solid", "regular", "clock"},
		{":fab-github:", "solid", "brands", "github"},
		{":fal-flask:", "solid", "solid", "flask"},          // free-tier substitution
		{":fad-bug:", "solid", "solid", "bug"},              // free-tier substitution
		{":fa-database:", "regular", "regular", "database"}, // default override
	}
	for _, tc := range cases {
		t.Run(tc.shortcode, func(t *testing.T) {
			got := parseInlines(tc.shortcode, tc.defaultStyle)
			assert.Equal(t, []Inline{IconRef{Style: tc.expectedStyle, Name: tc.expectedName}}, got)
		})
	}
}

func TestStripFAShortcodes(t *testing.T) {
	got := stripFAShortcodes(":fa-truck-fast: Orders and :fab-github: GitHub")
	assert.Equal(t, " Orders and  GitHub", got)
}
