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
	Max       int64  `name:"max" aliases:"limit" help:"Max results (max allowed: 1000)" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
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

	fetch := func(pageToken string) ([]*tasks.TaskList, string, error) {
		call := svc.Tasklists.List().MaxResults(c.Max).Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, "", err
		}
		return resp.Items, resp.NextPageToken, nil
	}

	var items []*tasks.TaskList
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		items = all
	} else {
		var err error
		items, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
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
	printNextPageHint(u, nextPageToken)
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

	svc, err := newTasksService(ctx, account)
	if err != nil {
		return err
	}

	created, err := svc.Tasklists.Insert(&tasks.TaskList{Title: title}).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"tasklist": created})
	}
	u.Out().Printf("id\t%s", created.Id)
	u.Out().Printf("title\t%s", created.Title)
	return nil
}
