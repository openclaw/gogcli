package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/docs/v1"
	gapi "google.golang.org/api/googleapi"
)

// markdownImage holds a parsed image reference from a markdown file.
type markdownImage struct {
	index       int     // sequential index (0, 1, 2, ...)
	alt         string  // alt text
	originalRef string  // original path or URL
	token       string  // unique token per extraction to avoid collisions
	widthPt     float64 // optional width in points (0 = use default)
	heightPt    float64 // optional height in points (0 = use default)
}

// placeholder returns the placeholder string for this image.
// Uses a unique token so it cannot collide with user content.
func (m markdownImage) placeholder() string {
	return fmt.Sprintf("<<IMG_%s_%d>>", m.token, m.index)
}

// isRemote returns true if the image reference is a remote URL.
func (m markdownImage) isRemote() bool {
	return strings.HasPrefix(m.originalRef, "http://") || strings.HasPrefix(m.originalRef, "https://")
}

var mdImageRe = regexp.MustCompile(`!\[([^\]]*)\]\((?:<([^>]+)>|([^)\s]+))(?:\s+(?:"[^"]*"|'[^']*'|\([^)]*\)))?\)(?:\{([^}]*)\})?`)

// parseImageDimAttrs parses Pandoc-style image dimension attributes from the
// content inside {…} (e.g. "width=200 height=150" or "w=200 h=150").
// Values are returned as integers; unspecified dimensions are 0.
func parseImageDimAttrs(attrs string) (width, height int) {
	for _, part := range strings.Fields(attrs) {
		switch {
		case strings.HasPrefix(part, "width="):
			val := strings.TrimPrefix(part, "width=")
			val = strings.TrimSuffix(val, "px")
			val = strings.TrimSuffix(val, "%")
			if n, err := strconv.Atoi(val); err == nil {
				width = n
			}
		case strings.HasPrefix(part, "height="):
			val := strings.TrimPrefix(part, "height=")
			val = strings.TrimSuffix(val, "px")
			val = strings.TrimSuffix(val, "%")
			if n, err := strconv.Atoi(val); err == nil {
				height = n
			}
		case strings.HasPrefix(part, "w="):
			val := strings.TrimPrefix(part, "w=")
			if n, err := strconv.Atoi(val); err == nil {
				width = n
			}
		case strings.HasPrefix(part, "h="):
			val := strings.TrimPrefix(part, "h=")
			if n, err := strconv.Atoi(val); err == nil {
				height = n
			}
		}
	}
	return width, height
}

// imgPlaceholderToken generates a random hex token for image placeholders.
var imgPlaceholderToken = func() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback — still very unlikely to collide with user text.
		return "x0x0"
	}
	return hex.EncodeToString(b)
}

// extractMarkdownImages finds all ![alt](url) references in content,
// replaces them with unique <<IMG_token_N>> placeholders, and returns the
// cleaned content along with the extracted images.
func extractMarkdownImages(content string) (string, []markdownImage) {
	token := imgPlaceholderToken()
	var images []markdownImage
	idx := 0
	cleaned := mdImageRe.ReplaceAllStringFunc(content, func(match string) string {
		subs := mdImageRe.FindStringSubmatch(match)
		if len(subs) < 4 {
			return match
		}
		ref := subs[2]
		if ref == "" {
			ref = subs[3]
		}
		img := markdownImage{
			index:       idx,
			alt:         subs[1],
			originalRef: ref,
			token:       token,
		}
		if len(subs) > 4 && subs[4] != "" {
			w, h := parseImageDimAttrs(subs[4])
			img.widthPt = float64(w)
			img.heightPt = float64(h)
		}
		images = append(images, img)
		placeholder := img.placeholder()
		idx++
		return placeholder
	})
	return cleaned, images
}

// docRange represents a start/end character index range in a Google Doc.
type docRange struct {
	startIndex int64
	endIndex   int64
}

// findPlaceholderIndices walks a Google Doc body to locate image placeholders
// and returns a map from placeholder string to its position.
//
// The search recurses into tables (where Drive's markdown converter places
// images from markdown table cells) and concatenates text runs within each
// paragraph to handle placeholders split across formatting boundaries.
func findPlaceholderIndices(doc *docs.Document, images []markdownImage) map[string]docRange {
	result := make(map[string]docRange)
	if doc == nil || doc.Body == nil || len(images) == 0 {
		return result
	}

	placeholders := make([]string, len(images))
	for i, img := range images {
		placeholders[i] = img.placeholder()
	}

	searchElements(doc.Body.Content, placeholders, result)
	return result
}

// searchElements walks structural elements (paragraphs, tables) looking for
// placeholder strings. Results are written into the result map.
func searchElements(elements []*docs.StructuralElement, placeholders []string, result map[string]docRange) {
	for _, el := range elements {
		switch {
		case el.Paragraph != nil:
			searchParagraph(el.Paragraph, placeholders, result)
		case el.Table != nil:
			for _, row := range el.Table.TableRows {
				for _, cell := range row.TableCells {
					searchElements(cell.Content, placeholders, result)
				}
			}
		}
	}
}

// runSpan tracks the byte offset in concatenated paragraph text and the
// corresponding absolute UTF-16 document index from the API.
type runSpan struct {
	byteStart int
	absStart  int64
}

// searchParagraph concatenates all text runs in a paragraph and searches for
// placeholders, mapping byte offsets back to absolute UTF-16 document indices.
func searchParagraph(para *docs.Paragraph, placeholders []string, result map[string]docRange) {
	var paraText strings.Builder
	var spans []runSpan
	for _, pe := range para.Elements {
		if pe.TextRun == nil {
			continue
		}
		spans = append(spans, runSpan{
			byteStart: paraText.Len(),
			absStart:  pe.StartIndex,
		})
		paraText.WriteString(pe.TextRun.Content)
	}
	if paraText.Len() == 0 {
		return
	}

	full := paraText.String()
	for _, ph := range placeholders {
		pos := strings.Index(full, ph)
		if pos == -1 {
			continue
		}
		// Map byte offset back to absolute UTF-16 index.
		var baseAbs int64
		var baseByteOff int
		for i := len(spans) - 1; i >= 0; i-- {
			if spans[i].byteStart <= pos {
				baseAbs = spans[i].absStart
				baseByteOff = spans[i].byteStart
				break
			}
		}
		absStart := baseAbs + utf16Len(full[baseByteOff:pos])
		absEnd := absStart + utf16Len(ph)
		result[ph] = docRange{
			startIndex: absStart,
			endIndex:   absEnd,
		}
	}
}

// insertImagesIntoDocs reads back a Google Doc to find <<IMG_token_N>> placeholders,
// resolves image URLs (remote URLs used directly; local files are rejected),
// and replaces the placeholders with inline images via BatchUpdate.
func insertImagesIntoDocs(ctx context.Context, svc *docs.Service, docID string, images []markdownImage, tabID string) error {
	// Read back the document to find placeholder positions.
	doc, err := svc.Documents.Get(docID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("read back document: %w", err)
	}

	placeholders := findPlaceholderIndices(doc, images)
	if len(placeholders) == 0 {
		return nil
	}

	// Resolve image URLs. The Docs API fetches InsertInlineImage URIs itself, so
	// local files need to be hosted at a normal public HTTPS URL first.
	imageURLs := make(map[int]string)

	for _, img := range images {
		if _, ok := placeholders[img.placeholder()]; !ok {
			continue
		}
		if img.isRemote() {
			imageURLs[img.index] = img.originalRef
			continue
		}
		return fmt.Errorf("local markdown image %q cannot be inserted automatically; Google Docs image insertion requires a public HTTPS image URL, so upload the image to a public host and use that URL", img.originalRef)
	}

	reqs := buildImageInsertRequests(placeholders, images, imageURLs, tabID)
	if len(reqs) == 0 {
		return nil
	}

	return batchUpdateImageInsertRequests(ctx, svc, docID, reqs)
}

var docsImageInsertRetryDelays = []time.Duration{
	2 * time.Second,
	5 * time.Second,
	10 * time.Second,
	20 * time.Second,
}

func batchUpdateImageInsertRequests(ctx context.Context, svc *docs.Service, docID string, reqs []*docs.Request) error {
	var lastErr error
	for attempt := 0; ; attempt++ {
		_, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
			Requests: reqs,
		}).Context(ctx).Do()
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt >= len(docsImageInsertRetryDelays) || !isRetryableDocsImageInsertError(err) {
			return lastErr
		}
		if err := waitDocsImageInsertRetry(ctx, docsImageInsertRetryDelays[attempt]); err != nil {
			return err
		}
	}
}

func waitDocsImageInsertRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isRetryableDocsImageInsertError(err error) bool {
	var apiErr *gapi.Error
	if errors.As(err, &apiErr) {
		if apiErr.Code >= 500 {
			return true
		}
		if apiErr.Code == 400 && strings.Contains(apiErr.Message, "retrieving the image") {
			return true
		}
	}
	errStr := err.Error()
	return strings.Contains(errStr, "retrieving the image") ||
		strings.Contains(errStr, "provided image should be publicly accessible")
}

// defaultImageMaxWidthPt is the maximum width for inserted inline images in points.
// 468pt = US Letter (612pt) minus default 1-inch margins (72pt each side).
// Setting only width lets the API scale height proportionally to maintain aspect ratio.
const defaultImageMaxWidthPt = 468.0

// buildImageInsertRequests creates the Docs API batch update requests to replace
// placeholder text with inline images. Requests are ordered in reverse index order
// so earlier positions are not invalidated as the document is modified.
func buildImageInsertRequests(placeholders map[string]docRange, images []markdownImage, imageURLs map[int]string, tabID string) []*docs.Request {
	// Collect entries sorted by start index descending.
	type entry struct {
		image markdownImage
		dr    docRange
		url   string
	}
	var entries []entry
	for _, img := range images {
		ph := img.placeholder()
		dr, ok := placeholders[ph]
		if !ok {
			continue
		}
		u, ok := imageURLs[img.index]
		if !ok {
			continue
		}
		entries = append(entries, entry{image: img, dr: dr, url: u})
	}

	// Sort by start index descending; process from end of document to start.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].dr.startIndex > entries[j].dr.startIndex
	})

	reqs := make([]*docs.Request, 0, len(entries)*2)
	for _, e := range entries {
		// First delete the placeholder text.
		reqs = append(reqs, &docs.Request{
			DeleteContentRange: &docs.DeleteContentRangeRequest{
				Range: &docs.Range{
					StartIndex: e.dr.startIndex,
					EndIndex:   e.dr.endIndex,
					TabId:      tabID,
				},
			},
		})
		// Then insert the image at that position.
		objSize := &docs.Size{}
		switch {
		case e.image.widthPt > 0 && e.image.heightPt > 0:
			objSize.Width = &docs.Dimension{Magnitude: e.image.widthPt, Unit: "PT"}
			objSize.Height = &docs.Dimension{Magnitude: e.image.heightPt, Unit: "PT"}
		case e.image.widthPt > 0:
			objSize.Width = &docs.Dimension{Magnitude: e.image.widthPt, Unit: "PT"}
		case e.image.heightPt > 0:
			objSize.Height = &docs.Dimension{Magnitude: e.image.heightPt, Unit: "PT"}
		default:
			objSize.Width = &docs.Dimension{Magnitude: defaultImageMaxWidthPt, Unit: "PT"}
		}
		reqs = append(reqs, &docs.Request{
			InsertInlineImage: &docs.InsertInlineImageRequest{
				Uri: e.url,
				Location: &docs.Location{
					Index: e.dr.startIndex,
					TabId: tabID,
				},
				ObjectSize: objSize,
			},
		})
	}
	return reqs
}
