//nolint:wsl_v5 // Projection fixtures stay compact around structural assertions.
package docssed

import (
	"reflect"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestProjectDocumentLegacyContent(t *testing.T) {
	t.Parallel()
	nested := &docs.Table{
		TableRows: []*docs.TableRow{{
			TableCells: []*docs.TableCell{{
				Content: []*docs.StructuralElement{
					testParagraph(30, 37, "nested\n"),
				},
			}},
		}},
	}
	document := &docs.Document{
		RevisionId: "revision-1",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			testParagraphWithImage(1, 9, "before ", "inline-1"),
			{
				StartIndex: 9,
				EndIndex:   40,
				Table: &docs.Table{
					Rows:    1,
					Columns: 1,
					TableRows: []*docs.TableRow{{
						StartIndex: 9,
						EndIndex:   40,
						TableCells: []*docs.TableCell{{
							StartIndex: 9,
							EndIndex:   40,
							TableCellStyle: &docs.TableCellStyle{
								RowSpan:    2,
								ColumnSpan: 3,
							},
							Content: []*docs.StructuralElement{
								testParagraph(10, 15, "cell\n"),
								{StartIndex: 20, EndIndex: 38, Table: nested},
							},
						}},
					}},
				},
			},
		}},
		InlineObjects: map[string]docs.InlineObject{
			"inline-1": {
				InlineObjectProperties: &docs.InlineObjectProperties{
					EmbeddedObject: &docs.EmbeddedObject{Title: "logo", Description: "fallback"},
				},
			},
		},
		PositionedObjects: map[string]docs.PositionedObject{
			"positioned-b": {
				PositionedObjectProperties: &docs.PositionedObjectProperties{
					EmbeddedObject: &docs.EmbeddedObject{Description: "second"},
				},
			},
			"positioned-a": {
				PositionedObjectProperties: &docs.PositionedObjectProperties{
					EmbeddedObject: &docs.EmbeddedObject{Title: "first"},
				},
			},
		},
	}
	document.Body.Content[0].Paragraph.PositionedObjectIds = []string{"positioned-b"}
	document.Body.Content[0].Paragraph.ParagraphStyle = &docs.ParagraphStyle{NamedStyleType: "HEADING_2"}
	document.Body.Content[0].Paragraph.Bullet = &docs.Bullet{ListId: "list-1", NestingLevel: 2}

	projection := ProjectDocument(document)
	if projection.RevisionID != "revision-1" || projection.Legacy == nil {
		t.Fatalf("projection = %+v", projection)
	}
	legacy := projection.Legacy
	if legacy.BodyStartIndex != 1 || legacy.BodyEndIndex != 40 {
		t.Fatalf("body range = %d:%d", legacy.BodyStartIndex, legacy.BodyEndIndex)
	}
	if got := textRunTexts(legacy.TextRuns); !reflect.DeepEqual(got, []string{"before ", "cell\n", "nested\n"}) {
		t.Fatalf("text runs = %v", got)
	}
	if len(legacy.Paragraphs) != 3 || len(legacy.Tables) != 2 {
		t.Fatalf("paragraphs=%d tables=%d", len(legacy.Paragraphs), len(legacy.Tables))
	}
	if got := legacy.Paragraphs[0]; got.NamedStyle != "HEADING_2" ||
		got.BulletListID != "list-1" || got.BulletNestingLevel != 2 {
		t.Fatalf("paragraph = %+v", got)
	}
	if got := legacy.Tables[0]; got.DeclaredRows != 1 || got.DeclaredColumns != 1 ||
		got.Rows[0].StartIndex != 9 || got.Rows[0].EndIndex != 40 {
		t.Fatalf("table = %+v", got)
	}
	if got := legacy.Tables[0].Rows[0].Cells[0]; got.Text != "cell\n" ||
		got.StartIndex != 9 || got.EndIndex != 40 ||
		got.TextStartIndex != 10 || got.TextEndIndex != 15 ||
		got.RowSpan != 2 || got.ColumnSpan != 3 {
		t.Fatalf("cell = %+v", got)
	}
	wantBlocks := []DocumentBlock{
		{Kind: DocumentBlockParagraph, StartIndex: 1, EndIndex: 9, ItemIndex: 0},
		{Kind: DocumentBlockTable, StartIndex: 9, EndIndex: 40, ItemIndex: 0},
	}
	if !reflect.DeepEqual(legacy.Blocks, wantBlocks) {
		t.Fatalf("blocks = %#v, want %#v", legacy.Blocks, wantBlocks)
	}
	wantImages := []DocumentImage{
		{ObjectID: "positioned-b", Index: 1, Alt: "second", IsPositioned: true},
		{ObjectID: "inline-1", Index: 8, Alt: "logo"},
		{ObjectID: "positioned-a", Alt: "first", IsPositioned: true},
	}
	if !reflect.DeepEqual(legacy.Images, wantImages) {
		t.Fatalf("images = %#v, want %#v", legacy.Images, wantImages)
	}
}

func TestProjectDocumentTabs(t *testing.T) {
	t.Parallel()
	document := &docs.Document{
		RevisionId: "revision-tabs",
		Tabs: []*docs.Tab{{
			TabProperties: &docs.TabProperties{TabId: "tab.main", Title: "Main"},
			DocumentTab: &docs.DocumentTab{
				Body: &docs.Body{Content: []*docs.StructuralElement{
					testParagraph(1, 7, "main\n"),
				}},
			},
			ChildTabs: []*docs.Tab{{
				TabProperties: &docs.TabProperties{TabId: "tab.child", Title: "Child"},
				DocumentTab: &docs.DocumentTab{
					Body: &docs.Body{Content: []*docs.StructuralElement{
						testParagraph(1, 8, "\tchild\n"),
					}},
				},
			}, {
				TabProperties: &docs.TabProperties{TabId: "tab.empty", Title: "Empty"},
			}},
		}},
	}

	projection := ProjectDocument(document)
	if projection.Legacy != nil {
		t.Fatalf("legacy = %+v, want nil", projection.Legacy)
	}
	if len(projection.Tabs) != 3 {
		t.Fatalf("tabs = %d, want 3", len(projection.Tabs))
	}
	if projection.Tabs[0].TabID != "tab.main" || projection.Tabs[0].Title != "Main" {
		t.Fatalf("main tab = %+v", projection.Tabs[0])
	}
	if projection.Tabs[1].TabID != "tab.child" ||
		len(projection.Tabs[1].Paragraphs) != 1 ||
		!projection.Tabs[1].Paragraphs[0].LeadingTab {
		t.Fatalf("child tab = %+v", projection.Tabs[1])
	}
	if projection.Tabs[2].TabID != "tab.empty" || projection.Tabs[2].Title != "Empty" {
		t.Fatalf("empty tab = %+v", projection.Tabs[2])
	}
}

func TestProjectDocumentTopLevelBlockOrder(t *testing.T) {
	t.Parallel()
	document := &docs.Document{Body: &docs.Body{Content: []*docs.StructuralElement{
		nil,
		{StartIndex: 0, EndIndex: 1, SectionBreak: &docs.SectionBreak{}},
		testParagraph(1, 7, "first\n"),
		{StartIndex: 7, EndIndex: 12, TableOfContents: &docs.TableOfContents{}},
		{StartIndex: 12, EndIndex: 20, Table: &docs.Table{TableRows: []*docs.TableRow{nil}}},
		testParagraph(20, 27, "last\n"),
	}}}

	projection := ProjectDocument(document)
	if projection.Legacy == nil {
		t.Fatal("legacy projection is nil")
	}
	wantKinds := []DocumentBlockKind{
		DocumentBlockSectionBreak,
		DocumentBlockParagraph,
		DocumentBlockTableOfContents,
		DocumentBlockTable,
		DocumentBlockParagraph,
	}
	gotKinds := make([]DocumentBlockKind, len(projection.Legacy.Blocks))
	for index, block := range projection.Legacy.Blocks {
		gotKinds[index] = block.Kind
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("block kinds = %v, want %v", gotKinds, wantKinds)
	}
	if projection.Legacy.Blocks[1].ItemIndex != 0 ||
		projection.Legacy.Blocks[3].ItemIndex != 0 ||
		projection.Legacy.Blocks[4].ItemIndex != 1 {
		t.Fatalf("blocks = %+v", projection.Legacy.Blocks)
	}
}

func TestProjectDocumentPreservesProviderUTF16Ranges(t *testing.T) {
	t.Parallel()
	document := &docs.Document{Body: &docs.Body{Content: []*docs.StructuralElement{{
		StartIndex: 7,
		EndIndex:   14,
		Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{
			{StartIndex: 7, EndIndex: 11, TextRun: &docs.TextRun{Content: "A😀B"}},
			{StartIndex: 11, EndIndex: 14, TextRun: &docs.TextRun{Content: "A\n"}},
		}},
	}}}}

	projection := ProjectDocument(document)
	runs := projection.Legacy.TextRuns
	if len(runs) != 2 ||
		runs[0].StartIndex != 7 || runs[0].EndIndex != 11 ||
		runs[1].StartIndex != 11 || runs[1].EndIndex != 14 {
		t.Fatalf("runs = %+v", runs)
	}
	if projection.Legacy.Paragraphs[0].Text != "A😀BA\n" {
		t.Fatalf("paragraph = %+v", projection.Legacy.Paragraphs[0])
	}
}

func TestProjectDocumentKeepsLegacyAndTabsIndependent(t *testing.T) {
	t.Parallel()
	document := &docs.Document{
		RevisionId: "revision-both",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			testParagraph(1, 8, "legacy\n"),
		}},
		Tabs: []*docs.Tab{{
			TabProperties: &docs.TabProperties{TabId: "tab-1", Title: "One"},
			DocumentTab: &docs.DocumentTab{Body: &docs.Body{Content: []*docs.StructuralElement{
				testParagraph(1, 5, "tab\n"),
			}}},
		}},
	}

	projection := ProjectDocument(document)
	if projection.Legacy == nil || len(projection.Tabs) != 1 {
		t.Fatalf("projection = %+v", projection)
	}
	if projection.Legacy.Paragraphs[0].Text != "legacy\n" ||
		projection.Tabs[0].Paragraphs[0].Text != "tab\n" {
		t.Fatalf("projection = %+v", projection)
	}
}

func TestProjectDocumentEmptyLegacySegment(t *testing.T) {
	t.Parallel()
	projection := ProjectDocument(&docs.Document{Body: &docs.Body{Content: []*docs.StructuralElement{nil}}})
	if projection.Legacy == nil {
		t.Fatal("legacy projection is nil")
	}
	if !reflect.DeepEqual(*projection.Legacy, DocumentSegment{}) {
		t.Fatalf("legacy = %+v", projection.Legacy)
	}
}

func TestProjectDocumentNil(t *testing.T) {
	t.Parallel()
	if got := ProjectDocument(nil); !reflect.DeepEqual(got, DocumentProjection{}) {
		t.Fatalf("projection = %+v", got)
	}
}

func testParagraph(startIndex, endIndex int64, text string) *docs.StructuralElement {
	return &docs.StructuralElement{
		StartIndex: startIndex,
		EndIndex:   endIndex,
		Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{
			StartIndex: startIndex,
			EndIndex:   endIndex,
			TextRun:    &docs.TextRun{Content: text},
		}}},
	}
}

func testParagraphWithImage(startIndex, endIndex int64, text, objectID string) *docs.StructuralElement {
	element := testParagraph(startIndex, endIndex, text)
	element.Paragraph.Elements = append(element.Paragraph.Elements, &docs.ParagraphElement{
		StartIndex:          endIndex - 1,
		EndIndex:            endIndex,
		InlineObjectElement: &docs.InlineObjectElement{InlineObjectId: objectID},
	})
	return element
}

func textRunTexts(runs []DocumentTextRun) []string {
	texts := make([]string, len(runs))
	for index, run := range runs {
		texts[index] = run.Text
	}
	return texts
}
