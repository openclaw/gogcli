package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const gmailFilterCreateMaxRetries = 3

var errMatchingGmailFilterNotFound = errors.New("matching gmail filter not found")

var sleepBeforeGmailFilterRetry = func(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func writeGmailFiltersList(ctx context.Context, filters []*gmail.Filter) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"filters": filters})
	}

	u := ui.FromContext(ctx)
	if len(filters) == 0 {
		u.Err().Println("No filters")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tFROM\tTO\tSUBJECT\tQUERY")
	for _, f := range filters {
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
	return tw.Flush()
}

func writeGmailFilter(ctx context.Context, filter *gmail.Filter) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"filter": filter})
	}
	printGmailFilterDetails(ui.FromContext(ctx), filter, true)
	return nil
}

func writeCreatedGmailFilter(ctx context.Context, filter *gmail.Filter) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"filter": filter})
	}

	u := ui.FromContext(ctx)
	u.Out().Println("Filter created successfully")
	printGmailFilterDetails(u, filter, false)
	return nil
}

func printGmailFilterDetails(u *ui.UI, filter *gmail.Filter, includeActions bool) {
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
	if !includeActions || filter.Action == nil {
		return
	}

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

func (c *GmailFiltersCreateCmd) validate() (string, error) {
	forwardTarget := strings.TrimSpace(c.Forward)
	if c.From == "" && c.To == "" && c.Subject == "" && c.Query == "" && !c.HasAttachment {
		return "", errors.New("must specify at least one criteria flag (--from, --to, --subject, --query, or --has-attachment)")
	}
	if c.AddLabel == "" && c.RemoveLabel == "" && !c.Archive && !c.MarkRead && !c.Star && forwardTarget == "" && !c.Trash && !c.NeverSpam && !c.Important {
		return "", errors.New("must specify at least one action flag (--add-label, --remove-label, --archive, --mark-read, --star, --forward, --trash, --never-spam, or --important)")
	}
	return forwardTarget, nil
}

func (c *GmailFiltersCreateCmd) dryRunPayload(forwardTarget string) map[string]any {
	return map[string]any{
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
			"forward":      forwardTarget,
			"trash":        c.Trash,
			"never_spam":   c.NeverSpam,
			"important":    c.Important,
		},
	}
}

func (c *GmailFiltersCreateCmd) buildFilter(svc *gmail.Service, forwardTarget string) (*gmail.Filter, error) {
	action, err := c.buildAction(svc, forwardTarget)
	if err != nil {
		return nil, err
	}
	return &gmail.Filter{
		Criteria: c.buildCriteria(),
		Action:   action,
	}, nil
}

func (c *GmailFiltersCreateCmd) buildCriteria() *gmail.FilterCriteria {
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
	return criteria
}

func (c *GmailFiltersCreateCmd) buildAction(svc *gmail.Service, forwardTarget string) (*gmail.FilterAction, error) {
	action := &gmail.FilterAction{}

	var (
		err      error
		labelMap map[string]string
	)
	if c.AddLabel != "" || c.RemoveLabel != "" {
		labelMap, err = fetchLabelNameToID(svc)
		if err != nil {
			return nil, err
		}
	}

	if c.AddLabel != "" {
		action.AddLabelIds = resolveLabelIDs(splitCSV(c.AddLabel), labelMap)
	}
	if c.RemoveLabel != "" {
		action.RemoveLabelIds = resolveLabelIDs(splitCSV(c.RemoveLabel), labelMap)
	}
	if c.Archive {
		action.RemoveLabelIds = append(action.RemoveLabelIds, "INBOX")
	}
	if c.MarkRead {
		action.RemoveLabelIds = append(action.RemoveLabelIds, "UNREAD")
	}
	if c.Star {
		action.AddLabelIds = append(action.AddLabelIds, "STARRED")
	}
	if forwardTarget != "" {
		action.Forward = forwardTarget
	}
	if c.Trash {
		action.AddLabelIds = append(action.AddLabelIds, "TRASH")
	}
	if c.NeverSpam {
		action.RemoveLabelIds = append(action.RemoveLabelIds, "SPAM")
	}
	if c.Important {
		action.AddLabelIds = append(action.AddLabelIds, "IMPORTANT")
	}

	return action, nil
}

func createGmailFilterWithRetry(ctx context.Context, svc *gmail.Service, filter *gmail.Filter) (*gmail.Filter, error) {
	var lastErr error

	for attempt := 0; attempt <= gmailFilterCreateMaxRetries; attempt++ {
		created, err := svc.Users.Settings.Filters.Create("me", filter).Do()
		if err == nil {
			return created, nil
		}

		lastErr = err

		switch classifyGmailFilterCreateError(err) {
		case gmailFilterCreateDuplicate:
			existing, lookupErr := findMatchingGmailFilter(svc, filter)
			if lookupErr == nil && existing != nil {
				return existing, nil
			}
			if errors.Is(lookupErr, errMatchingGmailFilterNotFound) {
				return nil, err
			}
			if lookupErr != nil {
				return nil, lookupErr
			}
			return nil, err
		case gmailFilterCreateRetryablePrecondition:
			if attempt == gmailFilterCreateMaxRetries {
				return nil, err
			}
			if sleepErr := sleepBeforeGmailFilterRetry(ctx, time.Duration(attempt+1)*300*time.Millisecond); sleepErr != nil {
				return nil, sleepErr
			}
		default:
			return nil, err
		}
	}

	return nil, lastErr
}

type gmailFilterCreateDisposition int

const (
	gmailFilterCreateOther gmailFilterCreateDisposition = iota
	gmailFilterCreateRetryablePrecondition
	gmailFilterCreateDuplicate
)

func classifyGmailFilterCreateError(err error) gmailFilterCreateDisposition {
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) {
		return gmailFilterCreateOther
	}

	reason := ""
	detail := strings.TrimSpace(gerr.Message)
	if len(gerr.Errors) > 0 {
		if detail == "" && strings.TrimSpace(gerr.Errors[0].Message) != "" {
			detail = strings.TrimSpace(gerr.Errors[0].Message)
		}
		reason = strings.TrimSpace(gerr.Errors[0].Reason)
	}

	if gerr.Code != 400 {
		return gmailFilterCreateOther
	}

	lowDetail := strings.ToLower(detail)
	lowReason := strings.ToLower(reason)

	if strings.Contains(lowDetail, "filter already exists") {
		return gmailFilterCreateDuplicate
	}

	if lowReason == "failedprecondition" && (lowDetail == "" || strings.Contains(lowDetail, "precondition check failed")) {
		return gmailFilterCreateRetryablePrecondition
	}

	return gmailFilterCreateOther
}

func findMatchingGmailFilter(svc *gmail.Service, want *gmail.Filter) (*gmail.Filter, error) {
	resp, err := svc.Users.Settings.Filters.List("me").Do()
	if err != nil {
		return nil, err
	}

	for _, existing := range resp.Filter {
		if gmailFiltersEqual(existing, want) {
			return existing, nil
		}
	}

	return nil, errMatchingGmailFilterNotFound
}

func gmailFiltersEqual(a, b *gmail.Filter) bool {
	if a == nil || b == nil {
		return a == b
	}

	return gmailFilterCriteriaEqual(a.Criteria, b.Criteria) &&
		gmailFilterActionEqual(a.Action, b.Action)
}

func gmailFilterCriteriaEqual(a, b *gmail.FilterCriteria) bool {
	if a == nil || b == nil {
		return a == b
	}

	return strings.TrimSpace(a.From) == strings.TrimSpace(b.From) &&
		strings.TrimSpace(a.To) == strings.TrimSpace(b.To) &&
		strings.TrimSpace(a.Subject) == strings.TrimSpace(b.Subject) &&
		strings.TrimSpace(a.Query) == strings.TrimSpace(b.Query) &&
		a.HasAttachment == b.HasAttachment &&
		strings.TrimSpace(a.NegatedQuery) == strings.TrimSpace(b.NegatedQuery) &&
		a.Size == b.Size &&
		strings.TrimSpace(a.SizeComparison) == strings.TrimSpace(b.SizeComparison) &&
		a.ExcludeChats == b.ExcludeChats
}

func gmailFilterActionEqual(a, b *gmail.FilterAction) bool {
	if a == nil || b == nil {
		return a == b
	}

	return sameStrings(a.AddLabelIds, b.AddLabelIds) &&
		sameStrings(a.RemoveLabelIds, b.RemoveLabelIds) &&
		strings.TrimSpace(a.Forward) == strings.TrimSpace(b.Forward)
}

func sameStrings(a, b []string) bool {
	na := normalizeStringSlice(a)
	nb := normalizeStringSlice(b)
	return slices.Equal(na, nb)
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}

	slices.Sort(out)
	return out
}
