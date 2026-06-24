package cmd

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"google.golang.org/api/docs/v1"
)

type DocsLayoutFlags struct {
	PageSize     string `name:"page-size" help:"Named page size: A4, A5, Letter, Legal, Tabloid"`
	PageWidth    string `name:"page-width" help:"Set page width (points by default; supports pt, in, cm, mm)"`
	PageHeight   string `name:"page-height" help:"Set page height (points by default; supports pt, in, cm, mm)"`
	MarginLeft   string `name:"margin-left" help:"Set left page margin (points by default; supports pt, in, cm, mm)"`
	MarginRight  string `name:"margin-right" help:"Set right page margin (points by default; supports pt, in, cm, mm)"`
	MarginTop    string `name:"margin-top" help:"Set top page margin (points by default; supports pt, in, cm, mm)"`
	MarginBottom string `name:"margin-bottom" help:"Set bottom page margin (points by default; supports pt, in, cm, mm)"`
}

func (f DocsLayoutFlags) any() bool {
	return strings.TrimSpace(f.PageSize) != "" ||
		strings.TrimSpace(f.PageWidth) != "" ||
		strings.TrimSpace(f.PageHeight) != "" ||
		strings.TrimSpace(f.MarginLeft) != "" ||
		strings.TrimSpace(f.MarginRight) != "" ||
		strings.TrimSpace(f.MarginTop) != "" ||
		strings.TrimSpace(f.MarginBottom) != ""
}

func (f DocsLayoutFlags) dryRunPayload() map[string]any {
	payload := map[string]any{}
	if strings.TrimSpace(f.PageSize) != "" {
		payload["pageSize"] = f.PageSize
	}
	if strings.TrimSpace(f.PageWidth) != "" {
		payload["pageWidth"] = f.PageWidth
	}
	if strings.TrimSpace(f.PageHeight) != "" {
		payload["pageHeight"] = f.PageHeight
	}
	if strings.TrimSpace(f.MarginLeft) != "" {
		payload["marginLeft"] = f.MarginLeft
	}
	if strings.TrimSpace(f.MarginRight) != "" {
		payload["marginRight"] = f.MarginRight
	}
	if strings.TrimSpace(f.MarginTop) != "" {
		payload["marginTop"] = f.MarginTop
	}
	if strings.TrimSpace(f.MarginBottom) != "" {
		payload["marginBottom"] = f.MarginBottom
	}
	return payload
}

type docsDocumentStyleOptions struct {
	Mode string
	// TabID, when set, targets a specific tab. Document-style fields (e.g.
	// documentMode/pageless) are per-tab; an empty TabID applies to the
	// document's default tab.
	TabID string
	DocsLayoutFlags
}

func setDocumentPageless(ctx context.Context, svc *docs.Service, docID string) error {
	return setDocumentMode(ctx, svc, docID, docsDocumentModePageless)
}

func setDocumentMode(ctx context.Context, svc *docs.Service, docID, mode string) error {
	return setDocumentStyle(ctx, svc, docID, docsDocumentStyleOptions{Mode: mode})
}

func setDocumentStyle(ctx context.Context, svc *docs.Service, docID string, opts docsDocumentStyleOptions) error {
	req, err := buildUpdateDocumentStyleRequest(opts)
	if err != nil {
		return err
	}
	_, err = svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{UpdateDocumentStyle: req}},
	}).Context(ctx).Do()
	return err
}

func buildUpdateDocumentStyleRequest(opts docsDocumentStyleOptions) (*docs.UpdateDocumentStyleRequest, error) {
	style := &docs.DocumentStyle{}
	fields := []string{}

	if strings.TrimSpace(opts.Mode) != "" {
		style.DocumentFormat = &docs.DocumentFormat{DocumentMode: opts.Mode}
		fields = append(fields, "documentFormat")
	}

	// The Docs schema says pageSize/margins are not rendered in PAGELESS mode,
	// but live Docs readback shows pageless table/content width still follows
	// documentStyle.pageSize minus margins. Keep these flags explicit, not
	// automatic, so --pageless never widens existing docs unless requested.
	pageWidthRaw, pageHeightRaw, err := resolveDocsPageSize(opts.PageSize, opts.PageWidth, opts.PageHeight)
	if err != nil {
		return nil, err
	}

	pageWidth, ok, err := parseDocsDimension("page-width", pageWidthRaw, false)
	if err != nil {
		return nil, err
	}
	if ok {
		if style.PageSize == nil {
			style.PageSize = &docs.Size{}
		}
		style.PageSize.Width = pageWidth
		fields = append(fields, "pageSize.width")
	}

	pageHeight, ok, err := parseDocsDimension("page-height", pageHeightRaw, false)
	if err != nil {
		return nil, err
	}
	if ok {
		if style.PageSize == nil {
			style.PageSize = &docs.Size{}
		}
		style.PageSize.Height = pageHeight
		fields = append(fields, "pageSize.height")
	}

	if err := setMarginDimension(&fields, "margin-left", "marginLeft", opts.MarginLeft, &style.MarginLeft); err != nil {
		return nil, err
	}
	if err := setMarginDimension(&fields, "margin-right", "marginRight", opts.MarginRight, &style.MarginRight); err != nil {
		return nil, err
	}
	if err := setMarginDimension(&fields, "margin-top", "marginTop", opts.MarginTop, &style.MarginTop); err != nil {
		return nil, err
	}
	if err := setMarginDimension(&fields, "margin-bottom", "marginBottom", opts.MarginBottom, &style.MarginBottom); err != nil {
		return nil, err
	}

	if len(fields) == 0 {
		return nil, usage("no document style changes requested")
	}
	return &docs.UpdateDocumentStyleRequest{
		DocumentStyle: style,
		Fields:        strings.Join(fields, ","),
		TabId:         strings.TrimSpace(opts.TabID),
	}, nil
}

type docsPageSizePreset struct {
	widthPt  float64
	heightPt float64
}

var docsPageSizePresets = map[string]docsPageSizePreset{
	"a4":      {widthPt: 595.275, heightPt: 841.890},
	"a5":      {widthPt: 419.528, heightPt: 595.275},
	"letter":  {widthPt: 612, heightPt: 792},
	"legal":   {widthPt: 612, heightPt: 1008},
	"tabloid": {widthPt: 792, heightPt: 1224},
}

func resolveDocsPageSize(pageSize, pageWidth, pageHeight string) (string, string, error) {
	pageSize = strings.TrimSpace(pageSize)
	if pageSize == "" {
		return pageWidth, pageHeight, nil
	}
	if strings.TrimSpace(pageWidth) != "" || strings.TrimSpace(pageHeight) != "" {
		return "", "", usage("--page-size cannot be combined with --page-width or --page-height")
	}
	preset, ok := docsPageSizePresets[strings.ToLower(pageSize)]
	if !ok {
		return "", "", usage("--page-size must be one of A4, A5, Letter, Legal, Tabloid")
	}
	return fmt.Sprintf("%.3fpt", preset.widthPt), fmt.Sprintf("%.3fpt", preset.heightPt), nil
}

func setMarginDimension(fields *[]string, flagName, fieldName, raw string, target **docs.Dimension) error {
	dim, ok, err := parseDocsDimension(flagName, raw, true)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	*target = dim
	*fields = append(*fields, fieldName)
	return nil
}

func parseDocsDimension(flagName, raw string, allowZero bool) (*docs.Dimension, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false, nil
	}

	unit := "pt"
	number := raw
	for _, suffix := range []string{"pt", "in", "cm", "mm"} {
		if strings.HasSuffix(strings.ToLower(raw), suffix) {
			unit = suffix
			number = strings.TrimSpace(raw[:len(raw)-len(suffix)])
			break
		}
	}
	if number == "" {
		return nil, false, usage(fmt.Sprintf("invalid --%s %q", flagName, raw))
	}
	value, err := strconv.ParseFloat(number, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || (!allowZero && value == 0) {
		expected := "positive length"
		if allowZero {
			expected = "non-negative length"
		}
		return nil, false, usage(fmt.Sprintf("invalid --%s %q (expected %s, e.g. 72, 1in, 2.54cm)", flagName, raw, expected))
	}

	switch unit {
	case "pt":
	case "in":
		value *= 72
	case "cm":
		value *= 72 / 2.54
	case "mm":
		value *= 72 / 25.4
	}
	dim := &docs.Dimension{Magnitude: value, Unit: "PT"}
	if value == 0 {
		dim.ForceSendFields = []string{"Magnitude"}
	}
	return dim, true, nil
}
