package docsedit

import (
	"strings"

	"github.com/steipete/gogcli/internal/docsmarkdown"
)

type MarkdownWriteMode uint8

const (
	MarkdownWriteDriveReplace MarkdownWriteMode = iota
	MarkdownWriteLocalAppend
	MarkdownWriteLocalReplace
)

type MarkdownWriteReason uint8

const (
	MarkdownWriteWholeDocument MarkdownWriteReason = iota
	MarkdownWriteAppend
	MarkdownWriteTab
	MarkdownWriteTableCellBreaks
)

type MarkdownWriteOptions struct {
	Markdown           string
	ImageCount         int
	Append             bool
	Replace            bool
	Tab                string
	CheckOrphans       bool
	ApplyDocumentStyle bool
}

type MarkdownWritePlan struct {
	Mode                     MarkdownWriteMode
	Reason                   MarkdownWriteReason
	Markdown                 string
	Tab                      string
	ImageCount               int
	ExplicitHeadingAnchors   []docsmarkdown.ExplicitHeadingAnchor
	CheckOrphans             bool
	OrphanScopeWholeDocument bool
	RewriteHeadingLinks      bool
	InsertImages             bool
	ApplyDocumentStyle       bool
	RequiresDriveService     bool
	RequiresDocumentsService bool
}

func PlanMarkdownWrite(options MarkdownWriteOptions) (MarkdownWritePlan, error) {
	if options.Append && options.Replace {
		return MarkdownWritePlan{}, invalid("--append cannot be combined with --replace")
	}

	if options.CheckOrphans && (!options.Replace || options.Append) {
		return MarkdownWritePlan{}, invalid("--check-orphans requires --replace --markdown")
	}

	if options.ImageCount < 0 {
		return MarkdownWritePlan{}, invalid("image count must be >= 0")
	}

	tab := strings.TrimSpace(options.Tab)

	mode, reason, err := planMarkdownWriteMode(options.Append, options.Replace, tab, options.Markdown)
	if err != nil {
		return MarkdownWritePlan{}, err
	}

	markdown := options.Markdown
	var anchors []docsmarkdown.ExplicitHeadingAnchor

	if mode == MarkdownWriteDriveReplace {
		markdown = docsmarkdown.NormalizeTablesForDriveImport(markdown)
		anchors = docsmarkdown.ImportExplicitHeadingAnchors(markdown)
		markdown = docsmarkdown.StripHeadingAnchors(markdown)
	} else {
		anchors = docsmarkdown.ExplicitHeadingAnchors(markdown)
	}

	rewriteHeadingLinks := strings.Contains(markdown, "](#")
	insertImages := options.ImageCount > 0
	requiresDrive := mode == MarkdownWriteDriveReplace || options.CheckOrphans
	requiresDocuments := mode != MarkdownWriteDriveReplace ||
		options.CheckOrphans ||
		rewriteHeadingLinks ||
		insertImages ||
		options.ApplyDocumentStyle

	return MarkdownWritePlan{
		Mode:                     mode,
		Reason:                   reason,
		Markdown:                 markdown,
		Tab:                      tab,
		ImageCount:               options.ImageCount,
		ExplicitHeadingAnchors:   anchors,
		CheckOrphans:             options.CheckOrphans,
		OrphanScopeWholeDocument: options.CheckOrphans && mode == MarkdownWriteDriveReplace,
		RewriteHeadingLinks:      rewriteHeadingLinks,
		InsertImages:             insertImages,
		ApplyDocumentStyle:       options.ApplyDocumentStyle,
		RequiresDriveService:     requiresDrive,
		RequiresDocumentsService: requiresDocuments,
	}, nil
}

func planMarkdownWriteMode(appendMode, replaceMode bool, tab, markdown string) (MarkdownWriteMode, MarkdownWriteReason, error) {
	switch {
	case appendMode:
		return MarkdownWriteLocalAppend, MarkdownWriteAppend, nil
	case !replaceMode:
		return 0, 0, invalid("--markdown requires --replace or --append")
	case tab != "":
		return MarkdownWriteLocalReplace, MarkdownWriteTab, nil
	case docsmarkdown.HasTableCellBreaks(markdown):
		return MarkdownWriteLocalReplace, MarkdownWriteTableCellBreaks, nil
	default:
		return MarkdownWriteDriveReplace, MarkdownWriteWholeDocument, nil
	}
}
