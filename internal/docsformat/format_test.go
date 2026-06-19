package docsformat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildRequests(t *testing.T) {
	requests, err := BuildRequests(Options{
		FontFamily:   "Georgia",
		FontSize:     14,
		TextColor:    "#3366cc",
		Background:   "#fff",
		ClearBold:    true,
		Italic:       true,
		Alignment:    "center",
		LineSpacing:  150,
		HeadingLevel: intPointer(2),
	}, 3, 9, "t.second")
	if err != nil {
		t.Fatalf("BuildRequests: %v", err)
	}

	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}

	text := requests[0].UpdateTextStyle
	if text == nil || text.Fields != "weightedFontFamily,fontSize,foregroundColor,backgroundColor,bold,italic" {
		t.Fatalf("unexpected text request: %#v", requests[0])
	}

	encoded, err := json.Marshal(text.TextStyle)
	if err != nil {
		t.Fatalf("marshal text style: %v", err)
	}

	if !strings.Contains(string(encoded), `"bold":false`) {
		t.Fatalf("clearing bold must force-send false: %s", encoded)
	}

	paragraph := requests[1].UpdateParagraphStyle
	if paragraph == nil || paragraph.ParagraphStyle.Alignment != "CENTER" ||
		paragraph.ParagraphStyle.LineSpacing != 150 ||
		paragraph.ParagraphStyle.NamedStyleType != "HEADING_2" {
		t.Fatalf("unexpected paragraph request: %#v", requests[1])
	}

	if paragraph.Range.TabId != "t.second" {
		t.Fatalf("tab id = %q, want t.second", paragraph.Range.TabId)
	}
}

func TestBuildRequestsValidation(t *testing.T) {
	tests := []Options{
		{TextColor: "oops"},
		{Link: "https://example.com", ClearLink: true},
		{Bold: true, ClearBold: true},
		{Alignment: "sideways"},
		{Code: true, FontFamily: "Arial"},
		{Code: true, Background: "#fff"},
		{HeadingLevel: intPointer(1), NamedStyle: "TITLE"},
	}
	for _, options := range tests {
		if _, err := BuildRequests(options, 1, 2, ""); err == nil {
			t.Fatalf("BuildRequests(%#v) expected error", options)
		}
	}
}

func TestOptionsAny(t *testing.T) {
	if (Options{}).Any() {
		t.Fatal("zero options should be empty")
	}

	if !(Options{NamedStyle: "TITLE"}).Any() {
		t.Fatal("named style should count as formatting")
	}

	if !(Options{SpaceBelow: floatPointer(0)}).Any() {
		t.Fatal("explicit zero dimension should count as formatting")
	}
}

func TestBuildRequestsParagraphControlsAndBullets(t *testing.T) {
	requests, err := BuildRequests(Options{
		Ordered:           true,
		IndentStart:       floatPointer(36),
		IndentFirstLine:   floatPointer(18),
		IndentEnd:         floatPointer(0),
		SpaceAbove:        floatPointer(6),
		SpaceBelow:        floatPointer(12),
		KeepWithNext:      boolPointer(true),
		KeepLinesTogether: boolPointer(false),
	}, 3, 20, "t.second")
	if err != nil {
		t.Fatalf("BuildRequests: %v", err)
	}

	if len(requests) != 2 {
		t.Fatalf("requests = %d, want paragraph style and bullets", len(requests))
	}

	paragraph := requests[0].UpdateParagraphStyle
	if paragraph == nil {
		t.Fatalf("missing paragraph request: %#v", requests[0])
	}

	if paragraph.Fields != "indentStart,indentFirstLine,indentEnd,spaceAbove,spaceBelow,keepWithNext,keepLinesTogether" {
		t.Fatalf("fields = %q", paragraph.Fields)
	}

	style := paragraph.ParagraphStyle
	if style.IndentStart.Magnitude != 36 || style.IndentFirstLine.Magnitude != 18 || style.IndentEnd.Magnitude != 0 ||
		style.SpaceAbove.Magnitude != 6 || style.SpaceBelow.Magnitude != 12 || !style.KeepWithNext || style.KeepLinesTogether {
		t.Fatalf("unexpected paragraph style: %#v", style)
	}

	encoded, err := json.Marshal(style)
	if err != nil {
		t.Fatalf("marshal paragraph style: %v", err)
	}

	if !strings.Contains(string(encoded), `"keepLinesTogether":false`) {
		t.Fatalf("clearing keep-lines-together must force-send false: %s", encoded)
	}

	if !strings.Contains(string(encoded), `"indentEnd":{"magnitude":0,"unit":"PT"}`) {
		t.Fatalf("explicit zero dimension must force-send zero: %s", encoded)
	}

	bullets := requests[1].CreateParagraphBullets
	if bullets == nil || bullets.BulletPreset != BulletPresetNumbered || bullets.Range.TabId != "t.second" {
		t.Fatalf("unexpected bullets request: %#v", requests[1])
	}
}

func TestBuildRequestsBulletOperations(t *testing.T) {
	tests := []struct {
		name       string
		options    Options
		wantPreset string
		wantDelete bool
	}{
		{name: "bullets", options: Options{Bullets: true}, wantPreset: BulletPresetDisc},
		{name: "custom case insensitive", options: Options{BulletPreset: "bullet_checkbox"}, wantPreset: "BULLET_CHECKBOX"},
		{name: "remove", options: Options{ClearBullets: true}, wantDelete: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests, err := BuildRequests(tt.options, 1, 5, "")
			if err != nil {
				t.Fatalf("BuildRequests: %v", err)
			}

			if len(requests) != 1 {
				t.Fatalf("requests = %#v", requests)
			}

			if tt.wantDelete {
				if requests[0].DeleteParagraphBullets == nil {
					t.Fatalf("missing delete request: %#v", requests[0])
				}

				return
			}

			if got := requests[0].CreateParagraphBullets; got == nil || got.BulletPreset != tt.wantPreset {
				t.Fatalf("unexpected create request: %#v", requests[0])
			}
		})
	}
}

func TestBuildRequestsBulletRemovalPrecedesParagraphControls(t *testing.T) {
	requests, err := BuildRequests(Options{
		ClearBullets:    true,
		IndentStart:     floatPointer(0),
		IndentFirstLine: floatPointer(0),
	}, 1, 5, "")
	if err != nil {
		t.Fatalf("BuildRequests: %v", err)
	}

	if len(requests) != 2 || requests[0].DeleteParagraphBullets == nil || requests[1].UpdateParagraphStyle == nil {
		t.Fatalf("bullet removal must precede paragraph controls: %#v", requests)
	}
}

func TestBuildRequestsBulletCreationUsesPostBulletParagraphRange(t *testing.T) {
	requests, err := BuildRequests(Options{
		Bullets:                  true,
		IndentStart:              floatPointer(54),
		PostBulletParagraphStart: 1,
		PostBulletParagraphEnd:   8,
	}, 3, 9, "")
	if err != nil {
		t.Fatalf("BuildRequests: %v", err)
	}

	if len(requests) != 2 || requests[0].CreateParagraphBullets == nil || requests[1].UpdateParagraphStyle == nil {
		t.Fatalf("bullet creation must precede paragraph controls: %#v", requests)
	}

	if got := requests[1].UpdateParagraphStyle.Range; got.StartIndex != 1 || got.EndIndex != 8 {
		t.Fatalf("post-bullet paragraph range = %#v", got)
	}
}

func TestBuildRequestsParagraphControlValidation(t *testing.T) {
	tests := []Options{
		{IndentStart: floatPointer(-1)},
		{SpaceBelow: floatPointer(-1)},
		{Bullets: true, Ordered: true},
		{Bullets: true, ClearBullets: true},
		{BulletPreset: "not_a_preset"},
	}
	for _, options := range tests {
		if _, err := BuildRequests(options, 1, 2, ""); err == nil {
			t.Fatalf("BuildRequests(%#v) expected error", options)
		}
	}
}

func intPointer(value int) *int {
	return &value
}

func floatPointer(value float64) *float64 {
	return &value
}

func boolPointer(value bool) *bool {
	return &value
}
