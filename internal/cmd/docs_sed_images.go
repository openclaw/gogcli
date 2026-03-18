package cmd

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/ui"
)

func (c *DocsSedCmd) runImageReplace(ctx context.Context, u *ui.UI, account, docID string, ref *ImageRefPattern, replacement string, global bool) error {
	docsSvc, err := newDocsService(ctx, account)
	if err != nil {
		return fmt.Errorf("create docs service: %w", err)
	}

	// Get document to find images
	var doc *docs.Document
	err = retryOnQuota(ctx, func() error {
		var e error
		doc, e = docsSvc.Documents.Get(docID).Context(ctx).Do()
		return e
	})
	if err != nil {
		return fmt.Errorf("get document: %w", err)
	}

	// Find all images in document
	allImages := findDocImages(doc)
	if len(allImages) == 0 {
		return sedOutputOK(ctx, u, docID, sedOutputKV{"replaced", 0}, sedOutputKV{"message", "no images found in document"})
	}

	// Match images against pattern
	matched := matchImages(allImages, ref)
	if len(matched) == 0 {
		return sedOutputOK(ctx, u, docID, sedOutputKV{"replaced", 0}, sedOutputKV{"message", "no images matched pattern"})
	}

	// If not global, only process first match
	if !global && len(matched) > 1 {
		matched = matched[:1]
	}

	// Parse replacement - could be new image, text, or empty (delete)
	var requests []*docs.Request
	isDelete := replacement == ""
	newImage := parseImageSyntax(replacement)
	if newImage == nil && strings.HasPrefix(replacement, "!(") && strings.HasSuffix(replacement, ")") {
		// Check for !(url) shorthand
		inner := replacement[2 : len(replacement)-1]
		if strings.HasPrefix(inner, "http://") || strings.HasPrefix(inner, "https://") {
			newImage = &ImageSpec{URL: inner}
		}
	}

	// Build requests for each matched image
	for _, img := range matched {
		switch {
		case isDelete:
			// Delete the image
			if img.IsPositioned {
				requests = append(requests, &docs.Request{
					DeletePositionedObject: &docs.DeletePositionedObjectRequest{
						ObjectId: img.ObjectID,
					},
				})
			} else {
				// For inline objects, delete the content range
				requests = append(requests, &docs.Request{
					DeleteContentRange: &docs.DeleteContentRangeRequest{
						Range: &docs.Range{
							StartIndex: img.Index,
							EndIndex:   img.Index + 1,
						},
					},
				})
			}
		case newImage != nil:
			// Replace with new image
			if !img.IsPositioned {
				// Use ReplaceImage for inline images
				replaceReq := &docs.ReplaceImageRequest{
					ImageObjectId: img.ObjectID,
					Uri:           newImage.URL,
				}
				requests = append(requests, &docs.Request{
					ReplaceImage: replaceReq,
				})
			} else {
				// For positioned objects, delete and insert new
				requests = append(requests, &docs.Request{
					DeletePositionedObject: &docs.DeletePositionedObjectRequest{
						ObjectId: img.ObjectID,
					},
				})
				// Note: Can't easily insert positioned object, so this is a limitation
			}
		default:
			// Replace with text - delete image, insert text
			if img.IsPositioned {
				requests = append(requests, &docs.Request{
					DeletePositionedObject: &docs.DeletePositionedObjectRequest{
						ObjectId: img.ObjectID,
					},
				})
			} else {
				requests = append(requests, &docs.Request{
					DeleteContentRange: &docs.DeleteContentRangeRequest{
						Range: &docs.Range{
							StartIndex: img.Index,
							EndIndex:   img.Index + 1,
						},
					},
				})
				if replacement != "" {
					requests = append(requests, &docs.Request{
						InsertText: &docs.InsertTextRequest{
							Location: &docs.Location{Index: img.Index},
							Text:     replacement,
						},
					})
				}
			}
		}
	}

	if len(requests) == 0 {
		return sedOutputOK(ctx, u, docID, sedOutputKV{"replaced", 0})
	}

	// Execute batch update
	err = retryOnQuota(ctx, func() error {
		_, e := docsSvc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
			Requests: requests,
		}).Context(ctx).Do()
		return e
	})
	if err != nil {
		return fmt.Errorf("update document: %w", err)
	}

	return sedOutputOK(ctx, u, docID, sedOutputKV{"replaced", len(matched)})
}

// ImageSpec holds the URL and optional dimensions for an inline image insertion.
type ImageSpec struct {
	URL     string
	Alt     string
	Caption string // from title in ![alt](url "title")
	Width   int    // in pixels, 0 if not specified
	Height  int    // in pixels, 0 if not specified
}

// ImageRefPattern holds a parsed image reference pattern (for finding existing images)
type ImageRefPattern struct {
	ByPosition bool           // true if matching by position (!(n))
	Position   int            // 1-based position, negative for from-end, 0 for all (*)
	AllImages  bool           // true if matching all images (!(*)  )
	ByAlt      bool           // true if matching by alt text regex (![regex])
	AltRegex   *regexp.Regexp // compiled regex for alt text matching
}

// DocImage represents an image found in the document
type DocImage struct {
	ObjectID     string // inline object ID or positioned object ID
	Index        int64  // position in document
	Alt          string // alt text if available
	IsPositioned bool   // true if floating/positioned, false if inline
}

// parseImageRefPattern parses image reference patterns for finding existing images
// Patterns: !(1), !(-1), !(*), ![regex], ![](1), ![](-1), ![](*)
func parseImageRefPattern(pattern string) *ImageRefPattern {
	// !(n) or !(*) - positional reference
	if strings.HasPrefix(pattern, "!(") && strings.HasSuffix(pattern, ")") {
		inner := pattern[2 : len(pattern)-1]
		if inner == "*" {
			return &ImageRefPattern{ByPosition: true, AllImages: true}
		}
		if n, err := strconv.Atoi(inner); err == nil {
			return &ImageRefPattern{ByPosition: true, Position: n}
		}
		// Could be a URL, not a reference
		if strings.HasPrefix(inner, "http://") || strings.HasPrefix(inner, "https://") {
			return nil
		}
		return nil
	}

	// ![](n) or ![](*) - positional reference with empty alt
	if strings.HasPrefix(pattern, "![](") && strings.HasSuffix(pattern, ")") {
		inner := pattern[4 : len(pattern)-1]
		if inner == "*" {
			return &ImageRefPattern{ByPosition: true, AllImages: true}
		}
		if n, err := strconv.Atoi(inner); err == nil {
			return &ImageRefPattern{ByPosition: true, Position: n}
		}
		return nil
	}

	// ![regex] - alt text regex match (no URL part)
	if strings.HasPrefix(pattern, "![") && strings.HasSuffix(pattern, "]") && !strings.Contains(pattern, "](") {
		regexStr := pattern[2 : len(pattern)-1]
		if regexStr == "" {
			return nil
		}
		// Compile as regex, anchor if it looks like exact match
		re, err := regexp.Compile(regexStr)
		if err != nil {
			return nil
		}
		return &ImageRefPattern{ByAlt: true, AltRegex: re}
	}

	return nil
}

// findDocImages walks a document and returns all images with their metadata
func findDocImages(doc *docs.Document) []DocImage {
	var images []DocImage

	if doc.InlineObjects != nil {
		// Build a map of inline object IDs to their properties
		inlineProps := make(map[string]*docs.InlineObjectProperties)
		for id, obj := range doc.InlineObjects {
			if obj.InlineObjectProperties != nil {
				inlineProps[id] = obj.InlineObjectProperties
			}
		}

		// Walk document to find inline object elements and their positions
		var walkContent func(content []*docs.StructuralElement)
		walkContent = func(content []*docs.StructuralElement) {
			for _, elem := range content {
				if elem.Paragraph != nil {
					for _, pe := range elem.Paragraph.Elements {
						if pe.InlineObjectElement != nil {
							objID := pe.InlineObjectElement.InlineObjectId
							alt := ""
							if props, ok := inlineProps[objID]; ok && props.EmbeddedObject != nil {
								alt = props.EmbeddedObject.Title // or Description
								if alt == "" {
									alt = props.EmbeddedObject.Description
								}
							}
							images = append(images, DocImage{
								ObjectID:     objID,
								Index:        pe.StartIndex,
								Alt:          alt,
								IsPositioned: false,
							})
						}
					}
				}
				if elem.Table != nil {
					for _, row := range elem.Table.TableRows {
						for _, cell := range row.TableCells {
							walkContent(cell.Content)
						}
					}
				}
			}
		}

		if doc.Body != nil {
			walkContent(doc.Body.Content)
		}
	}

	// Also check positioned objects
	if doc.PositionedObjects != nil {
		for id, obj := range doc.PositionedObjects {
			alt := ""
			if obj.PositionedObjectProperties != nil && obj.PositionedObjectProperties.EmbeddedObject != nil {
				alt = obj.PositionedObjectProperties.EmbeddedObject.Title
				if alt == "" {
					alt = obj.PositionedObjectProperties.EmbeddedObject.Description
				}
			}
			images = append(images, DocImage{
				ObjectID:     id,
				Index:        0, // positioned objects don't have a fixed index
				Alt:          alt,
				IsPositioned: true,
			})
		}
	}

	return images
}

// matchImages returns images that match the reference pattern
func matchImages(images []DocImage, ref *ImageRefPattern) []DocImage {
	if ref.AllImages {
		return images
	}

	if ref.ByPosition {
		pos := ref.Position
		if pos > 0 && pos <= len(images) {
			idx := pos - 1
			return []DocImage{images[idx]} //nolint:gosec // idx is range-checked above
		}
		if pos < 0 && -pos <= len(images) {
			idx := len(images) + pos
			return []DocImage{images[idx]}
		}
		return nil
	}

	if ref.ByAlt && ref.AltRegex != nil {
		var matched []DocImage
		for _, img := range images {
			if ref.AltRegex.MatchString(img.Alt) {
				matched = append(matched, img)
			}
		}
		return matched
	}

	return nil
}

// parseImageSyntax parses markdown image syntax: ![alt](url "title"){width=X height=Y}
// Returns nil if the text is not an image
func parseImageSyntax(text string) *ImageSpec {
	// Must start with ![
	if !strings.HasPrefix(text, "![") {
		return nil
	}

	// Find the closing ] for alt text
	altEnd := strings.Index(text, "](")
	if altEnd == -1 {
		return nil
	}
	alt := text[2:altEnd]

	// Find the URL - starts after ]( and ends at ) or " or {
	rest := text[altEnd+2:]

	// Find where URL ends
	urlEnd := -1
	for i, c := range rest {
		if c == '"' || c == ')' || c == '{' {
			urlEnd = i
			break
		}
	}
	if urlEnd == -1 {
		// URL goes to end, look for closing )
		if strings.HasSuffix(rest, ")") {
			urlEnd = len(rest) - 1
		} else {
			return nil
		}
	}

	url := strings.TrimSpace(rest[:urlEnd])
	rest = rest[urlEnd:]

	spec := &ImageSpec{
		URL: url,
		Alt: alt,
	}

	// Parse optional title in quotes: "title")
	if strings.HasPrefix(rest, " \"") || strings.HasPrefix(rest, "\"") {
		rest = strings.TrimPrefix(rest, " ")
		if strings.HasPrefix(rest, "\"") {
			titleEnd := strings.Index(rest[1:], "\"")
			if titleEnd != -1 {
				spec.Caption = rest[1 : titleEnd+1]
				rest = rest[titleEnd+2:]
			}
		}
	}

	// Skip closing paren if present
	rest = strings.TrimPrefix(rest, ")")

	// Parse optional Pandoc-style attributes: {width=X height=Y}
	if strings.HasPrefix(rest, "{") {
		attrEnd := strings.Index(rest, "}")
		if attrEnd != -1 {
			spec.Width, spec.Height = parseImageDimAttrs(rest[1:attrEnd])
		}
	}

	return spec
}
