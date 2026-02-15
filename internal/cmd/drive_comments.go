package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/outfmt"
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
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := normalizeGoogleID(strings.TrimSpace(c.FileID))
	if fileID == "" {
		return usage("empty fileId")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*drive.Comment, string, error) {
		var call *drive.CommentsListCall
		if c.IncludeQuoted {
			call = svc.Comments.List(fileID).
				IncludeDeleted(false).
				PageSize(c.Max).
				Fields("nextPageToken", "comments(id,author,content,createdTime,modifiedTime,resolved,quotedFileContent,replies)").
				Context(ctx)
		} else {
			call = svc.Comments.List(fileID).
				IncludeDeleted(false).
				PageSize(c.Max).
				Fields("nextPageToken", "comments(id,author,content,createdTime,modifiedTime,resolved,replies)").
				Context(ctx)
		}
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, "", err
		}
		return resp.Comments, resp.NextPageToken, nil
	}

	var comments []*drive.Comment
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		comments = all
	} else {
		var err error
		comments, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"fileId":        fileID,
			"comments":      comments,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(comments) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(comments) == 0 {
		u.Err().Println("No comments")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	if c.IncludeQuoted {
		fmt.Fprintln(w, "ID\tAUTHOR\tQUOTED\tCONTENT\tCREATED\tRESOLVED\tREPLIES")
	} else {
		fmt.Fprintln(w, "ID\tAUTHOR\tCONTENT\tCREATED\tRESOLVED\tREPLIES")
	}
	for _, comment := range comments {
		author := ""
		if comment.Author != nil {
			author = comment.Author.DisplayName
		}
		content := truncateString(comment.Content, 50)
		replyCount := len(comment.Replies)
		if c.IncludeQuoted {
			quoted := ""
			if comment.QuotedFileContent != nil {
				quoted = truncateString(comment.QuotedFileContent.Value, 30)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%t\t%d\n",
				comment.Id,
				author,
				quoted,
				content,
				formatDateTime(comment.CreatedTime),
				comment.Resolved,
				replyCount,
			)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%t\t%d\n",
				comment.Id,
				author,
				content,
				formatDateTime(comment.CreatedTime),
				comment.Resolved,
				replyCount,
			)
		}
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

type DriveCommentsGetCmd struct {
	FileID    string `arg:"" name:"fileId" help:"File ID"`
	CommentID string `arg:"" name:"commentId" help:"Comment ID"`
}

func (c *DriveCommentsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := normalizeGoogleID(strings.TrimSpace(c.FileID))
	commentID := strings.TrimSpace(c.CommentID)
	if fileID == "" {
		return usage("empty fileId")
	}
	if commentID == "" {
		return usage("empty commentId")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	comment, err := svc.Comments.Get(fileID, commentID).
		Fields("id, author, content, createdTime, modifiedTime, resolved, quotedFileContent, anchor, replies").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"comment": comment})
	}

	u.Out().Printf("id\t%s", comment.Id)
	if comment.Author != nil {
		u.Out().Printf("author\t%s", comment.Author.DisplayName)
	}
	u.Out().Printf("content\t%s", comment.Content)
	u.Out().Printf("created\t%s", comment.CreatedTime)
	u.Out().Printf("modified\t%s", comment.ModifiedTime)
	u.Out().Printf("resolved\t%t", comment.Resolved)
	if comment.QuotedFileContent != nil && comment.QuotedFileContent.Value != "" {
		u.Out().Printf("quoted\t%s", comment.QuotedFileContent.Value)
	}
	if len(comment.Replies) > 0 {
		u.Out().Printf("replies\t%d", len(comment.Replies))
	}
	return nil
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

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	comment := &drive.Comment{
		Content: content,
	}

	// If quoted text is provided, anchor the comment to that text
	if quoted != "" {
		comment.QuotedFileContent = &drive.CommentQuotedFileContent{
			Value: quoted,
		}
	}

	created, err := svc.Comments.Create(fileID, comment).
		Fields("id, author, content, createdTime, quotedFileContent").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"comment": created})
	}

	u.Out().Printf("id\t%s", created.Id)
	u.Out().Printf("content\t%s", created.Content)
	u.Out().Printf("created\t%s", created.CreatedTime)
	return nil
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

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	comment := &drive.Comment{
		Content: content,
	}

	updated, err := svc.Comments.Update(fileID, commentID, comment).
		Fields("id, author, content, modifiedTime").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"comment": updated})
	}

	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("content\t%s", updated.Content)
	u.Out().Printf("modified\t%s", updated.ModifiedTime)
	return nil
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

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Comments.Delete(fileID, commentID).Context(ctx).Do(); err != nil {
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

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	reply := &drive.Reply{
		Content: content,
	}

	created, err := svc.Replies.Create(fileID, commentID, reply).
		Fields("id, author, content, createdTime").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"reply": created})
	}

	u.Out().Printf("id\t%s", created.Id)
	u.Out().Printf("content\t%s", created.Content)
	u.Out().Printf("created\t%s", created.CreatedTime)
	return nil
}

// truncateString truncates a string to maxLen and adds "..." if truncated
func truncateString(s string, maxLen int) string {
	// Replace newlines with spaces for table display
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
