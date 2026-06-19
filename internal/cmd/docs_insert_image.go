package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/docsedit"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DocsInsertImageCmd struct {
	DocID        string  `arg:"" name:"docId" help:"Doc ID"`
	File         string  `name:"file" help:"Local PNG, JPEG, or GIF image to upload and insert" type:"existingfile"`
	URL          string  `name:"url" help:"Public HTTPS image URL to insert directly"`
	At           *string `name:"at" help:"Placeholder text to delete and replace, or 'end' to append"`
	Before       *string `name:"before" help:"Insert before the first literal text match without deleting it"`
	After        *string `name:"after" help:"Insert after the first literal text match without deleting it"`
	Width        float64 `name:"width" help:"Image width in points; default 468pt" default:"468"`
	Height       float64 `name:"height" help:"Image height in points (optional; width-only preserves aspect ratio)"`
	Parent       string  `name:"parent" help:"Drive folder ID for the uploaded image"`
	Name         string  `name:"name" help:"Override uploaded Drive filename"`
	OnRestricted string  `name:"on-restricted" help:"If public sharing is blocked: error|link" default:"error" enum:"error,link"`
	Tab          string  `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
}

type docsInsertImageResult struct {
	documentID       string
	uploadedFileID   string
	uploadedFileName string
	permissionID     string
	atIndex          int64
	tabID            string
	requests         int
	revoked          bool
	fallbackLink     bool
	sourceURL        string
}

type docsInsertImageSource struct {
	localPath string
	name      string
	mimeType  string
	imageURL  string
}

type docsImageAnchorMode string

const (
	docsImageAnchorReplace docsImageAnchorMode = "at"
	docsImageAnchorBefore  docsImageAnchorMode = "before"
	docsImageAnchorAfter   docsImageAnchorMode = "after"
)

type docsImageTarget struct {
	anchor string
	mode   docsImageAnchorMode
}

func (c *DocsInsertImageCmd) Run(ctx context.Context, flags *RootFlags) error {
	docID := strings.TrimSpace(c.DocID)
	if docID == "" {
		return usage("empty docId")
	}
	if c.Width < 0 || c.Height < 0 {
		return usage("--width and --height must be non-negative")
	}
	source, err := c.resolveSource()
	if err != nil {
		return err
	}
	target, err := c.resolveTarget()
	if err != nil {
		return err
	}
	dryRunPayload := map[string]any{
		"documentId":        docID,
		"width":             c.Width,
		"height":            c.Height,
		"tab":               c.Tab,
		string(target.mode): target.anchor,
	}
	if source.imageURL != "" {
		dryRunPayload["url"] = source.imageURL
	} else {
		dryRunPayload["file"] = source.localPath
		dryRunPayload["name"] = source.name
		dryRunPayload["mimeType"] = source.mimeType
		dryRunPayload["parent"] = c.Parent
		dryRunPayload["onRestricted"] = c.OnRestricted
	}
	if dryRunErr := dryRunExit(ctx, flags, "docs.insert-image", dryRunPayload); dryRunErr != nil {
		return dryRunErr
	}
	if source.imageURL == "" {
		if confirmErr := confirmDestructiveChecked(ctx, flagsWithoutDryRun(flags), fmt.Sprintf("temporarily share uploaded image %s with anyone (public) so Google Docs can fetch it", source.name)); confirmErr != nil {
			return confirmErr
		}
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	docsSvc, err := docsService(ctx, account)
	if err != nil {
		return err
	}

	var result docsInsertImageResult
	if source.imageURL != "" {
		result, err = c.runURL(ctx, docsSvc, docID, source.imageURL, target)
	} else {
		driveSvc, driveErr := driveService(ctx, account)
		if driveErr != nil {
			return driveErr
		}
		result, err = c.runFile(ctx, docsSvc, driveSvc, docID, source.localPath, source.name, source.mimeType, target)
	}
	if err != nil {
		return err
	}
	return writeDocsInsertImageResult(ctx, result)
}

func (c *DocsInsertImageCmd) resolveTarget() (docsImageTarget, error) {
	set := 0
	for _, value := range []*string{c.At, c.Before, c.After} {
		if value != nil {
			set++
		}
	}
	if set > 1 {
		return docsImageTarget{}, usage("--at, --before, and --after are mutually exclusive")
	}

	target := docsImageTarget{anchor: docsAtIndexEnd, mode: docsImageAnchorReplace}
	switch {
	case c.At != nil:
		target.anchor = strings.TrimSpace(*c.At)
	case c.Before != nil:
		target.anchor = strings.TrimSpace(*c.Before)
		target.mode = docsImageAnchorBefore
	case c.After != nil:
		target.anchor = strings.TrimSpace(*c.After)
		target.mode = docsImageAnchorAfter
	}
	if target.anchor == "" {
		return docsImageTarget{}, usage(fmt.Sprintf("empty --%s", target.mode))
	}
	return target, nil
}

func (c *DocsInsertImageCmd) resolveSource() (docsInsertImageSource, error) {
	localFile := strings.TrimSpace(c.File)
	imageURL := strings.TrimSpace(c.URL)
	if localFile == "" && imageURL == "" {
		return docsInsertImageSource{}, usage("required: --file or --url")
	}
	if localFile != "" && imageURL != "" {
		return docsInsertImageSource{}, usage("--file and --url are mutually exclusive")
	}
	if imageURL != "" {
		if strings.TrimSpace(c.Parent) != "" || strings.TrimSpace(c.Name) != "" || strings.EqualFold(c.OnRestricted, "link") {
			return docsInsertImageSource{}, usage("--parent, --name, and --on-restricted=link require --file")
		}
		parsed, err := url.ParseRequestURI(imageURL)
		if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" || parsed.User != nil {
			return docsInsertImageSource{}, usage("--url must be a public HTTPS image URL without embedded credentials")
		}
		return docsInsertImageSource{imageURL: parsed.String()}, nil
	}

	localPath, err := config.ExpandPath(localFile)
	if err != nil {
		return docsInsertImageSource{}, err
	}
	mimeType := guessMimeType(localPath)
	if !isDocsInsertImageMime(mimeType) {
		return docsInsertImageSource{}, usage("--file must be a PNG, JPEG, or GIF image")
	}
	name := strings.TrimSpace(c.Name)
	if name == "" {
		name = filepath.Base(localPath)
	}
	return docsInsertImageSource{
		localPath: localPath,
		name:      name,
		mimeType:  mimeType,
	}, nil
}

func writeDocsInsertImageResult(ctx context.Context, result docsInsertImageResult) error {
	u := ui.FromContext(ctx)
	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": result.documentID,
			"atIndex":    result.atIndex,
			"requests":   result.requests,
		}
		if result.sourceURL != "" {
			payload["url"] = result.sourceURL
		} else {
			payload["uploadedFileId"] = result.uploadedFileID
			payload["uploadedFileName"] = result.uploadedFileName
			payload["permissionId"] = result.permissionID
			payload["revoked"] = result.revoked
			payload["fallbackLink"] = result.fallbackLink
		}
		if result.tabID != "" {
			payload["tabId"] = result.tabID
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}
	u.Out().Linef("documentId\t%s", result.documentID)
	if result.sourceURL != "" {
		u.Out().Linef("url\t%s", result.sourceURL)
	} else {
		u.Out().Linef("uploadedFileId\t%s", result.uploadedFileID)
		u.Out().Linef("revoked\t%t", result.revoked)
		if result.fallbackLink {
			u.Out().Linef("fallbackLink\ttrue")
		}
	}
	u.Out().Linef("atIndex\t%d", result.atIndex)
	u.Out().Linef("requests\t%d", result.requests)
	if result.tabID != "" {
		u.Out().Linef("tabId\t%s", result.tabID)
	}
	return nil
}

func (c *DocsInsertImageCmd) runURL(ctx context.Context, docsSvc *docs.Service, docID, imageURL string, target docsImageTarget) (docsInsertImageResult, error) {
	result := docsInsertImageResult{sourceURL: imageURL}
	return c.insertImageURL(ctx, docsSvc, docID, imageURL, target, result)
}

func (c *DocsInsertImageCmd) runFile(ctx context.Context, docsSvc *docs.Service, driveSvc *drive.Service, docID, localPath, name, mimeType string, target docsImageTarget) (result docsInsertImageResult, err error) {
	uploaded, err := uploadDocsInlineImage(ctx, driveSvc, localPath, name, mimeType, strings.TrimSpace(c.Parent))
	if err != nil {
		return result, err
	}
	result.uploadedFileID = uploaded.Id
	result.uploadedFileName = uploaded.Name

	perm, err := shareDocsImagePublicly(ctx, driveSvc, uploaded.Id)
	if err != nil {
		if strings.EqualFold(c.OnRestricted, "link") && isDrivePublicSharingRestricted(err) {
			return c.insertRestrictedImageFallback(ctx, docsSvc, docID, uploaded, target, result)
		}
		return result, fmt.Errorf("share uploaded image publicly: %w", err)
	}
	result.permissionID = perm.Id

	defer func() {
		if perm.Id == "" {
			return
		}
		result.revoked, err = finishDocsImagePublicShare(ctx, driveSvc, uploaded.Id, perm.Id, err)
	}()

	imageURL := driveImageDownloadURL(uploaded.Id)
	return c.insertImageURL(ctx, docsSvc, docID, imageURL, target, result)
}

func (c *DocsInsertImageCmd) insertImageURL(ctx context.Context, docsSvc *docs.Service, docID, imageURL string, target docsImageTarget, result docsInsertImageResult) (docsInsertImageResult, error) {
	reqs, index, tabID, err := c.buildInsertRequests(ctx, docsSvc, docID, target, imageURL)
	if err != nil {
		return result, err
	}
	if err := batchUpdateImageInsertRequests(ctx, docsSvc, docID, reqs); err != nil {
		return result, fmt.Errorf("insert image: %w", err)
	}
	result.documentID = docID
	result.atIndex = index
	result.tabID = tabID
	result.requests = len(reqs)
	return result, nil
}

func (c *DocsInsertImageCmd) insertRestrictedImageFallback(ctx context.Context, docsSvc *docs.Service, docID string, uploaded *drive.File, target docsImageTarget, result docsInsertImageResult) (docsInsertImageResult, error) {
	link := uploaded.WebViewLink
	if link == "" {
		link = bestEffortWebURL("drive", uploaded.Id)
	}
	reqs, index, tabID, err := c.buildLinkFallbackRequests(ctx, docsSvc, docID, target, link)
	if err != nil {
		return result, err
	}
	_, err = docsSvc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{Requests: reqs}).Context(ctx).Do()
	if err != nil {
		return result, fmt.Errorf("insert image link fallback: %w", err)
	}
	result.documentID = docID
	result.atIndex = index
	result.tabID = tabID
	result.requests = len(reqs)
	result.fallbackLink = true
	return result, nil
}

func uploadDocsInlineImage(ctx context.Context, svc *drive.Service, localPath, name, mimeType, parent string) (*drive.File, error) {
	fh, err := os.Open(localPath) //nolint:gosec // user-provided path
	if err != nil {
		return nil, fmt.Errorf("open image: %w", err)
	}
	defer fh.Close()

	meta := &drive.File{Name: name, MimeType: mimeType}
	if parent != "" {
		meta.Parents = []string{parent}
	}
	created, err := svc.Files.Create(meta).
		Media(fh, gapi.ContentType(mimeType)).
		SupportsAllDrives(true).
		Fields("id,name,mimeType,webViewLink").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("upload image: %w", err)
	}
	return created, nil
}

func shareDocsImagePublicly(ctx context.Context, svc *drive.Service, fileID string) (*drive.Permission, error) {
	return svc.Permissions.Create(fileID, &drive.Permission{Type: "anyone", Role: drivePermRoleReader}).
		SupportsAllDrives(true).
		Fields("id,type,role").
		Context(ctx).
		Do()
}

func finishDocsImagePublicShare(
	ctx context.Context,
	svc *drive.Service,
	fileID string,
	permissionID string,
	operationErr error,
) (bool, error) {
	if permissionID == "" {
		return false, operationErr
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	revokeErr := svc.Permissions.Delete(fileID, permissionID).SupportsAllDrives(true).Context(cleanupCtx).Do()
	if revokeErr == nil {
		return true, operationErr
	}
	if operationErr != nil {
		return false, fmt.Errorf("%w; additionally failed to revoke temporary public permission %s on %s: %w", operationErr, permissionID, fileID, revokeErr)
	}
	return false, fmt.Errorf("revoke temporary public permission %s on %s: %w", permissionID, fileID, revokeErr)
}

func (c *DocsInsertImageCmd) buildInsertRequests(ctx context.Context, svc *docs.Service, docID string, target docsImageTarget, imageURL string) ([]*docs.Request, int64, string, error) {
	index, placeholder, tabID, err := c.resolveImageTarget(ctx, svc, docID, target)
	if err != nil {
		return nil, 0, "", err
	}
	objSize := &docs.Size{}
	if c.Width > 0 {
		objSize.Width = &docs.Dimension{Magnitude: c.Width, Unit: "PT"}
	}
	if c.Height > 0 {
		objSize.Height = &docs.Dimension{Magnitude: c.Height, Unit: "PT"}
	}
	reqs := make([]*docs.Request, 0, 2)
	if placeholder != nil {
		reqs = append(reqs, &docs.Request{DeleteContentRange: &docs.DeleteContentRangeRequest{
			Range: &docs.Range{StartIndex: placeholder.StartIndex, EndIndex: placeholder.EndIndex, TabId: tabID},
		}})
	}
	reqs = append(reqs, &docs.Request{InsertInlineImage: &docs.InsertInlineImageRequest{
		Uri:        imageURL,
		Location:   &docs.Location{Index: index, TabId: tabID},
		ObjectSize: objSize,
	}})
	return reqs, index, tabID, nil
}

func (c *DocsInsertImageCmd) buildLinkFallbackRequests(ctx context.Context, svc *docs.Service, docID string, target docsImageTarget, link string) ([]*docs.Request, int64, string, error) {
	index, placeholder, tabID, err := c.resolveImageTarget(ctx, svc, docID, target)
	if err != nil {
		return nil, 0, "", err
	}
	reqs := make([]*docs.Request, 0, 2)
	if placeholder != nil {
		reqs = append(reqs, &docs.Request{DeleteContentRange: &docs.DeleteContentRangeRequest{
			Range: &docs.Range{StartIndex: placeholder.StartIndex, EndIndex: placeholder.EndIndex, TabId: tabID},
		}})
	}
	reqs = append(reqs, &docs.Request{InsertText: &docs.InsertTextRequest{
		Location: &docs.Location{Index: index, TabId: tabID},
		Text:     link,
	}})
	return reqs, index, tabID, nil
}

func (c *DocsInsertImageCmd) resolveImageTarget(ctx context.Context, svc *docs.Service, docID string, target docsImageTarget) (int64, *docsedit.TextRange, string, error) {
	if target.mode == docsImageAnchorReplace && strings.EqualFold(target.anchor, docsAtIndexEnd) {
		endIndex, tabID, err := docsTargetEndIndexAndTabID(ctx, svc, docID, c.Tab)
		if err != nil {
			return 0, nil, "", err
		}
		return docsedit.AppendIndex(endIndex), nil, tabID, nil
	}
	loaded, err := loadDocsTargetDocument(ctx, svc, docID, c.Tab)
	if err != nil {
		return 0, nil, "", err
	}
	matches := docsedit.FindTextRanges(loaded.target, target.anchor, docsedit.SearchOptions{
		MatchCase:            true,
		PreserveHTMLEntities: true,
		RequireTextSegment:   true,
	})
	if len(matches) == 0 {
		return 0, nil, "", fmt.Errorf("anchor not found: %q", target.anchor)
	}
	match := matches[0]
	switch target.mode {
	case docsImageAnchorBefore:
		return match.StartIndex, nil, loaded.tabID, nil
	case docsImageAnchorAfter:
		return match.EndIndex, nil, loaded.tabID, nil
	default:
		return match.StartIndex, &match, loaded.tabID, nil
	}
}

func isDocsInsertImageMime(mimeType string) bool {
	switch mimeType {
	case mimePNG, "image/jpeg", "image/gif":
		return true
	default:
		return false
	}
}

func driveImageDownloadURL(fileID string) string {
	return "https://drive.google.com/uc?export=download&id=" + url.QueryEscape(fileID)
}

func isDrivePublicSharingRestricted(err error) bool {
	var apiErr *gapi.Error
	if errors.As(err, &apiErr) {
		for _, e := range apiErr.Errors {
			if strings.Contains(e.Reason, "publishOutNotPermitted") {
				return true
			}
		}
		return strings.Contains(apiErr.Message, "publishOutNotPermitted")
	}
	return strings.Contains(err.Error(), "publishOutNotPermitted")
}
