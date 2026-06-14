package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/docssed"
	"github.com/steipete/gogcli/internal/ui"
)

func (c *DocsSedCmd) runImageReplace(ctx context.Context, u *ui.UI, account, docID string, ref *ImageRefPattern, replacement string, global bool) error {
	docsSvc, err := docsService(ctx, account)
	if err != nil {
		return fmt.Errorf("create docs service: %w", err)
	}

	// Get document to find images
	doc, err := getDoc(ctx, docsSvc, docID)
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
	newImage := docssed.ParseImageSyntax(replacement)
	if newImage == nil && strings.HasPrefix(replacement, "!(") && strings.HasSuffix(replacement, ")") {
		// Check for !(url) shorthand
		inner := replacement[2 : len(replacement)-1]
		if strings.HasPrefix(inner, "http://") || strings.HasPrefix(inner, "https://") {
			newImage = &docssed.ImageSpec{URL: inner}
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
	_, err = batchUpdate(ctx, docsSvc, docID, requests)
	if err != nil {
		return fmt.Errorf("update document: %w", err)
	}

	return sedOutputOK(ctx, u, docID, sedOutputKV{"replaced", len(matched)})
}

// DocImage is the projection-owned image metadata used by the command executor.
type DocImage = docssed.DocumentImage

// findDocImages returns first-tab image metadata, preserving current sed behavior.
func findDocImages(doc *docs.Document) []DocImage {
	projection := docssed.ProjectDocument(doc)
	if projection.Legacy == nil {
		return nil
	}
	return projection.Legacy.Images
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
