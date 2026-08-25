package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/outfmt"
)

func TestSlidesListSlidesUsesRuntimeOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/presentations/deck") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"presentationId":"deck","title":"Deck","slides":[{"objectId":"slide-1","slideProperties":{"isSkipped":true}},{"objectId":"slide-2"}]}`)
	}))
	defer server.Close()

	var output bytes.Buffer
	ctx := withSlidesTestService(
		newCmdRuntimeOutputContext(t, &output, io.Discard),
		newMockSlidesService(t, server),
	)
	if err := (&SlidesListSlidesCmd{PresentationID: "deck"}).Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := output.String(); !strings.Contains(got, "Presentation: Deck (2 slides)") ||
		!strings.Contains(got, "OBJECT ID") || !strings.Contains(got, "SKIPPED") ||
		!strings.Contains(got, "1  slide-1    true") || !strings.Contains(got, "2  slide-2    false") {
		t.Fatalf("output = %q", got)
	}
}

func TestSlidesListSlidesJSONIncludesSkippedState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/presentations/deck") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"presentationId":"deck","title":"Deck","slides":[{"objectId":"slide-1","slideProperties":{"isSkipped":true}},{"objectId":"slide-2"}]}`)
	}))
	defer server.Close()

	var output bytes.Buffer
	ctx := withSlidesTestService(
		newCmdRuntimeJSONOutputContext(t, &output, io.Discard),
		newMockSlidesService(t, server),
	)
	if err := (&SlidesListSlidesCmd{PresentationID: "deck"}).Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var got struct {
		Slides []struct {
			ObjectID  string `json:"objectId"`
			IsSkipped bool   `json:"isSkipped"`
		} `json:"slides"`
	}
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v\n%s", err, output.String())
	}
	if len(got.Slides) != 2 || !got.Slides[0].IsSkipped || got.Slides[1].IsSkipped {
		t.Fatalf("unexpected slides: %#v", got.Slides)
	}
}

func TestSlidesListSlidesPlainPreservesExistingColumns(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"presentationId":"deck","title":"Deck","slides":[{"objectId":"slide-1","slideProperties":{"isSkipped":true}}]}`)
	}))
	defer server.Close()

	var output bytes.Buffer
	ctx := outfmt.WithMode(newCmdRuntimeOutputContext(t, &output, io.Discard), outfmt.Mode{Plain: true})
	ctx = withSlidesTestService(ctx, newMockSlidesService(t, server))
	if err := (&SlidesListSlidesCmd{PresentationID: "deck"}).Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := output.String(); strings.Contains(got, "SKIPPED") || strings.Contains(got, "true") ||
		!strings.Contains(got, "OBJECT ID") || !strings.Contains(got, "slide-1") {
		t.Fatalf("plain output changed: %q", got)
	}
}
