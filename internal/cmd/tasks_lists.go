package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/tasks/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type TasksListsCmd struct {
	List   TasksListsListCmd   `cmd:"" default:"withargs" help:"List task lists"`
	Create TasksListsCreateCmd `cmd:"" name:"create" help:"Create a task list" aliases:"add,new"`
}

type TasksListsListCmd struct {
	Max  int64  `name:"max" aliases:"limit" help:"Max results (max allowed: 1000)" default:"100"`
	Page string `name:"page" help:"Page token"`
}

func (c *TasksListsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newTasksService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Tasklists.List().MaxResults(c.Max).PageToken(c.Page)
	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"tasklists":     resp.Items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Items) == 0 {
		u.Err().Println("No task lists")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tTITLE")
	for _, tl := range resp.Items {
		fmt.Fprintf(w, "%s\t%s\n", tl.Id, tl.Title)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type TasksListsCreateCmd struct {
	Title []string `arg:"" name:"title" help:"Task list title"`
}

func (c *TasksListsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	title := strings.TrimSpace(strings.Join(c.Title, " "))
	if title == "" {
		return usage("empty title")
	}

	svc, err := newTasksService(ctx, account)
	if err != nil {
		return err
	}

	created, err := svc.Tasklists.Insert(&tasks.TaskList{Title: title}).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"tasklist": created})
	}
	u.Out().Printf("id\t%s", created.Id)
	u.Out().Printf("title\t%s", created.Title)
	return nil
}
