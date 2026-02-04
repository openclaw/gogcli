package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	alertcenter "google.golang.org/api/alertcenter/v1beta1"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newAlertCenterService = googleapi.NewAlertCenter

type AlertsCmd struct {
	List     AlertsListCmd     `cmd:"" name:"list" aliases:"ls" help:"List alerts"`
	Get      AlertsGetCmd      `cmd:"" name:"get" help:"Get alert"`
	Delete   AlertsDeleteCmd   `cmd:"" name:"delete" aliases:"rm" help:"Delete alert"`
	Undelete AlertsUndeleteCmd `cmd:"" name:"undelete" help:"Undelete alert"`
	Feedback AlertsFeedbackCmd `cmd:"" name:"feedback" help:"Manage alert feedback"`
	Settings AlertsSettingsCmd `cmd:"" name:"settings" help:"Alert settings"`
}

type AlertsListCmd struct {
	Filter  string `name:"filter" help:"Filter alerts"`
	OrderBy string `name:"order-by" help:"Order by (e.g. create_time desc)"`
	Max     int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page    string `name:"page" help:"Page token"`
}

func (c *AlertsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAlertCenterService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Alerts.List()
	if c.Filter != "" {
		call = call.Filter(c.Filter)
	}
	if c.OrderBy != "" {
		call = call.OrderBy(c.OrderBy)
	}
	if c.Max > 0 {
		call = call.PageSize(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list alerts: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Alerts) == 0 {
		u.Err().Println("No alerts found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ALERT ID\tTYPE\tSOURCE\tCREATED\tUPDATED\tDELETED")
	for _, alert := range resp.Alerts {
		if alert == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%t\n",
			sanitizeTab(alert.AlertId),
			sanitizeTab(alert.Type),
			sanitizeTab(alert.Source),
			sanitizeTab(alert.CreateTime),
			sanitizeTab(alert.UpdateTime),
			alert.Deleted,
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type AlertsGetCmd struct {
	AlertID string `arg:"" name:"alert-id" help:"Alert ID"`
}

func (c *AlertsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAlertCenterService(ctx, account)
	if err != nil {
		return err
	}

	alert, err := svc.Alerts.Get(c.AlertID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get alert %s: %w", c.AlertID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, alert)
	}

	u.Out().Printf("Alert ID:    %s\n", alert.AlertId)
	u.Out().Printf("Type:        %s\n", alert.Type)
	u.Out().Printf("Source:      %s\n", alert.Source)
	u.Out().Printf("Created:     %s\n", alert.CreateTime)
	u.Out().Printf("Updated:     %s\n", alert.UpdateTime)
	u.Out().Printf("Deleted:     %t\n", alert.Deleted)
	if alert.StartTime != "" {
		u.Out().Printf("Start Time:  %s\n", alert.StartTime)
	}
	if alert.EndTime != "" {
		u.Out().Printf("End Time:    %s\n", alert.EndTime)
	}
	return nil
}

type AlertsDeleteCmd struct {
	AlertID string `arg:"" name:"alert-id" help:"Alert ID"`
}

func (c *AlertsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if err = confirmDestructive(ctx, flags, fmt.Sprintf("delete alert %s", c.AlertID)); err != nil {
		return err
	}

	svc, err := newAlertCenterService(ctx, account)
	if err != nil {
		return err
	}

	if _, err := svc.Alerts.Delete(c.AlertID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete alert %s: %w", c.AlertID, err)
	}

	u.Out().Printf("Deleted alert: %s\n", c.AlertID)
	return nil
}

type AlertsUndeleteCmd struct {
	AlertID string `arg:"" name:"alert-id" help:"Alert ID"`
}

func (c *AlertsUndeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAlertCenterService(ctx, account)
	if err != nil {
		return err
	}

	if _, err := svc.Alerts.Undelete(c.AlertID, &alertcenter.UndeleteAlertRequest{}).Context(ctx).Do(); err != nil {
		return fmt.Errorf("undelete alert %s: %w", c.AlertID, err)
	}

	u.Out().Printf("Undeleted alert: %s\n", c.AlertID)
	return nil
}

type AlertsFeedbackCmd struct {
	List   AlertsFeedbackListCmd   `cmd:"" name:"list" help:"List feedback for alert"`
	Create AlertsFeedbackCreateCmd `cmd:"" name:"create" help:"Create feedback for alert"`
}

type AlertsFeedbackListCmd struct {
	AlertID string `name:"alert" help:"Alert ID"`
}

func (c *AlertsFeedbackListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	alertID := strings.TrimSpace(c.AlertID)
	if alertID == "" {
		return usage("--alert is required")
	}

	svc, err := newAlertCenterService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Alerts.Feedback.List(alertID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list feedback for %s: %w", alertID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Feedback) == 0 {
		u.Err().Println("No feedback found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "FEEDBACK ID\tTYPE\tEMAIL\tCREATED")
	for _, fb := range resp.Feedback {
		if fb == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(fb.FeedbackId),
			sanitizeTab(fb.Type),
			sanitizeTab(fb.Email),
			sanitizeTab(fb.CreateTime),
		)
	}
	return nil
}

type AlertsFeedbackCreateCmd struct {
	AlertID string `arg:"" name:"alert-id" help:"Alert ID"`
	Type    string `name:"type" required:"" enum:"NOT_USEFUL,SOMEWHAT_USEFUL,VERY_USEFUL" help:"Feedback type"`
}

func (c *AlertsFeedbackCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAlertCenterService(ctx, account)
	if err != nil {
		return err
	}

	feedback := &alertcenter.AlertFeedback{Type: c.Type}
	created, err := svc.Alerts.Feedback.Create(c.AlertID, feedback).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create feedback for %s: %w", c.AlertID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u.Out().Printf("Created feedback for alert: %s\n", c.AlertID)
	return nil
}

type AlertsSettingsCmd struct {
	Get    AlertsSettingsGetCmd    `cmd:"" name:"get" help:"Get alert settings"`
	Update AlertsSettingsUpdateCmd `cmd:"" name:"update" help:"Update alert settings"`
}

type AlertsSettingsGetCmd struct{}

func (c *AlertsSettingsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAlertCenterService(ctx, account)
	if err != nil {
		return err
	}

	settings, err := svc.V1beta1.GetSettings().Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get alert settings: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, settings)
	}

	u.Out().Printf("Notifications: %d\n", len(settings.Notifications))
	for _, n := range settings.Notifications {
		if n == nil || n.CloudPubsubTopic == nil {
			continue
		}
		u.Out().Printf("- %s\n", n.CloudPubsubTopic.TopicName)
	}
	return nil
}

type AlertsSettingsUpdateCmd struct {
	Notifications string `name:"notifications" help:"Comma-separated Pub/Sub topic names"`
}

func (c *AlertsSettingsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if strings.TrimSpace(c.Notifications) == "" {
		return usage("--notifications is required")
	}

	svc, err := newAlertCenterService(ctx, account)
	if err != nil {
		return err
	}

	topics := splitCSV(c.Notifications)
	notifications := make([]*alertcenter.Notification, 0, len(topics))
	for _, topic := range topics {
		topic = strings.TrimSpace(topic)
		if topic == "" {
			continue
		}
		notifications = append(notifications, &alertcenter.Notification{
			CloudPubsubTopic: &alertcenter.CloudPubsubTopic{TopicName: topic},
		})
	}
	if len(notifications) == 0 {
		return usage("no notifications specified")
	}

	settings := &alertcenter.Settings{Notifications: notifications}
	settings.ForceSendFields = append(settings.ForceSendFields, "Notifications")

	updated, err := svc.V1beta1.UpdateSettings(settings).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("update alert settings: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, updated)
	}

	u.Out().Printf("Updated alert settings (notifications: %d)\n", len(updated.Notifications))
	return nil
}
