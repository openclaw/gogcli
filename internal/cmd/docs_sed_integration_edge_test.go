package cmd

import (
	"context"
	"io"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// =============================================================================
// Integration Tests: Error Cases
// =============================================================================

func TestSedIntegration_InvalidRegex(t *testing.T) {
	doc := buildDoc(para(plain("test")))
	err := runSedIntegrationErr(t, doc, "s/[invalid/replacement/", nil)
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestSedIntegration_EmptyDocID(t *testing.T) {
	doc := buildDoc(para(plain("test")))

	srv := mockDocsServerAdvanced(t, doc, nil)
	defer srv.Close()

	origNewDocs := newDocsService
	newDocsService = func(ctx context.Context, account string) (*docs.Service, error) {
		return docs.NewService(ctx,
			option.WithoutAuthentication(),
			option.WithEndpoint(srv.URL+"/"),
		)
	}
	defer func() { newDocsService = origNewDocs }()

	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})
	cmd := &DocsSedCmd{DocID: "", Expression: "s/a/b/"}
	flags := &RootFlags{Account: "test@example.com"}
	err := cmd.Run(ctx, flags)
	if err == nil {
		t.Error("expected error for empty doc ID")
	}
}

func TestSedIntegration_NoExpression(t *testing.T) {
	doc := buildDoc(para(plain("test")))

	srv := mockDocsServerAdvanced(t, doc, nil)
	defer srv.Close()

	origNewDocs := newDocsService
	newDocsService = func(ctx context.Context, account string) (*docs.Service, error) {
		return docs.NewService(ctx,
			option.WithoutAuthentication(),
			option.WithEndpoint(srv.URL+"/"),
		)
	}
	defer func() { newDocsService = origNewDocs }()

	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})
	cmd := &DocsSedCmd{DocID: "test-doc-id"}
	flags := &RootFlags{Account: "test@example.com"}
	err := cmd.Run(ctx, flags)
	if err == nil {
		t.Error("expected error for no expressions")
	}
}

func TestSedIntegration_InvalidExpression(t *testing.T) {
	doc := buildDoc(para(plain("test")))
	err := runSedIntegrationErr(t, doc, "not-a-sed-expression", nil)
	if err == nil {
		t.Error("expected error for invalid sed expression")
	}
}

// =============================================================================
// Integration Tests: Complex Real-World Scenarios
// =============================================================================

func TestSedIntegration_EmailObfuscation(t *testing.T) {
	doc := buildDoc(para(plain("Contact us at user@example.com for info")))
	reqs := runSedIntegration(t, doc, `s/(\w+)@(\w+)\.(\w+)/$1[at]$2[dot]$3/g`, nil)

	// Capture group replacement uses manual path
	hasReplace := false
	for _, r := range reqs {
		if r.InsertText != nil && strings.Contains(r.InsertText.Text, "[at]") {
			hasReplace = true
		}
		if r.ReplaceAllText != nil {
			hasReplace = true
		}
	}
	if !hasReplace {
		t.Error("expected email obfuscation replacement")
	}
}

func TestSedIntegration_VersionBump(t *testing.T) {
	doc := buildDoc(
		para(plain("Version: v1.2.3")),
		para(plain("Released: 2026-01-15")),
	)
	reqs := runSedIntegration(t, doc, "s/v1\\.2\\.3/v1.3.0/", nil)

	found := false
	for _, r := range reqs {
		if r.InsertText != nil && r.InsertText.Text == "v1.3.0" {
			found = true
		}
	}
	if !found {
		t.Error("expected version bump replacement")
	}
}

func TestSedIntegration_CollapseWhitespace(t *testing.T) {
	doc := buildDoc(para(plain("too    many     spaces    here")))
	reqs := runSedIntegration(t, doc, `s/ {2,}/ /g`, nil)

	// Regex global may use native or manual path
	hasReplace := false
	for _, r := range reqs {
		if r.ReplaceAllText != nil || r.InsertText != nil {
			hasReplace = true
		}
	}
	if !hasReplace {
		t.Error("expected replacement requests for whitespace collapse")
	}
}

func TestSedIntegration_MarkdownBulletToPlain(t *testing.T) {
	doc := buildDoc(para(plain("- Item one\n- Item two\n- Item three")))
	reqs := runSedIntegration(t, doc, "s/^- //g", nil)

	// Should strip bullet markers
	if len(reqs) == 0 {
		t.Error("expected requests for bullet strip")
	}
}

func TestSedIntegration_LongDocument(t *testing.T) {
	// Build a document with many paragraphs
	paras := make([]testDocParagraph, 50)
	for i := range paras {
		if i%5 == 0 {
			paras[i] = para(plain("Replace this target word here"))
		} else {
			paras[i] = para(plain("No matches in this paragraph at all"))
		}
	}
	doc := buildDoc(paras...)
	reqs := runSedIntegration(t, doc, "s/target/REPLACED/g", nil)

	// Global plain text uses native ReplaceAllText (single API call)
	hasNative := false
	for _, r := range reqs {
		if r.ReplaceAllText != nil && r.ReplaceAllText.ReplaceText == "REPLACED" {
			hasNative = true
		}
	}
	if !hasNative {
		t.Error("expected native ReplaceAllText for long document global replace")
	}
}

func TestSedIntegration_UnicodeContent(t *testing.T) {
	doc := buildDoc(para(plain("Héllo wörld 你好 🌍")))
	reqs := runSedIntegration(t, doc, "s/wörld/world/", nil)

	found := false
	for _, r := range reqs {
		if r.InsertText != nil && r.InsertText.Text == "world" {
			found = true
		}
	}
	if !found {
		t.Error("expected unicode replacement")
	}
}

func TestSedIntegration_NewlineInReplacement(t *testing.T) {
	doc := buildDoc(para(plain("Line one. Line two.")))
	reqs := runSedIntegration(t, doc, `s/\. /.\n/g`, nil)

	if len(reqs) == 0 {
		t.Error("expected requests for newline insertion")
	}
}

// =============================================================================
// Integration Tests: Image Syntax
// =============================================================================

func TestSedIntegration_ImageInsert(t *testing.T) {
	doc := buildDoc(para(plain("[LOGO_PLACEHOLDER]")))
	reqs := runSedIntegration(t, doc, "s/\\[LOGO_PLACEHOLDER\\]/!(https:\\/\\/example.com\\/logo.png)/", nil)

	// Image insert generates: delete old text + InsertInlineImage
	hasDelete := false
	hasImageInsert := false
	for _, r := range reqs {
		if r.DeleteContentRange != nil {
			hasDelete = true
		}
		if r.InsertInlineImage != nil {
			hasImageInsert = true
		}
	}
	if !hasDelete && !hasImageInsert {
		// At minimum, there should be a delete of the placeholder
		t.Error("expected delete or image insert request")
	}
}

func TestSedIntegration_ImageWithDimensions(t *testing.T) {
	doc := buildDoc(para(plain("[BANNER]")))
	reqs := runSedIntegration(t, doc, "s/\\[BANNER\\]/!(https:\\/\\/example.com\\/banner.png){width=600 height=200}/", nil)

	// Should at least delete the placeholder text and attempt image insert
	hasDelete := false
	hasImageInsert := false
	for _, r := range reqs {
		if r.DeleteContentRange != nil {
			hasDelete = true
		}
		if r.InsertInlineImage != nil {
			hasImageInsert = true
		}
	}
	if !hasDelete && !hasImageInsert {
		t.Error("expected delete or image insert request")
	}
}

// =============================================================================
// Integration Tests: Table Creation via sedmat
// =============================================================================

func TestSedIntegration_TableCreate(t *testing.T) {
	doc := buildDoc(para(plain("[TABLE_HERE]")))
	reqs := runSedIntegration(t, doc, "s/\\[TABLE_HERE\\]/|3x4|/", nil)

	hasTableInsert := false
	for _, r := range reqs {
		if r.InsertTable != nil {
			hasTableInsert = true
			if r.InsertTable.Rows != 3 {
				t.Errorf("expected 3 rows, got %d", r.InsertTable.Rows)
			}
			if r.InsertTable.Columns != 4 {
				t.Errorf("expected 4 columns, got %d", r.InsertTable.Columns)
			}
		}
	}
	if !hasTableInsert {
		t.Error("expected table insert request")
	}
}

func TestSedIntegration_TableCreateWithHeader(t *testing.T) {
	doc := buildDoc(para(plain("[DATA]")))
	reqs := runSedIntegration(t, doc, "s/\\[DATA\\]/|5x3:header|/", nil)

	hasTableInsert := false
	for _, r := range reqs {
		if r.InsertTable != nil {
			hasTableInsert = true
		}
	}
	if !hasTableInsert {
		t.Error("expected table insert with header")
	}
}

// =============================================================================
// Integration Tests: Reverse Index Ordering
// =============================================================================

func TestSedIntegration_RequestsOrderedReverseIndex(t *testing.T) {
	doc := buildDoc(
		para(plain("aaa bbb ccc")),
	)
	reqs := runSedIntegration(t, doc, "s/[abc]{3}/X/g", nil)

	// Delete/insert requests should be ordered from end to start
	// so indices don't shift during processing
	var deleteIndices []int64
	for _, r := range reqs {
		if r.DeleteContentRange != nil {
			deleteIndices = append(deleteIndices, r.DeleteContentRange.Range.StartIndex)
		}
	}
	for i := 1; i < len(deleteIndices); i++ {
		if deleteIndices[i] > deleteIndices[i-1] {
			t.Errorf("delete requests not in reverse order: index %d (%d) > index %d (%d)",
				i, deleteIndices[i], i-1, deleteIndices[i-1])
		}
	}
}

// =============================================================================
// Integration Tests: Native API Fast Path
// =============================================================================

func TestSedIntegration_NativeFastPath(t *testing.T) {
	// Plain text global replace should use the native ReplaceAllText API
	doc := buildDoc(para(plain("Hello world")))
	reqs := runSedIntegration(t, doc, "s/Hello/Hi/g", nil)

	hasNative := false
	for _, r := range reqs {
		if r.ReplaceAllText != nil {
			hasNative = true
		}
	}
	if !hasNative {
		t.Error("expected native ReplaceAllText for plain text global replace")
	}
}

func TestSedIntegration_FormattingForcesManualPath(t *testing.T) {
	// Markdown formatting should NOT use native path
	doc := buildDoc(para(plain("Hello world")))
	reqs := runSedIntegration(t, doc, "s/Hello/**Hi**/", nil)

	hasNative := false
	for _, r := range reqs {
		if r.ReplaceAllText != nil {
			hasNative = true
		}
	}
	if hasNative {
		t.Error("formatting replacement should not use native ReplaceAllText")
	}
}

func TestSedIntegration_HeadingFormat(t *testing.T) {
	doc := buildDoc(para(plain("H4_TARGET")))
	reqs := runSedIntegration(t, doc, `s/H4_TARGET/#### Heading 4 Demo/`, nil)

	// Should have InsertText + UpdateParagraphStyle with HEADING_4
	foundHeading := false
	for _, r := range reqs {
		if r.UpdateParagraphStyle != nil {
			t.Logf("UpdateParagraphStyle: %+v", r.UpdateParagraphStyle)
			if r.UpdateParagraphStyle.ParagraphStyle != nil && r.UpdateParagraphStyle.ParagraphStyle.NamedStyleType == "HEADING_4" {
				foundHeading = true
			}
		}
		if r.InsertText != nil {
			t.Logf("InsertText: %q", r.InsertText.Text)
		}
		if r.DeleteContentRange != nil {
			t.Logf("Delete: %d-%d", r.DeleteContentRange.Range.StartIndex, r.DeleteContentRange.Range.EndIndex)
		}
	}
	if !foundHeading {
		t.Errorf("expected HEADING_4 paragraph style, got requests: %d", len(reqs))
		for i, r := range reqs {
			t.Logf("  req[%d]: %+v", i, r)
		}
	}
}

func TestSedIntegration_BulletList(t *testing.T) {
	doc := buildDoc(para(plain("bullet_item")))
	reqs := runSedIntegration(t, doc, `s/bullet_item/- First bullet/`, nil)

	foundBullet := false
	for _, r := range reqs {
		if r.CreateParagraphBullets != nil {
			t.Logf("CreateParagraphBullets: %+v", r.CreateParagraphBullets)
			foundBullet = true
		}
		if r.InsertText != nil {
			t.Logf("InsertText: %q", r.InsertText.Text)
		}
	}
	if !foundBullet {
		t.Errorf("expected CreateParagraphBullets request, got requests: %d", len(reqs))
	}
}
