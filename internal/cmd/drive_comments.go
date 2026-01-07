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
	List   DriveCommentsListCmd   `cmd:"" name:"list" help:"List comments on a file"`
	Get    DriveCommentsGetCmd    `cmd:"" name:"get" help:"Get a comment by ID"`
	Create DriveCommentsCreateCmd `cmd:"" name:"create" help:"Create a comment on a file"`
	Update DriveCommentsUpdateCmd `cmd:"" name:"update" help:"Update a comment"`
	Delete DriveCommentsDeleteCmd `cmd:"" name:"delete" help:"Delete a comment"`
	Reply  DriveCommentReplyCmd   `cmd:"" name:"reply" help:"Reply to a comment"`
}

type DriveCommentsListCmd struct {
	FileID        string `arg:"" name:"fileId" help:"File ID"`
	Max           int64  `name:"max" help:"Max results" default:"100"`
	Page          string `name:"page" help:"Page token"`
	IncludeQuoted bool   `name:"include-quoted" help:"Include the quoted content the comment is anchored to"`
}

func (c *DriveCommentsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := strings.TrimSpace(c.FileID)
	if fileID == "" {
		return usage("empty fileId")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

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
	if strings.TrimSpace(c.Page) != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"fileId":        fileID,
			"comments":      resp.Comments,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Comments) == 0 {
		u.Err().Println("No comments")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	if c.IncludeQuoted {
		fmt.Fprintln(w, "ID\tAUTHOR\tQUOTED\tCONTENT\tCREATED\tRESOLVED\tREPLIES")
	} else {
		fmt.Fprintln(w, "ID\tAUTHOR\tCONTENT\tCREATED\tRESOLVED\tREPLIES")
	}
	for _, comment := range resp.Comments {
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
	printNextPageHint(u, resp.NextPageToken)
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
	fileID := strings.TrimSpace(c.FileID)
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{"comment": comment})
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
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := strings.TrimSpace(c.FileID)
	content := strings.TrimSpace(c.Content)
	if fileID == "" {
		return usage("empty fileId")
	}
	if content == "" {
		return usage("empty content")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	comment := &drive.Comment{
		Content: content,
	}

	// If quoted text is provided, anchor the comment to that text
	if quoted := strings.TrimSpace(c.Quoted); quoted != "" {
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{"comment": created})
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
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := strings.TrimSpace(c.FileID)
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{"comment": updated})
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
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := strings.TrimSpace(c.FileID)
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

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Comments.Delete(fileID, commentID).Context(ctx).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"deleted":   true,
			"fileId":    fileID,
			"commentId": commentID,
		})
	}

	u.Out().Printf("deleted\ttrue")
	u.Out().Printf("file_id\t%s", fileID)
	u.Out().Printf("comment_id\t%s", commentID)
	return nil
}

type DriveCommentReplyCmd struct {
	FileID    string `arg:"" name:"fileId" help:"File ID"`
	CommentID string `arg:"" name:"commentId" help:"Comment ID"`
	Content   string `arg:"" name:"content" help:"Reply text"`
}

func (c *DriveCommentReplyCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := strings.TrimSpace(c.FileID)
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{"reply": created})
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
