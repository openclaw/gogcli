package cmd

import (
	"context"
	"fmt"
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
	Page          string `name:"page" aliases:"cursor" help:"Page token"`
	All           bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty     bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
	ShowCompleted bool   `name:"show-completed" help:"Include completed tasks (requires --show-hidden for some clients)" default:"true"`
	ShowDeleted   bool   `name:"show-deleted" help:"Include deleted tasks"`
	ShowHidden    bool   `name:"show-hidden" help:"Include hidden tasks"`
	ShowAssigned  bool   `name:"show-assigned" help:"Include tasks assigned to current user" default:"true"`
	DueMin        string `name:"due-min" help:"Lower bound for due date filter (RFC3339)"`
	DueMax        string `name:"due-max" help:"Upper bound for due date filter (RFC3339)"`
	CompletedMin  string `name:"completed-min" help:"Lower bound for completion date filter (RFC3339)"`
	CompletedMax  string `name:"completed-max" help:"Upper bound for completion date filter (RFC3339)"`
	UpdatedMin    string `name:"updated-min" help:"Lower bound for updated time filter (RFC3339)"`
}

func (c *TasksListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	tasklistID := strings.TrimSpace(c.TasklistID)
	if tasklistID == "" {
		return usage("empty tasklistId")
	}
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
	tasklistID, err = resolveTasklistID(ctx, svc, tasklistID)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*tasks.Task, string, error) {
		call := svc.Tasks.List(tasklistID).
			MaxResults(c.Max).
			ShowCompleted(c.ShowCompleted).
			ShowDeleted(c.ShowDeleted).
			ShowHidden(c.ShowHidden).
			ShowAssigned(c.ShowAssigned)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(strings.TrimSpace(pageToken))
		}
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

		resp, callErr := call.Context(ctx).Do()
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
			"tasks":         items,
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
		u.Err().Println("No tasks")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tTITLE\tSTATUS\tDUE\tUPDATED")
	for _, t := range items {
		status := strings.TrimSpace(t.Status)
		if status == "" {
			status = taskStatusNeedsAction
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t.Id, t.Title, status, strings.TrimSpace(t.Due), strings.TrimSpace(t.Updated))
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}

type TasksGetCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Task list ID"`
	TaskID     string `arg:"" name:"taskId" help:"Task ID"`
}

func (c *TasksGetCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	svc, err := tasksService(ctx, account)
	if err != nil {
		return err
	}
	tasklistID, err = resolveTasklistID(ctx, svc, tasklistID)
	if err != nil {
		return err
	}

	task, err := svc.Tasks.Get(tasklistID, taskID).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"task": task})
	}
	u.Out().Linef("id\t%s", task.Id)
	u.Out().Linef("title\t%s", task.Title)
	if strings.TrimSpace(task.Status) != "" {
		u.Out().Linef("status\t%s", task.Status)
	}
	if strings.TrimSpace(task.Due) != "" {
		u.Out().Linef("due\t%s", task.Due)
	}
	if strings.TrimSpace(task.WebViewLink) != "" {
		u.Out().Linef("link\t%s", task.WebViewLink)
	}
	return nil
}

type TasksAddCmd struct {
	TasklistID  string `arg:"" name:"tasklistId" help:"Task list ID"`
	Title       string `name:"title" help:"Task title (required)"`
	Notes       string `name:"notes" help:"Task notes/description"`
	Due         string `name:"due" help:"Due date (RFC3339 or YYYY-MM-DD; time may be ignored by Google Tasks)"`
	Parent      string `name:"parent" help:"Parent task ID (create as subtask)"`
	Previous    string `name:"previous" help:"Previous sibling task ID (controls ordering)"`
	Repeat      string `name:"repeat" help:"Materialize repeated tasks: daily, weekly, monthly, yearly"`
	Recur       string `name:"recur" help:"Alias for --repeat cadence: daily, weekly, monthly, yearly"`
	RecurRRule  string `name:"recur-rrule" help:"Alias for --repeat cadence via RRULE (supports FREQ + optional INTERVAL)"`
	RepeatCount int    `name:"repeat-count" help:"Number of occurrences to create (requires --repeat, --recur, or --recur-rrule)"`
	RepeatUntil string `name:"repeat-until" help:"Repeat until date/time (RFC3339 or YYYY-MM-DD; requires --repeat, --recur, or --recur-rrule)"`
}

func (c *TasksAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	plan, err := newTasksAddPlan(tasksAddInput{
		TasklistID:  c.TasklistID,
		Title:       c.Title,
		Notes:       c.Notes,
		Due:         c.Due,
		Parent:      c.Parent,
		Previous:    c.Previous,
		Repeat:      c.Repeat,
		Recur:       c.Recur,
		RecurRRule:  c.RecurRRule,
		RepeatCount: c.RepeatCount,
		RepeatUntil: c.RepeatUntil,
	})
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "tasks.add", plan.dryRunPayload()); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if !plan.repeating() {
		svc, svcErr := tasksService(ctx, account)
		if svcErr != nil {
			return svcErr
		}
		plan.TasklistID, err = resolveTasklistID(ctx, svc, plan.TasklistID)
		if err != nil {
			return err
		}
		if !outfmt.IsJSON(ctx) {
			warnTasksDueTime(u, plan.Due)
		}
		task := &tasks.Task{
			Title: plan.Title,
			Notes: plan.Notes,
			Due:   plan.Date.DueValue,
		}
		call := svc.Tasks.Insert(plan.TasklistID, task)
		if plan.Parent != "" {
			call = call.Parent(plan.Parent)
		}
		if plan.Previous != "" {
			call = call.Previous(plan.Previous)
		}

		created, createErr := call.Do()
		if createErr != nil {
			return createErr
		}
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"task": created})
		}
		u.Out().Linef("id\t%s", created.Id)
		u.Out().Linef("title\t%s", created.Title)
		if strings.TrimSpace(created.Status) != "" {
			u.Out().Linef("status\t%s", created.Status)
		}
		if strings.TrimSpace(created.Due) != "" {
			u.Out().Linef("due\t%s", created.Due)
		}
		if strings.TrimSpace(created.WebViewLink) != "" {
			u.Out().Linef("link\t%s", created.WebViewLink)
		}
		return nil
	}

	if !outfmt.IsJSON(ctx) {
		warnTasksDueTime(u, plan.Due)
	}

	schedule, err := plan.repeatSchedule()
	if err != nil {
		return err
	}

	svc, svcErr := tasksService(ctx, account)
	if svcErr != nil {
		return svcErr
	}
	plan.TasklistID, err = resolveTasklistID(ctx, svc, plan.TasklistID)
	if err != nil {
		return err
	}

	createdTasks := make([]*tasks.Task, 0, len(schedule))

	for i, due := range schedule {
		title := plan.Title
		if len(schedule) > 1 {
			title = fmt.Sprintf("%s (#%d/%d)", plan.Title, i+1, len(schedule))
		}
		task := &tasks.Task{
			Title: title,
			Notes: plan.Notes,
			Due:   formatTaskDue(due, plan.Date.DueHasTime),
		}
		call := svc.Tasks.Insert(plan.TasklistID, task)
		if plan.Parent != "" {
			call = call.Parent(plan.Parent)
		}
		if plan.Previous != "" {
			call = call.Previous(plan.Previous)
		}
		created, createErr := call.Do()
		if createErr != nil {
			return createErr
		}
		createdTasks = append(createdTasks, created)
		if plan.Previous != "" {
			plan.Previous = created.Id
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"tasks": createdTasks,
			"count": len(createdTasks),
		})
	}
	if len(createdTasks) == 1 {
		created := createdTasks[0]
		u.Out().Linef("id\t%s", created.Id)
		u.Out().Linef("title\t%s", created.Title)
		if strings.TrimSpace(created.Status) != "" {
			u.Out().Linef("status\t%s", created.Status)
		}
		if strings.TrimSpace(created.Due) != "" {
			u.Out().Linef("due\t%s", created.Due)
		}
		if strings.TrimSpace(created.WebViewLink) != "" {
			u.Out().Linef("link\t%s", created.WebViewLink)
		}
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tTITLE\tDUE")
	for _, task := range createdTasks {
		fmt.Fprintf(w, "%s\t%s\t%s\n", task.Id, task.Title, strings.TrimSpace(task.Due))
	}
	return nil
}

type TasksUpdateCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Task list ID"`
	TaskID     string `arg:"" name:"taskId" help:"Task ID"`
	Title      string `name:"title" help:"New title (set empty to clear)"`
	Notes      string `name:"notes" help:"New notes (set empty to clear)"`
	Due        string `name:"due" help:"New due date (RFC3339 or YYYY-MM-DD; time may be ignored; set empty to clear)"`
	Status     string `name:"status" help:"New status: needsAction|completed (set empty to clear)"`
}

func (c *TasksUpdateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	plan, err := newTasksUpdatePlan(tasksUpdateInput{
		TasklistID: c.TasklistID,
		TaskID:     c.TaskID,
		Title:      c.Title,
		Notes:      c.Notes,
		Due:        c.Due,
		Status:     c.Status,
	}, tasksUpdateFieldsFromContext(kctx))
	if err != nil {
		return err
	}
	if plan.WarnDue != "" && !outfmt.IsJSON(ctx) {
		warnTasksDueTime(u, plan.WarnDue)
	}

	if dryRunErr := dryRunExit(ctx, flags, "tasks.update", plan.dryRunPayload()); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := tasksService(ctx, account)
	if err != nil {
		return err
	}
	plan.TasklistID, err = resolveTasklistID(ctx, svc, plan.TasklistID)
	if err != nil {
		return err
	}

	updated, err := svc.Tasks.Patch(plan.TasklistID, plan.TaskID, plan.Patch).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"task": updated})
	}
	u.Out().Linef("id\t%s", updated.Id)
	u.Out().Linef("title\t%s", updated.Title)
	if strings.TrimSpace(updated.Status) != "" {
		u.Out().Linef("status\t%s", updated.Status)
	}
	if strings.TrimSpace(updated.Due) != "" {
		u.Out().Linef("due\t%s", updated.Due)
	}
	if strings.TrimSpace(updated.WebViewLink) != "" {
		u.Out().Linef("link\t%s", updated.WebViewLink)
	}
	return nil
}

type TasksDoneCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Task list ID"`
	TaskID     string `arg:"" name:"taskId" help:"Task ID"`
}

func (c *TasksDoneCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	tasklistID := strings.TrimSpace(c.TasklistID)
	taskID := strings.TrimSpace(c.TaskID)
	if tasklistID == "" {
		return usage("empty tasklistId")
	}
	if taskID == "" {
		return usage("empty taskId")
	}

	if dryRunErr := dryRunExit(ctx, flags, "tasks.done", map[string]any{
		"tasklist_id": tasklistID,
		"task_id":     taskID,
	}); dryRunErr != nil {
		return dryRunErr
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

	updated, err := svc.Tasks.Patch(tasklistID, taskID, &tasks.Task{Status: taskStatusCompleted}).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"task": updated})
	}
	u.Out().Linef("id\t%s", updated.Id)
	u.Out().Linef("status\t%s", strings.TrimSpace(updated.Status))
	return nil
}

type TasksUndoCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Task list ID"`
	TaskID     string `arg:"" name:"taskId" help:"Task ID"`
}

func (c *TasksUndoCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	tasklistID := strings.TrimSpace(c.TasklistID)
	taskID := strings.TrimSpace(c.TaskID)
	if tasklistID == "" {
		return usage("empty tasklistId")
	}
	if taskID == "" {
		return usage("empty taskId")
	}

	if dryRunErr := dryRunExit(ctx, flags, "tasks.undo", map[string]any{
		"tasklist_id": tasklistID,
		"task_id":     taskID,
	}); dryRunErr != nil {
		return dryRunErr
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

	updated, err := svc.Tasks.Patch(tasklistID, taskID, &tasks.Task{Status: "needsAction"}).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"task": updated})
	}
	u.Out().Linef("id\t%s", updated.Id)
	u.Out().Linef("status\t%s", strings.TrimSpace(updated.Status))
	return nil
}

type TasksDeleteCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Task list ID"`
	TaskID     string `arg:"" name:"taskId" help:"Task ID"`
}

func (c *TasksDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	tasklistID := strings.TrimSpace(c.TasklistID)
	taskID := strings.TrimSpace(c.TaskID)
	if tasklistID == "" {
		return usage("empty tasklistId")
	}
	if taskID == "" {
		return usage("empty taskId")
	}

	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "tasks.delete", map[string]any{
		"tasklist_id": tasklistID,
		"task_id":     taskID,
	}, fmt.Sprintf("delete task %s from list %s", taskID, tasklistID)); confirmErr != nil {
		return confirmErr
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

	if err := svc.Tasks.Delete(tasklistID, taskID).Do(); err != nil {
		return err
	}
	return writeResult(ctx, u,
		kv("deleted", true),
		kv("id", taskID),
	)
}

type TasksClearCmd struct {
	TasklistID string `arg:"" name:"tasklistId" help:"Task list ID"`
}

func (c *TasksClearCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	tasklistID := strings.TrimSpace(c.TasklistID)
	if tasklistID == "" {
		return usage("empty tasklistId")
	}

	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "tasks.clear", map[string]any{
		"tasklist_id": tasklistID,
	}, fmt.Sprintf("clear completed tasks from list %s", tasklistID)); confirmErr != nil {
		return confirmErr
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

	if err := svc.Tasks.Clear(tasklistID).Do(); err != nil {
		return err
	}
	return writeResult(ctx, u,
		kv("cleared", true),
		kv("tasklistId", tasklistID),
	)
}
