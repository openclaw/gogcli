package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/drivelabels/v2"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newDriveLabelsService = googleapi.NewDriveLabels

type LabelsCmd struct {
	List    LabelsListCmd    `cmd:"" name:"list" aliases:"ls" help:"List Drive labels"`
	Get     LabelsGetCmd     `cmd:"" name:"get" help:"Get a Drive label"`
	Create  LabelsCreateCmd  `cmd:"" name:"create" help:"Create a Drive label"`
	Update  LabelsUpdateCmd  `cmd:"" name:"update" help:"Update a Drive label"`
	Delete  LabelsDeleteCmd  `cmd:"" name:"delete" aliases:"rm" help:"Delete a Drive label"`
	Publish LabelsPublishCmd `cmd:"" name:"publish" help:"Publish a Drive label"`
	Disable LabelsDisableCmd `cmd:"" name:"disable" help:"Disable a Drive label"`
}

type LabelsListCmd struct {
	Max  int64  `name:"max" aliases:"limit" default:"50" help:"Max results"`
	Page string `name:"page" help:"Page token"`
}

func (c *LabelsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newDriveLabelsService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Labels.List()
	if c.Max > 0 {
		call = call.PageSize(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list labels: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Labels) == 0 {
		u.Err().Println("No labels found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tTITLE\tTYPE\tSTATE")
	for _, label := range resp.Labels {
		if label == nil {
			continue
		}
		title := ""
		if label.Properties != nil {
			title = label.Properties.Title
		}
		state := ""
		if label.Lifecycle != nil {
			state = label.Lifecycle.State
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(label.Name),
			sanitizeTab(title),
			sanitizeTab(label.LabelType),
			sanitizeTab(state),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type LabelsGetCmd struct {
	LabelID string `arg:"" name:"label-id" help:"Label ID or name"`
}

func (c *LabelsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	label := strings.TrimSpace(c.LabelID)
	if label == "" {
		return usage("label-id is required")
	}
	label = normalizeLabelName(label)

	svc, err := newDriveLabelsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Labels.Get(label).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get label %s: %w", label, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	title := ""
	if resp.Properties != nil {
		title = resp.Properties.Title
	}
	u.Out().Printf("Name: %s\n", resp.Name)
	u.Out().Printf("Title: %s\n", title)
	u.Out().Printf("Type: %s\n", resp.LabelType)
	if resp.Lifecycle != nil {
		u.Out().Printf("State: %s\n", resp.Lifecycle.State)
	}
	return nil
}

type LabelsCreateCmd struct {
	Name string `name:"name" help:"Label title" required:""`
	Type string `name:"type" help:"Label type: ADMIN|SHARED" default:"ADMIN"`
}

func (c *LabelsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("--name is required")
	}

	labelType := strings.ToUpper(strings.TrimSpace(c.Type))
	switch labelType {
	case "ADMIN", "SHARED":
	default:
		return usage("invalid --type (expected ADMIN|SHARED)")
	}

	svc, err := newDriveLabelsService(ctx, account)
	if err != nil {
		return err
	}

	label := &drivelabels.GoogleAppsDriveLabelsV2Label{
		LabelType: labelType,
		Properties: &drivelabels.GoogleAppsDriveLabelsV2LabelProperties{
			Title: name,
		},
	}
	created, err := svc.Labels.Create(label).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create label: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u.Out().Printf("Created label: %s\n", created.Name)
	return nil
}

type LabelsUpdateCmd struct {
	LabelID string  `arg:"" name:"label-id" help:"Label ID or name"`
	Name    *string `name:"name" help:"New label title"`
}

func (c *LabelsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	label := strings.TrimSpace(c.LabelID)
	if label == "" {
		return usage("label-id is required")
	}
	label = normalizeLabelName(label)

	if c.Name == nil {
		return usage("no updates specified")
	}
	newTitle := strings.TrimSpace(*c.Name)
	if newTitle == "" {
		return usage("--name cannot be empty")
	}

	svc, err := newDriveLabelsService(ctx, account)
	if err != nil {
		return err
	}

	req := &drivelabels.GoogleAppsDriveLabelsV2DeltaUpdateLabelRequest{
		Requests: []*drivelabels.GoogleAppsDriveLabelsV2DeltaUpdateLabelRequestRequest{
			{
				UpdateLabel: &drivelabels.GoogleAppsDriveLabelsV2DeltaUpdateLabelRequestUpdateLabelPropertiesRequest{
					Properties: &drivelabels.GoogleAppsDriveLabelsV2LabelProperties{Title: newTitle},
					UpdateMask: "title",
				},
			},
		},
	}

	updated, err := svc.Labels.Delta(label, req).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("update label: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, updated)
	}

	u.Out().Printf("Updated label: %s\n", label)
	return nil
}

type LabelsDeleteCmd struct {
	LabelID string `arg:"" name:"label-id" help:"Label ID or name"`
}

func (c *LabelsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	label := strings.TrimSpace(c.LabelID)
	if label == "" {
		return usage("label-id is required")
	}
	label = normalizeLabelName(label)

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete label %s", label)); err != nil {
		return err
	}

	svc, err := newDriveLabelsService(ctx, account)
	if err != nil {
		return err
	}

	if _, err := svc.Labels.Delete(label).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete label: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"label": label, "deleted": true})
	}

	u.Out().Printf("Deleted label: %s\n", label)
	return nil
}

type LabelsPublishCmd struct {
	LabelID string `arg:"" name:"label-id" help:"Label ID or name"`
}

func (c *LabelsPublishCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	label := strings.TrimSpace(c.LabelID)
	if label == "" {
		return usage("label-id is required")
	}
	label = normalizeLabelName(label)

	svc, err := newDriveLabelsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Labels.Publish(label, &drivelabels.GoogleAppsDriveLabelsV2PublishLabelRequest{}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("publish label: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	u.Out().Printf("Published label: %s\n", label)
	return nil
}

type LabelsDisableCmd struct {
	LabelID string `arg:"" name:"label-id" help:"Label ID or name"`
}

func (c *LabelsDisableCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	label := strings.TrimSpace(c.LabelID)
	if label == "" {
		return usage("label-id is required")
	}
	label = normalizeLabelName(label)

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("disable label %s", label)); err != nil {
		return err
	}

	svc, err := newDriveLabelsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Labels.Disable(label, &drivelabels.GoogleAppsDriveLabelsV2DisableLabelRequest{}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("disable label: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	u.Out().Printf("Disabled label: %s\n", label)
	return nil
}

func normalizeLabelName(input string) string {
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(trimmed, "labels/") {
		return trimmed
	}
	return "labels/" + trimmed
}
