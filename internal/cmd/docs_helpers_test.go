package cmd

import (
	"net/http"
	"testing"

	"google.golang.org/api/docs/v1"
	gapi "google.golang.org/api/googleapi"
)

func TestDocsWebViewLink(t *testing.T) {
	if docsWebViewLink("", "") != "" {
		t.Fatalf("expected empty link")
	}
	link := docsWebViewLink("abc", "")
	if link != "https://docs.google.com/document/d/abc/edit" {
		t.Fatalf("unexpected link: %q", link)
	}
	link = docsWebViewLink("abc", "tab1")
	if link != "https://docs.google.com/document/d/abc/edit?tab=tab1" {
		t.Fatalf("unexpected tab link: %q", link)
	}
}

func TestDocsPlainText(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{{TextRun: &docs.TextRun{Content: "Hello "}}, {TextRun: &docs.TextRun{Content: "World"}}},
					},
				},
				{
					Table: &docs.Table{
						TableRows: []*docs.TableRow{
							{
								TableCells: []*docs.TableCell{
									{Content: []*docs.StructuralElement{{Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{TextRun: &docs.TextRun{Content: "A"}}}}}}},
									{Content: []*docs.StructuralElement{{Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{TextRun: &docs.TextRun{Content: "B"}}}}}}},
								},
							},
						},
					},
				},
			},
		},
	}

	text := docsPlainText(doc, 0)
	if text == "" {
		t.Fatalf("expected text output")
	}
	if text != "Hello WorldA\tB" {
		t.Fatalf("unexpected docs text: %q", text)
	}

	limited := docsPlainText(doc, 5)
	if limited != "Hello" {
		t.Fatalf("unexpected limited text: %q", limited)
	}
}

func TestIsDocsNotFound(t *testing.T) {
	if isDocsNotFound(&gapi.Error{Code: http.StatusNotFound}) != true {
		t.Fatalf("expected not found")
	}
	if isDocsNotFound(&gapi.Error{Code: http.StatusForbidden}) {
		t.Fatalf("unexpected not found")
	}
}
