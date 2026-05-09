package cmd

import (
	"strings"
	"testing"
)

func TestExtractMarkdownImages_AngleBracketRefWithSpaces(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantRef string
	}{
		{
			name:    "no title",
			content: "before ![chart](<images/weekly chart.png>) after",
			wantRef: "images/weekly chart.png",
		},
		{
			name:    "double quoted title",
			content: "![chart](<images/weekly chart.png> \"Quarterly\")",
			wantRef: "images/weekly chart.png",
		},
		{
			name:    "single quoted title",
			content: "![chart](<images/weekly chart.png> 'Quarterly')",
			wantRef: "images/weekly chart.png",
		},
	}

	origToken := imgPlaceholderToken
	t.Cleanup(func() { imgPlaceholderToken = origToken })
	imgPlaceholderToken = func() string { return "test" }

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cleaned, images := extractMarkdownImages(tc.content)
			if len(images) != 1 {
				t.Fatalf("expected 1 image, got %d", len(images))
			}
			if images[0].originalRef != tc.wantRef {
				t.Fatalf("originalRef = %q, want %q", images[0].originalRef, tc.wantRef)
			}
			if !strings.Contains(cleaned, "<<IMG_test_0>>") {
				t.Fatalf("expected placeholder in cleaned content, got %q", cleaned)
			}
		})
	}
}
