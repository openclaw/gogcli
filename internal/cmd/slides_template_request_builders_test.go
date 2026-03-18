package cmd

import (
	"testing"

	"google.golang.org/api/slides/v1"
)

func TestTemplateReplacementSearchText(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		exact bool
		want  string
	}{
		{name: "exact keeps raw key", key: "name", exact: true, want: "name"},
		{name: "non-exact wraps bare key", key: "name", want: "{{name}}"},
		{name: "non-exact keeps wrapped key", key: "{{name}}", want: "{{name}}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := templateReplacementSearchText(tt.key, tt.exact); got != tt.want {
				t.Fatalf("templateReplacementSearchText(%q, %v) = %q, want %q", tt.key, tt.exact, got, tt.want)
			}
		})
	}
}

func TestCollectTemplateReplacementStats(t *testing.T) {
	requests := []*slides.Request{
		{
			ReplaceAllText: &slides.ReplaceAllTextRequest{
				ContainsText: &slides.SubstringMatchCriteria{Text: "{{name}}"},
				ReplaceText:  "Peter",
			},
		},
		{
			ReplaceAllText: &slides.ReplaceAllTextRequest{
				ContainsText: &slides.SubstringMatchCriteria{Text: "TITLE"},
				ReplaceText:  "CEO",
			},
		},
	}
	replies := []*slides.Response{
		{ReplaceAllText: &slides.ReplaceAllTextResponse{OccurrencesChanged: 2}},
		nil,
		{ReplaceAllText: &slides.ReplaceAllTextResponse{OccurrencesChanged: 9}},
	}

	got := collectTemplateReplacementStats(requests, replies)
	if got["name"] != 2 {
		t.Fatalf("expected wrapped key stat, got %#v", got)
	}
	if _, ok := got["TITLE"]; ok {
		t.Fatalf("unexpected stat for unmatched/nil reply: %#v", got)
	}
}
