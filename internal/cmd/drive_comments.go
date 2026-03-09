package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/steipete/gogcli/internal/ui"
)

// DriveCommentsCmd is the parent command for comments subcommands
type DriveCommentsCmd struct {
	List   DriveCommentsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List comments on a file"`
	Get    DriveCommentsGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get a comment by ID"`
	Create DriveCommentsCreateCmd `cmd:"" name:"create" aliases:"add,new" help:"Create a comment on a file"`
	Update DriveCommentsUpdateCmd `cmd:"" name:"update" aliases:"edit,set" help:"Update a comment"`
	Delete DriveCommentsDeleteCmd `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Delete a comment"`
	Reply  DriveCommentReplyCmd   `cmd:"" name:"reply" aliases:"respond" help:"Reply to a comment"`
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

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete comment %s from file %s", commentID, fileID)); confirmErr != nil {
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

	if err := dryRunExit(ctx, flags, "drive.comments.reply", map[string]any{
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
	created, err := createDriveReply(ctx, svc, fileID, commentID, content)
	if err != nil {
		return err
	}
	return writeDriveReplyMutation(ctx, u, created, false, "", "", "")
}
