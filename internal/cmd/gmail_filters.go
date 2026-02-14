package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailFiltersCmd struct {
	List   GmailFiltersListCmd   `cmd:"" name:"list" aliases:"ls" help:"List all email filters"`
	Get    GmailFiltersGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get a specific filter"`
	Create GmailFiltersCreateCmd `cmd:"" name:"create" aliases:"add,new" help:"Create a new email filter"`
	Delete GmailFiltersDeleteCmd `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Delete a filter"`
}

type GmailFiltersListCmd struct{}

func (c *GmailFiltersListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Users.Settings.Filters.List("me").Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"filters": resp.Filter})
	}

	if len(resp.Filter) == 0 {
		u.Err().Println("No filters")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tFROM\tTO\tSUBJECT\tQUERY")
	for _, f := range resp.Filter {
		criteria := f.Criteria
		from := ""
		to := ""
		subject := ""
		query := ""
		if criteria != nil {
			from = criteria.From
			to = criteria.To
			subject = criteria.Subject
			query = criteria.Query
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			f.Id,
			sanitizeTab(from),
			sanitizeTab(to),
			sanitizeTab(subject),
			sanitizeTab(query))
	}
	_ = tw.Flush()
	return nil
}

type GmailFiltersGetCmd struct {
	FilterID string `arg:"" name:"filterId" help:"Filter ID"`
}

func (c *GmailFiltersGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	filterID := strings.TrimSpace(c.FilterID)
	if filterID == "" {
		return usage("empty filterId")
	}
	filter, err := svc.Users.Settings.Filters.Get("me", filterID).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"filter": filter})
	}

	u.Out().Printf("id\t%s", filter.Id)
	if filter.Criteria != nil {
		c := filter.Criteria
		if c.From != "" {
			u.Out().Printf("from\t%s", c.From)
		}
		if c.To != "" {
			u.Out().Printf("to\t%s", c.To)
		}
		if c.Subject != "" {
			u.Out().Printf("subject\t%s", c.Subject)
		}
		if c.Query != "" {
			u.Out().Printf("query\t%s", c.Query)
		}
		if c.HasAttachment {
			u.Out().Printf("has_attachment\ttrue")
		}
		if c.NegatedQuery != "" {
			u.Out().Printf("negated_query\t%s", c.NegatedQuery)
		}
		if c.Size != 0 {
			u.Out().Printf("size\t%d", c.Size)
		}
		if c.SizeComparison != "" {
			u.Out().Printf("size_comparison\t%s", c.SizeComparison)
		}
		if c.ExcludeChats {
			u.Out().Printf("exclude_chats\ttrue")
		}
	}
	if filter.Action != nil {
		a := filter.Action
		if len(a.AddLabelIds) > 0 {
			u.Out().Printf("add_label_ids\t%s", strings.Join(a.AddLabelIds, ","))
		}
		if len(a.RemoveLabelIds) > 0 {
			u.Out().Printf("remove_label_ids\t%s", strings.Join(a.RemoveLabelIds, ","))
		}
		if a.Forward != "" {
			u.Out().Printf("forward\t%s", a.Forward)
		}
	}
	return nil
}

type GmailFiltersCreateCmd struct {
	From          string `name:"from" help:"Match messages from this sender"`
	To            string `name:"to" help:"Match messages to this recipient"`
	Subject       string `name:"subject" help:"Match messages with this subject"`
	Query         string `name:"query" help:"Advanced Gmail search query for matching"`
	HasAttachment bool   `name:"has-attachment" help:"Match messages with attachments"`
	AddLabel      string `name:"add-label" help:"Label(s) to add to matching messages (comma-separated, name or ID)"`
	RemoveLabel   string `name:"remove-label" help:"Label(s) to remove from matching messages (comma-separated, name or ID)"`
	Archive       bool   `name:"archive" help:"Archive matching messages (skip inbox)"`
	MarkRead      bool   `name:"mark-read" help:"Mark matching messages as read"`
	Star          bool   `name:"star" help:"Star matching messages"`
	Forward       string `name:"forward" help:"Forward to this email address"`
	Trash         bool   `name:"trash" help:"Move matching messages to trash"`
	NeverSpam     bool   `name:"never-spam" help:"Never mark as spam"`
	Important     bool   `name:"important" help:"Mark as important"`
}

func (c *GmailFiltersCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	// Validate that at least one criteria is specified
	if c.From == "" && c.To == "" && c.Subject == "" && c.Query == "" && !c.HasAttachment {
		return errors.New("must specify at least one criteria flag (--from, --to, --subject, --query, or --has-attachment)")
	}

	// Validate that at least one action is specified
	if c.AddLabel == "" && c.RemoveLabel == "" && !c.Archive && !c.MarkRead && !c.Star && c.Forward == "" && !c.Trash && !c.NeverSpam && !c.Important {
		return errors.New("must specify at least one action flag (--add-label, --remove-label, --archive, --mark-read, --star, --forward, --trash, --never-spam, or --important)")
	}

	if err := dryRunExit(ctx, flags, "gmail.filters.create", map[string]any{
		"criteria": map[string]any{
			"from":           strings.TrimSpace(c.From),
			"to":             strings.TrimSpace(c.To),
			"subject":        strings.TrimSpace(c.Subject),
			"query":          strings.TrimSpace(c.Query),
			"has_attachment": c.HasAttachment,
		},
		"actions": map[string]any{
			"add_label":    splitCSV(c.AddLabel),
			"remove_label": splitCSV(c.RemoveLabel),
			"archive":      c.Archive,
			"mark_read":    c.MarkRead,
			"star":         c.Star,
			"forward":      strings.TrimSpace(c.Forward),
			"trash":        c.Trash,
			"never_spam":   c.NeverSpam,
			"important":    c.Important,
		},
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	// Build filter criteria
	criteria := &gmail.FilterCriteria{}
	if c.From != "" {
		criteria.From = c.From
	}
	if c.To != "" {
		criteria.To = c.To
	}
	if c.Subject != "" {
		criteria.Subject = c.Subject
	}
	if c.Query != "" {
		criteria.Query = c.Query
	}
	if c.HasAttachment {
		criteria.HasAttachment = true
	}

	// Build filter actions
	action := &gmail.FilterAction{}

	// Resolve label names to IDs for add/remove operations
	var labelMap map[string]string
	if c.AddLabel != "" || c.RemoveLabel != "" {
		labelMap, err = fetchLabelNameToID(svc)
		if err != nil {
			return err
		}
	}

	if c.AddLabel != "" {
		addLabels := splitCSV(c.AddLabel)
		addIDs := resolveLabelIDs(addLabels, labelMap)
		action.AddLabelIds = addIDs
	}

	if c.RemoveLabel != "" {
		removeLabels := splitCSV(c.RemoveLabel)
		removeIDs := resolveLabelIDs(removeLabels, labelMap)
		action.RemoveLabelIds = removeIDs
	}

	if c.Archive {
		// Archive means remove from INBOX
		if action.RemoveLabelIds == nil {
			action.RemoveLabelIds = []string{}
		}
		action.RemoveLabelIds = append(action.RemoveLabelIds, "INBOX")
	}

	if c.MarkRead {
		// Mark as read means remove UNREAD label
		if action.RemoveLabelIds == nil {
			action.RemoveLabelIds = []string{}
		}
		action.RemoveLabelIds = append(action.RemoveLabelIds, "UNREAD")
	}

	if c.Star {
		// Star means add STARRED label
		if action.AddLabelIds == nil {
			action.AddLabelIds = []string{}
		}
		action.AddLabelIds = append(action.AddLabelIds, "STARRED")
	}

	if c.Forward != "" {
		action.Forward = c.Forward
	}

	if c.Trash {
		// Trash means add TRASH label
		if action.AddLabelIds == nil {
			action.AddLabelIds = []string{}
		}
		action.AddLabelIds = append(action.AddLabelIds, "TRASH")
	}

	if c.NeverSpam {
		// Never spam means remove SPAM label
		if action.RemoveLabelIds == nil {
			action.RemoveLabelIds = []string{}
		}
		action.RemoveLabelIds = append(action.RemoveLabelIds, "SPAM")
	}

	if c.Important {
		// Important means add IMPORTANT label
		if action.AddLabelIds == nil {
			action.AddLabelIds = []string{}
		}
		action.AddLabelIds = append(action.AddLabelIds, "IMPORTANT")
	}

	filter := &gmail.Filter{
		Criteria: criteria,
		Action:   action,
	}

	created, err := svc.Users.Settings.Filters.Create("me", filter).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"filter": created})
	}

	u.Out().Println("Filter created successfully")
	u.Out().Printf("id\t%s", created.Id)
	if created.Criteria != nil {
		c := created.Criteria
		if c.From != "" {
			u.Out().Printf("from\t%s", c.From)
		}
		if c.To != "" {
			u.Out().Printf("to\t%s", c.To)
		}
		if c.Subject != "" {
			u.Out().Printf("subject\t%s", c.Subject)
		}
		if c.Query != "" {
			u.Out().Printf("query\t%s", c.Query)
		}
	}
	return nil
}

type GmailFiltersDeleteCmd struct {
	FilterID string `arg:"" name:"filterId" help:"Filter ID"`
}

func (c *GmailFiltersDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	filterID := strings.TrimSpace(c.FilterID)
	if filterID == "" {
		return usage("empty filterId")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete gmail filter %s", filterID)); confirmErr != nil {
		return confirmErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	err = svc.Users.Settings.Filters.Delete("me", filterID).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"success":  true,
			"filterId": filterID,
		})
	}

	u.Out().Printf("Filter %s deleted successfully", filterID)
	return nil
}
