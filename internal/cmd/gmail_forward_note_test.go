package cmd

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

const forwardedHTMLMarker = `<div class="gmail_quote">`

// noteRegion returns the part of a forward body above the forwarded message.
func noteRegion(t *testing.T, body, marker string) string {
	t.Helper()
	before, _, found := strings.Cut(body, marker)
	if !found {
		t.Fatalf("no %q in forward body:\n%s", marker, body)
	}
	return before
}

func TestGmailForward_PlainNoteKeepsLineBreaksInHTML(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	const note = "First paragraph.\n\nSecond paragraph, with <tags> & ampersands."
	for _, cmd := range forwardCommands {
		t.Run(cmd.name, func(t *testing.T) {
			plain, html := forwardTextParts(t, forwardArgs(cmd.verb, "--note", note), cmd.finalizePath, mockForwardSourceMessage)

			if got := strings.TrimSpace(noteRegion(t, plain, forwardedSeparator)); got != note {
				t.Errorf("plain note = %q, want %q", got, note)
			}
			const wantHTML = "First paragraph.<br>\n<br>\nSecond paragraph, with &lt;tags&gt; &amp; ampersands."
			if got := noteRegion(t, html, forwardedHTMLMarker); !strings.Contains(got, wantHTML) {
				t.Errorf("HTML note lost its line breaks or escaping; want %q in:\n%s", wantHTML, got)
			}
		})
	}
}

func TestGmailForward_HTMLNote(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	const noteHTML = "<p>First <b>bold</b> paragraph.</p><p>Second paragraph.</p>"
	cases := []struct {
		name      string
		source    func() map[string]any
		flags     []string
		wantPlain string // the whole plain note
		wantHTML  string // contained in the HTML note
		notInHTML string // the plain note, which must not reach the HTML part
		original  string // the forwarded original in the HTML part
	}{
		{
			name:      "HTML original",
			source:    mockForwardSourceMessage,
			flags:     []string{"--note-html", noteHTML},
			wantPlain: "First bold paragraph.\nSecond paragraph.",
			wantHTML:  noteHTML,
			original:  "<p>Hello, this is the body.</p>",
		},
		{
			name:      "plain-only original",
			source:    mockPlainOnlyForwardSource,
			flags:     []string{"--note-html", noteHTML},
			wantPlain: "First bold paragraph.\nSecond paragraph.",
			wantHTML:  noteHTML,
			original:  "1 &lt;b&gt; &amp; 2<br>\nsecond line",
		},
		{
			name:      "plain and HTML notes",
			source:    mockForwardSourceMessage,
			flags:     []string{"--note", "Plain twin.", "--note-html", "<p>HTML <i>twin</i>.</p>"},
			wantPlain: "Plain twin.",
			wantHTML:  "<p>HTML <i>twin</i>.</p>",
			notInHTML: "Plain twin.",
			original:  "<p>Hello, this is the body.</p>",
		},
	}
	for _, cmd := range forwardCommands {
		for _, tc := range cases {
			t.Run(cmd.name+"/"+tc.name, func(t *testing.T) {
				plain, html := forwardTextParts(t, forwardArgs(cmd.verb, tc.flags...), cmd.finalizePath, tc.source)

				if got := strings.TrimSpace(noteRegion(t, plain, forwardedSeparator)); got != tc.wantPlain {
					t.Errorf("plain note = %q, want %q", got, tc.wantPlain)
				}
				htmlNote := noteRegion(t, html, forwardedHTMLMarker)
				if !strings.Contains(htmlNote, tc.wantHTML) {
					t.Errorf("HTML note missing %q:\n%s", tc.wantHTML, htmlNote)
				}
				if tc.notInHTML != "" && strings.Contains(htmlNote, tc.notInHTML) {
					t.Errorf("plain note leaked into the HTML part:\n%s", htmlNote)
				}
				if !strings.Contains(html, tc.original) {
					t.Errorf("forwarded original missing from the HTML part; want %q in:\n%s", tc.original, html)
				}
			})
		}
	}
}

func TestGmailForward_WithoutNoteKeepsPlainPart(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	for _, cmd := range forwardCommands {
		t.Run(cmd.name, func(t *testing.T) {
			plain, _ := forwardTextParts(t, forwardArgs(cmd.verb), cmd.finalizePath, mockForwardSourceMessage)
			if !strings.HasPrefix(plain, forwardedSeparator) || !strings.Contains(plain, "Hello, this is the body.") {
				t.Errorf("plain part should be the forwarded message alone:\n%s", plain)
			}
		})
	}
}

// An HTML note file holding only whitespace (a template's trailing newline,
// say) counts as no HTML note.
func TestGmailForward_WhitespaceOnlyHTMLNoteCountsAsMissing(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	blank := writeTempFile(t, "note.html", " \n")
	for _, cmd := range forwardCommands {
		t.Run(cmd.name+"/alone", func(t *testing.T) {
			plain, html := forwardTextParts(t, forwardArgs(cmd.verb, "--note-html-file", blank), cmd.finalizePath, mockForwardSourceMessage)
			if !strings.HasPrefix(plain, forwardedSeparator) {
				t.Errorf("plain part should be the forwarded message alone:\n%s", plain)
			}
			if got := noteRegion(t, html, forwardedHTMLMarker); got != "" {
				t.Errorf("HTML note = %q, want none", got)
			}
		})
		t.Run(cmd.name+"/beside a plain note", func(t *testing.T) {
			plain, html := forwardTextParts(t, forwardArgs(cmd.verb, "--note", "FYI", "--note-html-file", blank), cmd.finalizePath, mockForwardSourceMessage)
			if got := strings.TrimSpace(noteRegion(t, plain, forwardedSeparator)); got != "FYI" {
				t.Errorf("plain note = %q, want %q", got, "FYI")
			}
			if got := noteRegion(t, html, forwardedHTMLMarker); got != "<div>FYI</div><br>" {
				t.Errorf("HTML note = %q, want the plain note escaped", got)
			}
		})
	}
}

// An image-only HTML note has no plain-text rendering. Like an image-only reply,
// the forward then has no plain part, rather than one without the note, unless
// a plain note or a signature with text gives the plain part text of its own.
func TestGmailForward_ImageOnlyHTMLNote(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	const image = `<img src="https://example.com/note.png">`
	cases := []struct {
		name      string
		plainNote string
		signature string
		wantPlain string // how the plain part opens; "" for no plain part
	}{
		{name: "no signature"},
		{name: "image signature", signature: `<img src="https://example.com/sig.png">`},
		{name: "text signature", signature: "<div>Kind regards</div>", wantPlain: "--\nKind regards\n\n" + forwardedSeparator},
		{name: "plain note too", plainNote: "Plain twin.", wantPlain: "Plain twin.\n\n" + forwardedSeparator},
	}
	for _, cmd := range forwardCommands {
		for _, tc := range cases {
			t.Run(cmd.name+"/"+tc.name, func(t *testing.T) {
				svc, got, cleanup := newSignatureComposeService(t, map[string]string{"me@example.com": tc.signature}, nil)
				defer cleanup()
				flags := []string{"--note-html", image}
				if tc.plainNote != "" {
					flags = append(flags, "--note", tc.plainNote)
				}
				if tc.signature != "" {
					flags = append(flags, "--signature")
				}
				if result := executeWithGmailTestService(t, forwardArgs(cmd.verb, flags...), svc); result.err != nil {
					t.Fatalf("forward: %v", result.err)
				}

				if hasPlain := strings.Contains(got.raw, "Content-Type: text/plain"); hasPlain != (tc.wantPlain != "") {
					t.Fatalf("text/plain part present = %v, want %v:\n%s", hasPlain, tc.wantPlain != "", got.raw)
				}
				if tc.wantPlain != "" {
					plain := strings.ReplaceAll(rawTextPart(t, got.raw, "text/plain"), "\r\n", "\n")
					if !strings.HasPrefix(plain, tc.wantPlain) {
						t.Errorf("plain part should open with %q; got:\n%s", tc.wantPlain, plain)
					}
				}
				if html := rawTextPart(t, got.raw, "text/html"); !strings.Contains(noteRegion(t, html, forwardedHTMLMarker), image) {
					t.Errorf("image note missing above the forwarded message:\n%s", html)
				}
			})
		}
	}
}

func TestGmailForward_HTMLNoteSitsAboveSignature(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	for _, cmd := range forwardCommands {
		t.Run(cmd.name, func(t *testing.T) {
			svc, got, cleanup := newSignatureComposeService(t, map[string]string{"me@example.com": "<div>Kind regards</div>"}, nil)
			defer cleanup()
			args := forwardArgs(cmd.verb, "--note-html", "<p>See <b>below</b>.</p>", "--signature")
			if result := executeWithGmailTestService(t, args, svc); result.err != nil {
				t.Fatalf("forward: %v", result.err)
			}

			plain := strings.ReplaceAll(rawTextPart(t, got.raw, "text/plain"), "\r\n", "\n")
			if want := "See below.\n\n--\nKind regards\n\n" + forwardedSeparator; !strings.Contains(plain, want) {
				t.Errorf("plain part not note, signature, forward; want %q in:\n%s", want, plain)
			}
			html := rawTextPart(t, got.raw, "text/html")
			if want := `<div><p>See <b>below</b>.</p></div><br><div class="gmail_signature"><div>Kind regards</div></div><br>` + forwardedHTMLMarker; !strings.Contains(html, want) {
				t.Errorf("HTML part not note, signature, forward; want %q in:\n%s", want, html)
			}
		})
	}
}

func TestGmailForward_NoteFlagConflictsFailBeforeAPICalls(t *testing.T) {
	notePath := writeTempFile(t, "note.txt", "Note from a file")
	cases := []struct {
		name  string
		flags []string
		want  string
	}{
		{"note and note-file", []string{"--note", "x", "--note-file", notePath}, "use only one of --note or --note-file"},
		{"note-html and note-html-file", []string{"--note-html", "<p>x</p>", "--note-html-file", notePath}, "use only one of --note-html or --note-html-file"},
		{"both files from stdin", []string{"--note-file", "-", "--note-html-file", "-"}, "use stdin for only one of --note-file or --note-html-file"},
	}
	for _, cmd := range forwardCommands {
		for _, tc := range cases {
			t.Run(cmd.name+"/"+tc.name, func(t *testing.T) {
				setTestConfigHome(t)
				requests := 0
				svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
					requests++
					http.NotFound(w, r)
				})
				defer cleanup()

				result := executeWithGmailTestService(t, forwardArgs(cmd.verb, tc.flags...), svc)
				if result.err == nil || !strings.Contains(result.err.Error(), tc.want) {
					t.Fatalf("error = %v, want %q", result.err, tc.want)
				}
				if requests != 0 {
					t.Fatalf("expected no Gmail API requests, got %d", requests)
				}
			})
		}
	}
}

func TestGmailDraftsForward_NoteHTMLFileStdin(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	svc, cleanup, created, rawPtr := newForwardDraftCaptureService(t)
	defer cleanup()

	ctx := withGmailTestService(
		newCmdRuntimeIOContext(t, strings.NewReader("<p>Note from <b>stdin</b></p>\n"), io.Discard, io.Discard),
		svc,
	)
	if err := runKong(t, &GmailDraftsForwardCmd{}, []string{"orig-msg-1", "--to", "r@example.com", "--note-html-file", "-"}, ctx, &RootFlags{Account: "me@example.com"}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !*created {
		t.Fatal("expected Drafts.Create to be called")
	}
	if html := rawTextPart(t, *rawPtr, "text/html"); !strings.Contains(html, "<div><p>Note from <b>stdin</b></p></div>") {
		t.Fatalf("expected stdin HTML note in the HTML part:\n%s", html)
	}
	if plain := rawTextPart(t, *rawPtr, "text/plain"); !strings.Contains(plain, "Note from stdin") {
		t.Fatalf("expected the derived plain note in the plain part:\n%s", plain)
	}
}
