package cmd

import (
	"testing"

	"google.golang.org/api/docs/v1"
)

// testDoc returns a realistic Google Doc with multiple paragraph types.
func testDoc() *docs.Document {
	return &docs.Document{
		DocumentId: "test-doc-1",
		RevisionId: "rev-abc",
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					SectionBreak: &docs.SectionBreak{},
					StartIndex:   0,
					EndIndex:     0,
				},
				{
					StartIndex: 0,
					EndIndex:   27,
					Paragraph: &docs.Paragraph{
						ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "TITLE"},
						Elements: []*docs.ParagraphElement{
							{TextRun: &docs.TextRun{Content: "Meeting Notes 2026-02-23\n"}},
						},
					},
				},
				{
					StartIndex: 27,
					EndIndex:   38,
					Paragraph: &docs.Paragraph{
						ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_1"},
						Elements: []*docs.ParagraphElement{
							{TextRun: &docs.TextRun{Content: "Attendees\n"}},
						},
					},
				},
				{
					StartIndex: 38,
					EndIndex:   57,
					Paragraph: &docs.Paragraph{
						ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "NORMAL_TEXT"},
						Elements: []*docs.ParagraphElement{
							{TextRun: &docs.TextRun{Content: "Alice, Bob, Carol\n"}},
						},
					},
				},
				{
					StartIndex: 57,
					EndIndex:   68,
					Paragraph: &docs.Paragraph{
						ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_1"},
						Elements: []*docs.ParagraphElement{
							{TextRun: &docs.TextRun{Content: "Discussion\n"}},
						},
					},
				},
				{
					StartIndex: 68,
					EndIndex:   94,
					Paragraph: &docs.Paragraph{
						ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "NORMAL_TEXT"},
						Bullet:         &docs.Bullet{NestingLevel: 0},
						Elements: []*docs.ParagraphElement{
							{TextRun: &docs.TextRun{Content: "Very fun! Delightful to use\n"}},
						},
					},
				},
				{
					StartIndex: 94,
					EndIndex:   115,
					Paragraph: &docs.Paragraph{
						ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "NORMAL_TEXT"},
						Bullet:         &docs.Bullet{NestingLevel: 0},
						Elements: []*docs.ParagraphElement{
							{TextRun: &docs.TextRun{Content: "Dev sandbox is cool\n"}},
						},
					},
				},
			},
		},
	}
}

func TestBuildParagraphMap_Basic(t *testing.T) {
	doc := testDoc()
	pm, err := buildParagraphMap(doc, "")
	if err != nil {
		t.Fatalf("buildParagraphMap: %v", err)
	}

	if pm.DocumentID != "test-doc-1" {
		t.Fatalf("unexpected documentId: %s", pm.DocumentID)
	}
	if pm.RevisionID != "rev-abc" {
		t.Fatalf("unexpected revisionId: %s", pm.RevisionID)
	}

	// Should have 6 paragraphs (section break skipped).
	if len(pm.Paragraphs) != 6 {
		t.Fatalf("expected 6 paragraphs, got %d", len(pm.Paragraphs))
	}

	// Check first paragraph (title).
	p1 := pm.Paragraphs[0]
	if p1.Num != 1 {
		t.Errorf("p1.Num = %d, want 1", p1.Num)
	}
	if p1.Type != "TITLE" {
		t.Errorf("p1.Type = %q, want TITLE", p1.Type)
	}
	if p1.Text != "Meeting Notes 2026-02-23" {
		t.Errorf("p1.Text = %q, want 'Meeting Notes 2026-02-23'", p1.Text)
	}
	if p1.IsBullet {
		t.Error("p1 should not be a bullet")
	}
	if p1.ElemType != "paragraph" {
		t.Errorf("p1.ElemType = %q, want paragraph", p1.ElemType)
	}

	// Check heading.
	p2 := pm.Paragraphs[1]
	if p2.Type != "HEADING_1" {
		t.Errorf("p2.Type = %q, want HEADING_1", p2.Type)
	}

	// Check bullet paragraph.
	p5 := pm.Paragraphs[4]
	if !p5.IsBullet {
		t.Error("p5 should be a bullet")
	}
	if p5.NestLevel != 0 {
		t.Errorf("p5.NestLevel = %d, want 0", p5.NestLevel)
	}
	if p5.Text != "Very fun! Delightful to use" {
		t.Errorf("p5.Text = %q", p5.Text)
	}

	// Check indices.
	if p1.StartIndex != 0 || p1.EndIndex != 27 {
		t.Errorf("p1 indices: start=%d end=%d, want 0-27", p1.StartIndex, p1.EndIndex)
	}
}

func TestBuildParagraphMap_WithTable(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "doc-table",
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					SectionBreak: &docs.SectionBreak{},
					StartIndex:   0,
					EndIndex:     0,
				},
				{
					StartIndex: 0,
					EndIndex:   7,
					Paragraph: &docs.Paragraph{
						ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "NORMAL_TEXT"},
						Elements: []*docs.ParagraphElement{
							{TextRun: &docs.TextRun{Content: "Hello\n"}},
						},
					},
				},
				{
					StartIndex: 7,
					EndIndex:   30,
					Table: &docs.Table{
						TableRows: []*docs.TableRow{
							{
								TableCells: []*docs.TableCell{
									{Content: []*docs.StructuralElement{{Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{TextRun: &docs.TextRun{Content: "A"}}}}}}},
									{Content: []*docs.StructuralElement{{Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{TextRun: &docs.TextRun{Content: "B"}}}}}}},
								},
							},
							{
								TableCells: []*docs.TableCell{
									{Content: []*docs.StructuralElement{{Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{TextRun: &docs.TextRun{Content: "C"}}}}}}},
									{Content: []*docs.StructuralElement{{Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{TextRun: &docs.TextRun{Content: "D"}}}}}}},
								},
							},
						},
					},
				},
			},
		},
	}

	pm, err := buildParagraphMap(doc, "")
	if err != nil {
		t.Fatalf("buildParagraphMap: %v", err)
	}

	if len(pm.Paragraphs) != 2 {
		t.Fatalf("expected 2 paragraphs, got %d", len(pm.Paragraphs))
	}

	table := pm.Paragraphs[1]
	if table.ElemType != "table" {
		t.Fatalf("expected table, got %s", table.ElemType)
	}
	if table.TableRows != 2 || table.TableCols != 2 {
		t.Fatalf("table dimensions: %dx%d, want 2x2", table.TableRows, table.TableCols)
	}
	if table.Type != "TABLE" {
		t.Errorf("table.Type = %q, want TABLE", table.Type)
	}
	if table.Text != "A | B" {
		t.Errorf("table preview = %q, want 'A | B'", table.Text)
	}
}

func TestBuildParagraphMap_NilDoc(t *testing.T) {
	_, err := buildParagraphMap(nil, "")
	if err == nil {
		t.Fatal("expected error for nil doc")
	}
}

func TestBuildParagraphMap_NoBody(t *testing.T) {
	doc := &docs.Document{DocumentId: "no-body"}
	_, err := buildParagraphMap(doc, "")
	if err == nil {
		t.Fatal("expected error for doc with no body")
	}
}

func TestBuildParagraphMap_DefaultStyleType(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "doc-no-style",
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					StartIndex: 0,
					EndIndex:   6,
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{TextRun: &docs.TextRun{Content: "Hello\n"}},
						},
					},
				},
			},
		},
	}

	pm, err := buildParagraphMap(doc, "")
	if err != nil {
		t.Fatalf("buildParagraphMap: %v", err)
	}

	if len(pm.Paragraphs) != 1 {
		t.Fatalf("expected 1 paragraph, got %d", len(pm.Paragraphs))
	}
	if pm.Paragraphs[0].Type != "NORMAL_TEXT" {
		t.Errorf("expected NORMAL_TEXT default, got %q", pm.Paragraphs[0].Type)
	}
}

func TestParagraphMap_Get(t *testing.T) {
	pm := &paragraphMap{
		Paragraphs: []docParagraph{
			{Num: 1, Text: "first"},
			{Num: 2, Text: "second"},
		},
	}

	p, err := pm.get(1)
	if err != nil {
		t.Fatalf("get(1): %v", err)
	}
	if p.Text != "first" {
		t.Fatalf("get(1).Text = %q, want first", p.Text)
	}

	_, err = pm.get(0)
	if err == nil {
		t.Fatal("get(0) should fail")
	}

	_, err = pm.get(3)
	if err == nil {
		t.Fatal("get(3) should fail")
	}
}

func TestBuildParagraphMap_WithNestedBullets(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "doc-nested",
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					StartIndex: 0,
					EndIndex:   8,
					Paragraph: &docs.Paragraph{
						ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "NORMAL_TEXT"},
						Bullet:         &docs.Bullet{NestingLevel: 0},
						Elements: []*docs.ParagraphElement{
							{TextRun: &docs.TextRun{Content: "Top\n"}},
						},
					},
				},
				{
					StartIndex: 8,
					EndIndex:   18,
					Paragraph: &docs.Paragraph{
						ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "NORMAL_TEXT"},
						Bullet:         &docs.Bullet{NestingLevel: 1},
						Elements: []*docs.ParagraphElement{
							{TextRun: &docs.TextRun{Content: "Nested\n"}},
						},
					},
				},
			},
		},
	}

	pm, err := buildParagraphMap(doc, "")
	if err != nil {
		t.Fatalf("buildParagraphMap: %v", err)
	}

	if len(pm.Paragraphs) != 2 {
		t.Fatalf("expected 2 paragraphs, got %d", len(pm.Paragraphs))
	}

	if pm.Paragraphs[0].NestLevel != 0 {
		t.Errorf("p1 nest level = %d, want 0", pm.Paragraphs[0].NestLevel)
	}
	if pm.Paragraphs[1].NestLevel != 1 {
		t.Errorf("p2 nest level = %d, want 1", pm.Paragraphs[1].NestLevel)
	}
}

func TestBuildParagraphMap_WithTab(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "doc-tabs",
		RevisionId: "rev-tab",
		Tabs: []*docs.Tab{
			{
				TabProperties: &docs.TabProperties{
					TabId: "t.0",
					Title: "Main",
				},
				DocumentTab: &docs.DocumentTab{
					Body: &docs.Body{
						Content: []*docs.StructuralElement{
							{
								StartIndex: 0,
								EndIndex:   10,
								Paragraph: &docs.Paragraph{
									ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "NORMAL_TEXT"},
									Elements: []*docs.ParagraphElement{
										{TextRun: &docs.TextRun{Content: "Tab text\n"}},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	pm, err := buildParagraphMap(doc, "Main")
	if err != nil {
		t.Fatalf("buildParagraphMap with tab: %v", err)
	}

	if pm.TabID != "t.0" {
		t.Errorf("tabID = %q, want t.0", pm.TabID)
	}
	if len(pm.Paragraphs) != 1 {
		t.Fatalf("expected 1 paragraph, got %d", len(pm.Paragraphs))
	}
	if pm.Paragraphs[0].Text != "Tab text" {
		t.Errorf("text = %q, want 'Tab text'", pm.Paragraphs[0].Text)
	}
}

func TestBuildParagraphMap_TabNotFound(t *testing.T) {
	doc := &docs.Document{
		DocumentId: "doc-tabs",
		Tabs: []*docs.Tab{
			{
				TabProperties: &docs.TabProperties{
					TabId: "t.0",
					Title: "Main",
				},
				DocumentTab: &docs.DocumentTab{
					Body: &docs.Body{},
				},
			},
		},
	}

	_, err := buildParagraphMap(doc, "Nonexistent")
	if err == nil {
		t.Fatal("expected tab not found error")
	}
}
