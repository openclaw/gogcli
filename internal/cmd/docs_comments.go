package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

// DocsCommentsCmd is the parent command for comment operations on a Google Doc.
type DocsCommentsCmd struct {
	List    DocsCommentsListCmd    `cmd:"" name:"list" aliases:"ls" help:"List comments on a Google Doc"`
	Poll    DocsCommentsPollCmd    `cmd:"" name:"poll" help:"Poll new and modified comments with persisted state"`
	Get     DocsCommentsGetCmd     `cmd:"" name:"get" aliases:"info,show" help:"Get a comment by ID"`
	Add     DocsCommentsAddCmd     `cmd:"" name:"add" aliases:"create,new" help:"Add a comment to a Google Doc"`
	Locate  DocsCommentsLocateCmd  `cmd:"" name:"locate" help:"Resolve a comment quote to Docs API index ranges"`
	Reply   DocsCommentsReplyCmd   `cmd:"" name:"reply" aliases:"respond" help:"Reply to a comment"`
	Resolve DocsCommentsResolveCmd `cmd:"" name:"resolve" help:"Resolve a comment (mark as done)"`
	Reopen  DocsCommentsReopenCmd  `cmd:"" name:"reopen" help:"Reopen a previously resolved comment"`
	Delete  DocsCommentsDeleteCmd  `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Delete a comment"`
}

// DocsCommentsListCmd lists comments on a Google Doc.
type DocsCommentsListCmd struct {
	DocID           string `arg:"" name:"docId" help:"Google Doc ID or URL"`
	IncludeResolved bool   `name:"include-resolved" aliases:"resolved" help:"Include resolved comments (default: open only)"`
	Max             int64  `name:"max" aliases:"limit" help:"Max results per page" default:"100"`
	Page            string `name:"page" aliases:"cursor" help:"Page token for pagination"`
	All             bool   `name:"all" aliases:"all-pages" help:"Fetch all pages"`
	FailEmpty       bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
	Since           string `name:"since" help:"Only return comments modified at or after this RFC3339 timestamp"`
	Locate          bool   `name:"locate" help:"Attach each comment's tab and index ranges (one extra Docs fetch)"`
	Tab             string `name:"tab" help:"Only comments located in this tab by title or ID (implies --locate)"`
	TabID           string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
}

func (c *DocsCommentsListCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	if docID == "" {
		return usage("empty docId")
	}
	if c.Max <= 0 {
		return usage("max must be > 0")
	}
	since, err := normalizeDriveCommentSince(c.Since)
	if err != nil {
		return err
	}
	tab, err := resolveTabArg(ctx, c.Tab, c.TabID)
	if err != nil {
		return err
	}
	if tab == "" && (flagProvided(kctx, "tab") || flagProvided(kctx, "tab-id")) {
		return usage("--tab requires a non-empty tab title or ID")
	}
	c.Tab = tab

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}
	listOpts := driveCommentListOptions{
		resourceKey:     "docId",
		resourceID:      docID,
		includeResolved: c.IncludeResolved,
		scanForOpen:     true,
		page:            c.Page,
		since:           since,
		all:             c.All,
		failEmpty:       c.FailEmpty,
		max:             c.Max,
		emptyMessage:    "No comments",
		mode:            driveCommentListModeExpanded,
	}

	if c.Locate || tab != "" {
		return c.runLocated(ctx, u, flags, svc, docID, tab, listOpts)
	}

	comments, nextPageToken, err := listDriveComments(ctx, svc, docID, listOpts)
	if err != nil {
		return err
	}
	return writeDriveCommentList(ctx, u, driveCommentListOptions{
		resourceKey:  "docId",
		resourceID:   docID,
		failEmpty:    c.FailEmpty,
		emptyMessage: "No comments",
		mode:         driveCommentListModeExpanded,
	}, comments, nextPageToken)
}

// runLocated shares a single documents.get across every comment, and across
// every page walked while looking for tab matches.
func (c *DocsCommentsListCmd) runLocated(
	ctx context.Context,
	u *ui.UI,
	flags *RootFlags,
	svc *drive.Service,
	docID string,
	tab string,
	listOpts driveCommentListOptions,
) error {
	docsSvc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	locator, err := newDocsCommentLocator(ctx, docsSvc, docID, tab)
	if err != nil {
		return err
	}

	located, nextPageToken, err := c.collectLocatedComments(ctx, svc, docID, listOpts, locator)
	if err != nil {
		return err
	}
	return writeDocsCommentListWithLocations(ctx, u, docID, c.FailEmpty, locator.targetTab, located, nextPageToken)
}

// collectLocatedComments walks Drive pages until the tab filter yields at
// least one comment, mirroring how listDriveComments scans for open comments.
func (c *DocsCommentsListCmd) collectLocatedComments(
	ctx context.Context,
	svc *drive.Service,
	docID string,
	listOpts driveCommentListOptions,
	locator *docsCommentLocator,
) ([]*driveCommentWithLocation, string, error) {
	seen := map[string]bool{}
	pageToken := listOpts.page
	for {
		listOpts.page = pageToken
		comments, nextPageToken, err := listDriveComments(ctx, svc, docID, listOpts)
		if err != nil {
			return nil, "", err
		}
		located := locator.attach(comments)
		nextPageToken = strings.TrimSpace(nextPageToken)
		if locator.targetTab == nil || len(located) > 0 || nextPageToken == "" || seen[nextPageToken] {
			return located, nextPageToken, nil
		}
		seen[nextPageToken] = true
		pageToken = nextPageToken
	}
}

func writeDocsCommentListWithLocations(
	ctx context.Context,
	u *ui.UI,
	docID string,
	failEmpty bool,
	targetTab *docs.Tab,
	located []*driveCommentWithLocation,
	nextPageToken string,
) error {
	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"docId":         docID,
			"comments":      located,
			"nextPageToken": nextPageToken,
		}
		if tab := newDocsCommentListTab(targetTab); tab != nil {
			payload["tab"] = tab
		}
		return writePagedJSONResult(ctx, payload, len(located), failEmpty)
	}

	if len(located) == 0 {
		if targetTab != nil {
			u.Err().Linef("No comments located in tab %q", docsCommentTabLabel(targetTab))
		} else {
			u.Err().Println("No comments")
		}
		return failEmptyExit(failEmpty)
	}

	printExpandedCommentRows(ctx, located, true)
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}

// DocsCommentsGetCmd retrieves a single comment by ID.
type DocsCommentsGetCmd struct {
	DocID     string `arg:"" name:"docId" help:"Google Doc ID or URL"`
	CommentID string `arg:"" name:"commentId" help:"Comment ID"`
}

func (c *DocsCommentsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	commentID := strings.TrimSpace(c.CommentID)
	if docID == "" {
		return usage("empty docId")
	}
	if commentID == "" {
		return usage("empty commentId")
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	comment, err := getDriveComment(ctx, svc, docID, commentID)
	if err != nil {
		return err
	}
	return writeDriveCommentDetail(ctx, u, comment, true, true)
}

// DocsCommentsAddCmd creates a comment on a Google Doc.
type DocsCommentsAddCmd struct {
	DocID   string `arg:"" name:"docId" help:"Google Doc ID or URL"`
	Content string `arg:"" name:"content" help:"Comment text"`
	Quoted  string `name:"quoted" help:"Quoted text to attach to the comment (shown in UIs when available)"`
	Anchor  string `name:"anchor" help:"Anchor JSON string (advanced; editor UIs may still treat as unanchored)"`
}

func (c *DocsCommentsAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	content := strings.TrimSpace(c.Content)
	quoted := strings.TrimSpace(c.Quoted)
	anchor := strings.TrimSpace(c.Anchor)
	if docID == "" {
		return usage("empty docId")
	}
	if content == "" {
		return usage("empty content")
	}
	if err := validateDocsCommentAnchor(anchor); err != nil {
		return err
	}

	if err := dryRunExit(ctx, flags, "docs.comments.add", map[string]any{
		"doc_id":  docID,
		"content": content,
		"quoted":  quoted,
		"anchor":  anchor,
	}); err != nil {
		return err
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	created, err := createDriveComment(ctx, svc, docID, content, quoted, anchor)
	if err != nil {
		return err
	}
	return writeDriveCommentMutation(ctx, u, created, true)
}

func validateDocsCommentAnchor(anchor string) error {
	if strings.TrimSpace(anchor) == "" {
		return nil
	}
	if !json.Valid([]byte(anchor)) {
		return usage("invalid --anchor JSON")
	}
	return nil
}

// DocsCommentsReplyCmd replies to a comment on a Google Doc.
type DocsCommentsReplyCmd struct {
	DocID     string `arg:"" name:"docId" help:"Google Doc ID or URL"`
	CommentID string `arg:"" name:"commentId" help:"Comment ID"`
	Content   string `arg:"" name:"content" help:"Reply text"`
	Action    string `name:"action" enum:"resolve,reopen," default:"" help:"Optional action to take on the parent comment alongside the reply: resolve|reopen"`
}

func (c *DocsCommentsReplyCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runDriveCommentReply(ctx, flags, driveCommentReplyCommand{
		op:            "docs.comments.reply",
		resourceKey:   "docId",
		payloadKey:    "doc_id",
		rawResourceID: c.DocID,
		commentID:     c.CommentID,
		content:       c.Content,
		action:        c.Action,
	})
}

// DocsCommentsResolveCmd resolves a comment by posting an empty reply with action "resolve".
// The Drive API resolves a comment when a reply is created with action="resolve".
type DocsCommentsResolveCmd struct {
	DocID     string `arg:"" name:"docId" help:"Google Doc ID or URL"`
	CommentID string `arg:"" name:"commentId" help:"Comment ID"`
	Message   string `name:"message" short:"m" help:"Optional message to include when resolving"`
}

func (c *DocsCommentsResolveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	commentID := strings.TrimSpace(c.CommentID)
	if docID == "" {
		return usage("empty docId")
	}
	if commentID == "" {
		return usage("empty commentId")
	}

	if err := dryRunExit(ctx, flags, "docs.comments.resolve", map[string]any{
		"doc_id":     docID,
		"comment_id": commentID,
		"message":    strings.TrimSpace(c.Message),
	}); err != nil {
		return err
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	created, err := resolveDriveComment(ctx, svc, docID, commentID, c.Message)
	if err != nil {
		return err
	}
	return writeDriveReplyMutationWithAction(ctx, u, created, true, driveReplyActionResolve, "docId", docID, commentID)
}

// DocsCommentsReopenCmd reopens a previously resolved comment on a Google Doc.
// The Drive API reopens a comment when a reply is created with action="reopen".
type DocsCommentsReopenCmd struct {
	DocID     string `arg:"" name:"docId" help:"Google Doc ID or URL"`
	CommentID string `arg:"" name:"commentId" help:"Comment ID"`
	Message   string `name:"message" short:"m" help:"Optional message to include when reopening"`
}

func (c *DocsCommentsReopenCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	commentID := strings.TrimSpace(c.CommentID)
	if docID == "" {
		return usage("empty docId")
	}
	if commentID == "" {
		return usage("empty commentId")
	}

	if err := dryRunExit(ctx, flags, "docs.comments.reopen", map[string]any{
		"doc_id":     docID,
		"comment_id": commentID,
		"message":    strings.TrimSpace(c.Message),
	}); err != nil {
		return err
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	created, err := reopenDriveComment(ctx, svc, docID, commentID, c.Message)
	if err != nil {
		return err
	}
	return writeDriveReplyMutationWithAction(ctx, u, created, true, driveReplyActionReopen, "docId", docID, commentID)
}

// DocsCommentsDeleteCmd deletes a comment on a Google Doc.
type DocsCommentsDeleteCmd struct {
	DocID     string `arg:"" name:"docId" help:"Google Doc ID or URL"`
	CommentID string `arg:"" name:"commentId" help:"Comment ID"`
}

func (c *DocsCommentsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	commentID := strings.TrimSpace(c.CommentID)
	if docID == "" {
		return usage("empty docId")
	}
	if commentID == "" {
		return usage("empty commentId")
	}

	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "docs.comments.delete", map[string]any{
		"doc_id":     docID,
		"comment_id": commentID,
	}, fmt.Sprintf("delete comment %s from doc %s", commentID, docID)); confirmErr != nil {
		return confirmErr
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	if err := deleteDriveComment(ctx, svc, docID, commentID); err != nil {
		return err
	}

	return writeResult(ctx, u,
		kv("deleted", true),
		kv("docId", docID),
		kv("commentId", commentID),
	)
}
