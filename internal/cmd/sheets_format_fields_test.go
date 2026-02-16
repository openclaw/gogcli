package cmd

import (
	"testing"

	"google.golang.org/api/sheets/v4"
)

func TestApplyForceSendFields_TextFormatBold(t *testing.T) {
	format := sheets.CellFormat{}
	if err := applyForceSendFields(&format, []string{"textFormat.bold"}); err != nil {
		t.Fatalf("applyForceSendFields: %v", err)
	}
	if format.TextFormat == nil {
		t.Fatalf("expected textFormat to be allocated")
	}
	if !hasString(format.TextFormat.ForceSendFields, "Bold") {
		t.Fatalf("expected Bold to be force-sent, got %#v", format.TextFormat.ForceSendFields)
	}
}

func TestApplyForceSendFields_UnknownField(t *testing.T) {
	format := sheets.CellFormat{}
	if err := applyForceSendFields(&format, []string{"nope"}); err == nil {
		t.Fatalf("expected error for unknown field")
	}
}

func TestApplyForceSendFields_NumberFormatType(t *testing.T) {
	format := sheets.CellFormat{}
	if err := applyForceSendFields(&format, []string{"numberFormat.type"}); err != nil {
		t.Fatalf("applyForceSendFields: %v", err)
	}
	if format.NumberFormat == nil {
		t.Fatalf("expected numberFormat to be allocated")
	}
	if !hasString(format.NumberFormat.ForceSendFields, "Type") {
		t.Fatalf("expected Type to be force-sent, got %#v", format.NumberFormat.ForceSendFields)
	}
}

func TestApplyForceSendFields_BordersTopStyle(t *testing.T) {
	format := sheets.CellFormat{}
	if err := applyForceSendFields(&format, []string{"borders.top.style"}); err != nil {
		t.Fatalf("applyForceSendFields: %v", err)
	}
	if format.Borders == nil || format.Borders.Top == nil {
		t.Fatalf("expected borders.top to be allocated, got %#v", format.Borders)
	}
	if !hasString(format.Borders.Top.ForceSendFields, "Style") {
		t.Fatalf("expected Style to be force-sent, got %#v", format.Borders.Top.ForceSendFields)
	}
}

func TestApplyForceSendFields_NilFormat(t *testing.T) {
	if err := applyForceSendFields(nil, []string{"textFormat.bold"}); err == nil {
		t.Fatalf("expected error for nil format")
	}
}

func hasString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestNormalizeFormatMask(t *testing.T) {
	normalized, paths := normalizeFormatMask("textFormat.bold, userEnteredFormat.textFormat.italic, userEnteredValue")
	if normalized != "userEnteredFormat.textFormat.bold,userEnteredFormat.textFormat.italic,userEnteredValue" {
		t.Fatalf("unexpected normalized mask: %s", normalized)
	}
	if len(paths) != 2 || paths[0] != "textFormat.bold" || paths[1] != "textFormat.italic" {
		t.Fatalf("unexpected format paths: %#v", paths)
	}
}

func TestNormalizeFormatMask_UserEnteredFormatOnly(t *testing.T) {
	normalized, paths := normalizeFormatMask("userEnteredFormat")
	if normalized != "userEnteredFormat" {
		t.Fatalf("unexpected normalized mask: %s", normalized)
	}
	if len(paths) != 0 {
		t.Fatalf("unexpected format paths: %#v", paths)
	}
}

func TestNormalizeFormatMask_LeavesUnknowns(t *testing.T) {
	normalized, paths := normalizeFormatMask("note")
	if normalized != "note" {
		t.Fatalf("unexpected normalized mask: %s", normalized)
	}
	if len(paths) != 0 {
		t.Fatalf("unexpected format paths: %#v", paths)
	}
}

func TestHasBoardersTypo(t *testing.T) {
	if !hasBoardersTypo("boarders.top.style") {
		t.Fatalf("expected typo detection for boarders")
	}
	if !hasBoardersTypo("userEnteredFormat.boarders.top.style") {
		t.Fatalf("expected typo detection for userEnteredFormat.boarders")
	}
	if hasBoardersTypo("borders.top.style") {
		t.Fatalf("did not expect typo detection for borders")
	}
}
