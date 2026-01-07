package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/tasks/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const (
	taskStatusNeedsAction = "needsAction"
	taskStatusCompleted   = "completed"
)

type TasksListCmd struct {
	TasklistID    string `arg:"" name:"tasklistId" help:"Task list ID"`
	Max           int64  `name:"max" aliases:"limit" help:"Max results (max allowed: 100)" default:"20"`
	Page          string `name:"page" help:"Page token"`
	ShowCompleted bool   `name:"show-completed" help:"Include completed tasks (requires --show-hidden for some clients)" default:"true"`
	ShowDeleted   bool   `name:"show-deleted" help:"Include deleted tasks"`
	ShowHidden    bool   `name:"show-hidden" help:"Include hidden tasks"`
	ShowAssigned  bool   `name:"show-assigned" help:"Include tasks assigned to current user"`
	DueMin        string `name:"due-min" help:"Lower bound for due date filter (RFC3339)"`
	DueMax        string `name:"due-max" help:"Upper bound for due date filter (RFC3339)"`
	CompletedMin  string `name:"completed-min" help:"Lower bound for completion date filter (RFC3339)"`
	CompletedMax  string `name:"completed-max" help:"Upper bound for completion date filter (RFC3339)"`
	UpdatedMin    string `name:"updated-min" help:"Lower bound for updated time filter (RFC3339)"`
}

func (c *TasksListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	tasklistID := strings.TrimSpace(c.TasklistID)
	if tasklistID == "" {
		return usage("empty tasklistId")
	}

	svc, err := newTasksService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Tasks.List(tasklistID).
		MaxResults(c.Max).
		PageToken(c.Page).
		ShowCompleted(c.ShowCompleted).
		ShowDeleted(c.ShowDeleted).
		ShowHidden(c.ShowHidden).
		ShowAssigned(c.ShowAssigned)
	if strings.TrimSpace(c.DueMin) != "" {
		call = call.DueMin(strings.TrimSpace(c.DueMin))
	}
	if strings.TrimSpace(c.DueMax) != "" {
		call = call.DueMax(strings.TrimSpace(c.DueMax))
	}
	if strings.TrimSpace(c.CompletedMin) != "" {
		call = call.CompletedMin(strings.TrimSpace(c.CompletedMin))
	}
	if strings.TrimSpace(c.CompletedMax) != "" {
		call = call.CompletedMax(strings.TrimSpace(c.CompletedMax))
	}
	if strings.TrimSpace(c.UpdatedMin) != "" {
		call = call.UpdatedMin(strings.TrimSpace(c.UpdatedMin))
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"tasks":         resp.Items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Items) == 0 {
		u.Err().Println("No tasks")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tTITLE\tSTATUS\tDUE\tUPDATED")
	for _, t := range resp.Items {
		status := strings.TrimSpace(t.Status)
		if status == "" {
			status = taskStatusNeedsAction
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t.Id, t.Title, status, strings.TrimSpace(t.Due), strings.TrimSpace(t.Updated))
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type TasksAddCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Task list ID"`
	Title      string `name:"title" help:"Task title (required)"`
	Notes      string `name:"notes" help:"Task notes/description"`
	Due        string `name:"due" help:"Due date/time (RFC3339)"`
	Parent     string `name:"parent" help:"Parent task ID (create as subtask)"`
	Previous   string `name:"previous" help:"Previous sibling task ID (controls ordering)"`
}

func (c *TasksAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	tasklistID := strings.TrimSpace(c.TasklistID)
	if tasklistID == "" {
		return usage("empty tasklistId")
	}
	if strings.TrimSpace(c.Title) == "" {
		return usage("required: --title")
	}

	svc, err := newTasksService(ctx, account)
	if err != nil {
		return err
	}

	task := &tasks.Task{
		Title: strings.TrimSpace(c.Title),
		Notes: strings.TrimSpace(c.Notes),
		Due:   strings.TrimSpace(c.Due),
	}
	call := svc.Tasks.Insert(tasklistID, task)
	if strings.TrimSpace(c.Parent) != "" {
		call = call.Parent(strings.TrimSpace(c.Parent))
	}
	if strings.TrimSpace(c.Previous) != "" {
		call = call.Previous(strings.TrimSpace(c.Previous))
	}

	created, err := call.Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"task": created})
	}
	u.Out().Printf("id\t%s", created.Id)
	u.Out().Printf("title\t%s", created.Title)
	if strings.TrimSpace(created.Status) != "" {
		u.Out().Printf("status\t%s", created.Status)
	}
	if strings.TrimSpace(created.Due) != "" {
		u.Out().Printf("due\t%s", created.Due)
	}
	if strings.TrimSpace(created.WebViewLink) != "" {
		u.Out().Printf("link\t%s", created.WebViewLink)
	}
	return nil
}

type TasksUpdateCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Task list ID"`
	TaskID     string `arg:"" name:"taskId" help:"Task ID"`
	Title      string `name:"title" help:"New title (set empty to clear)"`
	Notes      string `name:"notes" help:"New notes (set empty to clear)"`
	Due        string `name:"due" help:"New due date/time (RFC3339; set empty to clear)"`
	Status     string `name:"status" help:"New status: needsAction|completed (set empty to clear)"`
}

func (c *TasksUpdateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	tasklistID := strings.TrimSpace(c.TasklistID)
	taskID := strings.TrimSpace(c.TaskID)
	if tasklistID == "" {
		return usage("empty tasklistId")
	}
	if taskID == "" {
		return usage("empty taskId")
	}

	patch := &tasks.Task{}
	changed := false
	if flagProvided(kctx, "title") {
		patch.Title = strings.TrimSpace(c.Title)
		changed = true
	}
	if flagProvided(kctx, "notes") {
		patch.Notes = strings.TrimSpace(c.Notes)
		changed = true
	}
	if flagProvided(kctx, "due") {
		patch.Due = strings.TrimSpace(c.Due)
		changed = true
	}
	if flagProvided(kctx, "status") {
		patch.Status = strings.TrimSpace(c.Status)
		changed = true
	}
	if !changed {
		return usage("no fields to update (set at least one of: --title, --notes, --due, --status)")
	}

	if flagProvided(kctx, "status") && patch.Status != "" && patch.Status != taskStatusNeedsAction && patch.Status != taskStatusCompleted {
		return usage("invalid --status (expected needsAction or completed)")
	}

	svc, err := newTasksService(ctx, account)
	if err != nil {
		return err
	}

	updated, err := svc.Tasks.Patch(tasklistID, taskID, patch).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"task": updated})
	}
	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("title\t%s", updated.Title)
	if strings.TrimSpace(updated.Status) != "" {
		u.Out().Printf("status\t%s", updated.Status)
	}
	if strings.TrimSpace(updated.Due) != "" {
		u.Out().Printf("due\t%s", updated.Due)
	}
	if strings.TrimSpace(updated.WebViewLink) != "" {
		u.Out().Printf("link\t%s", updated.WebViewLink)
	}
	return nil
}

type TasksDoneCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Task list ID"`
	TaskID     string `arg:"" name:"taskId" help:"Task ID"`
}

func (c *TasksDoneCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	tasklistID := strings.TrimSpace(c.TasklistID)
	taskID := strings.TrimSpace(c.TaskID)
	if tasklistID == "" {
		return usage("empty tasklistId")
	}
	if taskID == "" {
		return usage("empty taskId")
	}

	svc, err := newTasksService(ctx, account)
	if err != nil {
		return err
	}

	updated, err := svc.Tasks.Patch(tasklistID, taskID, &tasks.Task{Status: taskStatusCompleted}).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"task": updated})
	}
	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("status\t%s", strings.TrimSpace(updated.Status))
	return nil
}

type TasksUndoCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Task list ID"`
	TaskID     string `arg:"" name:"taskId" help:"Task ID"`
}

func (c *TasksUndoCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	tasklistID := strings.TrimSpace(c.TasklistID)
	taskID := strings.TrimSpace(c.TaskID)
	if tasklistID == "" {
		return usage("empty tasklistId")
	}
	if taskID == "" {
		return usage("empty taskId")
	}

	svc, err := newTasksService(ctx, account)
	if err != nil {
		return err
	}

	updated, err := svc.Tasks.Patch(tasklistID, taskID, &tasks.Task{Status: "needsAction"}).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"task": updated})
	}
	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("status\t%s", strings.TrimSpace(updated.Status))
	return nil
}

type TasksDeleteCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Task list ID"`
	TaskID     string `arg:"" name:"taskId" help:"Task ID"`
}

func (c *TasksDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	tasklistID := strings.TrimSpace(c.TasklistID)
	taskID := strings.TrimSpace(c.TaskID)
	if tasklistID == "" {
		return usage("empty tasklistId")
	}
	if taskID == "" {
		return usage("empty taskId")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete task %s from list %s", taskID, tasklistID)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newTasksService(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Tasks.Delete(tasklistID, taskID).Do(); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"deleted": true,
			"id":      taskID,
		})
	}
	u.Out().Printf("deleted\ttrue")
	u.Out().Printf("id\t%s", taskID)
	return nil
}

type TasksClearCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Task list ID"`
}

func (c *TasksClearCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	tasklistID := strings.TrimSpace(c.TasklistID)
	if tasklistID == "" {
		return usage("empty tasklistId")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("clear completed tasks from list %s", tasklistID)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newTasksService(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Tasks.Clear(tasklistID).Do(); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"cleared":    true,
			"tasklistId": tasklistID,
		})
	}
	u.Out().Printf("cleared\ttrue")
	u.Out().Printf("tasklistId\t%s", tasklistID)
	return nil
}
