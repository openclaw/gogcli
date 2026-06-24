package gmailcontent

import "testing"

func TestLooksLikeHTML_DocumentPrefixes(t *testing.T) {
	cases := []struct {
		name string
		html string
	}{
		{"doctype", `<!doctype html><html><body>Hi</body></html>`},
		{"html", `<html><body>Hi</body></html>`},
		{"head", `<head><title>X</title></head>`},
		{"body", `<body>Hi</body>`},
		{"meta", `<meta charset="utf-8">`},
		{"html-contained", `Hello <html> world`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !LooksLikeHTML(tc.html) {
				t.Fatalf("expected true for %s", tc.name)
			}
		})
	}
}

func TestLooksLikeHTMLFragment_FragmentMarkers(t *testing.T) {
	cases := []struct {
		name string
		html string
	}{
		{"style-block", `<style type="text/css">a { color: red; }</style><table><tr><td>Hi</td></tr></table>`},
		{"table", `<table><tr><td>Hi</td></tr></table>`},
		{"div", `<div>Hello</div>`},
		{"p", `<p>Hello</p>`},
		{"br", `<br>`},
		{"span", `<span>text</span>`},
		{"section", `<section>content</section>`},
		{"blockquote", `<blockquote>quoted</blockquote>`},
		{"anchor", `<a href="https://example.com">link</a>`},
		{"img", `<img src="x.png" alt="x">`},
		{"font", `<font face="Arial">text</font>`},
		{"comment", `<!-- comment --><div>x</div>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !LooksLikeHTMLFragment(tc.html) {
				t.Fatalf("expected true for fragment %s: %q", tc.name, tc.html)
			}
		})
	}
}

func TestLooksLikeHTML_PlainText(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"empty", ""},
		{"whitespace", "   "},
		{"plain", "Best,\nMartin Kessler"},
		{"url", "see <https://example.com> for details"},
		{"less-than", "a < b"},
		{"angle-path", "<path>"},
		{"angle-project", "<project>"},
		{"angle-email", "<paul@example.com>"},
		{"angle-private", "<PRIVATE_PERSON>"},
		{"angle-in-sentence", "see <path> for details"},
		{"angle-name", "Best,\n<PRIVATE_PERSON>"},
		{"angle-multiword", "the <quick brown> fox"},
		{"angle-a-email", "<a@example.com>"},
		{"angle-b-email", "<b@example.com>"},
		{"angle-i-email", "<i@example.com>"},
		{"angle-p-email", "<p@example.com>"},
		{"angle-div-class", "<div.class>"},
		{"angle-a-colon", "<a:hello>"},
		{"angle-b-dot", "<b.foo>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if LooksLikeHTMLFragment(tc.text) {
				t.Fatalf("expected false for %s: %q", tc.name, tc.text)
			}
		})
	}
}

func TestLooksLikeHTML_DoesNotBroadenToFragments(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"<style>p { color: red }</style>", "<table><tr><td>Hi</td></tr></table>", "<p>Hi</p>"} {
		if LooksLikeHTML(value) {
			t.Errorf("LooksLikeHTML(%q) = true, want legacy document-only classification", value)
		}

		if !LooksLikeHTMLFragment(value) {
			t.Errorf("LooksLikeHTMLFragment(%q) = false, want true", value)
		}
	}
}
