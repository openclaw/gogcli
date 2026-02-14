package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// Debug flag for slides creation
var debugSlides = false

var newSlidesService = googleapi.NewSlides

type SlidesCmd struct {
	Export             SlidesExportCmd             `cmd:"" name:"export" aliases:"download,dl" help:"Export a Google Slides deck (pdf|pptx)"`
	Info               SlidesInfoCmd               `cmd:"" name:"info" aliases:"get,show" help:"Get Google Slides presentation metadata"`
	Create             SlidesCreateCmd             `cmd:"" name:"create" aliases:"add,new" help:"Create a Google Slides presentation"`
	CreateFromMarkdown SlidesCreateFromMarkdownCmd `cmd:"" name:"create-from-markdown" help:"Create a Google Slides presentation from markdown"`
	Copy               SlidesCopyCmd               `cmd:"" name:"copy" aliases:"cp,duplicate" help:"Copy a Google Slides presentation"`
	AddSlide           SlidesAddSlideCmd           `cmd:"" name:"add-slide" help:"Add a slide with a full-bleed image and optional speaker notes"`
	ListSlides         SlidesListSlidesCmd         `cmd:"" name:"list-slides" help:"List all slides with their object IDs"`
	DeleteSlide        SlidesDeleteSlideCmd        `cmd:"" name:"delete-slide" help:"Delete a slide by object ID"`
	ReadSlide          SlidesReadSlideCmd          `cmd:"" name:"read-slide" help:"Read slide content: speaker notes, text elements, and images"`
	UpdateNotes        SlidesUpdateNotesCmd        `cmd:"" name:"update-notes" help:"Update speaker notes on an existing slide"`
	ReplaceSlide       SlidesReplaceSlideCmd       `cmd:"" name:"replace-slide" help:"Replace the image on an existing slide in-place"`
}

type SlidesExportCmd struct {
	PresentationID string         `arg:"" name:"presentationId" help:"Presentation ID"`
	Output         OutputPathFlag `embed:""`
	Format         string         `name:"format" help:"Export format: pdf|pptx" default:"pptx"`
}

func (c *SlidesExportCmd) Run(ctx context.Context, flags *RootFlags) error {
	return exportViaDrive(ctx, flags, exportViaDriveOptions{
		ArgName:       "presentationId",
		ExpectedMime:  "application/vnd.google-apps.presentation",
		KindLabel:     "Google Slides presentation",
		DefaultFormat: "pptx",
	}, c.PresentationID, c.Output.Path, c.Format)
}

type SlidesInfoCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
}

func (c *SlidesInfoCmd) Run(ctx context.Context, flags *RootFlags) error {
	return infoViaDrive(ctx, flags, infoViaDriveOptions{
		ArgName:      "presentationId",
		ExpectedMime: "application/vnd.google-apps.presentation",
		KindLabel:    "Google Slides presentation",
	}, c.PresentationID)
}

type SlidesCreateCmd struct {
	Title    string `arg:"" name:"title" help:"Presentation title"`
	Parent   string `name:"parent" help:"Destination folder ID"`
	Template string `name:"template" help:"Template presentation ID to copy from"`
}

func (c *SlidesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	title := strings.TrimSpace(c.Title)
	if title == "" {
		return usage("empty title")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	var created *drive.File

	// If template is provided, copy from it
	if c.Template != "" {
		f := &drive.File{
			Name: title,
		}
		parent := strings.TrimSpace(c.Parent)
		if parent != "" {
			f.Parents = []string{parent}
		}

		created, err = svc.Files.Copy(c.Template, f).
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
		parent := strings.TrimSpace(c.Parent)
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
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{strFile: created})
	}

	u.Out().Printf("id\t%s", created.Id)
	u.Out().Printf("name\t%s", created.Name)
	u.Out().Printf("mime\t%s", created.MimeType)
	if created.WebViewLink != "" {
		u.Out().Printf("link\t%s", created.WebViewLink)
	}
	return nil
}

type SlidesCreateFromMarkdownCmd struct {
	Title       string `arg:"" name:"title" help:"Presentation title"`
	Content     string `name:"content" help:"Markdown content (inline)"`
	ContentFile string `name:"content-file" help:"Read markdown content from file"`
	Parent      string `name:"parent" help:"Destination folder ID"`
	Debug       bool   `name:"debug" help:"Show debug output"`
}

func (c *SlidesCreateFromMarkdownCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	title := strings.TrimSpace(c.Title)
	if title == "" {
		return usage("empty title")
	}

	// Get markdown content
	var markdown string
	switch {
	case c.ContentFile != "":
		var data []byte
		data, err = os.ReadFile(c.ContentFile)
		if err != nil {
			return fmt.Errorf("failed to read content file: %w", err)
		}
		markdown = string(data)
	case c.Content != "":
		markdown = c.Content
	default:
		return usage("either --content or --content-file is required")
	}

	if c.Debug {
		debugSlides = true
	}

	// Create Slides service
	slidesSvc, err := newSlidesService(ctx, account)
	if err != nil {
		return err
	}

	// Create presentation from markdown
	presentation, err := CreatePresentationFromMarkdown(title, markdown, slidesSvc)
	if err != nil {
		return err
	}

	// Move to parent folder if specified
	if c.Parent != "" {
		var parentDriveSvc *drive.Service
		parentDriveSvc, err = newDriveService(ctx, account)
		if err != nil {
			return err
		}

		_, err = parentDriveSvc.Files.Update(presentation.PresentationId, &drive.File{}).
			AddParents(c.Parent).
			SupportsAllDrives(true).
			Context(ctx).
			Do()
		if err != nil {
			return fmt.Errorf("failed to move presentation to folder: %w", err)
		}
	}

	// Get presentation link
	var driveSvc *drive.Service
	driveSvc, err = newDriveService(ctx, account)
	if err != nil {
		return err
	}

	file, err := driveSvc.Files.Get(presentation.PresentationId).
		Fields("id, name, webViewLink").
		SupportsAllDrives(true).
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"presentation": presentation,
			"file":         file,
		})
	}

	u.Out().Printf("Created presentation with %d slides", len(ParseMarkdownToSlides(markdown)))
	u.Out().Printf("id\t%s", presentation.PresentationId)
	u.Out().Printf("name\t%s", file.Name)
	if file.WebViewLink != "" {
		u.Out().Printf("link\t%s", file.WebViewLink)
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
		ArgName:      "presentationId",
		ExpectedMime: "application/vnd.google-apps.presentation",
		KindLabel:    "Google Slides presentation",
	}, c.PresentationID, c.Title, c.Parent)
}
