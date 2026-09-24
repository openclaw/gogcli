package cmd

import (
	"context"
	"net/url"
	"strings"
)

// TasksRawCmd dumps the full Tasks.Get response as JSON.
//
// REST reference: https://developers.google.com/tasks/reference/rest/v1/tasks/get
// Go type: https://pkg.go.dev/google.golang.org/api/tasks/v1#Task
type TasksRawCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Task list ID"`
	TaskID     string `arg:"" name:"taskId" help:"Task ID"`
	Pretty     bool   `name:"pretty" help:"Pretty-print JSON (default: compact single-line)"`
}

func (c *TasksRawCmd) Run(ctx context.Context, flags *RootFlags) error {
	tasklistID := strings.TrimSpace(c.TasklistID)
	taskID := strings.TrimSpace(c.TaskID)
	if tasklistID == "" {
		return usage("empty tasklistId")
	}
	if taskID == "" {
		return usage("empty taskId")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := tasksService(ctx, account)
	if err != nil {
		return err
	}
	tasklistID, err = resolveTasklistID(ctx, svc, tasklistID)
	if err != nil {
		return err
	}

	client, err := tasksHTTPClient(ctx, account)
	if err != nil {
		return err
	}
	endpoint := "https://tasks.googleapis.com/tasks/v1/lists/" + url.PathEscape(tasklistID) + "/tasks/" + url.PathEscape(taskID)
	task, err := readRawObject(ctx, client, endpoint, nil, "task")
	if err != nil {
		return err
	}

	return writeRawJSON(ctx, task, c.Pretty)
}
