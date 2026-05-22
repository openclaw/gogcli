package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/steipete/gogcli/internal/ui"
)

// DriveCommentsCmd is the parent command for comments subcommands
type DriveCommentsCmd struct {
	List    DriveCommentsListCmd    `cmd:"" name:"list" aliases:"ls" help:"List comments on a file"`
	Get     DriveCommentsGetCmd     `cmd:"" name:"get" aliases:"info,show" help:"Get a comment by ID"`
	Create  DriveCommentsCreateCmd  `cmd:"" name:"create" aliases:"add,new" help:"Create a comment on a file"`
	Update  DriveCommentsUpdateCmd  `cmd:"" name:"update" aliases:"edit,set" help:"Update a comment"`
	Delete  DriveCommentsDeleteCmd  `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Delete a comment"`
	Reply   DriveCommentReplyCmd    `cmd:"" name:"reply" aliases:"respond" help:"Reply to a comment"`
	Resolve DriveCommentsResolveCmd `cmd:"" name:"resolve" help:"Resolve a comment (mark as done)"`
	Reopen  DriveCommentsReopenCmd  `cmd:"" name:"reopen" help:"Reopen a previously resolved comment"`
}

type DriveCommentsListCmd struct {
	FileID        string `arg:"" name:"fileId" help:"File ID"`
	Max           int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page          string `name:"page" aliases:"cursor" help:"Page token"`
	All           bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty     bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
	IncludeQuoted bool   `name:"include-quoted" help:"Include the quoted content the comment is anchored to"`
}

func (c *DriveCommentsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	fileID := normalizeGoogleID(strings.TrimSpace(c.FileID))
	if fileID == "" {
		return usage("empty fileId")
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}
	comments, nextPageToken, err := listDriveComments(ctx, svc, fileID, driveCommentListOptions{
		resourceKey:   "fileId",
		resourceID:    fileID,
		includeQuoted: c.IncludeQuoted,
		page:          c.Page,
		all:           c.All,
		failEmpty:     c.FailEmpty,
		max:           c.Max,
		emptyMessage:  "No comments",
		mode:          driveCommentListModeCompact,
	})
	if err != nil {
		return err
	}
	return writeDriveCommentList(ctx, u, driveCommentListOptions{
		resourceKey:   "fileId",
		resourceID:    fileID,
		includeQuoted: c.IncludeQuoted,
		failEmpty:     c.FailEmpty,
		emptyMessage:  "No comments",
		mode:          driveCommentListModeCompact,
	}, comments, nextPageToken)
}

type DriveCommentsGetCmd struct {
	FileID    string `arg:"" name:"fileId" help:"File ID"`
	CommentID string `arg:"" name:"commentId" help:"Comment ID"`
}

func (c *DriveCommentsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	fileID := normalizeGoogleID(strings.TrimSpace(c.FileID))
	commentID := strings.TrimSpace(c.CommentID)
	if fileID == "" {
		return usage("empty fileId")
	}
	if commentID == "" {
		return usage("empty commentId")
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	comment, err := getDriveComment(ctx, svc, fileID, commentID)
	if err != nil {
		return err
	}
	return writeDriveCommentDetail(ctx, u, comment, false, false)
}

type DriveCommentsCreateCmd struct {
	FileID  string `arg:"" name:"fileId" help:"File ID"`
	Content string `arg:"" name:"content" help:"Comment text"`
	Quoted  string `name:"quoted" help:"Text to anchor the comment to (for Google Docs)"`
}

func (c *DriveCommentsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	fileID := normalizeGoogleID(strings.TrimSpace(c.FileID))
	content := strings.TrimSpace(c.Content)
	quoted := strings.TrimSpace(c.Quoted)
	if fileID == "" {
		return usage("empty fileId")
	}
	if content == "" {
		return usage("empty content")
	}

	if err := dryRunExit(ctx, flags, "drive.comments.create", map[string]any{
		"file_id": fileID,
		"content": content,
		"quoted":  quoted,
	}); err != nil {
		return err
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}
	created, err := createDriveComment(ctx, svc, fileID, content, quoted, "")
	if err != nil {
		return err
	}
	return writeDriveCommentMutation(ctx, u, created, false)
}

type DriveCommentsUpdateCmd struct {
	FileID    string `arg:"" name:"fileId" help:"File ID"`
	CommentID string `arg:"" name:"commentId" help:"Comment ID"`
	Content   string `arg:"" name:"content" help:"New comment text"`
}

func (c *DriveCommentsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	fileID := normalizeGoogleID(strings.TrimSpace(c.FileID))
	commentID := strings.TrimSpace(c.CommentID)
	content := strings.TrimSpace(c.Content)
	if fileID == "" {
		return usage("empty fileId")
	}
	if commentID == "" {
		return usage("empty commentId")
	}
	if content == "" {
		return usage("empty content")
	}

	if err := dryRunExit(ctx, flags, "drive.comments.update", map[string]any{
		"file_id":    fileID,
		"comment_id": commentID,
		"content":    content,
	}); err != nil {
		return err
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}
	updated, err := updateDriveComment(ctx, svc, fileID, commentID, content)
	if err != nil {
		return err
	}
	return writeDriveCommentMutation(ctx, u, updated, false)
}

type DriveCommentsDeleteCmd struct {
	FileID    string `arg:"" name:"fileId" help:"File ID"`
	CommentID string `arg:"" name:"commentId" help:"Comment ID"`
}

func (c *DriveCommentsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	fileID := normalizeGoogleID(strings.TrimSpace(c.FileID))
	commentID := strings.TrimSpace(c.CommentID)
	if fileID == "" {
		return usage("empty fileId")
	}
	if commentID == "" {
		return usage("empty commentId")
	}

	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "drive.comments.delete", map[string]any{
		"file_id":    fileID,
		"comment_id": commentID,
	}, fmt.Sprintf("delete comment %s from file %s", commentID, fileID)); confirmErr != nil {
		return confirmErr
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	if err := deleteDriveComment(ctx, svc, fileID, commentID); err != nil {
		return err
	}

	return writeResult(ctx, u,
		kv("deleted", true),
		kv("fileId", fileID),
		kv("commentId", commentID),
	)
}

type DriveCommentReplyCmd struct {
	FileID    string `arg:"" name:"fileId" help:"File ID"`
	CommentID string `arg:"" name:"commentId" help:"Comment ID"`
	Content   string `arg:"" name:"content" help:"Reply text"`
	Action    string `name:"action" enum:"resolve,reopen," default:"" help:"Optional action to take on the parent comment alongside the reply: resolve|reopen"`
}

func (c *DriveCommentReplyCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	fileID := normalizeGoogleID(strings.TrimSpace(c.FileID))
	commentID := strings.TrimSpace(c.CommentID)
	content := strings.TrimSpace(c.Content)
	if fileID == "" {
		return usage("empty fileId")
	}
	if commentID == "" {
		return usage("empty commentId")
	}
	if content == "" {
		return usage("empty content")
	}
	action, err := validateDriveReplyAction(c.Action)
	if err != nil {
		return usage(err.Error())
	}

	if dryRunErr := dryRunExit(ctx, flags, "drive.comments.reply", map[string]any{
		"file_id":    fileID,
		"comment_id": commentID,
		"content":    content,
		"action":     action,
	}); dryRunErr != nil {
		return dryRunErr
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}
	created, err := createDriveReplyWithAction(ctx, svc, fileID, commentID, content, action)
	if err != nil {
		return err
	}
	resolved := action == driveReplyActionResolve || action == driveReplyActionReopen
	return writeDriveReplyMutationWithAction(ctx, u, created, resolved, action, "fileId", fileID, commentID)
}

// DriveCommentsResolveCmd resolves a comment by posting an action="resolve" reply.
type DriveCommentsResolveCmd struct {
	FileID    string `arg:"" name:"fileId" help:"File ID"`
	CommentID string `arg:"" name:"commentId" help:"Comment ID"`
	Message   string `name:"message" short:"m" help:"Optional message to include when resolving"`
}

func (c *DriveCommentsResolveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	fileID := normalizeGoogleID(strings.TrimSpace(c.FileID))
	commentID := strings.TrimSpace(c.CommentID)
	if fileID == "" {
		return usage("empty fileId")
	}
	if commentID == "" {
		return usage("empty commentId")
	}

	if err := dryRunExit(ctx, flags, "drive.comments.resolve", map[string]any{
		"file_id":    fileID,
		"comment_id": commentID,
		"message":    strings.TrimSpace(c.Message),
	}); err != nil {
		return err
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}
	created, err := resolveDriveComment(ctx, svc, fileID, commentID, c.Message)
	if err != nil {
		return err
	}
	return writeDriveReplyMutationWithAction(ctx, u, created, true, driveReplyActionResolve, "fileId", fileID, commentID)
}

// DriveCommentsReopenCmd reopens a previously resolved comment by posting an
// action="reopen" reply.
type DriveCommentsReopenCmd struct {
	FileID    string `arg:"" name:"fileId" help:"File ID"`
	CommentID string `arg:"" name:"commentId" help:"Comment ID"`
	Message   string `name:"message" short:"m" help:"Optional message to include when reopening"`
}

func (c *DriveCommentsReopenCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	fileID := normalizeGoogleID(strings.TrimSpace(c.FileID))
	commentID := strings.TrimSpace(c.CommentID)
	if fileID == "" {
		return usage("empty fileId")
	}
	if commentID == "" {
		return usage("empty commentId")
	}

	if err := dryRunExit(ctx, flags, "drive.comments.reopen", map[string]any{
		"file_id":    fileID,
		"comment_id": commentID,
		"message":    strings.TrimSpace(c.Message),
	}); err != nil {
		return err
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}
	created, err := reopenDriveComment(ctx, svc, fileID, commentID, c.Message)
	if err != nil {
		return err
	}
	return writeDriveReplyMutationWithAction(ctx, u, created, true, driveReplyActionReopen, "fileId", fileID, commentID)
}
