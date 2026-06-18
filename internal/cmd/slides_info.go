package cmd

import (
	"context"
	"fmt"
	"math"
	"strings"

	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type slidesPresentationInfo struct {
	Title      string             `json:"title,omitempty"`
	Locale     string             `json:"locale,omitempty"`
	RevisionID string             `json:"revisionId,omitempty"`
	SlideCount int                `json:"slideCount"`
	PageSize   *slidesPageSize    `json:"pageSize,omitempty"`
	Masters    []slidesMasterInfo `json:"masters"`
	Layouts    []slidesLayoutInfo `json:"layouts"`
}

type slidesPageSize struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Unit   string  `json:"unit"`
}

type slidesMasterInfo struct {
	ObjectID    string                 `json:"objectId"`
	DisplayName string                 `json:"displayName,omitempty"`
	ThemeColors []slidesThemeColorInfo `json:"themeColors"`
}

type slidesThemeColorInfo struct {
	Type string `json:"type"`
	RGB  string `json:"rgb"`
}

type slidesLayoutInfo struct {
	ObjectID       string `json:"objectId"`
	Name           string `json:"name,omitempty"`
	DisplayName    string `json:"displayName,omitempty"`
	MasterObjectID string `json:"masterObjectId,omitempty"`
}

func (c *SlidesInfoCmd) Run(ctx context.Context, flags *RootFlags) error {
	file, err := loadInfoViaDrive(ctx, flags, infoViaDriveOptions{
		ArgName:      "presentationId",
		ExpectedMime: "application/vnd.google-apps.presentation",
		KindLabel:    "Google Slides presentation",
	}, c.PresentationID)
	if err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := slidesService(ctx, account)
	if err != nil {
		return err
	}
	presentation, err := svc.Presentations.Get(file.Id).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get presentation: %w", err)
	}
	info, err := buildSlidesPresentationInfo(presentation)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			strFile:        file,
			"presentation": info,
		})
	}
	if err := writeInfoViaDrive(ctx, file); err != nil {
		return err
	}
	u := ui.FromContext(ctx)
	if info.Title != "" && info.Title != file.Name {
		u.Out().Linef("slidesTitle\t%s", info.Title)
	}
	if info.Locale != "" {
		u.Out().Linef("locale\t%s", info.Locale)
	}
	u.Out().Linef("slides\t%d", info.SlideCount)
	if info.PageSize != nil {
		u.Out().Linef("pageSize\t%.2f x %.2f %s", info.PageSize.Width, info.PageSize.Height, info.PageSize.Unit)
	}
	u.Out().Linef("masters\t%d", len(info.Masters))
	u.Out().Linef("layouts\t%d", len(info.Layouts))
	for _, master := range info.Masters {
		colors := make([]string, 0, len(master.ThemeColors))
		for _, color := range master.ThemeColors {
			colors = append(colors, color.Type+"="+color.RGB)
		}
		u.Out().Linef("master\t%s\t%s\t%s", master.ObjectID, master.DisplayName, strings.Join(colors, ","))
	}
	return nil
}

func buildSlidesPresentationInfo(presentation *slides.Presentation) (slidesPresentationInfo, error) {
	info := slidesPresentationInfo{
		Masters: []slidesMasterInfo{},
		Layouts: []slidesLayoutInfo{},
	}
	if presentation == nil {
		return info, fmt.Errorf("presentation not found")
	}
	info.Title = presentation.Title
	info.Locale = presentation.Locale
	info.RevisionID = presentation.RevisionId
	info.SlideCount = len(presentation.Slides)
	if bounds, ok, err := slidesPageBounds(presentation.PageSize); err != nil {
		return info, err
	} else if ok {
		info.PageSize = &slidesPageSize{Width: bounds.Width, Height: bounds.Height, Unit: bounds.Unit}
	}

	for _, master := range presentation.Masters {
		if master == nil {
			continue
		}
		masterInfo := slidesMasterInfo{ObjectID: master.ObjectId, ThemeColors: []slidesThemeColorInfo{}}
		if master.MasterProperties != nil {
			masterInfo.DisplayName = master.MasterProperties.DisplayName
		}
		if master.PageProperties != nil && master.PageProperties.ColorScheme != nil {
			for _, pair := range master.PageProperties.ColorScheme.Colors {
				if pair == nil || pair.Color == nil {
					continue
				}
				masterInfo.ThemeColors = append(masterInfo.ThemeColors, slidesThemeColorInfo{
					Type: pair.Type,
					RGB:  slidesRGBHex(pair.Color),
				})
			}
		}
		info.Masters = append(info.Masters, masterInfo)
	}
	for _, layout := range presentation.Layouts {
		if layout == nil {
			continue
		}
		layoutInfo := slidesLayoutInfo{ObjectID: layout.ObjectId}
		if layout.LayoutProperties != nil {
			layoutInfo.Name = layout.LayoutProperties.Name
			layoutInfo.DisplayName = layout.LayoutProperties.DisplayName
			layoutInfo.MasterObjectID = layout.LayoutProperties.MasterObjectId
		}
		info.Layouts = append(info.Layouts, layoutInfo)
	}
	return info, nil
}

func slidesRGBHex(color *slides.RgbColor) string {
	if color == nil {
		return ""
	}
	component := func(value float64) int {
		return int(math.Round(math.Max(0, math.Min(1, value)) * 255))
	}
	return fmt.Sprintf("#%02X%02X%02X", component(color.Red), component(color.Green), component(color.Blue))
}
