package cmd

import (
	"context"
	"os"
	"strings"

	formsapi "google.golang.org/api/forms/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// FormsWatchCmd groups watch subcommands.
type FormsWatchCmd struct {
	Create FormsWatchCreateCmd `cmd:"" name:"create" aliases:"new,add" help:"Create a watch for new responses"`
	List   FormsWatchListCmd   `cmd:"" name:"list" aliases:"ls" help:"List active watches"`
	Delete FormsWatchDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove" help:"Delete a watch"`
	Renew  FormsWatchRenewCmd  `cmd:"" name:"renew" aliases:"refresh" help:"Renew a watch (extends 7 days)"`
}

// FormsWatchCreateCmd creates a push notification watch on form responses.
type FormsWatchCreateCmd struct {
	FormID    string `arg:"" name:"formId" help:"Form ID"`
	TopicID   string `name:"topic" help:"Cloud Pub/Sub topic name (projects/{project}/topics/{topic})" required:""`
	EventType string `name:"event-type" help:"Event type to watch" default:"RESPONSES" enum:"RESPONSES,SCHEMA"`
}

func (c *FormsWatchCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	formID := strings.TrimSpace(normalizeGoogleID(c.FormID))
	if formID == "" {
		return usage("empty formId")
	}
	topicID := strings.TrimSpace(c.TopicID)
	if topicID == "" {
		return usage("empty --topic")
	}

	if dryRunErr := dryRunExit(ctx, flags, "forms.watches.create", map[string]any{
		"form_id":    formID,
		"topic":      topicID,
		"event_type": c.EventType,
	}); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := newFormsService(ctx, account)
	if err != nil {
		return err
	}

	req := &formsapi.CreateWatchRequest{
		Watch: &formsapi.Watch{
			Target: &formsapi.WatchTarget{
				Topic: &formsapi.CloudPubsubTopic{
					TopicName: topicID,
				},
			},
			EventType: c.EventType,
		},
	}

	watch, err := svc.Forms.Watches.Create(formID, req).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"created": true,
			"form_id": formID,
			"watch":   watch,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("created\ttrue")
	u.Out().Printf("watch_id\t%s", watch.Id)
	u.Out().Printf("form_id\t%s", formID)
	u.Out().Printf("event_type\t%s", watch.EventType)
	u.Out().Printf("state\t%s", watch.State)
	if watch.ExpireTime != "" {
		u.Out().Printf("expires\t%s", watch.ExpireTime)
	}
	return nil
}

// FormsWatchListCmd lists active watches for a form.
type FormsWatchListCmd struct {
	FormID string `arg:"" name:"formId" help:"Form ID"`
}

func (c *FormsWatchListCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	formID := strings.TrimSpace(normalizeGoogleID(c.FormID))
	if formID == "" {
		return usage("empty formId")
	}

	svc, err := newFormsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Forms.Watches.List(formID).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"form_id": formID,
			"watches": resp.Watches,
		})
	}

	u := ui.FromContext(ctx)
	if len(resp.Watches) == 0 {
		u.Out().Println("No active watches.")
		return nil
	}
	u.Out().Println("WATCH_ID\tEVENT_TYPE\tSTATE\tEXPIRES")
	for _, w := range resp.Watches {
		if w == nil {
			continue
		}
		u.Out().Printf("%s\t%s\t%s\t%s", w.Id, w.EventType, w.State, w.ExpireTime)
	}
	return nil
}

// FormsWatchDeleteCmd removes a watch.
type FormsWatchDeleteCmd struct {
	FormID  string `arg:"" name:"formId" help:"Form ID"`
	WatchID string `arg:"" name:"watchId" help:"Watch ID"`
}

func (c *FormsWatchDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	formID := strings.TrimSpace(normalizeGoogleID(c.FormID))
	if formID == "" {
		return usage("empty formId")
	}
	watchID := strings.TrimSpace(c.WatchID)
	if watchID == "" {
		return usage("empty watchId")
	}

	if dryRunErr := dryRunExit(ctx, flags, "forms.watches.delete", map[string]any{
		"form_id":  formID,
		"watch_id": watchID,
	}); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := newFormsService(ctx, account)
	if err != nil {
		return err
	}

	if _, err := svc.Forms.Watches.Delete(formID, watchID).Context(ctx).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"deleted":  true,
			"form_id":  formID,
			"watch_id": watchID,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("deleted\ttrue")
	u.Out().Printf("form_id\t%s", formID)
	u.Out().Printf("watch_id\t%s", watchID)
	return nil
}

// FormsWatchRenewCmd renews an existing watch for another 7 days.
type FormsWatchRenewCmd struct {
	FormID  string `arg:"" name:"formId" help:"Form ID"`
	WatchID string `arg:"" name:"watchId" help:"Watch ID"`
}

func (c *FormsWatchRenewCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	formID := strings.TrimSpace(normalizeGoogleID(c.FormID))
	if formID == "" {
		return usage("empty formId")
	}
	watchID := strings.TrimSpace(c.WatchID)
	if watchID == "" {
		return usage("empty watchId")
	}

	if dryRunErr := dryRunExit(ctx, flags, "forms.watches.renew", map[string]any{
		"form_id":  formID,
		"watch_id": watchID,
	}); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := newFormsService(ctx, account)
	if err != nil {
		return err
	}

	watch, err := svc.Forms.Watches.Renew(formID, watchID, &formsapi.RenewWatchRequest{}).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"renewed": true,
			"form_id": formID,
			"watch":   watch,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("renewed\ttrue")
	u.Out().Printf("watch_id\t%s", watch.Id)
	u.Out().Printf("form_id\t%s", formID)
	u.Out().Printf("expires\t%s", watch.ExpireTime)
	return nil
}
