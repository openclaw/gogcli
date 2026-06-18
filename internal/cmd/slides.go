package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/slidesmarkdown"
	"github.com/steipete/gogcli/internal/ui"
)

type SlidesCmd struct {
	Export             SlidesExportCmd             `cmd:"" name:"export" aliases:"download,dl" help:"Export a Google Slides deck (pdf|pptx)"`
	Info               SlidesInfoCmd               `cmd:"" name:"info" aliases:"get,show" help:"Get Google Slides presentation metadata"`
	Create             SlidesCreateCmd             `cmd:"" name:"create" aliases:"add,new" help:"Create a Google Slides presentation"`
	CreateFromMarkdown SlidesCreateFromMarkdownCmd `cmd:"" name:"create-from-markdown" help:"Create a Google Slides presentation from markdown"`
	CreateFromTemplate SlidesCreateFromTemplateCmd `cmd:"" name:"create-from-template" help:"Create a presentation from template with text replacements"`
	Copy               SlidesCopyCmd               `cmd:"" name:"copy" aliases:"cp,duplicate" help:"Copy a Google Slides presentation"`
	AddSlide           SlidesAddSlideCmd           `cmd:"" name:"add-slide" help:"Add a slide with a full-bleed image and optional speaker notes"`
	NewSlide           SlidesNewSlideCmd           `cmd:"" name:"new-slide" help:"Create a native themed slide"`
	DuplicateSlide     SlidesDuplicateSlideCmd     `cmd:"" name:"duplicate-slide" help:"Duplicate a slide by object ID"`
	MoveSlide          SlidesMoveSlideCmd          `cmd:"" name:"move-slide" help:"Move a slide to a zero-based insertion index"`
	ListSlides         SlidesListSlidesCmd         `cmd:"" name:"list-slides" help:"List all slides with their object IDs"`
	DeleteSlide        SlidesDeleteSlideCmd        `cmd:"" name:"delete-slide" help:"Delete a slide by object ID"`
	ReadSlide          SlidesReadSlideCmd          `cmd:"" name:"read-slide" help:"Read slide content: speaker notes, text elements, and images"`
	Locate             SlidesLocateCmd             `cmd:"" name:"locate" aliases:"find-element" help:"Locate text in shapes and table cells with object IDs and UTF-16 ranges"`
	Thumbnail          SlidesThumbnailCmd          `cmd:"" name:"thumbnail" aliases:"thumb" help:"Get or download a rendered thumbnail for a slide"`
	UpdateNotes        SlidesUpdateNotesCmd        `cmd:"" name:"update-notes" help:"Update speaker notes on an existing slide"`
	ReplaceSlide       SlidesReplaceSlideCmd       `cmd:"" name:"replace-slide" help:"Replace an existing slide image from a local file or public URL"`
	InsertImage        SlidesInsertImageCmd        `cmd:"" name:"insert-image" help:"Insert a local or public image at a position and size"`
	InsertText         SlidesInsertTextCmd         `cmd:"" name:"insert-text" help:"Insert text into an existing page element (shape or table) by objectId"`
	Table              SlidesTableCmd              `cmd:"" name:"table" help:"Create and update native tables"`
	StyleText          SlidesStyleTextCmd          `cmd:"" name:"style-text" help:"Apply range-scoped text styling to one page element"`
	Link               SlidesLinkCmd               `cmd:"" name:"link" help:"Apply a hyperlink to a text range in one page element"`
	Bullets            SlidesBulletsCmd            `cmd:"" name:"bullets" help:"Turn paragraph bullets on or off in one page element"`
	ReplaceText        SlidesReplaceTextCmd        `cmd:"" name:"replace-text" help:"Find-and-replace text in an explicit object, slide, or presentation scope"`
	Raw                SlidesRawCmd                `cmd:"" name:"raw" help:"Dump raw Google Slides API response as JSON (Presentations.Get; lossless; for scripting and LLM consumption)"`
}

// SlidesRawCmd dumps the full Presentations.Get response as JSON. The
// Slides API has no field mask, so output is unconditionally lossless.
// Note: response may contain short-lived authenticated image/video URLs
// (see docs/raw-audit.md for the risk assessment).
//
// REST reference: https://developers.google.com/slides/api/reference/rest/v1/presentations/get
// Go type: https://pkg.go.dev/google.golang.org/api/slides/v1#Presentation
type SlidesRawCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	Pretty         bool   `name:"pretty" help:"Pretty-print JSON (default: compact single-line)"`
}

func (c *SlidesRawCmd) Run(ctx context.Context, flags *RootFlags) error {
	id := strings.TrimSpace(c.PresentationID)
	if id == "" {
		return usage("empty presentationId")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := slidesService(ctx, account)
	if err != nil {
		return err
	}

	pres, err := svc.Presentations.Get(id).Context(ctx).Do()
	if err != nil {
		return err
	}
	pres, err = requireRawResponse(pres, "presentation not found")
	if err != nil {
		return err
	}

	return writeRawJSON(ctx, pres, c.Pretty)
}

type SlidesExportCmd struct {
	PresentationID string         `arg:"" name:"presentationId" help:"Presentation ID"`
	Output         OutputPathFlag `embed:""`
	Format         string         `name:"format" help:"Export format: pdf|pptx" default:"pptx"`
	Overwrite      bool           `name:"overwrite" help:"Overwrite an existing output file"`
}

func (c *SlidesExportCmd) Run(ctx context.Context, flags *RootFlags) error {
	return exportViaDrive(ctx, flags, exportViaDriveOptions{
		ArgName:       "presentationId",
		ExpectedMime:  "application/vnd.google-apps.presentation",
		KindLabel:     "Google Slides presentation",
		DefaultFormat: "pptx",
	}, c.PresentationID, c.Output.Path, c.Format, c.Overwrite)
}

type SlidesInfoCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
}

type SlidesCreateCmd struct {
	Title    string `arg:"" name:"title" help:"Presentation title"`
	Parent   string `name:"parent" help:"Destination folder ID"`
	Template string `name:"template" help:"Template presentation ID to copy from"`
}

func (c *SlidesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	title := strings.TrimSpace(c.Title)
	if title == "" {
		return usage("empty title")
	}
	parent := strings.TrimSpace(c.Parent)
	template := strings.TrimSpace(c.Template)
	if err := dryRunExit(ctx, flags, "slides.create", map[string]any{
		"title":    title,
		"parent":   parent,
		"template": template,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := driveService(ctx, account)
	if err != nil {
		return err
	}

	var created *drive.File

	// If template is provided, copy from it
	if c.Template != "" {
		f := &drive.File{
			Name: title,
		}
		if parent != "" {
			f.Parents = []string{parent}
		}

		created, err = svc.Files.Copy(template, f).
			SupportsAllDrives(true).
			Fields("id, name, mimeType, webViewLink").
			Context(ctx).
			Do()
		if err != nil {
			return fmt.Errorf("failed to copy template: %w", err)
		}
	} else {
		// Create blank presentation
		f := &drive.File{
			Name:     title,
			MimeType: "application/vnd.google-apps.presentation",
		}
		if parent != "" {
			f.Parents = []string{parent}
		}

		created, err = svc.Files.Create(f).
			SupportsAllDrives(true).
			Fields("id, name, mimeType, webViewLink").
			Context(ctx).
			Do()
		if err != nil {
			return err
		}
	}

	if created == nil {
		return errors.New("create failed")
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{strFile: created})
	}

	u.Out().Linef("id\t%s", created.Id)
	u.Out().Linef("name\t%s", created.Name)
	u.Out().Linef("mime\t%s", created.MimeType)
	if created.WebViewLink != "" {
		u.Out().Linef("link\t%s", created.WebViewLink)
	}
	return nil
}

type SlidesCreateFromMarkdownCmd struct {
	Title          string `arg:"" name:"title" help:"Presentation title"`
	Content        string `name:"content" help:"Markdown content (inline)"`
	ContentFile    string `name:"content-file" help:"Read markdown content from file"`
	Parent         string `name:"parent" help:"Destination folder ID"`
	Debug          bool   `name:"debug" help:"Show debug output"`
	FAStyle        string `name:"fa-style" help:"Default Font Awesome style when shortcode has no prefix" default:"solid"`
	MMDC           string `name:"mmdc" help:"Path to mermaid CLI (mmdc); empty disables diagram rendering" default:"mmdc"`
	Strict         bool   `name:"strict" help:"Treat skipped FA/diagram assets as fatal"`
	KeepTempImages bool   `name:"keep-temp-images" help:"Don't delete temporary Drive uploads after import"`
	NoNotes        bool   `name:"no-notes" help:"Discard ## Notes sections instead of inserting as speaker notes"`
}

func (c *SlidesCreateFromMarkdownCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	title := strings.TrimSpace(c.Title)
	if title == "" {
		return usage("empty title")
	}

	var markdown string
	var err error
	switch {
	case c.ContentFile != "":
		var data []byte
		data, err = os.ReadFile(c.ContentFile)
		if err != nil {
			return fmt.Errorf("read content file: %w", err)
		}
		markdown = string(data)
	case c.Content != "":
		markdown = c.Content
	default:
		return usage("either --content or --content-file is required")
	}

	parsed, err := slidesmarkdown.Parse(markdown, slidesmarkdown.ParseOptions{DefaultFAStyle: c.FAStyle})
	if err != nil {
		return fmt.Errorf("parse markdown: %w", err)
	}
	if len(parsed) == 0 {
		return fmt.Errorf("no slides found in markdown")
	}
	if c.Debug {
		fmt.Fprintf(stderrWriter(ctx), "parsed %d slides\n", len(parsed))
	}

	pipelineCfg := DefaultAssetPipelineConfig()
	pipelineCfg.MMDCPath = c.MMDC
	pipelineCfg.Strict = c.Strict
	pipelineCfg.KeepTempImages = c.KeepTempImages
	pipelineCfg.DefaultFAStyle = c.FAStyle

	opts := CreatePresentationFromMarkdownOptions{
		Title:    title,
		Parent:   c.Parent,
		Slides:   parsed,
		Pipeline: pipelineCfg,
		NoNotes:  c.NoNotes,
	}
	if flags.DryRun {
		return dryRunExit(ctx, flags, "slides.create-from-markdown", map[string]any{
			"title":        title,
			"slides":       len(parsed),
			"parent":       strings.TrimSpace(c.Parent),
			"content_file": strings.TrimSpace(c.ContentFile),
			"no_notes":     c.NoNotes,
			"batch_update": buildSlideyDryRunBatchUpdate(parsed),
		})
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	slidesSvc, err := slidesService(ctx, account)
	if err != nil {
		return err
	}
	driveSvc, err := driveService(ctx, account)
	if err != nil {
		return err
	}

	opts.SlidesService = slidesSvc
	opts.DriveService = driveSvc

	created, err := CreatePresentationFromMarkdownV2(ctx, opts)
	if err != nil {
		return err
	}

	file, err := driveSvc.Files.Get(created.PresentationId).
		Fields("id, name, webViewLink").
		SupportsAllDrives(true).
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		presentation, err := slidesSvc.Presentations.Get(created.PresentationId).Context(ctx).Do()
		if err != nil {
			return err
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"presentation": presentation,
			"file":         file,
		})
	}

	if created != nil {
		u.Out().Linef("Created presentation with %d slides", len(parsed))
		u.Out().Linef("id\t%s", created.PresentationId)
		u.Out().Linef("name\t%s", file.Name)
		if file.WebViewLink != "" {
			u.Out().Linef("link\t%s", file.WebViewLink)
		}
	}
	return nil
}

type SlidesCopyCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	Title          string `arg:"" name:"title" help:"New title"`
	Parent         string `name:"parent" help:"Destination folder ID"`
}

func (c *SlidesCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	return copyViaDrive(ctx, flags, copyViaDriveOptions{
		Op:           "slides.copy",
		ArgName:      "presentationId",
		ExpectedMime: "application/vnd.google-apps.presentation",
		KindLabel:    "Google Slides presentation",
	}, c.PresentationID, c.Title, c.Parent)
}
