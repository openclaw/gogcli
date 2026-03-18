package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/steipete/gogcli/internal/ui"
)

// DocsCommentsCmd is the parent command for comment operations on a Google Doc.
type DocsCommentsCmd struct {
	List    DocsCommentsListCmd    `cmd:"" name:"list" aliases:"ls" help:"List comments on a Google Doc"`
	Get     DocsCommentsGetCmd     `cmd:"" name:"get" aliases:"info,show" help:"Get a comment by ID"`
	Add     DocsCommentsAddCmd     `cmd:"" name:"add" aliases:"create,new" help:"Add a comment to a Google Doc"`
	Reply   DocsCommentsReplyCmd   `cmd:"" name:"reply" aliases:"respond" help:"Reply to a comment"`
	Resolve DocsCommentsResolveCmd `cmd:"" name:"resolve" help:"Resolve a comment (mark as done)"`
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
}

func (c *DocsCommentsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	if docID == "" {
		return usage("empty docId")
	}
	if c.Max <= 0 {
		return usage("max must be > 0")
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}
	comments, nextPageToken, err := listDriveComments(ctx, svc, docID, driveCommentListOptions{
		resourceKey:     "docId",
		resourceID:      docID,
		includeResolved: c.IncludeResolved,
		scanForOpen:     true,
		page:            c.Page,
		all:             c.All,
		failEmpty:       c.FailEmpty,
		max:             c.Max,
		emptyMessage:    "No comments",
		mode:            driveCommentListModeExpanded,
	})
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

// DocsCommentsReplyCmd replies to a comment on a Google Doc.
type DocsCommentsReplyCmd struct {
	DocID     string `arg:"" name:"docId" help:"Google Doc ID or URL"`
	CommentID string `arg:"" name:"commentId" help:"Comment ID"`
	Content   string `arg:"" name:"content" help:"Reply text"`
}

func (c *DocsCommentsReplyCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	commentID := strings.TrimSpace(c.CommentID)
	content := strings.TrimSpace(c.Content)
	if docID == "" {
		return usage("empty docId")
	}
	if commentID == "" {
		return usage("empty commentId")
	}
	if content == "" {
		return usage("empty content")
	}

	if err := dryRunExit(ctx, flags, "docs.comments.reply", map[string]any{
		"doc_id":     docID,
		"comment_id": commentID,
		"content":    content,
	}); err != nil {
		return err
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	created, err := createDriveReply(ctx, svc, docID, commentID, content)
	if err != nil {
		return err
	}
	return writeDriveReplyMutation(ctx, u, created, false, "", "", "")
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
	return writeDriveReplyMutation(ctx, u, created, true, "docId", docID, commentID)
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

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete comment %s from doc %s", commentID, docID)); confirmErr != nil {
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
