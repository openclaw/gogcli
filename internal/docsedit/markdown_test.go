package docsedit

import (
	"errors"
	"strings"
	"testing"
)

func TestPlanMarkdownWriteModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		options    MarkdownWriteOptions
		wantMode   MarkdownWriteMode
		wantReason MarkdownWriteReason
		wantErr    string
	}{
		{
			name:       "whole document Drive replace",
			options:    MarkdownWriteOptions{Markdown: "# Title", Replace: true},
			wantMode:   MarkdownWriteDriveReplace,
			wantReason: MarkdownWriteWholeDocument,
		},
		{
			name:       "append",
			options:    MarkdownWriteOptions{Markdown: "# Title", Append: true},
			wantMode:   MarkdownWriteLocalAppend,
			wantReason: MarkdownWriteAppend,
		},
		{
			name:       "tab replace",
			options:    MarkdownWriteOptions{Markdown: "# Title", Replace: true, Tab: " Second "},
			wantMode:   MarkdownWriteLocalReplace,
			wantReason: MarkdownWriteTab,
		},
		{
			name: "table cell breaks",
			options: MarkdownWriteOptions{
				Markdown: "| Value |\n| --- |\n| Alice<br>Bob |",
				Replace:  true,
			},
			wantMode:   MarkdownWriteLocalReplace,
			wantReason: MarkdownWriteTableCellBreaks,
		},
		{
			name:    "missing mode",
			options: MarkdownWriteOptions{Markdown: "# Title"},
			wantErr: "--markdown requires --replace or --append",
		},
		{
			name:    "conflicting mode",
			options: MarkdownWriteOptions{Markdown: "# Title", Append: true, Replace: true},
			wantErr: "--append cannot be combined with --replace",
		},
		{
			name: "invalid orphan mode",
			options: MarkdownWriteOptions{
				Markdown:     "# Title",
				Append:       true,
				CheckOrphans: true,
			},
			wantErr: "--check-orphans requires --replace --markdown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := PlanMarkdownWrite(tt.options)
			if tt.wantErr != "" {
				var validationErr ValidationError
				if !errors.As(err, &validationErr) || err.Error() != tt.wantErr {
					t.Fatalf("error = %v, want validation error %q", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("PlanMarkdownWrite: %v", err)
			}

			if got.Mode != tt.wantMode || got.Reason != tt.wantReason {
				t.Fatalf("mode/reason = %d/%d, want %d/%d", got.Mode, got.Reason, tt.wantMode, tt.wantReason)
			}

			if tt.options.Tab != "" && got.Tab != strings.TrimSpace(tt.options.Tab) {
				t.Fatalf("tab = %q, want %q", got.Tab, strings.TrimSpace(tt.options.Tab))
			}
		})
	}
}

func TestPlanMarkdownWritePreparesDriveImport(t *testing.T) {
	t.Parallel()

	markdown := "|     |     |\n|-----|-----|\n| Label | Value |\n\n# Files {#attachments}\n\n[Jump](#attachments)\n"

	got, err := PlanMarkdownWrite(MarkdownWriteOptions{
		Markdown:   markdown,
		Replace:    true,
		ImageCount: 2,
	})
	if err != nil {
		t.Fatalf("PlanMarkdownWrite: %v", err)
	}

	if strings.Contains(got.Markdown, "|     |     |") {
		t.Fatalf("Markdown still has empty table header: %q", got.Markdown)
	}

	if !strings.Contains(got.Markdown, "| Label | Value |\n|-----|-----|") {
		t.Fatalf("Markdown missing normalized table header: %q", got.Markdown)
	}

	if strings.Contains(got.Markdown, "{#attachments}") {
		t.Fatalf("Markdown still has explicit heading anchor: %q", got.Markdown)
	}

	if len(got.ExplicitHeadingAnchors) != 1 {
		t.Fatalf("anchors = %#v, want one", got.ExplicitHeadingAnchors)
	}

	anchor := got.ExplicitHeadingAnchors[0]
	if anchor.Anchor != "attachments" || anchor.Text != "Files" || anchor.Occurrence != 1 {
		t.Fatalf("anchor = %#v", anchor)
	}

	if !got.RewriteHeadingLinks || !got.InsertImages {
		t.Fatalf("post actions = %#v, want heading rewrite and image insertion", got)
	}

	if !got.RequiresDriveService || !got.RequiresDocumentsService {
		t.Fatalf("service requirements = %#v, want Drive and Docs", got)
	}
}

func TestPlanMarkdownWritePreservesLocalMarkdown(t *testing.T) {
	t.Parallel()

	markdown := "# Files {#attachments}\n\n[Jump](#attachments)\n"

	got, err := PlanMarkdownWrite(MarkdownWriteOptions{
		Markdown: markdown,
		Append:   true,
	})
	if err != nil {
		t.Fatalf("PlanMarkdownWrite: %v", err)
	}

	if got.Markdown != markdown {
		t.Fatalf("Markdown = %q, want source unchanged", got.Markdown)
	}

	if len(got.ExplicitHeadingAnchors) != 1 || got.ExplicitHeadingAnchors[0].Anchor != "attachments" {
		t.Fatalf("anchors = %#v", got.ExplicitHeadingAnchors)
	}

	if got.RequiresDriveService || !got.RequiresDocumentsService {
		t.Fatalf("service requirements = %#v, want Docs only", got)
	}
}

func TestPlanMarkdownWriteServiceAndOrphanRequirements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		options   MarkdownWriteOptions
		wantDrive bool
		wantDocs  bool
		wholeDoc  bool
	}{
		{
			name:      "plain Drive replace",
			options:   MarkdownWriteOptions{Markdown: "text", Replace: true},
			wantDrive: true,
		},
		{
			name: "Drive replace with style",
			options: MarkdownWriteOptions{
				Markdown:           "text",
				Replace:            true,
				ApplyDocumentStyle: true,
			},
			wantDrive: true,
			wantDocs:  true,
		},
		{
			name: "whole document orphan check",
			options: MarkdownWriteOptions{
				Markdown:     "text",
				Replace:      true,
				CheckOrphans: true,
			},
			wantDrive: true,
			wantDocs:  true,
			wholeDoc:  true,
		},
		{
			name: "local table replace orphan check",
			options: MarkdownWriteOptions{
				Markdown:     "| A |\n| --- |\n| x<br>y |",
				Replace:      true,
				CheckOrphans: true,
			},
			wantDrive: true,
			wantDocs:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := PlanMarkdownWrite(tt.options)
			if err != nil {
				t.Fatalf("PlanMarkdownWrite: %v", err)
			}

			if got.RequiresDriveService != tt.wantDrive || got.RequiresDocumentsService != tt.wantDocs {
				t.Fatalf(
					"service requirements = Drive %t, Docs %t; want Drive %t, Docs %t",
					got.RequiresDriveService,
					got.RequiresDocumentsService,
					tt.wantDrive,
					tt.wantDocs,
				)
			}

			if got.OrphanScopeWholeDocument != tt.wholeDoc {
				t.Fatalf("whole-document orphan scope = %t, want %t", got.OrphanScopeWholeDocument, tt.wholeDoc)
			}
		})
	}
}
