package cmd

import (
	"fmt"
	"math"
	"strings"

	"google.golang.org/api/slides/v1"
)

const (
	slidesEMUPerPoint    = 12700
	slidesElementTable   = "table"
	slidesElementUnknown = "unknown"
)

type slidesElementDetail struct {
	ObjectID       string                 `json:"objectId"`
	Type           string                 `json:"type"`
	ParentObjectID string                 `json:"parentObjectId,omitempty"`
	Title          string                 `json:"title,omitempty"`
	Description    string                 `json:"description,omitempty"`
	Geometry       *slidesElementGeometry `json:"geometry,omitempty"`
	Shape          *slidesShapeDetail     `json:"shape,omitempty"`
	Image          *slidesImageDetail     `json:"image,omitempty"`
	Table          *slidesTableDetail     `json:"table,omitempty"`
	WordArt        string                 `json:"wordArt,omitempty"`
}

type slidesElementGeometry struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Unit   string  `json:"unit"`
}

type slidesShapeDetail struct {
	ShapeType       string            `json:"shapeType,omitempty"`
	PlaceholderType string            `json:"placeholderType,omitempty"`
	Text            *slidesTextDetail `json:"text,omitempty"`
}

type slidesImageDetail struct {
	ContentURL string `json:"contentUrl,omitempty"`
	SourceURL  string `json:"sourceUrl,omitempty"`
}

type slidesTableDetail struct {
	Rows    int64                   `json:"rows"`
	Columns int64                   `json:"columns"`
	Cells   []slidesTableCellDetail `json:"cells"`
}

type slidesTableCellDetail struct {
	RowIndex    int64             `json:"rowIndex"`
	ColumnIndex int64             `json:"columnIndex"`
	RowSpan     int64             `json:"rowSpan"`
	ColumnSpan  int64             `json:"columnSpan"`
	Text        *slidesTextDetail `json:"text,omitempty"`
}

type slidesTextDetail struct {
	Content    string                  `json:"content"`
	Runs       []slidesTextRunDetail   `json:"runs"`
	Paragraphs []slidesParagraphDetail `json:"paragraphs"`
}

type slidesTextRunDetail struct {
	StartIndex      int64             `json:"startIndex"`
	EndIndex        int64             `json:"endIndex"`
	Content         string            `json:"content"`
	Style           *slides.TextStyle `json:"style,omitempty"`
	ForegroundColor string            `json:"foregroundColor,omitempty"`
	BackgroundColor string            `json:"backgroundColor,omitempty"`
	AutoTextType    string            `json:"autoTextType,omitempty"`
}

type slidesParagraphDetail struct {
	StartIndex int64                  `json:"startIndex"`
	EndIndex   int64                  `json:"endIndex"`
	Style      *slides.ParagraphStyle `json:"style,omitempty"`
	Bullet     *slides.Bullet         `json:"bullet,omitempty"`
}

type slidesAffineMatrix struct {
	a float64
	b float64
	c float64
	d float64
	e float64
	f float64
}

func slidesElementDetails(elements []*slides.PageElement) []slidesElementDetail {
	details := make([]slidesElementDetail, 0, len(elements))
	identity := slidesAffineMatrix{a: 1, d: 1}
	for _, element := range elements {
		appendSlidesElementDetails(&details, element, "", identity)
	}
	return details
}

func appendSlidesElementDetails(details *[]slidesElementDetail, element *slides.PageElement, parentID string, parentTransform slidesAffineMatrix) {
	if element == nil {
		return
	}

	localTransform, ok := slidesTransformMatrix(element.Transform)
	if !ok {
		localTransform = slidesAffineMatrix{a: 1, d: 1}
	}
	absoluteTransform := slidesComposeTransforms(parentTransform, localTransform)
	detail := slidesElementDetail{
		ObjectID:       element.ObjectId,
		Type:           slidesElementType(element),
		ParentObjectID: parentID,
		Title:          element.Title,
		Description:    element.Description,
		Geometry:       slidesElementBounds(element.Size, absoluteTransform),
	}

	if element.Shape != nil {
		detail.Shape = &slidesShapeDetail{ShapeType: element.Shape.ShapeType}
		if element.Shape.Placeholder != nil {
			detail.Shape.PlaceholderType = element.Shape.Placeholder.Type
		}
		if element.Shape.Text != nil {
			detail.Shape.Text = slidesTextContentDetail(element.Shape.Text)
		}
	}
	if element.Image != nil {
		detail.Image = &slidesImageDetail{
			ContentURL: element.Image.ContentUrl,
			SourceURL:  element.Image.SourceUrl,
		}
	}
	if element.Table != nil {
		detail.Table = slidesTableContentDetail(element.Table)
	}
	if element.WordArt != nil {
		detail.WordArt = element.WordArt.RenderedText
	}

	*details = append(*details, detail)
	if element.ElementGroup == nil {
		return
	}
	for _, child := range element.ElementGroup.Children {
		appendSlidesElementDetails(details, child, element.ObjectId, absoluteTransform)
	}
}

func slidesElementType(element *slides.PageElement) string {
	switch {
	case element == nil:
		return slidesElementUnknown
	case element.ElementGroup != nil:
		return "group"
	case element.Shape != nil:
		return "shape"
	case element.Image != nil:
		return "image"
	case element.Video != nil:
		return "video"
	case element.Line != nil:
		return "line"
	case element.Table != nil:
		return slidesElementTable
	case element.WordArt != nil:
		return "wordArt"
	case element.SheetsChart != nil:
		return "sheetsChart"
	case element.SpeakerSpotlight != nil:
		return "speakerSpotlight"
	default:
		return slidesElementUnknown
	}
}

func slidesTextContentDetail(content *slides.TextContent) *slidesTextDetail {
	detail := &slidesTextDetail{
		Runs:       []slidesTextRunDetail{},
		Paragraphs: []slidesParagraphDetail{},
	}
	if content == nil {
		return detail
	}

	var text strings.Builder
	for _, element := range content.TextElements {
		if element == nil {
			continue
		}
		if element.TextRun != nil {
			text.WriteString(element.TextRun.Content)
			run := slidesTextRunDetail{
				StartIndex: element.StartIndex,
				EndIndex:   element.EndIndex,
				Content:    element.TextRun.Content,
				Style:      element.TextRun.Style,
			}
			if element.TextRun.Style != nil {
				run.ForegroundColor = slidesOptionalColorValue(element.TextRun.Style.ForegroundColor)
				run.BackgroundColor = slidesOptionalColorValue(element.TextRun.Style.BackgroundColor)
			}
			detail.Runs = append(detail.Runs, run)
		}
		if element.AutoText != nil {
			text.WriteString(element.AutoText.Content)
			detail.Runs = append(detail.Runs, slidesTextRunDetail{
				StartIndex:   element.StartIndex,
				EndIndex:     element.EndIndex,
				Content:      element.AutoText.Content,
				Style:        element.AutoText.Style,
				AutoTextType: element.AutoText.Type,
			})
		}
		if element.ParagraphMarker != nil {
			detail.Paragraphs = append(detail.Paragraphs, slidesParagraphDetail{
				StartIndex: element.StartIndex,
				EndIndex:   element.EndIndex,
				Style:      element.ParagraphMarker.Style,
				Bullet:     element.ParagraphMarker.Bullet,
			})
		}
	}
	detail.Content = strings.TrimRight(text.String(), "\n")
	return detail
}

func slidesOptionalColorValue(color *slides.OptionalColor) string {
	if color == nil || color.OpaqueColor == nil {
		return ""
	}
	if color.OpaqueColor.RgbColor != nil {
		return slidesRGBHex(color.OpaqueColor.RgbColor)
	}
	if color.OpaqueColor.ThemeColor != "" {
		return "theme:" + color.OpaqueColor.ThemeColor
	}
	return ""
}

func slidesTableContentDetail(table *slides.Table) *slidesTableDetail {
	detail := &slidesTableDetail{Cells: []slidesTableCellDetail{}}
	if table == nil {
		return detail
	}
	detail.Rows = table.Rows
	detail.Columns = table.Columns
	for rowIndex, row := range table.TableRows {
		if row == nil {
			continue
		}
		for columnIndex, cell := range row.TableCells {
			if cell == nil {
				continue
			}
			cellRow := int64(rowIndex)
			cellColumn := int64(columnIndex)
			if cell.Location != nil {
				cellRow = cell.Location.RowIndex
				cellColumn = cell.Location.ColumnIndex
			}
			rowSpan := cell.RowSpan
			if rowSpan == 0 {
				rowSpan = 1
			}
			columnSpan := cell.ColumnSpan
			if columnSpan == 0 {
				columnSpan = 1
			}
			detail.Cells = append(detail.Cells, slidesTableCellDetail{
				RowIndex:    cellRow,
				ColumnIndex: cellColumn,
				RowSpan:     rowSpan,
				ColumnSpan:  columnSpan,
				Text:        slidesTextContentDetail(cell.Text),
			})
		}
	}
	return detail
}

func slidesTransformMatrix(transform *slides.AffineTransform) (slidesAffineMatrix, bool) {
	if transform == nil {
		return slidesAffineMatrix{a: 1, d: 1}, true
	}
	tx, ok := slidesMagnitudeInPoints(transform.TranslateX, transform.Unit)
	if !ok && transform.TranslateX != 0 {
		return slidesAffineMatrix{}, false
	}
	ty, ok := slidesMagnitudeInPoints(transform.TranslateY, transform.Unit)
	if !ok && transform.TranslateY != 0 {
		return slidesAffineMatrix{}, false
	}
	return slidesAffineMatrix{
		a: transform.ScaleX,
		b: transform.ShearY,
		c: transform.ShearX,
		d: transform.ScaleY,
		e: tx,
		f: ty,
	}, true
}

func slidesComposeTransforms(parent, child slidesAffineMatrix) slidesAffineMatrix {
	return slidesAffineMatrix{
		a: parent.a*child.a + parent.c*child.b,
		b: parent.b*child.a + parent.d*child.b,
		c: parent.a*child.c + parent.c*child.d,
		d: parent.b*child.c + parent.d*child.d,
		e: parent.a*child.e + parent.c*child.f + parent.e,
		f: parent.b*child.e + parent.d*child.f + parent.f,
	}
}

func slidesElementBounds(size *slides.Size, transform slidesAffineMatrix) *slidesElementGeometry {
	if size == nil || size.Width == nil || size.Height == nil {
		return nil
	}
	width, ok := slidesMagnitudeInPoints(size.Width.Magnitude, size.Width.Unit)
	if !ok {
		return nil
	}
	height, ok := slidesMagnitudeInPoints(size.Height.Magnitude, size.Height.Unit)
	if !ok {
		return nil
	}

	points := [][2]float64{
		slidesTransformPoint(transform, 0, 0),
		slidesTransformPoint(transform, width, 0),
		slidesTransformPoint(transform, 0, height),
		slidesTransformPoint(transform, width, height),
	}
	minX, maxX := points[0][0], points[0][0]
	minY, maxY := points[0][1], points[0][1]
	for _, point := range points[1:] {
		minX = math.Min(minX, point[0])
		maxX = math.Max(maxX, point[0])
		minY = math.Min(minY, point[1])
		maxY = math.Max(maxY, point[1])
	}
	return &slidesElementGeometry{
		X:      minX,
		Y:      minY,
		Width:  maxX - minX,
		Height: maxY - minY,
		Unit:   "PT",
	}
}

func slidesTransformPoint(transform slidesAffineMatrix, x, y float64) [2]float64 {
	return [2]float64{
		transform.a*x + transform.c*y + transform.e,
		transform.b*x + transform.d*y + transform.f,
	}
}

func slidesMagnitudeInPoints(magnitude float64, unit string) (float64, bool) {
	switch strings.ToUpper(strings.TrimSpace(unit)) {
	case "PT":
		return magnitude, true
	case "EMU":
		return magnitude / slidesEMUPerPoint, true
	default:
		return 0, false
	}
}

func slidesPageBounds(size *slides.Size) (slidesElementGeometry, bool, error) {
	if size == nil || size.Width == nil || size.Height == nil {
		return slidesElementGeometry{}, false, nil
	}
	width, ok := slidesMagnitudeInPoints(size.Width.Magnitude, size.Width.Unit)
	if !ok {
		return slidesElementGeometry{}, false, fmt.Errorf("unsupported page width unit %q", size.Width.Unit)
	}
	height, ok := slidesMagnitudeInPoints(size.Height.Magnitude, size.Height.Unit)
	if !ok {
		return slidesElementGeometry{}, false, fmt.Errorf("unsupported page height unit %q", size.Height.Unit)
	}
	return slidesElementGeometry{Width: width, Height: height, Unit: "PT"}, true, nil
}
