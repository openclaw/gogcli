package cmd

import (
	"context"
	"errors"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SlidesCmd struct {
	Export SlidesExportCmd `cmd:"" name:"export" help:"Export a Google Slides deck (pdf|pptx)"`
	Info   SlidesInfoCmd   `cmd:"" name:"info" help:"Get Google Slides presentation metadata"`
	Create SlidesCreateCmd `cmd:"" name:"create" help:"Create a Google Slides presentation"`
	Copy   SlidesCopyCmd   `cmd:"" name:"copy" help:"Copy a Google Slides presentation"`
}

type SlidesExportCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	Out            string `name:"out" aliases:"output" help:"Output file path (default: gogcli config dir)"`
	Format         string `name:"format" help:"Export format: pdf|pptx" default:"pptx"`
}

func (c *SlidesExportCmd) Run(ctx context.Context, flags *RootFlags) error {
	return exportViaDrive(ctx, flags, exportViaDriveOptions{
		ArgName:       "presentationId",
		ExpectedMime:  "application/vnd.google-apps.presentation",
		KindLabel:     "Google Slides presentation",
		DefaultFormat: "pptx",
	}, c.PresentationID, c.Out, c.Format)
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
	Title  string `arg:"" name:"title" help:"Presentation title"`
	Parent string `name:"parent" help:"Destination folder ID"`
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

	f := &drive.File{
		Name:     title,
		MimeType: "application/vnd.google-apps.presentation",
	}
	parent := strings.TrimSpace(c.Parent)
	if parent != "" {
		f.Parents = []string{parent}
	}

	created, err := svc.Files.Create(f).
		SupportsAllDrives(true).
		Fields("id, name, mimeType, webViewLink").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}
	if created == nil {
		return errors.New("create failed")
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"file": created})
	}

	u.Out().Printf("id\t%s", created.Id)
	u.Out().Printf("name\t%s", created.Name)
	u.Out().Printf("mime\t%s", created.MimeType)
	if created.WebViewLink != "" {
		u.Out().Printf("link\t%s", created.WebViewLink)
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
