package cmd

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

const missingInlineTestHTML = `<p>Original HTML</p><img src="cid:image-1@example.com"><img src="cid:missing@example.com" alt="Missing illustration">`

func inlineTestMIME(htmlBody string) string {
	return "From: Alice <alice@example.com>\r\nTo: Me <me@example.com>\r\nSubject: Project update\r\nMessage-ID: <original@example.com>\r\nMIME-Version: 1.0\r\nContent-Type: multipart/related; boundary=outer\r\n\r\n" +
		"--outer\r\nContent-Type: multipart/alternative; boundary=alt\r\n\r\n" +
		"--alt\r\nContent-Type: text/plain\r\n\r\nOriginal plain\r\n" +
		"--alt\r\nContent-Type: text/html\r\n\r\n" + htmlBody + "\r\n--alt--\r\n" +
		"--outer\r\nContent-Type: image/png\r\nContent-ID: <image-1@example.com>\r\nContent-Transfer-Encoding: base64\r\n\r\n" + base64.StdEncoding.EncodeToString([]byte("png-data")) + "\r\n--outer--\r\n"
}

func TestMissingInlineImagesPreserveUntouchedHTML(t *testing.T) {
	const prefix = `<!--keep--><P class='x'>Original&nbsp;text</P><img src='cid:image-1@example.com' ALT='valid'>`
	const suffix = `<style>.keep { color: red; }</style>`
	input := prefix + `<IMG src='CID:missing%40example.com' alt='A &amp; &lt;B&gt;'><img src="cid:missing@example.com" alt="Second">` + suffix
	got, warnings, markers, err := replaceMissingInlineImages(input, "source-id", map[string]string{"missing@example.com": "missing@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	want := prefix + `[Inline image unavailable: missing@example.com — A &amp; &lt;B&gt;][Inline image unavailable: missing@example.com — Second]` + suffix
	if got != want {
		t.Fatalf("HTML changed outside replacement: %q", got)
	}
	if len(warnings) != 1 || warnings[0].Occurrences != 2 || warnings[0].ContentID != "missing@example.com" || warnings[0].SourceMessageID != "source-id" {
		t.Fatalf("warnings=%#v", warnings)
	}
	if len(markers) != 2 || !strings.Contains(markers[0], "A & <B>") || !strings.Contains(markers[1], "Second") {
		t.Fatalf("plain markers=%v", markers)
	}
}

func TestMissingInlineImagesRejectUnsupportedContexts(t *testing.T) {
	for _, body := range []string{
		`<div style="background:url(cid:missing@example.com)">text</div>`,
		`<style>p { background: url('cid:missing@example.com'); }</style>`,
		`<img src="https://example.com/image.png" srcset="cid:missing@example.com 2x">`,
		`<img src="cid:missing@example.com" srcset="https://example.com/image.png 2x">`,
		`<img src="cid:missing@example.com" style="background: url(cid:valid@example.com)">`,
		`<picture><img src="cid:missing@example.com"></picture>`,
		`<picture/><img src="cid:missing@example.com">`,
		`<template><img src="cid:missing@example.com"></template>`,
		`<svg><foreignObject><img src="cid:missing@example.com"></foreignObject></svg>`,
		`<svg><image href="cid:missing@example.com"/></svg>`,
		`<object data="cid:missing@example.com"></object>`,
		`<img src="cid:missing@example.com" src="https://example.com/image.png">`,
		`<img src="cid:missing@example.com" alt="one" alt="two">`,
		`<img src="cid:missing@example.com"><a href="cid:missing@example.com">linked</a>`,
	} {
		t.Run(body, func(t *testing.T) {
			if _, _, _, err := replaceMissingInlineImages(body, "source", map[string]string{"missing@example.com": "missing@example.com"}); err == nil {
				t.Fatal("unsupported replacement was accepted")
			}
		})
	}
}

func TestMissingInlineMIMEValidation(t *testing.T) {
	valid := inlineTestMIME(missingInlineTestHTML)
	missing := map[string]string{"missing@example.com": "missing@example.com"}
	for _, raw := range []string{valid, strings.TrimSuffix(valid, "\r\n"), "MIME-Version: 1.0\r\n\r\nPlain source"} {
		if err := validateMissingInlineMIME([]byte(raw), missing); err != nil {
			t.Fatalf("valid MIME rejected: %v", err)
		}
	}
	for name, raw := range map[string]string{
		"outer closing boundary": strings.TrimSuffix(valid, "--outer--\r\n"),
		"inner closing boundary": strings.Replace(valid, "--alt--\r\n", "", 1),
		"missing boundary":       strings.Replace(valid, "multipart/related; boundary=outer", "multipart/related", 1),
		"invalid content type":   strings.Replace(valid, "multipart/related; boundary=outer", `multipart/related; boundary="`, 1),
		"duplicate content type": "Content-Type: text/plain\r\n" + valid,
		"duplicate content ID":   strings.Replace(valid, "Content-ID: <image-1@example.com>", "Content-ID: <image-1@example.com>\r\nContent-ID: <image-1@example.com>", 1),
		"duplicate parts":        strings.Replace(valid, "--outer--", "--outer\r\nContent-ID: <IMAGE-1@example.com>\r\n\r\nAnother\r\n--outer--", 1),
		"present only in raw":    strings.Replace(valid, "Content-ID: <image-1@example.com>", "Content-ID: <missing@example.com>", 1),
		"invalid content ID":     strings.Replace(valid, "<image-1@example.com>", "<<image-1@example.com>>", 1),
		"invalid base64":         strings.Replace(valid, base64.StdEncoding.EncodeToString([]byte("png-data")), "not base64!", 1),
		"unknown encoding":       strings.Replace(valid, "Content-Transfer-Encoding: base64", "Content-Transfer-Encoding: unknown", 1),
		"encoded multipart":      "Content-Transfer-Encoding: base64\r\n" + valid,
		"invalid header":         "Not a header\r\n" + valid,
		"empty content type":     "Content-Type:\r\n\r\nbody",
		"empty disposition":      "Content-Disposition:\r\n\r\nbody",
		"empty encoding":         "Content-Transfer-Encoding:\r\n\r\nbody",
		"embedded message":       "Content-Type: message/rfc822\r\n\r\n" + valid,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateMissingInlineMIME([]byte(raw), missing); err == nil {
				t.Fatal("malformed or ambiguous MIME was accepted")
			}
		})
	}
}

func TestMissingInlineMIMELimits(t *testing.T) {
	t.Run("size", func(t *testing.T) {
		if err := validateMissingInlineMIME(make([]byte, maxGmailRawMessageBytes+1), nil); err == nil {
			t.Fatal("oversized source accepted")
		}
	})
	t.Run("headers", func(t *testing.T) {
		if err := validateMissingInlineMIME([]byte(strings.Repeat("X-Test: value\r\n", 6000)+"\r\nbody"), nil); err == nil {
			t.Fatal("oversized headers accepted")
		}
	})
	t.Run("depth", func(t *testing.T) {
		raw := "Content-Type: text/plain\r\n\r\nbody"
		for i := 0; i <= maxInlineMIMEDepth; i++ {
			raw = fmt.Sprintf("Content-Type: multipart/mixed; boundary=b%d\r\n\r\n--b%d\r\n%s\r\n--b%d--\r\n", i, i, raw, i)
		}
		if err := validateMissingInlineMIME([]byte(raw), nil); err == nil {
			t.Fatal("excessive nesting accepted")
		}
	})
	t.Run("parts", func(t *testing.T) {
		raw := "Content-Type: multipart/mixed; boundary=b\r\n\r\n" + strings.Repeat("--b\r\nContent-Type: text/plain\r\n\r\nbody\r\n", maxInlineMIMEParts) + "--b--\r\n"
		if err := validateMissingInlineMIME([]byte(raw), nil); err == nil {
			t.Fatal("too many MIME parts accepted")
		}
	})
}

func TestMissingInlineQuotedPrintableValidation(t *testing.T) {
	for _, body := range []string{"hello=20world", "soft=\r\nbreak", "soft=\nbreak"} {
		if err := validateInlineQuotedPrintable(strings.NewReader(body)); err != nil {
			t.Fatalf("valid quoted-printable %q: %v", body, err)
		}
	}
	for _, body := range []string{"=ZZ", "trailing=", "=A", "=\rX"} {
		if err := validateInlineQuotedPrintable(strings.NewReader(body)); err == nil {
			t.Fatalf("invalid quoted-printable accepted: %q", body)
		}
	}
}
