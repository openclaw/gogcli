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
	List   GmailLabelsListCmd   `cmd:"" name:"list" help:"List labels"`
	Get    GmailLabelsGetCmd    `cmd:"" name:"get" help:"Get label details (including counts)"`
	Create GmailLabelsCreateCmd `cmd:"" name:"create" help:"Create a new label"`
	Modify GmailLabelsModifyCmd `cmd:"" name:"modify" help:"Modify labels on threads"`
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{"label": l})
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

	label, err := svc.Users.Labels.Create("me", &gmail.Label{
		Name:                  name,
		LabelListVisibility:   "labelShow",
		MessageListVisibility: "show",
	}).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"label": label})
	}
	u.Out().Printf("Created label: %s (id: %s)", label.Name, label.Id)
	return nil
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{"labels": resp.Labels})
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{"results": results})
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
