package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/cloudresourcemanager/v3"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newCloudResourceService = googleapi.NewCloudResourceManager

type ProjectsCmd struct {
	List   ProjectsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List GCP projects"`
	Get    ProjectsGetCmd    `cmd:"" name:"get" help:"Get a GCP project"`
	Create ProjectsCreateCmd `cmd:"" name:"create" help:"Create a GCP project"`
	Delete ProjectsDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete a GCP project"`
}

type ProjectsListCmd struct {
	Parent      string `name:"parent" help:"Parent resource (organizations/ID or folders/ID)" required:""`
	Max         int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page        string `name:"page" help:"Page token"`
	ShowDeleted bool   `name:"show-deleted" help:"Include deleted projects"`
}

func (c *ProjectsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	parent := strings.TrimSpace(c.Parent)
	if parent == "" {
		return usage("--parent is required")
	}

	svc, err := newCloudResourceService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Projects.List().Parent(parent).PageSize(c.Max).ShowDeleted(c.ShowDeleted)
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}
	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Projects) == 0 {
		u.Err().Println("No projects found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "PROJECT ID\tNAME\tSTATE")
	for _, project := range resp.Projects {
		if project == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(project.ProjectId),
			sanitizeTab(project.DisplayName),
			sanitizeTab(project.State),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type ProjectsGetCmd struct {
	Project string `arg:"" name:"project" help:"Project ID or resource name"`
}

func (c *ProjectsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	project := strings.TrimSpace(c.Project)
	if project == "" {
		return usage("project is required")
	}

	svc, err := newCloudResourceService(ctx, account)
	if err != nil {
		return err
	}

	name := normalizeProjectName(project)
	resp, err := svc.Projects.Get(name).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get project %s: %w", project, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	fmt.Fprintf(os.Stdout, "Project ID: %s\n", resp.ProjectId)
	fmt.Fprintf(os.Stdout, "Name:       %s\n", resp.DisplayName)
	fmt.Fprintf(os.Stdout, "State:      %s\n", resp.State)
	fmt.Fprintf(os.Stdout, "Parent:     %s\n", resp.Parent)
	return nil
}

type ProjectsCreateCmd struct {
	ID     string `name:"id" help:"Project ID" required:""`
	Name   string `name:"name" help:"Display name" required:""`
	Parent string `name:"parent" help:"Parent resource (organizations/ID or folders/ID)" required:""`
}

func (c *ProjectsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	projectID := strings.TrimSpace(c.ID)
	name := strings.TrimSpace(c.Name)
	parent := strings.TrimSpace(c.Parent)
	if projectID == "" || name == "" || parent == "" {
		return usage("--id, --name, and --parent are required")
	}

	svc, err := newCloudResourceService(ctx, account)
	if err != nil {
		return err
	}

	project := &cloudresourcemanager.Project{
		ProjectId:   projectID,
		DisplayName: name,
		Parent:      parent,
	}
	op, err := svc.Projects.Create(project).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create project: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, op)
	}

	u.Out().Printf("Requested creation of project %s\n", projectID)
	if op.Name != "" {
		u.Out().Printf("Operation: %s\n", op.Name)
	}
	return nil
}

type ProjectsDeleteCmd struct {
	Project string `arg:"" name:"project" help:"Project ID or resource name"`
}

func (c *ProjectsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	project := strings.TrimSpace(c.Project)
	if project == "" {
		return usage("project is required")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete project %s", project)); err != nil {
		return err
	}

	svc, err := newCloudResourceService(ctx, account)
	if err != nil {
		return err
	}

	name := normalizeProjectName(project)
	op, err := svc.Projects.Delete(name).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("delete project %s: %w", project, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, op)
	}

	u.Out().Printf("Requested deletion of project %s\n", project)
	if op.Name != "" {
		u.Out().Printf("Operation: %s\n", op.Name)
	}
	return nil
}

func normalizeProjectName(project string) string {
	if strings.HasPrefix(project, "projects/") {
		return project
	}
	return "projects/" + project
}
