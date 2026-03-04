package cmd

import (
	"math"
	"testing"
)

func TestMarkdownToDocsRequests_BaseIndex(t *testing.T) {
	elements := []MarkdownElement{{Type: MDParagraph, Content: "**bold**"}}
	requests, text, tables := MarkdownToDocsRequests(elements, 42)

	if text != "bold\n" {
		t.Fatalf("unexpected text: %q", text)
	}
	if len(tables) != 0 {
		t.Fatalf("unexpected tables: %d", len(tables))
	}
	if len(requests) != 1 || requests[0].UpdateTextStyle == nil {
		t.Fatalf("expected one text-style request, got %#v", requests)
	}

	rng := requests[0].UpdateTextStyle.Range
	if rng.StartIndex != 42 || rng.EndIndex != 46 {
		t.Fatalf("unexpected range: [%d,%d]", rng.StartIndex, rng.EndIndex)
	}
}

func TestMarkdownToDocsRequests_TableStartIndexUsesBase(t *testing.T) {
	elements := []MarkdownElement{
		{Type: MDParagraph, Content: "A"},
		{Type: MDTable, TableCells: [][]string{{"h1", "h2"}, {"v1", "v2"}}},
	}
	_, text, tables := MarkdownToDocsRequests(elements, 10)

	if text != "A\n\n" {
		t.Fatalf("unexpected text: %q", text)
	}
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if tables[0].StartIndex != 12 {
		t.Fatalf("unexpected table start index: %d", tables[0].StartIndex)
	}
}

func TestParseTextColor_NamedColors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantR   float64
		wantG   float64
		wantB   float64
		wantErr bool
	}{
		{"red", "red", 1.0, 0.0, 0.0, false},
		{"blue", "blue", 0.0, 0.0, 1.0, false},
		{"green", "green", 0.0, 0.5, 0.0, false},
		{"uppercase", "RED", 1.0, 0.0, 0.0, false},
		{"mixed case", "Blue", 0.0, 0.0, 1.0, false},
		{"unknown", "chartreuse", 0, 0, 0, true},
		{"empty", "", 0, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			color, err := ParseTextColor(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			rgb := color.Color.RgbColor
			if rgb.Red != tt.wantR || rgb.Green != tt.wantG || rgb.Blue != tt.wantB {
				t.Fatalf("color mismatch: got R=%.2f G=%.2f B=%.2f, want R=%.2f G=%.2f B=%.2f",
					rgb.Red, rgb.Green, rgb.Blue, tt.wantR, tt.wantG, tt.wantB)
			}
		})
	}
}

func TestParseTextColor_HexColors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantR   float64
		wantG   float64
		wantB   float64
		wantErr bool
	}{
		{"6-digit", "#FF0000", 1.0, 0.0, 0.0, false},
		{"6-digit blue", "#0000FF", 0.0, 0.0, 1.0, false},
		{"6-digit mixed", "#006400", 0.0, 100.0 / 255.0, 0.0, false},
		{"3-digit", "#F00", 1.0, 0.0, 0.0, false},
		{"3-digit white", "#FFF", 1.0, 1.0, 1.0, false},
		{"no hash", "FF0000", 1.0, 0.0, 0.0, false},
		{"invalid hex", "#GGGGGG", 0, 0, 0, true},
		{"wrong length", "#FFFF", 0, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			color, err := ParseTextColor(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			rgb := color.Color.RgbColor
			if math.Abs(rgb.Red-tt.wantR) > 0.01 ||
				math.Abs(rgb.Green-tt.wantG) > 0.01 ||
				math.Abs(rgb.Blue-tt.wantB) > 0.01 {
				t.Fatalf("color mismatch: got R=%.4f G=%.4f B=%.4f, want R=%.4f G=%.4f B=%.4f",
					rgb.Red, rgb.Green, rgb.Blue, tt.wantR, tt.wantG, tt.wantB)
			}
		})
	}
}

func TestBuildColorRequest(t *testing.T) {
	color, err := ParseTextColor("blue")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := BuildColorRequest(10, 50, color)
	if req == nil {
		t.Fatal("expected non-nil request")
	}
	if req.UpdateTextStyle == nil {
		t.Fatal("expected UpdateTextStyle request")
	}
	if req.UpdateTextStyle.Range.StartIndex != 10 || req.UpdateTextStyle.Range.EndIndex != 50 {
		t.Fatalf("unexpected range: [%d, %d]", req.UpdateTextStyle.Range.StartIndex, req.UpdateTextStyle.Range.EndIndex)
	}
	if req.UpdateTextStyle.Fields != "foregroundColor" {
		t.Fatalf("unexpected fields: %q", req.UpdateTextStyle.Fields)
	}
	fg := req.UpdateTextStyle.TextStyle.ForegroundColor
	if fg == nil || fg.Color == nil || fg.Color.RgbColor == nil {
		t.Fatal("expected foreground color to be set")
	}
	if fg.Color.RgbColor.Blue != 1.0 {
		t.Fatalf("expected blue=1.0, got %f", fg.Color.RgbColor.Blue)
	}
}
