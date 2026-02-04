package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type ResourcesFeaturesCmd struct {
	List   ResourcesFeaturesListCmd   `cmd:"" name:"list" aliases:"ls" help:"List resource features"`
	Create ResourcesFeaturesCreateCmd `cmd:"" name:"create" aliases:"add" help:"Create resource feature"`
	Delete ResourcesFeaturesDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete resource feature"`
}

type ResourcesFeaturesListCmd struct {
	Max  int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page string `name:"page" help:"Page token"`
}

func (c *ResourcesFeaturesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Resources.Features.List(adminCustomerID())
	if c.Max > 0 {
		call = call.MaxResults(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list features: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Features) == 0 {
		u.Err().Println("No features found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME")
	for _, feature := range resp.Features {
		if feature == nil {
			continue
		}
		fmt.Fprintf(w, "%s\n", sanitizeTab(feature.Name))
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type ResourcesFeaturesCreateCmd struct {
	Name string `name:"name" help:"Feature name" required:""`
}

func (c *ResourcesFeaturesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("--name is required")
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	feature := &admin.Feature{Name: name}
	created, err := svc.Resources.Features.Insert(adminCustomerID(), feature).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create feature %s: %w", name, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u.Out().Printf("Created feature: %s\n", created.Name)
	return nil
}

type ResourcesFeaturesDeleteCmd struct {
	Name string `arg:"" name:"name" help:"Feature name"`
}

func (c *ResourcesFeaturesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("feature name is required")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete feature %s", name)); err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Resources.Features.Delete(adminCustomerID(), name).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete feature %s: %w", name, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"name": name, "deleted": true})
	}

	u.Out().Printf("Deleted feature: %s\n", name)
	return nil
}
