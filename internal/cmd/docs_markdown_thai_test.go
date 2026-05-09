package cmd

import (
	"testing"
	"time"
)

// withTimeout runs fn in a goroutine and fails the test if it does not return
// within d. Used to catch infinite loops without blocking the whole test run.
func withTimeout(t *testing.T, d time.Duration, name string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s: timed out after %s (suspected infinite loop)", name, d)
	}
}

// TestNextRune_SingleMultiByteRune is the direct unit-level regression test for
// the bug that caused `gog docs write --markdown --append` to hang on any
// content ending in a non-ASCII rune. The previous range-based nextRune
// returned size=0 for a string that contained exactly one multi-byte rune
// (e.g. a single Thai character), and ParseInlineFormatting then advanced
// currentByte by 0, looping forever.
func TestNextRune_SingleMultiByteRune(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantStr  string
		wantSize int
	}{
		{name: "empty", in: "", wantStr: "", wantSize: 0},
		{name: "single ascii", in: "a", wantStr: "a", wantSize: 1},
		{name: "two ascii", in: "ab", wantStr: "a", wantSize: 1},
		{name: "single thai (3 bytes)", in: "ก", wantStr: "ก", wantSize: 3},
		{name: "single emoji (4 bytes)", in: "😀", wantStr: "😀", wantSize: 4},
		{name: "two thai", in: "กข", wantStr: "ก", wantSize: 3},
		{name: "thai then ascii", in: "กa", wantStr: "ก", wantSize: 3},
		{name: "ascii then thai", in: "aก", wantStr: "a", wantSize: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotStr, gotSize := nextRune(tc.in)
			if gotStr != tc.wantStr || gotSize != tc.wantSize {
				t.Fatalf("nextRune(%q) = (%q, %d), want (%q, %d)",
					tc.in, gotStr, gotSize, tc.wantStr, tc.wantSize)
			}
		})
	}
}

// TestParseInlineFormatting_Thai ensures the parser returns in finite time on
// Thai content. With the buggy nextRune this hung forever; we cap each call at
// 2 seconds via withTimeout to keep the test suite fast even on regression.
func TestParseInlineFormatting_Thai(t *testing.T) {
	inputs := []string{
		"ก",         // single Thai rune
		"ส่วนคำถาม", // common heading text from the bug report
		"คำถามที่พบบ่อย",                 // FAQ heading
		"**ตัวหนา** ปกติ *เอียง* `code`", // bold/italic/code mixed with Thai
		"พิมพ์ภาษาไทย 😀",                 // emoji at the end (4-byte rune)
	}
	for _, in := range inputs {
		in := in
		t.Run(in, func(t *testing.T) {
			withTimeout(t, 2*time.Second, "ParseInlineFormatting", func() {
				styles, stripped := ParseInlineFormatting(in)
				_ = styles
				if stripped == "" {
					t.Fatalf("ParseInlineFormatting(%q) returned empty stripped text", in)
				}
			})
		})
	}
}

// TestMarkdownToDocsRequests_ThaiAppend exercises the full path used by
// `gog docs write --markdown --append <thai-md>`: parse markdown, then convert
// to Docs API requests at a non-zero base index. Each input ends in a Thai
// rune, which is the trigger condition for the original hang.
func TestMarkdownToDocsRequests_ThaiAppend(t *testing.T) {
	const sample = `## ส่วนคำถาม

คำถามที่พบบ่อยของลูกค้า

- ราคาเท่าไหร่
- ส่งของเมื่อไหร่

> ติดต่อสอบถามเพิ่มเติม
`
	withTimeout(t, 5*time.Second, "MarkdownToDocsRequests", func() {
		elements := ParseMarkdown(sample)
		if len(elements) == 0 {
			t.Fatal("ParseMarkdown returned no elements for Thai sample")
		}
		// baseIndex = 100 mimics appending at the tail of an existing doc.
		reqs, plain, _ := MarkdownToDocsRequests(elements, 100, "")
		if plain == "" {
			t.Fatal("MarkdownToDocsRequests returned empty plain text for Thai sample")
		}
		if len(reqs) == 0 {
			t.Fatal("MarkdownToDocsRequests returned no requests for Thai sample")
		}
	})
}
