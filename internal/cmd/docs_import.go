package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
)

// markdownImage holds a parsed image reference from a markdown file.
type markdownImage struct {
	index       int    // sequential index (0, 1, 2, ...)
	alt         string // alt text
	originalRef string // original path or URL
}

// placeholder returns the placeholder string for this image.
func (m markdownImage) placeholder() string {
	return fmt.Sprintf("<<IMG_%d>>", m.index)
}

// isRemote returns true if the image reference is a remote URL.
func (m markdownImage) isRemote() bool {
	return strings.HasPrefix(m.originalRef, "http://") || strings.HasPrefix(m.originalRef, "https://")
}

var mdImageRe = regexp.MustCompile(`!\[([^\]]*)\]\((?:<([^>]+)>|([^)\s]+))(?:\s+(?:"[^"]*"|'[^']*'|\([^)]*\)))?\)`)

// extractMarkdownImages finds all ![alt](url) references in content,
// replaces them with <<IMG_N>> placeholders, and returns the cleaned
// content along with the extracted images.
func extractMarkdownImages(content string) (string, []markdownImage) {
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

// findPlaceholderIndices walks a Google Doc body to locate <<IMG_N>> placeholders
// and returns a map from placeholder string to its position.
func findPlaceholderIndices(doc *docs.Document, count int) map[string]docRange {
	result := make(map[string]docRange)
	if doc == nil || doc.Body == nil || count == 0 {
		return result
	}

	// Build the set of placeholders we're looking for.
	placeholders := make([]string, count)
	for i := 0; i < count; i++ {
		placeholders[i] = fmt.Sprintf("<<IMG_%d>>", i)
	}

	for _, el := range doc.Body.Content {
		if el.Paragraph == nil {
			continue
		}
		for _, pe := range el.Paragraph.Elements {
			if pe.TextRun == nil {
				continue
			}
			text := pe.TextRun.Content
			for _, ph := range placeholders {
				pos := strings.Index(text, ph)
				if pos == -1 {
					continue
				}
				absStart := pe.StartIndex + int64(pos)
				absEnd := absStart + int64(len(ph))
				result[ph] = docRange{
					startIndex: absStart,
					endIndex:   absEnd,
				}
			}
		}
	}
	return result
}

// uploadLocalImage uploads a local image to Google Drive with public read access,
// returning the public URL and the Drive file ID (for cleanup).
func uploadLocalImage(ctx context.Context, driveSvc *drive.Service, path string) (url string, fileID string, err error) {
	ext := strings.ToLower(filepath.Ext(path))
	var mimeType string
	switch ext {
	case ".png":
		mimeType = "image/png"
	case ".jpg", ".jpeg":
		mimeType = "image/jpeg"
	case ".gif":
		mimeType = "image/gif"
	default:
		return "", "", fmt.Errorf("unsupported image format %q (use PNG, JPG, or GIF)", ext)
	}

	// #nosec G304 -- path is validated by resolveMarkdownImagePath before upload.
	f, err := os.Open(path)
	if err != nil {
		return "", "", fmt.Errorf("open image %q: %w", path, err)
	}
	defer f.Close()

	driveFile, err := driveSvc.Files.Create(&drive.File{
		Name:     filepath.Base(path),
		MimeType: mimeType,
	}).Media(f).Fields("id, webContentLink").Context(ctx).Do()
	if err != nil {
		return "", "", fmt.Errorf("upload image to Drive: %w", err)
	}

	// Make publicly readable so the Docs API can fetch it.
	_, err = driveSvc.Permissions.Create(driveFile.Id, &drive.Permission{
		Type: "anyone",
		Role: "reader",
	}).Context(ctx).Do()
	if err != nil {
		deleteDriveFileBestEffort(ctx, driveSvc, driveFile.Id)
		return "", "", fmt.Errorf("set image permissions: %w", err)
	}

	imageURL := driveFile.WebContentLink
	if imageURL == "" {
		got, err := driveSvc.Files.Get(driveFile.Id).Fields("webContentLink").Context(ctx).Do()
		if err != nil {
			deleteDriveFileBestEffort(ctx, driveSvc, driveFile.Id)
			return "", "", fmt.Errorf("get image URL: %w", err)
		}
		imageURL = got.WebContentLink
	}
	if imageURL == "" {
		deleteDriveFileBestEffort(ctx, driveSvc, driveFile.Id)
		return "", "", fmt.Errorf("could not obtain public URL for uploaded image %q", path)
	}

	return imageURL, driveFile.Id, nil
}

func cleanupDriveFileIDsBestEffort(ctx context.Context, driveSvc *drive.Service, fileIDs []string) {
	if len(fileIDs) == 0 {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()

	for _, id := range fileIDs {
		if strings.TrimSpace(id) == "" {
			continue
		}
		_ = driveSvc.Files.Delete(id).Context(cleanupCtx).Do()
	}
}

func deleteDriveFileBestEffort(ctx context.Context, driveSvc *drive.Service, fileID string) {
	if strings.TrimSpace(fileID) == "" {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = driveSvc.Files.Delete(fileID).Context(cleanupCtx).Do()
}

func resolveMarkdownImagePath(markdownFilePath string, imageRef string) (string, error) {
	mdDir, err := filepath.Abs(filepath.Dir(markdownFilePath))
	if err != nil {
		return "", fmt.Errorf("resolve markdown directory: %w", err)
	}

	realDir, err := filepath.EvalSymlinks(mdDir)
	if err != nil {
		return "", fmt.Errorf("resolve markdown directory: %w", err)
	}

	imgPath := imageRef
	if !filepath.IsAbs(imgPath) {
		imgPath = filepath.Join(mdDir, imgPath)
	}
	imgPath = filepath.Clean(imgPath)

	realPath, err := filepath.EvalSymlinks(imgPath)
	if err != nil {
		return "", fmt.Errorf("resolve image path %q: %w", imageRef, err)
	}

	if !pathWithinDir(realPath, realDir) {
		return "", fmt.Errorf("image path %q resolves outside markdown file directory", imageRef)
	}
	return realPath, nil
}

func pathWithinDir(path string, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	if rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// buildImageInsertRequests creates the Docs API batch update requests to replace
// placeholder text with inline images. Requests are ordered in reverse index order
// so earlier positions are not invalidated as the document is modified.
func buildImageInsertRequests(placeholders map[string]docRange, images []markdownImage, imageURLs map[int]string) []*docs.Request {
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
				},
			},
		})
		// Then insert the image at that position.
		reqs = append(reqs, &docs.Request{
			InsertInlineImage: &docs.InsertInlineImageRequest{
				Uri: e.url,
				Location: &docs.Location{
					Index: e.dr.startIndex,
				},
			},
		})
	}
	return reqs
}
