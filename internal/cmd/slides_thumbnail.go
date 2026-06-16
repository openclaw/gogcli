package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SlidesThumbnailCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	SlideID        string `arg:"" name:"slideId" help:"Slide object ID (use 'slides list-slides' to find IDs)"`
	Size           string `name:"size" help:"Thumbnail size: small|medium|large" default:"large"`
	Format         string `name:"format" help:"Thumbnail format: png|jpeg" default:"png"`
	Output         string `name:"out" aliases:"output" help:"Write the thumbnail image to a local file"`
	Overwrite      bool   `name:"overwrite" help:"Overwrite an existing output file"`
}

func (c *SlidesThumbnailCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	presentationID := strings.TrimSpace(c.PresentationID)
	if presentationID == "" {
		return usage("empty presentationId")
	}
	slideID := strings.TrimSpace(c.SlideID)
	if slideID == "" {
		return usage("empty slideId")
	}

	size, err := normalizeSlidesThumbnailSize(c.Size)
	if err != nil {
		return err
	}
	format, err := normalizeSlidesThumbnailFormat(c.Format)
	if err != nil {
		return err
	}
	outputPath := strings.TrimSpace(c.Output)
	if outputPath != "" {
		outputPath, err = config.ExpandPath(outputPath)
		if err != nil {
			return err
		}
	}
	if dryRunErr := dryRunExit(ctx, flags, "slides.thumbnail", map[string]any{
		"presentation_id": presentationID,
		"slide_id":        slideID,
		"size":            strings.ToLower(size),
		"format":          strings.ToLower(format),
		"out":             outputPath,
		"overwrite":       c.Overwrite,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	slidesSvc, err := slidesService(ctx, account)
	if err != nil {
		return err
	}

	call := slidesSvc.Presentations.Pages.GetThumbnail(presentationID, slideID).
		ThumbnailPropertiesThumbnailSize(size).
		ThumbnailPropertiesMimeType(format)

	thumb, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get thumbnail: %w", err)
	}
	if thumb == nil || strings.TrimSpace(thumb.ContentUrl) == "" {
		return fmt.Errorf("get thumbnail: empty content URL")
	}

	result := map[string]any{
		"presentationId": presentationID,
		"slideId":        slideID,
		"contentUrl":     thumb.ContentUrl,
		"width":          thumb.Width,
		"height":         thumb.Height,
		"size":           strings.ToLower(size),
		"format":         strings.ToLower(format),
	}

	if outputPath != "" {
		written, writtenPath, err := downloadSlidesThumbnail(ctx, thumb.ContentUrl, outputPath, c.Overwrite)
		if err != nil {
			return err
		}
		outputPath = writtenPath
		result["output"] = writtenPath
		result["bytes"] = written
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), result)
	}

	u.Out().Linef("presentationId\t%s", presentationID)
	u.Out().Linef("slideId\t%s", slideID)
	u.Out().Linef("url\t%s", thumb.ContentUrl)
	if thumb.Width > 0 {
		u.Out().Linef("width\t%d", thumb.Width)
	}
	if thumb.Height > 0 {
		u.Out().Linef("height\t%d", thumb.Height)
	}
	u.Out().Linef("size\t%s", strings.ToLower(size))
	u.Out().Linef("format\t%s", strings.ToLower(format))
	if outputPath != "" {
		u.Out().Linef("output\t%s", outputPath)
		if bytes, ok := result["bytes"].(int64); ok {
			u.Out().Linef("bytes\t%d", bytes)
		}
	}

	return nil
}

func normalizeSlidesThumbnailSize(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "large":
		return "LARGE", nil
	case "medium":
		return "MEDIUM", nil
	case "small":
		return "SMALL", nil
	default:
		return "", usagef("invalid thumbnail size %q (expected small, medium, or large)", v)
	}
}

func normalizeSlidesThumbnailFormat(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "png":
		return "PNG", nil
	case "jpeg", "jpg":
		return "JPEG", nil
	default:
		return "", usagef("invalid thumbnail format %q (expected png or jpeg)", v)
	}
}

func downloadSlidesThumbnail(ctx context.Context, url, outputPath string, overwrite bool) (int64, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, "", fmt.Errorf("build thumbnail download request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("download thumbnail: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, "", fmt.Errorf("download thumbnail: unexpected status %s", resp.Status)
	}

	f, expandedPath, err := openUserOutputFile(outputPath, outputFileOptions{
		Overwrite: overwrite,
		FileMode:  0o600,
		DirMode:   0o700,
	})
	if err != nil {
		return 0, "", fmt.Errorf("create output file: %w", err)
	}

	n, err := io.Copy(f, resp.Body)
	if err != nil {
		_ = f.Close()
		return 0, "", fmt.Errorf("write output file: %w", err)
	}
	if err := f.Close(); err != nil {
		return 0, "", fmt.Errorf("close output file: %w", err)
	}

	return n, expandedPath, nil
}
