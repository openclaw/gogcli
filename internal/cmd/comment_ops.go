package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const (
	driveCommentListFields        = "nextPageToken"
	driveCommentListCoreFields    = "comments(id,author,content,createdTime,modifiedTime,resolved,replies)"
	driveCommentListQuotedFields  = "comments(id,author,content,createdTime,modifiedTime,resolved,quotedFileContent,replies)"
	docsCommentListFields         = "comments(id,author,content,createdTime,modifiedTime,resolved,quotedFileContent,replies(id,author,content,createdTime,modifiedTime,action,deleted))"
	driveCommentDetailFields      = "id, author, content, createdTime, modifiedTime, resolved, quotedFileContent, anchor, replies"
	driveCommentCreateFields      = "id, author, content, createdTime, quotedFileContent, anchor"
	driveCommentUpdateFields      = "id, author, content, modifiedTime"
	driveReplyCreateFields        = "id, author, content, createdTime"
	driveResolveReplyCreateFields = "id, author, content, createdTime, action"
)

type driveCommentListMode int

const (
	driveCommentListModeCompact driveCommentListMode = iota
	driveCommentListModeExpanded
)

type driveCommentListOptions struct {
	resourceKey     string
	resourceID      string
	includeResolved bool
	includeQuoted   bool
	scanForOpen     bool
	page            string
	all             bool
	failEmpty       bool
	max             int64
	emptyMessage    string
	mode            driveCommentListMode
}

func listDriveComments(ctx context.Context, svc *drive.Service, fileID string, opts driveCommentListOptions) ([]*drive.Comment, string, error) {
	fetch := func(pageToken string) ([]*drive.Comment, string, error) {
		return fetchDriveCommentsPage(ctx, svc, fileID, opts.max, pageToken, driveCommentFieldsForList(opts))
	}

	if opts.all {
		comments, err := collectAllPages(opts.page, fetch)
		if err != nil {
			return nil, "", err
		}
		if !opts.includeResolved {
			comments = filterOpenComments(comments)
		}
		return comments, "", nil
	}

	if opts.includeResolved || !opts.scanForOpen {
		comments, nextPageToken, err := fetch(opts.page)
		if err != nil {
			return nil, "", err
		}
		return comments, nextPageToken, nil
	}

	pageToken := opts.page
	for {
		pageComments, nextPageToken, err := fetch(pageToken)
		if err != nil {
			return nil, "", err
		}
		open := filterOpenComments(pageComments)
		if len(open) > 0 || strings.TrimSpace(nextPageToken) == "" {
			return open, nextPageToken, nil
		}
		pageToken = nextPageToken
	}
}

func fetchDriveCommentsPage(ctx context.Context, svc *drive.Service, fileID string, pageSize int64, pageToken string, commentFields string) ([]*drive.Comment, string, error) {
	call := svc.Comments.List(fileID).
		IncludeDeleted(false).
		PageSize(pageSize).
		Fields(gapi.Field(driveCommentListFields), gapi.Field(commentFields)).
		Context(ctx)
	if strings.TrimSpace(pageToken) != "" {
		call = call.PageToken(pageToken)
	}
	resp, err := call.Do()
	if err != nil {
		return nil, "", err
	}
	return resp.Comments, resp.NextPageToken, nil
}

func driveCommentFieldsForList(opts driveCommentListOptions) string {
	if opts.mode == driveCommentListModeExpanded {
		return docsCommentListFields
	}
	if opts.includeQuoted {
		return driveCommentListQuotedFields
	}
	return driveCommentListCoreFields
}

func writeDriveCommentList(ctx context.Context, u *ui.UI, opts driveCommentListOptions, comments []*drive.Comment, nextPageToken string) error {
	if outfmt.IsJSON(ctx) {
		return writePagedJSONResult(ctx, map[string]any{
			opts.resourceKey: opts.resourceID,
			"comments":       comments,
			"nextPageToken":  nextPageToken,
		}, len(comments), opts.failEmpty)
	}

	if len(comments) == 0 {
		u.Err().Println(opts.emptyMessage)
		return failEmptyExit(opts.failEmpty)
	}

	if opts.mode == driveCommentListModeExpanded {
		printExpandedCommentTable(ctx, comments)
	} else {
		printCompactCommentTable(ctx, comments, opts.includeQuoted)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

func printExpandedCommentTable(ctx context.Context, comments []*drive.Comment) {
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "TYPE\tID\tAUTHOR\tQUOTED\tCONTENT\tCREATED\tRESOLVED\tACTION")
	for _, comment := range comments {
		if comment == nil {
			continue
		}
		author := ""
		if comment.Author != nil {
			author = comment.Author.DisplayName
		}
		quoted := ""
		if comment.QuotedFileContent != nil {
			quoted = truncateString(oneLineTSV(comment.QuotedFileContent.Value), 30)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%t\t%s\n",
			"comment",
			comment.Id,
			oneLineTSV(author),
			quoted,
			truncateString(oneLineTSV(comment.Content), 50),
			formatDateTime(comment.CreatedTime),
			comment.Resolved,
			"",
		)
		for _, reply := range comment.Replies {
			if reply == nil {
				continue
			}
			author = ""
			if reply.Author != nil {
				author = reply.Author.DisplayName
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				"reply",
				reply.Id,
				oneLineTSV(author),
				"",
				truncateString(oneLineTSV(reply.Content), 50),
				formatDateTime(reply.CreatedTime),
				"",
				oneLineTSV(reply.Action),
			)
		}
	}
}

func printCompactCommentTable(ctx context.Context, comments []*drive.Comment, includeQuoted bool) {
	w, flush := tableWriter(ctx)
	defer flush()
	if includeQuoted {
		fmt.Fprintln(w, "ID\tAUTHOR\tQUOTED\tCONTENT\tCREATED\tRESOLVED\tREPLIES")
	} else {
		fmt.Fprintln(w, "ID\tAUTHOR\tCONTENT\tCREATED\tRESOLVED\tREPLIES")
	}
	for _, comment := range comments {
		if comment == nil {
			continue
		}
		author := ""
		if comment.Author != nil {
			author = comment.Author.DisplayName
		}
		content := truncateString(comment.Content, 50)
		replyCount := len(comment.Replies)
		if includeQuoted {
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
			continue
		}
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

func getDriveComment(ctx context.Context, svc *drive.Service, fileID, commentID string) (*drive.Comment, error) {
	return svc.Comments.Get(fileID, commentID).
		Fields(driveCommentDetailFields).
		Context(ctx).
		Do()
}

func writeDriveCommentDetail(ctx context.Context, u *ui.UI, comment *drive.Comment, includeAnchor, includeReplyDetails bool) error {
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
	if includeAnchor && strings.TrimSpace(comment.Anchor) != "" {
		u.Out().Printf("anchor\t%s", comment.Anchor)
	}
	if len(comment.Replies) > 0 {
		u.Out().Printf("replies\t%d", len(comment.Replies))
	}
	if includeReplyDetails {
		for _, reply := range comment.Replies {
			if reply == nil {
				continue
			}
			author := ""
			if reply.Author != nil {
				author = reply.Author.DisplayName
			}
			action := ""
			if strings.TrimSpace(reply.Action) != "" {
				action = reply.Action
			}
			u.Out().Printf("  reply\t%s\t%s\t%s\t%s", reply.Id, author, truncateString(reply.Content, 60), action)
		}
	}
	return nil
}

func createDriveComment(ctx context.Context, svc *drive.Service, fileID, content, quoted, anchor string) (*drive.Comment, error) {
	comment := &drive.Comment{Content: content}
	if quoted != "" {
		comment.QuotedFileContent = &drive.CommentQuotedFileContent{Value: quoted}
	}
	if anchor != "" {
		comment.Anchor = anchor
	}
	return svc.Comments.Create(fileID, comment).
		Fields(driveCommentCreateFields).
		Context(ctx).
		Do()
}

func updateDriveComment(ctx context.Context, svc *drive.Service, fileID, commentID, content string) (*drive.Comment, error) {
	return svc.Comments.Update(fileID, commentID, &drive.Comment{Content: content}).
		Fields(driveCommentUpdateFields).
		Context(ctx).
		Do()
}

func writeDriveCommentMutation(ctx context.Context, u *ui.UI, comment *drive.Comment, includeAnchor bool) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"comment": comment})
	}
	u.Out().Printf("id\t%s", comment.Id)
	u.Out().Printf("content\t%s", comment.Content)
	if comment.CreatedTime != "" {
		u.Out().Printf("created\t%s", comment.CreatedTime)
	}
	if comment.ModifiedTime != "" {
		u.Out().Printf("modified\t%s", comment.ModifiedTime)
	}
	if includeAnchor && strings.TrimSpace(comment.Anchor) != "" {
		u.Out().Printf("anchor\t%s", comment.Anchor)
	}
	return nil
}

func createDriveReply(ctx context.Context, svc *drive.Service, fileID, commentID, content string) (*drive.Reply, error) {
	return svc.Replies.Create(fileID, commentID, &drive.Reply{Content: content}).
		Fields(driveReplyCreateFields).
		Context(ctx).
		Do()
}

func resolveDriveComment(ctx context.Context, svc *drive.Service, fileID, commentID, message string) (*drive.Reply, error) {
	reply := &drive.Reply{Action: "resolve"}
	if msg := strings.TrimSpace(message); msg != "" {
		reply.Content = msg
	}
	return svc.Replies.Create(fileID, commentID, reply).
		Fields(driveResolveReplyCreateFields).
		Context(ctx).
		Do()
}

func writeDriveReplyMutation(ctx context.Context, u *ui.UI, reply *drive.Reply, resolved bool, resourceKey, resourceID, commentID string) error {
	if outfmt.IsJSON(ctx) {
		if resolved {
			return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
				"resolved":  true,
				resourceKey: resourceID,
				"commentId": commentID,
				"reply":     reply,
			})
		}
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"reply": reply})
	}

	if resolved {
		u.Out().Printf("resolved\ttrue")
		u.Out().Printf("%s\t%s", resourceKey, resourceID)
		u.Out().Printf("commentId\t%s", commentID)
		return nil
	}

	u.Out().Printf("id\t%s", reply.Id)
	u.Out().Printf("content\t%s", reply.Content)
	u.Out().Printf("created\t%s", reply.CreatedTime)
	return nil
}

func deleteDriveComment(ctx context.Context, svc *drive.Service, fileID, commentID string) error {
	return svc.Comments.Delete(fileID, commentID).Context(ctx).Do()
}

func filterOpenComments(comments []*drive.Comment) []*drive.Comment {
	var open []*drive.Comment
	for _, comment := range comments {
		if comment == nil {
			continue
		}
		if !comment.Resolved {
			open = append(open, comment)
		}
	}
	return open
}

func oneLineTSV(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return strings.TrimSpace(s)
}

func truncateString(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
