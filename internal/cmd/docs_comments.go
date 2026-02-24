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
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	if docID == "" {
		return usage("empty docId")
	}
	if c.Max <= 0 {
		return usage("max must be > 0")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*drive.Comment, string, error) {
		call := svc.Comments.List(docID).
			IncludeDeleted(false).
			PageSize(c.Max).
			Fields("nextPageToken", "comments(id,author,content,createdTime,modifiedTime,resolved,quotedFileContent,replies(id,author,content,createdTime,modifiedTime,action,deleted))").
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

	var comments []*drive.Comment
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		comments = all
	} else {
		if c.IncludeResolved {
			var err error
			comments, nextPageToken, err = fetch(c.Page)
			if err != nil {
				return err
			}
		} else {
			// Default: open-only. Scan forward until we find at least one open comment (or run out of pages).
			pageToken := c.Page
			for {
				pageComments, token, err := fetch(pageToken)
				if err != nil {
					return err
				}
				open := filterOpenComments(pageComments)
				if len(open) > 0 {
					comments = open
					nextPageToken = token
					break
				}
				if strings.TrimSpace(token) == "" {
					comments = nil
					nextPageToken = ""
					break
				}
				pageToken = token
			}
		}
	}

	// Filter out resolved comments unless explicitly requested.
	if !c.IncludeResolved {
		comments = filterOpenComments(comments)
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"docId":         docID,
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
		for _, r := range comment.Replies {
			if r == nil {
				continue
			}
			rAuthor := ""
			if r.Author != nil {
				rAuthor = r.Author.DisplayName
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				"reply",
				r.Id,
				oneLineTSV(rAuthor),
				"",
				truncateString(oneLineTSV(r.Content), 50),
				formatDateTime(r.CreatedTime),
				"",
				oneLineTSV(r.Action),
			)
		}
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

// DocsCommentsGetCmd retrieves a single comment by ID.
type DocsCommentsGetCmd struct {
	DocID     string `arg:"" name:"docId" help:"Google Doc ID or URL"`
	CommentID string `arg:"" name:"commentId" help:"Comment ID"`
}

func (c *DocsCommentsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	commentID := strings.TrimSpace(c.CommentID)
	if docID == "" {
		return usage("empty docId")
	}
	if commentID == "" {
		return usage("empty commentId")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	comment, err := svc.Comments.Get(docID, commentID).
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
	if strings.TrimSpace(comment.Anchor) != "" {
		u.Out().Printf("anchor\t%s", comment.Anchor)
	}
	if len(comment.Replies) > 0 {
		u.Out().Printf("replies\t%d", len(comment.Replies))
		for _, r := range comment.Replies {
			rAuthor := ""
			if r.Author != nil {
				rAuthor = r.Author.DisplayName
			}
			action := ""
			if strings.TrimSpace(r.Action) != "" {
				action = r.Action
			}
			u.Out().Printf("  reply\t%s\t%s\t%s\t%s", r.Id, rAuthor, truncateString(r.Content, 60), action)
		}
	}
	return nil
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

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	comment := &drive.Comment{Content: content}
	if quoted != "" {
		comment.QuotedFileContent = &drive.CommentQuotedFileContent{Value: quoted}
	}
	if anchor != "" {
		comment.Anchor = anchor
	}

	created, err := svc.Comments.Create(docID, comment).
		Fields("id, author, content, createdTime, quotedFileContent, anchor").
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

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	created, err := svc.Replies.Create(docID, commentID, &drive.Reply{Content: content}).
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

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	reply := &drive.Reply{
		Action: "resolve",
	}
	if msg := strings.TrimSpace(c.Message); msg != "" {
		reply.Content = msg
	}

	created, err := svc.Replies.Create(docID, commentID, reply).
		Fields("id, author, content, createdTime, action").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"resolved":  true,
			"docId":     docID,
			"commentId": commentID,
			"reply":     created,
		})
	}

	u.Out().Printf("resolved\ttrue")
	u.Out().Printf("docId\t%s", docID)
	u.Out().Printf("commentId\t%s", commentID)
	return nil
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

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Comments.Delete(docID, commentID).Context(ctx).Do(); err != nil {
		return err
	}

	return writeResult(ctx, u,
		kv("deleted", true),
		kv("docId", docID),
		kv("commentId", commentID),
	)
}

// filterOpenComments returns only non-resolved comments.
func filterOpenComments(comments []*drive.Comment) []*drive.Comment {
	var open []*drive.Comment
	for _, c := range comments {
		if c == nil {
			continue
		}
		if !c.Resolved {
			open = append(open, c)
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
