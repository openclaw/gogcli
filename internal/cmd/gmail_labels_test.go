package cmd

import "testing"

func TestResolveLabelIDs(t *testing.T) {
	m := map[string]string{
		"inbox":  "INBOX",
		"custom": "Label_123",
	}
	got := resolveLabelIDs([]string{"INBOX", "custom", "Label_999"}, m)
	if len(got) != 3 {
		t.Fatalf("unexpected: %#v", got)
	}
	if got[0] != "INBOX" || got[1] != "Label_123" || got[2] != "Label_999" {
		t.Fatalf("unexpected: %#v", got)
	}
}

func TestResolveLabelIDs_DoesNotCaseFoldIDs(t *testing.T) {
	m := map[string]string{
		"custom": "Label_123",
	}
	got := resolveLabelIDs([]string{"label_123", "Custom"}, m)
	if len(got) != 2 {
		t.Fatalf("unexpected: %#v", got)
	}
	if got[0] != "label_123" {
		t.Fatalf("case-folded label ID: %#v", got)
	}
	if got[1] != "Label_123" {
		t.Fatalf("did not resolve label name: %#v", got)
	}
}

func TestResolveLabelIDs_ExactIDBeatsCaseFoldedName(t *testing.T) {
	m := map[string]string{
		"Label_9": "Label_9",
		"label_9": "Label_10",
	}
	got := resolveLabelIDs([]string{"Label_9", "label_9"}, m)
	if len(got) != 2 {
		t.Fatalf("unexpected: %#v", got)
	}
	if got[0] != "Label_9" {
		t.Fatalf("exact ID resolved through name collision: %#v", got)
	}
	if got[1] != "Label_10" {
		t.Fatalf("case-folded name did not resolve: %#v", got)
	}
}

func TestFetchLabelIDToNameBehavior(t *testing.T) {
	// Unit tests for the actual API call live in integration; here we just ensure
	// the helper exists and returns a map. (Compile-time coverage.)
	_ = fetchLabelIDToName
}
