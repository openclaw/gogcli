package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/slides/v1"
)

func readSlidePresResponse() map[string]any {
	return map[string]any{
		"presentationId": "pres1",
		"slides": []any{
			map[string]any{
				"objectId": "slide_1",
				"slideProperties": map[string]any{
					"notesPage": map[string]any{
						"notesProperties": map[string]any{
							"speakerNotesObjectId": "notes_1",
						},
						"pageElements": []any{
							map[string]any{
								"objectId": "notes_1",
								"shape": map[string]any{
									"placeholder": map[string]any{"type": "BODY"},
									"text": map[string]any{
										"textElements": []any{
											map[string]any{
												"textRun": map[string]any{
													"content": "These are speaker notes",
												},
											},
										},
									},
								},
							},
						},
					},
				},
				"pageElements": []any{
					map[string]any{
						"objectId": "text_el_1",
						"size": map[string]any{
							"width":  map[string]any{"magnitude": 100, "unit": "PT"},
							"height": map[string]any{"magnitude": 40, "unit": "PT"},
						},
						"transform": map[string]any{
							"scaleX": 1, "scaleY": 1, "translateX": 20, "translateY": 30, "unit": "PT",
						},
						"shape": map[string]any{
							"shapeType": "TEXT_BOX",
							"text": map[string]any{
								"textElements": []any{
									map[string]any{
										"startIndex": 0,
										"endIndex":   11,
										"paragraphMarker": map[string]any{
											"style":  map[string]any{"alignment": "CENTER"},
											"bullet": map[string]any{"glyph": "•", "listId": "list1"},
										},
									},
									map[string]any{
										"startIndex": 0,
										"endIndex":   11,
										"textRun": map[string]any{
											"content": "Slide Title",
											"style": map[string]any{
												"bold":       true,
												"fontFamily": "Inter",
												"fontSize":   map[string]any{"magnitude": 24, "unit": "PT"},
												"foregroundColor": map[string]any{
													"opaqueColor": map[string]any{"rgbColor": map[string]any{"red": 0.2, "green": 0.4, "blue": 0.8}},
												},
												"link": map[string]any{"url": "https://example.com/title"},
											},
										},
									},
								},
							},
						},
					},
					map[string]any{
						"objectId": "img_el_1",
						"image": map[string]any{
							"contentUrl": "https://example.com/image.png",
							"sourceUrl":  "https://cdn.example.com/source.png",
						},
					},
					map[string]any{
						"objectId": "table_el_1",
						"table": map[string]any{
							"rows":    1,
							"columns": 2,
							"tableRows": []any{
								map[string]any{
									"tableCells": []any{
										map[string]any{
											"location":   map[string]any{"rowIndex": 0, "columnIndex": 0},
											"rowSpan":    1,
											"columnSpan": 2,
											"text": map[string]any{
												"textElements": []any{
													map[string]any{
														"startIndex": 1,
														"endIndex":   11,
														"textRun": map[string]any{
															"content": "Cell value\n",
															"style":   map[string]any{"italic": true},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func newSlidesReadTestService(t *testing.T, response map[string]any) *slides.Service {
	t.Helper()

	svc, closeServer := newGoogleTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/presentations/pres1") && r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		http.NotFound(w, r)
	}), slides.NewService)
	t.Cleanup(closeServer)
	return svc
}

func TestSlidesReadSlide(t *testing.T) {
	svc := newSlidesReadTestService(t, readSlidePresResponse())
	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)

	cmd := &SlidesReadSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(out.String(), "Slide 1") {
		t.Errorf("expected slide number, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "These are speaker notes") {
		t.Errorf("expected speaker notes, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "Slide Title") {
		t.Errorf("expected text element, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "img_el_1") {
		t.Errorf("expected image element, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "Cell value") {
		t.Errorf("expected table cell text, got: %q", out.String())
	}
}

func TestSlidesReadSlide_JSON(t *testing.T) {
	svc := newSlidesReadTestService(t, readSlidePresResponse())
	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)

	cmd := &SlidesReadSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		Detail:         true,
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("JSON parse: %v\noutput: %q", err, out.String())
	}
	if result["slideNumber"] != float64(1) {
		t.Errorf("expected slideNumber=1, got %v", result["slideNumber"])
	}
	if result["notes"] != "These are speaker notes" {
		t.Errorf("expected notes text, got %v", result["notes"])
	}
	if result["slideObjectId"] != "slide_1" {
		t.Errorf("expected slideObjectId=slide_1, got %v", result["slideObjectId"])
	}

	textEls, ok := result["textElements"].([]any)
	if !ok || len(textEls) != 2 {
		t.Errorf("expected shape and table-cell text elements, got %v", result["textElements"])
	}

	imgs, ok := result["images"].([]any)
	if !ok || len(imgs) != 1 {
		t.Errorf("expected 1 image, got %v", result["images"])
	}
	image := imgs[0].(map[string]any)
	if image["sourceUrl"] != "https://cdn.example.com/source.png" {
		t.Errorf("expected sourceUrl, got %v", image)
	}
	tables, ok := result["tables"].([]any)
	if !ok || len(tables) != 1 {
		t.Fatalf("expected 1 table, got %v", result["tables"])
	}
	elements, ok := result["elements"].([]any)
	if !ok || len(elements) != 3 {
		t.Fatalf("expected 3 detailed elements, got %v", result["elements"])
	}
	textElement := elements[0].(map[string]any)
	geometry := textElement["geometry"].(map[string]any)
	if geometry["x"] != float64(20) || geometry["width"] != float64(100) {
		t.Errorf("unexpected normalized geometry: %v", geometry)
	}
	shape := textElement["shape"].(map[string]any)
	text := shape["text"].(map[string]any)
	runs := text["runs"].([]any)
	style := runs[0].(map[string]any)["style"].(map[string]any)
	if style["fontFamily"] != "Inter" {
		t.Errorf("expected run style, got %v", style)
	}
	if runs[0].(map[string]any)["foregroundColor"] != "#3366CC" {
		t.Errorf("expected normalized foreground color, got %v", runs[0])
	}
}

func TestSlidesReadSlide_JSONEmptyArrays(t *testing.T) {
	presResp := map[string]any{
		"presentationId": "pres1",
		"slides": []any{
			map[string]any{
				"objectId":     "slide_1",
				"pageElements": []any{},
			},
		},
	}

	svc := newSlidesReadTestService(t, presResp)
	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	cmd := &SlidesReadSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var result struct {
		TextElements []json.RawMessage `json:"textElements"`
		Images       []json.RawMessage `json:"images"`
		Tables       []json.RawMessage `json:"tables"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("JSON parse: %v\noutput: %q", err, out.String())
	}
	if result.TextElements == nil {
		t.Fatalf("textElements must be an empty array, got nil: %s", out.String())
	}
	if len(result.TextElements) != 0 {
		t.Fatalf("textElements len = %d, want 0", len(result.TextElements))
	}
	if result.Images == nil {
		t.Fatalf("images must be an empty array, got nil: %s", out.String())
	}
	if len(result.Images) != 0 {
		t.Fatalf("images len = %d, want 0", len(result.Images))
	}
	if result.Tables == nil {
		t.Fatalf("tables must be an empty array, got nil: %s", out.String())
	}
}

func TestSlidesReadSlide_NotFound(t *testing.T) {
	svc := newSlidesReadTestService(t, readSlidePresResponse())
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &SlidesReadSlideCmd{
		PresentationID: "pres1",
		SlideID:        "nonexistent",
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), `slide "nonexistent" not found`) {
		t.Fatalf("expected slide-not-found error, got: %v", err)
	}
}

func TestSlidesReadSlide_NoNotes(t *testing.T) {
	presResp := map[string]any{
		"presentationId": "pres1",
		"slides": []any{
			map[string]any{
				"objectId": "slide_1",
				"slideProperties": map[string]any{
					"notesPage": map[string]any{},
				},
				"pageElements": []any{},
			},
		},
	}

	svc := newSlidesReadTestService(t, presResp)
	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)

	cmd := &SlidesReadSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(out.String(), "Speaker Notes: (none)") {
		t.Errorf("expected '(none)' for empty notes, got: %q", out.String())
	}
}
