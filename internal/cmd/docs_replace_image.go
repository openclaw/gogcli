package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const docsImageReplaceMethodCenterCrop = "CENTER_CROP"

type DocsReplaceImageCmd struct {
	DocID    string `arg:"" name:"docId" help:"Doc ID"`
	File     string `name:"file" help:"Local PNG, JPEG, or GIF image to upload and use" type:"existingfile"`
	URL      string `name:"url" help:"Public HTTPS image URL to use directly"`
	ObjectID string `name:"object-id" help:"Exact image object ID from docs images list"`
	MatchAlt string `name:"match-alt" help:"Select the image whose alt text contains this value (case-insensitive)"`
	Parent   string `name:"parent" help:"Drive folder ID for an uploaded local image"`
	Name     string `name:"name" help:"Override the uploaded Drive filename"`
	Tab      string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
}

type docsReplaceImageTarget struct {
	image      docsImageListItem
	tabID      string
	revisionID string
}

type docsReplaceImageResult struct {
	documentID       string
	image            docsImageListItem
	tabID            string
	sourceURL        string
	uploadedFileID   string
	uploadedFileName string
	permissionID     string
	revoked          bool
}

func (c *DocsReplaceImageCmd) Run(ctx context.Context, flags *RootFlags) error {
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	if docID == "" {
		return usage("empty docId")
	}
	if strings.TrimSpace(c.ObjectID) != "" && strings.TrimSpace(c.MatchAlt) != "" {
		return usage("--object-id and --match-alt are mutually exclusive")
	}
	source, err := (&DocsInsertImageCmd{
		File:   c.File,
		URL:    c.URL,
		Parent: c.Parent,
		Name:   c.Name,
	}).resolveSource()
	if err != nil {
		return err
	}

	dryRunPayload := map[string]any{
		"documentId": docID,
		"objectId":   strings.TrimSpace(c.ObjectID),
		"matchAlt":   strings.TrimSpace(c.MatchAlt),
		"tab":        strings.TrimSpace(c.Tab),
		"method":     docsImageReplaceMethodCenterCrop,
	}
	if source.imageURL != "" {
		dryRunPayload["url"] = source.imageURL
	} else {
		dryRunPayload["file"] = source.localPath
		dryRunPayload["name"] = source.name
		dryRunPayload["mimeType"] = source.mimeType
		dryRunPayload["parent"] = strings.TrimSpace(c.Parent)
	}
	if dryRunErr := dryRunExit(ctx, flags, "docs.replace-image", dryRunPayload); dryRunErr != nil {
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
	target, err := c.resolveTarget(ctx, docsSvc, docID)
	if err != nil {
		return err
	}

	var result docsReplaceImageResult
	if source.imageURL != "" {
		result, err = c.replaceImageURL(ctx, docsSvc, docID, target, source.imageURL)
	} else {
		driveSvc, driveErr := driveService(ctx, account)
		if driveErr != nil {
			return driveErr
		}
		result, err = c.replaceImageFile(ctx, docsSvc, driveSvc, docID, target, source)
	}
	if err != nil {
		return err
	}
	return writeDocsReplaceImageResult(ctx, result)
}

func (c *DocsReplaceImageCmd) resolveTarget(ctx context.Context, svc *docs.Service, docID string) (docsReplaceImageTarget, error) {
	doc, tabID, err := loadDocsEnumeratorDocumentWithService(ctx, svc, docID, c.Tab)
	if err != nil {
		return docsReplaceImageTarget{}, err
	}
	image, err := selectDocsReplaceImage(enumerateDocsImages(doc), c.ObjectID, c.MatchAlt)
	if err != nil {
		return docsReplaceImageTarget{}, err
	}
	return docsReplaceImageTarget{image: image, tabID: tabID, revisionID: doc.RevisionId}, nil
}

func selectDocsReplaceImage(items []docsImageListItem, objectID, matchAlt string) (docsImageListItem, error) {
	objectID = strings.TrimSpace(objectID)
	matchAlt = strings.TrimSpace(matchAlt)
	if objectID != "" {
		for _, item := range items {
			if item.ObjectID == objectID {
				return item, nil
			}
		}
		return docsImageListItem{}, &ExitError{Code: emptyResultsExitCode, Err: fmt.Errorf("image object not found: %s", objectID)}
	}

	matches := items
	if matchAlt != "" {
		matches = nil
		needle := strings.ToLower(matchAlt)
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.Alt), needle) {
				matches = append(matches, item)
			}
		}
	}
	if len(matches) == 0 {
		if matchAlt != "" {
			return docsImageListItem{}, &ExitError{Code: emptyResultsExitCode, Err: fmt.Errorf("no image alt text contains %q", matchAlt)}
		}
		return docsImageListItem{}, &ExitError{Code: emptyResultsExitCode, Err: fmt.Errorf("no images found in document")}
	}
	if len(matches) > 1 {
		ids := make([]string, 0, len(matches))
		for _, item := range matches {
			ids = append(ids, item.ObjectID)
		}
		if matchAlt != "" {
			return docsImageListItem{}, usagef("ambiguous --match-alt %q matched %d images (%s); use --object-id", matchAlt, len(matches), strings.Join(ids, ", "))
		}
		return docsImageListItem{}, usagef("document contains %d images (%s); use --object-id or --match-alt", len(matches), strings.Join(ids, ", "))
	}
	return matches[0], nil
}

func (c *DocsReplaceImageCmd) replaceImageFile(
	ctx context.Context,
	docsSvc *docs.Service,
	driveSvc *drive.Service,
	docID string,
	target docsReplaceImageTarget,
	source docsInsertImageSource,
) (result docsReplaceImageResult, err error) {
	uploaded, err := uploadDocsInlineImage(ctx, driveSvc, source.localPath, source.name, source.mimeType, strings.TrimSpace(c.Parent))
	if err != nil {
		return result, err
	}
	result.uploadedFileID = uploaded.Id
	result.uploadedFileName = uploaded.Name

	permission, err := shareDocsImagePublicly(ctx, driveSvc, uploaded.Id)
	if err != nil {
		return result, fmt.Errorf("share uploaded image publicly: %w", err)
	}
	result.permissionID = permission.Id
	defer func() {
		result.revoked, err = finishDocsImagePublicShare(ctx, driveSvc, uploaded.Id, permission.Id, err)
	}()

	result, err = c.replaceImageURL(ctx, docsSvc, docID, target, driveImageDownloadURL(uploaded.Id))
	result.uploadedFileID = uploaded.Id
	result.uploadedFileName = uploaded.Name
	result.permissionID = permission.Id
	return result, err
}

func (c *DocsReplaceImageCmd) replaceImageURL(
	ctx context.Context,
	svc *docs.Service,
	docID string,
	target docsReplaceImageTarget,
	imageURL string,
) (docsReplaceImageResult, error) {
	request := &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{ReplaceImage: &docs.ReplaceImageRequest{
			ImageObjectId:      target.image.ObjectID,
			Uri:                imageURL,
			ImageReplaceMethod: docsImageReplaceMethodCenterCrop,
			TabId:              target.tabID,
		}}},
		WriteControl: docsRequiredRevisionWriteControl(target.revisionID),
	}
	if _, err := svc.Documents.BatchUpdate(docID, request).Context(ctx).Do(); err != nil {
		if isDocsNotFound(err) {
			return docsReplaceImageResult{}, fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return docsReplaceImageResult{}, fmt.Errorf("replace image: %w", err)
	}
	return docsReplaceImageResult{
		documentID: docID,
		image:      target.image,
		tabID:      target.tabID,
		sourceURL:  imageURL,
	}, nil
}

func writeDocsReplaceImageResult(ctx context.Context, result docsReplaceImageResult) error {
	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": result.documentID,
			"objectId":   result.image.ObjectID,
			"positioned": result.image.Positioned,
			"width":      result.image.Width,
			"height":     result.image.Height,
			"sizeUnit":   result.image.SizeUnit,
			"method":     docsImageReplaceMethodCenterCrop,
			"replaced":   true,
		}
		if result.tabID != "" {
			payload["tabId"] = result.tabID
		}
		if result.uploadedFileID != "" {
			payload["uploadedFileId"] = result.uploadedFileID
			payload["uploadedFileName"] = result.uploadedFileName
			payload["permissionId"] = result.permissionID
			payload["revoked"] = result.revoked
		} else {
			payload["url"] = result.sourceURL
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("documentId\t%s", result.documentID)
	u.Out().Linef("objectId\t%s", result.image.ObjectID)
	u.Out().Linef("positioned\t%t", result.image.Positioned)
	u.Out().Linef("width\t%s", formatOptionalFloat(result.image.Width))
	u.Out().Linef("height\t%s", formatOptionalFloat(result.image.Height))
	u.Out().Linef("sizeUnit\t%s", result.image.SizeUnit)
	u.Out().Linef("method\t%s", docsImageReplaceMethodCenterCrop)
	u.Out().Linef("replaced\ttrue")
	if result.tabID != "" {
		u.Out().Linef("tabId\t%s", result.tabID)
	}
	if result.uploadedFileID != "" {
		u.Out().Linef("uploadedFileId\t%s", result.uploadedFileID)
		u.Out().Linef("revoked\t%t", result.revoked)
	}
	return nil
}
