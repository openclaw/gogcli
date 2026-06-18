package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SlidesStyleTextCmd struct {
	PresentationID string  `arg:"" name:"presentationId" help:"Presentation ID"`
	ObjectID       string  `arg:"" name:"objectId" help:"Page element object ID containing the text"`
	Range          string  `name:"range" required:"" help:"UTF-16 text range as start:end"`
	Bold           bool    `name:"bold" help:"Set bold"`
	NoBold         bool    `name:"no-bold" help:"Clear bold"`
	Italic         bool    `name:"italic" help:"Set italic"`
	NoItalic       bool    `name:"no-italic" help:"Clear italic"`
	Underline      bool    `name:"underline" help:"Set underline"`
	NoUnderline    bool    `name:"no-underline" help:"Clear underline"`
	TextColor      string  `name:"text-color" help:"Text color as #RRGGBB or #RGB"`
	Size           float64 `name:"size" help:"Font size in points"`
	Font           string  `name:"font" help:"Font family, for example Arial or Georgia"`
}

func (c *SlidesStyleTextCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, objectID, textRange, err := resolveSlidesTextTarget(c.PresentationID, c.ObjectID, c.Range)
	if err != nil {
		return err
	}
	req, fields, err := buildSlidesStyleTextRequest(objectID, textRange, slidesStyleTextOptions{
		Bold:        c.Bold,
		NoBold:      c.NoBold,
		Italic:      c.Italic,
		NoItalic:    c.NoItalic,
		Underline:   c.Underline,
		NoUnderline: c.NoUnderline,
		Color:       c.TextColor,
		Size:        c.Size,
		Font:        c.Font,
	})
	if err != nil {
		return err
	}
	body := &slides.BatchUpdatePresentationRequest{Requests: []*slides.Request{{UpdateTextStyle: req}}}
	return runSlidesTextBatch(ctx, flags, "slides.style-text", presentationID, body, map[string]any{
		"presentation_id": presentationID,
		"object_id":       objectID,
		"range":           c.Range,
		"fields":          fields,
		"batch_update":    body,
	})
}

type SlidesLinkCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	ObjectID       string `arg:"" name:"objectId" help:"Page element object ID containing the text"`
	Range          string `name:"range" required:"" help:"UTF-16 text range as start:end"`
	URL            string `name:"url" help:"External URL to apply as the hyperlink"`
	Clear          bool   `name:"clear" help:"Remove the hyperlink from the selected range"`
}

func (c *SlidesLinkCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, objectID, textRange, err := resolveSlidesTextTarget(c.PresentationID, c.ObjectID, c.Range)
	if err != nil {
		return err
	}
	url := strings.TrimSpace(c.URL)
	if (url == "") == !c.Clear {
		return usage("exactly one of --url or --clear is required")
	}
	style := &slides.TextStyle{}
	if !c.Clear {
		style.Link = &slides.Link{Url: url}
	}
	body := &slides.BatchUpdatePresentationRequest{Requests: []*slides.Request{{
		UpdateTextStyle: &slides.UpdateTextStyleRequest{
			ObjectId:  objectID,
			TextRange: textRange,
			Style:     style,
			Fields:    "link",
		},
	}}}
	return runSlidesTextBatch(ctx, flags, "slides.link", presentationID, body, map[string]any{
		"presentation_id": presentationID,
		"object_id":       objectID,
		"range":           c.Range,
		"url":             url,
		"clear":           c.Clear,
		"batch_update":    body,
	})
}

type SlidesBulletsCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	ObjectID       string `arg:"" name:"objectId" help:"Page element object ID containing the text"`
	Range          string `name:"range" required:"" help:"UTF-16 paragraph range as start:end"`
	On             bool   `name:"on" help:"Turn bullets on for the selected paragraphs"`
	Off            bool   `name:"off" help:"Turn bullets off for the selected paragraphs"`
	Preset         string `name:"preset" help:"Slides bullet preset when using --on" default:"BULLET_DISC_CIRCLE_SQUARE"`
}

func (c *SlidesBulletsCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, objectID, textRange, err := resolveSlidesTextTarget(c.PresentationID, c.ObjectID, c.Range)
	if err != nil {
		return err
	}
	if c.On == c.Off {
		return usage("exactly one of --on or --off is required")
	}

	var requests []*slides.Request
	if c.On {
		preset := strings.TrimSpace(c.Preset)
		if preset == "" {
			return usage("empty bullet preset")
		}
		requests = append(requests, &slides.Request{
			CreateParagraphBullets: &slides.CreateParagraphBulletsRequest{
				ObjectId:     objectID,
				TextRange:    textRange,
				BulletPreset: preset,
			},
		})
	} else {
		requests = append(requests, &slides.Request{
			DeleteParagraphBullets: &slides.DeleteParagraphBulletsRequest{
				ObjectId:  objectID,
				TextRange: textRange,
			},
		})
	}

	body := &slides.BatchUpdatePresentationRequest{Requests: requests}
	return runSlidesTextBatch(ctx, flags, "slides.bullets", presentationID, body, map[string]any{
		"presentation_id": presentationID,
		"object_id":       objectID,
		"range":           c.Range,
		"on":              c.On,
		"off":             c.Off,
		"preset":          strings.TrimSpace(c.Preset),
		"batch_update":    body,
	})
}

type slidesStyleTextOptions struct {
	Bold        bool
	NoBold      bool
	Italic      bool
	NoItalic    bool
	Underline   bool
	NoUnderline bool
	Color       string
	Size        float64
	Font        string
}

func buildSlidesStyleTextRequest(objectID string, textRange *slides.Range, opts slidesStyleTextOptions) (*slides.UpdateTextStyleRequest, string, error) {
	style := &slides.TextStyle{}
	var fields []string
	var force []string

	if opts.Bold && opts.NoBold {
		return nil, "", usage("--bold and --no-bold are mutually exclusive")
	}
	if opts.Bold {
		style.Bold = true
		fields = append(fields, "bold")
	}
	if opts.NoBold {
		fields = append(fields, "bold")
		force = append(force, "Bold")
	}

	if opts.Italic && opts.NoItalic {
		return nil, "", usage("--italic and --no-italic are mutually exclusive")
	}
	if opts.Italic {
		style.Italic = true
		fields = append(fields, "italic")
	}
	if opts.NoItalic {
		fields = append(fields, "italic")
		force = append(force, "Italic")
	}

	if opts.Underline && opts.NoUnderline {
		return nil, "", usage("--underline and --no-underline are mutually exclusive")
	}
	if opts.Underline {
		style.Underline = true
		fields = append(fields, "underline")
	}
	if opts.NoUnderline {
		fields = append(fields, "underline")
		force = append(force, "Underline")
	}

	if opts.Color != "" {
		r, g, b, ok := parseHexColor(opts.Color)
		if !ok {
			return nil, "", usage("--text-color must be a #RRGGBB or #RGB hex color")
		}
		style.ForegroundColor = slidesOptionalColor(r, g, b)
		fields = append(fields, "foregroundColor")
	}
	if opts.Size < 0 {
		return nil, "", usage("--size must be > 0")
	}
	if opts.Size > 0 {
		style.FontSize = &slides.Dimension{Magnitude: opts.Size, Unit: "PT"}
		fields = append(fields, "fontSize")
	}
	if font := strings.TrimSpace(opts.Font); font != "" {
		style.FontFamily = font
		fields = append(fields, "fontFamily")
	}
	if len(fields) == 0 {
		return nil, "", usage("no text style options provided")
	}

	if len(force) > 0 {
		style.ForceSendFields = force
	}
	fieldMask := strings.Join(fields, ",")
	return &slides.UpdateTextStyleRequest{
		ObjectId:  objectID,
		TextRange: textRange,
		Style:     style,
		Fields:    fieldMask,
	}, fieldMask, nil
}

func resolveSlidesTextTarget(presentationID, objectID, rangeArg string) (string, string, *slides.Range, error) {
	presentationID = strings.TrimSpace(presentationID)
	if presentationID == "" {
		return "", "", nil, usage("empty presentationId")
	}
	objectID = strings.TrimSpace(objectID)
	if objectID == "" {
		return "", "", nil, usage("empty objectId")
	}
	textRange, err := parseSlidesTextRange(rangeArg)
	if err != nil {
		return "", "", nil, err
	}
	return presentationID, objectID, textRange, nil
}

func parseSlidesTextRange(value string) (*slides.Range, error) {
	value = strings.TrimSpace(value)
	parts := strings.Split(value, ":")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return nil, usage("--range must use start:end UTF-16 indexes")
	}
	start, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return nil, usage("--range start must be an integer")
	}
	end, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return nil, usage("--range end must be an integer")
	}
	return slidesFixedTextRange(start, end)
}

func slidesFixedTextRange(start, end int64) (*slides.Range, error) {
	if start < 0 {
		return nil, usage("--range start must be >= 0")
	}
	if end <= start {
		return nil, usage("--range end must be greater than start")
	}
	return &slides.Range{
		Type:       "FIXED_RANGE",
		StartIndex: int64Ptr(start),
		EndIndex:   int64Ptr(end),
	}, nil
}

func slidesOptionalColor(r, g, b float64) *slides.OptionalColor {
	return &slides.OptionalColor{
		OpaqueColor: &slides.OpaqueColor{
			RgbColor: &slides.RgbColor{Red: r, Green: g, Blue: b},
		},
	}
}

func runSlidesTextBatch(ctx context.Context, flags *RootFlags, op string, presentationID string, body *slides.BatchUpdatePresentationRequest, dryRun map[string]any) error {
	if err := dryRunExit(ctx, flags, op, dryRun); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	slidesSvc, err := slidesService(ctx, account)
	if err != nil {
		return err
	}
	resp, err := slidesSvc.Presentations.BatchUpdate(presentationID, body).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), resp)
	}

	revisionID := ""
	if resp != nil && resp.WriteControl != nil {
		revisionID = resp.WriteControl.RequiredRevisionId
	}
	replies := 0
	if resp != nil {
		replies = len(resp.Replies)
	}
	ui.FromContext(ctx).Out().Linef("ok | revisionId=%s | replies=%d", revisionID, replies)
	return nil
}
