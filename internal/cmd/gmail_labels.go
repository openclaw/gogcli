package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailLabelsCmd struct {
	List   GmailLabelsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List labels"`
	Get    GmailLabelsGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get label details (including counts)"`
	Create GmailLabelsCreateCmd `cmd:"" name:"create" aliases:"add,new" help:"Create a new label"`
	Rename GmailLabelsRenameCmd `cmd:"" name:"rename" aliases:"mv" help:"Rename a label"`
	Style  GmailLabelsStyleCmd  `cmd:"" name:"style" aliases:"color,colour" help:"Change a user label color or visibility"`
	Modify GmailLabelsModifyCmd `cmd:"" name:"modify" aliases:"update,edit,set" help:"Modify labels on threads"`
	Delete GmailLabelsDeleteCmd `cmd:"" name:"delete" aliases:"rm,del" help:"Delete a label"`
}

type GmailLabelsGetCmd struct {
	Label string `arg:"" name:"labelIdOrName" help:"Label ID or name"`
}

func (c *GmailLabelsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	idMap, err := fetchLabelNameToID(svc)
	if err != nil {
		return err
	}
	raw := strings.TrimSpace(c.Label)
	if raw == "" {
		return usage("empty label")
	}
	id := raw
	if v, ok := idMap[strings.ToLower(raw)]; ok {
		id = v
	}

	l, err := svc.Users.Labels.Get("me", id).Context(ctx).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"label": l})
	}
	u := ui.FromContext(ctx)
	u.Out().Printf("id\t%s", l.Id)
	u.Out().Printf("name\t%s", l.Name)
	u.Out().Printf("type\t%s", l.Type)
	u.Out().Printf("messages_total\t%d", l.MessagesTotal)
	u.Out().Printf("messages_unread\t%d", l.MessagesUnread)
	u.Out().Printf("threads_total\t%d", l.ThreadsTotal)
	u.Out().Printf("threads_unread\t%d", l.ThreadsUnread)
	return nil
}

type GmailLabelsCreateCmd struct {
	Name string `arg:"" help:"Label name"`
}

func (c *GmailLabelsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("label name is required")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	err = ensureLabelNameAvailable(svc, name)
	if err != nil {
		return err
	}

	label, err := createLabel(ctx, svc, name)
	if err != nil {
		return mapLabelCreateError(err, name)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"label": label})
	}
	u.Out().Printf("Created label: %s (id: %s)", label.Name, label.Id)
	return nil
}

func createLabel(ctx context.Context, svc *gmail.Service, name string) (*gmail.Label, error) {
	return svc.Users.Labels.Create("me", &gmail.Label{
		Name:                  name,
		LabelListVisibility:   "labelShow",
		MessageListVisibility: "show",
	}).Context(ctx).Do()
}

type GmailLabelsRenameCmd struct {
	Label   string `arg:"" name:"labelIdOrName" help:"Current label ID or name"`
	NewName string `arg:"" name:"newName" help:"New label name"`
}

func (c *GmailLabelsRenameCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	oldRaw := strings.TrimSpace(c.Label)
	if oldRaw == "" {
		return usage("label is required")
	}
	newName := strings.TrimSpace(c.NewName)
	if newName == "" {
		return usage("new name is required")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	label, err := resolveMutableGmailLabel(ctx, svc, oldRaw)
	if err != nil {
		return err
	}

	if label.Type == "system" {
		return fmt.Errorf("cannot rename system label %q", label.Name)
	}

	if validateErr := ensureLabelNameAvailable(svc, newName); validateErr != nil {
		return validateErr
	}

	if exit := dryRunExit(ctx, flags, "gmail.labels.rename", map[string]string{
		"id":      label.Id,
		"oldName": label.Name,
		"newName": newName,
	}); exit != nil {
		return exit
	}

	updated, err := svc.Users.Labels.Patch("me", label.Id, &gmail.Label{
		Name: newName,
	}).Context(ctx).Do()
	if err != nil {
		return mapLabelCreateError(err, newName)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"label": updated})
	}
	u.Out().Printf("Renamed label: %s → %s (id: %s)", label.Name, updated.Name, updated.Id)
	return nil
}

type GmailLabelsStyleCmd struct {
	Label                 string `arg:"" name:"labelIdOrName" help:"User label ID or name"`
	TextColor             string `name:"text-color" help:"Text color as #RRGGBB"`
	BackgroundColor       string `name:"background-color" help:"Background color as #RRGGBB"`
	LabelListVisibility   string `name:"label-list-visibility" help:"Label-list visibility: labelShow|labelShowIfUnread|labelHide"`
	MessageListVisibility string `name:"message-list-visibility" help:"Message-list visibility: show|hide"`
}

func (c *GmailLabelsStyleCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	label, err := resolveMutableGmailLabel(ctx, svc, c.Label)
	if err != nil {
		return err
	}
	if label.Type == "system" {
		return fmt.Errorf("cannot style system label %q", label.Name)
	}

	textColor, err := normalizeGmailLabelHexColor(c.TextColor, "--text-color")
	if err != nil {
		return err
	}
	backgroundColor, err := normalizeGmailLabelHexColor(c.BackgroundColor, "--background-color")
	if err != nil {
		return err
	}
	if textColor == "" && backgroundColor == "" && c.LabelListVisibility == "" && c.MessageListVisibility == "" {
		return usage("specify at least one style field")
	}
	if validateErr := validateGmailLabelVisibility(c.LabelListVisibility, "--label-list-visibility", "labelShow", "labelShowIfUnread", "labelHide"); validateErr != nil {
		return validateErr
	}
	if validateErr := validateGmailLabelVisibility(c.MessageListVisibility, "--message-list-visibility", "show", "hide"); validateErr != nil {
		return validateErr
	}

	patch := &gmail.Label{
		LabelListVisibility:   strings.TrimSpace(c.LabelListVisibility),
		MessageListVisibility: strings.TrimSpace(c.MessageListVisibility),
	}
	if textColor != "" || backgroundColor != "" {
		color := &gmail.LabelColor{}
		if label.Color != nil {
			color.TextColor = label.Color.TextColor
			color.BackgroundColor = label.Color.BackgroundColor
		}
		if textColor != "" {
			color.TextColor = textColor
		}
		if backgroundColor != "" {
			color.BackgroundColor = backgroundColor
		}
		patch.Color = color
	}

	if exit := dryRunExit(ctx, flags, "gmail.labels.style", map[string]any{
		"id":                    label.Id,
		"name":                  label.Name,
		"textColor":             textColor,
		"backgroundColor":       backgroundColor,
		"labelListVisibility":   c.LabelListVisibility,
		"messageListVisibility": c.MessageListVisibility,
	}); exit != nil {
		return exit
	}

	updated, err := svc.Users.Labels.Patch("me", label.Id, patch).Context(ctx).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"label": updated})
	}
	u.Out().Printf("Styled label: %s (id: %s)", updated.Name, updated.Id)
	return nil
}

func validateGmailLabelVisibility(value, field string, allowed ...string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return usagef("%s must be one of: %s", field, strings.Join(allowed, ", "))
}

type GmailLabelsListCmd struct{}

func (c *GmailLabelsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Users.Labels.List("me").Context(ctx).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"labels": resp.Labels})
	}
	if len(resp.Labels) == 0 {
		u.Err().Println("No labels")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tNAME\tTYPE")
	for _, l := range resp.Labels {
		fmt.Fprintf(w, "%s\t%s\t%s\n", l.Id, l.Name, l.Type)
	}
	return nil
}

type GmailLabelsModifyCmd struct {
	ThreadIDs []string `arg:"" name:"threadId" help:"Thread IDs"`
	Add       string   `name:"add" help:"Labels to add (comma-separated, name or ID)"`
	Remove    string   `name:"remove" help:"Labels to remove (comma-separated, name or ID)"`
}

func (c *GmailLabelsModifyCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	threadIDs := c.ThreadIDs
	addLabels := splitCSV(c.Add)
	removeLabels := splitCSV(c.Remove)
	if len(addLabels) == 0 && len(removeLabels) == 0 {
		return usage("must specify --add and/or --remove")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	idMap, err := fetchLabelNameToID(svc)
	if err != nil {
		return err
	}

	addIDs := resolveLabelIDs(addLabels, idMap)
	removeIDs := resolveLabelIDs(removeLabels, idMap)

	type result struct {
		ThreadID string `json:"threadId"`
		Success  bool   `json:"success"`
		Error    string `json:"error,omitempty"`
	}
	results := make([]result, 0, len(threadIDs))

	for _, tid := range threadIDs {
		_, err := svc.Users.Threads.Modify("me", tid, &gmail.ModifyThreadRequest{
			AddLabelIds:    addIDs,
			RemoveLabelIds: removeIDs,
		}).Context(ctx).Do()
		if err != nil {
			results = append(results, result{ThreadID: tid, Success: false, Error: err.Error()})
			if !outfmt.IsJSON(ctx) {
				u.Err().Errorf("%s: %s", tid, err.Error())
			}
			continue
		}
		results = append(results, result{ThreadID: tid, Success: true})
		if !outfmt.IsJSON(ctx) {
			u.Out().Printf("%s\tok", tid)
		}
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"results": results})
	}
	return nil
}

func fetchLabelNameToID(svc *gmail.Service) (map[string]string, error) {
	resp, err := svc.Users.Labels.List("me").Do()
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(resp.Labels))
	for _, l := range resp.Labels {
		if l.Id == "" {
			continue
		}
		m[strings.ToLower(l.Id)] = l.Id
		if l.Name != "" {
			m[strings.ToLower(l.Name)] = l.Id
		}
	}
	return m, nil
}

func fetchLabelNameOnlyToID(svc *gmail.Service) (map[string]string, error) {
	resp, err := svc.Users.Labels.List("me").Do()
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(resp.Labels))
	for _, l := range resp.Labels {
		if l.Id == "" || l.Name == "" {
			continue
		}
		m[strings.ToLower(l.Name)] = l.Id
	}
	return m, nil
}

type GmailLabelsDeleteCmd struct {
	Label string `arg:"" name:"labelIdOrName" help:"Label ID or name"`
}

func (c *GmailLabelsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	label, err := resolveMutableGmailLabel(ctx, svc, c.Label)
	if err != nil {
		return err
	}

	// System labels cannot be deleted
	if label.Type == "system" {
		return fmt.Errorf("cannot delete system label %q", label.Name)
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete label %q", label.Name)); confirmErr != nil {
		return confirmErr
	}

	if err := svc.Users.Labels.Delete("me", label.Id).Context(ctx).Do(); err != nil {
		return err
	}

	return writeResult(ctx, u,
		kv("deleted", true),
		kv("id", label.Id),
		kv("name", label.Name),
	)
}

func fetchLabelIDToName(svc *gmail.Service) (map[string]string, error) {
	resp, err := svc.Users.Labels.List("me").Do()
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(resp.Labels))
	for _, l := range resp.Labels {
		if l.Id == "" {
			continue
		}
		if l.Name != "" {
			m[l.Id] = l.Name
		} else {
			m[l.Id] = l.Id
		}
	}
	return m, nil
}
