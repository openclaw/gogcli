package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/api/drive/v3"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const (
	driveRevisionFields     = "id,mimeType,modifiedTime,keepForever,published,publishAuto,publishedOutsideDomain,publishedLink,lastModifyingUser,md5Checksum,size,originalFilename,exportLinks"
	driveRevisionListFields = "nextPageToken,revisions(" + driveRevisionFields + ")"
)

type DriveRevisionsCmd struct {
	List DriveRevisionsListCmd `cmd:"" name:"list" aliases:"ls" help:"List revisions for a file"`
	Get  DriveRevisionsGetCmd  `cmd:"" name:"get" help:"Get revision metadata"`
}

type DriveRevisionsListCmd struct {
	FileID    string `arg:"" name:"fileId" help:"File ID"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"200"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no revisions"`
}

func (c *DriveRevisionsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	fileID := strings.TrimSpace(c.FileID)
	if fileID == "" {
		return usage("empty fileId")
	}
	if c.Max <= 0 {
		return usage("max must be > 0")
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*drive.Revision, string, error) {
		call := svc.Revisions.List(fileID).
			PageSize(c.Max).
			Fields(gapi.Field(driveRevisionListFields)).
			Context(ctx)
		if page := strings.TrimSpace(pageToken); page != "" {
			call = call.PageToken(page)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return nil, "", callErr
		}
		return resp.Revisions, resp.NextPageToken, nil
	}

	revisions, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
	}
	if revisions == nil {
		revisions = []*drive.Revision{}
	}

	if outfmt.IsJSON(ctx) {
		return writePagedJSONResult(ctx, map[string]any{
			"fileId":        fileID,
			"revisions":     revisions,
			"nextPageToken": nextPageToken,
		}, len(revisions), c.FailEmpty)
	}
	if len(revisions) == 0 {
		u.Err().Println("No revisions")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tMODIFIED\tMIME\tUSER\tSIZE\tKEEP\tPUBLISHED\tEXPORTS")
	for _, revision := range revisions {
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\t%t\t%t\t%s\n",
			revision.Id,
			formatDateTime(revision.ModifiedTime),
			revision.MimeType,
			sanitizeTab(driveRevisionUser(revision)),
			formatDriveSize(revision.Size),
			revision.KeepForever,
			revision.Published,
			sanitizeTab(strings.Join(driveRevisionExportMIMEs(revision), ",")),
		)
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}

type DriveRevisionsGetCmd struct {
	FileID     string `arg:"" name:"fileId" help:"File ID"`
	RevisionID string `arg:"" name:"revisionId" help:"Revision ID"`
}

func (c *DriveRevisionsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	fileID := strings.TrimSpace(c.FileID)
	if fileID == "" {
		return usage("empty fileId")
	}
	revisionID := strings.TrimSpace(c.RevisionID)
	if revisionID == "" {
		return usage("empty revisionId")
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}
	revision, err := svc.Revisions.Get(fileID, revisionID).
		Fields(gapi.Field(driveRevisionFields)).
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"fileId":   fileID,
			"revision": revision,
		})
	}

	u.Out().Linef("fileId\t%s", fileID)
	u.Out().Linef("id\t%s", revision.Id)
	u.Out().Linef("modified\t%s", revision.ModifiedTime)
	u.Out().Linef("mime\t%s", revision.MimeType)
	if user := driveRevisionUser(revision); user != "" {
		u.Out().Linef("user\t%s", sanitizeTab(user))
	}
	if revision.LastModifyingUser != nil && revision.LastModifyingUser.EmailAddress != "" {
		u.Out().Linef("userEmail\t%s", revision.LastModifyingUser.EmailAddress)
	}
	u.Out().Linef("size\t%s", formatDriveSize(revision.Size))
	u.Out().Linef("keepForever\t%t", revision.KeepForever)
	u.Out().Linef("published\t%t", revision.Published)
	u.Out().Linef("publishAuto\t%t", revision.PublishAuto)
	u.Out().Linef("publishedOutsideDomain\t%t", revision.PublishedOutsideDomain)
	if revision.PublishedLink != "" {
		u.Out().Linef("publishedLink\t%s", revision.PublishedLink)
	}
	if revision.Md5Checksum != "" {
		u.Out().Linef("md5\t%s", revision.Md5Checksum)
	}
	if revision.OriginalFilename != "" {
		u.Out().Linef("originalFilename\t%s", sanitizeTab(revision.OriginalFilename))
	}
	for _, mimeType := range driveRevisionExportMIMEs(revision) {
		u.Out().Linef("export.%s\t%s", mimeType, revision.ExportLinks[mimeType])
	}
	return nil
}

func driveRevisionUser(revision *drive.Revision) string {
	if revision == nil || revision.LastModifyingUser == nil {
		return ""
	}
	if displayName := strings.TrimSpace(revision.LastModifyingUser.DisplayName); displayName != "" {
		return displayName
	}
	return strings.TrimSpace(revision.LastModifyingUser.EmailAddress)
}

func driveRevisionExportMIMEs(revision *drive.Revision) []string {
	if revision == nil || len(revision.ExportLinks) == 0 {
		return nil
	}
	mimeTypes := make([]string, 0, len(revision.ExportLinks))
	for mimeType := range revision.ExportLinks {
		mimeTypes = append(mimeTypes, mimeType)
	}
	sort.Strings(mimeTypes)
	return mimeTypes
}
