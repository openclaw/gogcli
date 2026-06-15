package cmd

import (
	"context"
	"fmt"
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
	Max       int64  `name:"max" aliases:"limit" help:"Max results (max allowed: 1000)" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *TasksListsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if c.Max <= 0 {
		return usage("max must be > 0")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := tasksService(ctx, account)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*tasks.TaskList, string, error) {
		call := svc.Tasklists.List().MaxResults(c.Max).Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return nil, "", callErr
		}
		return resp.Items, resp.NextPageToken, nil
	}

	items, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"tasklists":     items,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(items) == 0 {
		u.Err().Println("No task lists")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tTITLE")
	for _, tl := range items {
		fmt.Fprintf(w, "%s\t%s\n", tl.Id, tl.Title)
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}

type TasksListsCreateCmd struct {
	Title []string `arg:"" name:"title" help:"Task list title"`
}

func (c *TasksListsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	title := strings.TrimSpace(strings.Join(c.Title, " "))
	if title == "" {
		return usage("empty title")
	}

	if err := dryRunExit(ctx, flags, "tasks.lists.create", map[string]any{
		"title": title,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := tasksService(ctx, account)
	if err != nil {
		return err
	}

	created, err := svc.Tasklists.Insert(&tasks.TaskList{Title: title}).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"tasklist": created})
	}
	u.Out().Linef("id\t%s", created.Id)
	u.Out().Linef("title\t%s", created.Title)
	return nil
}
