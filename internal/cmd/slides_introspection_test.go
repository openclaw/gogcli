package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/slides/v1"
)

func TestSlidesElementDetails_GroupedGeometry(t *testing.T) {
	elements := []*slides.PageElement{
		{
			ObjectId: "group1",
			Transform: &slides.AffineTransform{
				ScaleX: 2, ScaleY: 2, TranslateX: 10, TranslateY: 20, Unit: "PT",
			},
			ElementGroup: &slides.Group{Children: []*slides.PageElement{
				{
					ObjectId: "child1",
					Size: &slides.Size{
						Width:  &slides.Dimension{Magnitude: 20, Unit: "PT"},
						Height: &slides.Dimension{Magnitude: 10, Unit: "PT"},
					},
					Transform: &slides.AffineTransform{
						ScaleX: 1, ScaleY: 1, TranslateX: 5, TranslateY: 7, Unit: "PT",
					},
					Shape: &slides.Shape{ShapeType: "RECTANGLE"},
				},
			}},
		},
	}

	details := slidesElementDetails(elements)
	require.Len(t, details, 2)
	assert.Equal(t, "group", details[0].Type)
	assert.Equal(t, "group1", details[1].ParentObjectID)
	require.NotNil(t, details[1].Geometry)
	assert.InDelta(t, 20, details[1].Geometry.X, 0.001)
	assert.InDelta(t, 34, details[1].Geometry.Y, 0.001)
	assert.InDelta(t, 40, details[1].Geometry.Width, 0.001)
	assert.InDelta(t, 20, details[1].Geometry.Height, 0.001)
}

func TestSlidesElementDetails_EMUAndTableCells(t *testing.T) {
	elements := []*slides.PageElement{
		{
			ObjectId: "table1",
			Size: &slides.Size{
				Width:  &slides.Dimension{Magnitude: 1270000, Unit: "EMU"},
				Height: &slides.Dimension{Magnitude: 635000, Unit: "EMU"},
			},
			Transform: &slides.AffineTransform{ScaleX: 1, ScaleY: 1, Unit: "EMU"},
			Table: &slides.Table{
				Rows: 1, Columns: 1,
				TableRows: []*slides.TableRow{{TableCells: []*slides.TableCell{{
					Location: &slides.TableCellLocation{RowIndex: 0, ColumnIndex: 0},
					Text: &slides.TextContent{TextElements: []*slides.TextElement{{
						StartIndex: 1,
						EndIndex:   5,
						TextRun:    &slides.TextRun{Content: "Cell", Style: &slides.TextStyle{Bold: true}},
					}}},
				}}}},
			},
		},
	}

	details := slidesElementDetails(elements)
	require.Len(t, details, 1)
	require.NotNil(t, details[0].Geometry)
	assert.InDelta(t, 100, details[0].Geometry.Width, 0.001)
	assert.InDelta(t, 50, details[0].Geometry.Height, 0.001)
	require.NotNil(t, details[0].Table)
	require.Len(t, details[0].Table.Cells, 1)
	assert.Equal(t, int64(1), details[0].Table.Cells[0].RowSpan)
	assert.Equal(t, "Cell", details[0].Table.Cells[0].Text.Content)
}

func TestLocateSlidesText_UTF16AndSplitRuns(t *testing.T) {
	content := &slides.TextContent{TextElements: []*slides.TextElement{
		{StartIndex: 1, EndIndex: 7, TextRun: &slides.TextRun{Content: "Hello "}},
		{StartIndex: 7, EndIndex: 14, TextRun: &slides.TextRun{Content: "😀World"}},
	}}

	matches := locateSlidesText(content, "o 😀", true)
	require.Len(t, matches, 1)
	assert.Equal(t, int64(5), matches[0].StartIndex)
	assert.Equal(t, int64(9), matches[0].EndIndex)

	matches = locateSlidesText(content, "world", false)
	require.Len(t, matches, 1)
	assert.Equal(t, int64(9), matches[0].StartIndex)
	assert.Equal(t, int64(14), matches[0].EndIndex)
}

func TestSlidesLocateCmd_ShapesAndTableCells(t *testing.T) {
	response := map[string]any{
		"presentationId": "pres1",
		"slides": []any{
			map[string]any{
				"objectId": "slide_1",
				"pageElements": []any{
					map[string]any{
						"objectId": "shape1",
						"shape": map[string]any{"text": map[string]any{"textElements": []any{
							map[string]any{"startIndex": 1, "endIndex": 8, "textRun": map[string]any{"content": "Needle\n"}},
						}}},
					},
					map[string]any{
						"objectId": "table1",
						"table": map[string]any{"rows": 1, "columns": 1, "tableRows": []any{
							map[string]any{"tableCells": []any{
								map[string]any{
									"location": map[string]any{"rowIndex": 0, "columnIndex": 0},
									"text": map[string]any{"textElements": []any{
										map[string]any{"startIndex": 1, "endIndex": 8, "textRun": map[string]any{"content": "needle\n"}},
									}},
								},
							}},
						}},
					},
				},
			},
		},
	}
	svc := newSlidesReadTestService(t, response)
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	cmd := &SlidesLocateCmd{PresentationID: "pres1", Text: "needle", All: true}
	require.NoError(t, cmd.Run(ctx, &RootFlags{Account: "a@b.com"}))

	var result slidesLocateResult
	require.NoError(t, json.Unmarshal(out.Bytes(), &result))
	require.Len(t, result.Matches, 2)
	assert.Equal(t, "shape", result.Matches[0].Kind)
	assert.Equal(t, int64(1), result.Matches[0].StartIndex)
	assert.Equal(t, int64(7), result.Matches[0].EndIndex)
	assert.Equal(t, "tableCell", result.Matches[1].Kind)
	require.NotNil(t, result.Matches[1].RowIndex)
	assert.Equal(t, int64(0), *result.Matches[1].RowIndex)
}
