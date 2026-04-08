package cmd

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
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
	svc, err := newTasksService(ctx, account)
	if err != nil {
		return err
	}
	tasklistID, err = resolveTasklistID(ctx, svc, tasklistID)
	if err != nil {
		return err
	}

	task, err := svc.Tasks.Get(tasklistID, taskID).Context(ctx).Do()
	if err != nil {
		return err
	}
	if task == nil {
		return errors.New("task not found")
	}

	return outfmt.WriteRaw(ctx, os.Stdout, task, outfmt.RawOptions{Pretty: c.Pretty})
}
