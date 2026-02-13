package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"google.golang.org/api/tasks/v1"

	"github.com/degree-analytics/ratatosk/internal/outfmt"
	"github.com/degree-analytics/ratatosk/internal/ui"
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
	ShowAssigned  bool   `name:"show-assigned" help:"Include tasks assigned to current user" default:"true"`
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

	svc, err := newTasksService(ctx, account)
	if err != nil {
		return err
	}

	task, err := svc.Tasks.Get(tasklistID, taskID).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"task": task})
	}
	u.Out().Printf("id\t%s", task.Id)
	u.Out().Printf("title\t%s", task.Title)
	if strings.TrimSpace(task.Status) != "" {
		u.Out().Printf("status\t%s", task.Status)
	}
	if strings.TrimSpace(task.Due) != "" {
		u.Out().Printf("due\t%s", task.Due)
	}
	if strings.TrimSpace(task.WebViewLink) != "" {
		u.Out().Printf("link\t%s", task.WebViewLink)
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
	Repeat      string `name:"repeat" help:"Repeat task: daily, weekly, monthly, yearly"`
	RepeatCount int    `name:"repeat-count" help:"Number of occurrences to create (requires --repeat)"`
	RepeatUntil string `name:"repeat-until" help:"Repeat until date/time (RFC3339 or YYYY-MM-DD; requires --repeat)"`
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

	repeatUnit, err := parseRepeatUnit(c.Repeat)
	if err != nil {
		return err
	}
	if repeatUnit == repeatNone && (strings.TrimSpace(c.RepeatUntil) != "" || c.RepeatCount != 0) {
		return usage("--repeat is required when using --repeat-count or --repeat-until")
	}

	if repeatUnit == repeatNone {
		svc, svcErr := newTasksService(ctx, account)
		if svcErr != nil {
			return svcErr
		}
		warnTasksDueTime(u, c.Due)
		dueValue, dueErr := normalizeTaskDue(c.Due)
		if dueErr != nil {
			return dueErr
		}
		task := &tasks.Task{
			Title: strings.TrimSpace(c.Title),
			Notes: strings.TrimSpace(c.Notes),
			Due:   dueValue,
		}
		call := svc.Tasks.Insert(tasklistID, task)
		if strings.TrimSpace(c.Parent) != "" {
			call = call.Parent(strings.TrimSpace(c.Parent))
		}
		if strings.TrimSpace(c.Previous) != "" {
			call = call.Previous(strings.TrimSpace(c.Previous))
		}

		created, createErr := call.Do()
		if createErr != nil {
			return createErr
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

	if strings.TrimSpace(c.Due) == "" {
		return usage("--due is required when using --repeat")
	}
	if c.RepeatCount < 0 {
		return usage("--repeat-count must be >= 0")
	}
	if strings.TrimSpace(c.RepeatUntil) == "" && c.RepeatCount == 0 {
		return usage("--repeat requires --repeat-count or --repeat-until")
	}

	warnTasksDueTime(u, c.Due)

	dueTime, dueHasTime, err := parseTaskDate(c.Due)
	if err != nil {
		return err
	}

	var until *time.Time
	if strings.TrimSpace(c.RepeatUntil) != "" {
		untilValue, untilHasTime, parseErr := parseTaskDate(c.RepeatUntil)
		if parseErr != nil {
			return parseErr
		}
		switch {
		case dueHasTime && !untilHasTime:
			untilValue = time.Date(
				untilValue.Year(),
				untilValue.Month(),
				untilValue.Day(),
				dueTime.Hour(),
				dueTime.Minute(),
				dueTime.Second(),
				dueTime.Nanosecond(),
				dueTime.Location(),
			)
		case !dueHasTime && untilHasTime:
			untilValue = time.Date(untilValue.Year(), untilValue.Month(), untilValue.Day(), 0, 0, 0, 0, time.UTC)
		}
		until = &untilValue
	}

	schedule := expandRepeatSchedule(dueTime, repeatUnit, c.RepeatCount, until)
	if len(schedule) == 0 {
		return usage("repeat produced no occurrences")
	}

	svc, svcErr := newTasksService(ctx, account)
	if svcErr != nil {
		return svcErr
	}

	parent := strings.TrimSpace(c.Parent)
	previous := strings.TrimSpace(c.Previous)
	baseTitle := strings.TrimSpace(c.Title)
	createdTasks := make([]*tasks.Task, 0, len(schedule))

	for i, due := range schedule {
		title := baseTitle
		if len(schedule) > 1 {
			title = fmt.Sprintf("%s (#%d/%d)", baseTitle, i+1, len(schedule))
		}
		task := &tasks.Task{
			Title: title,
			Notes: strings.TrimSpace(c.Notes),
			Due:   formatTaskDue(due, dueHasTime),
		}
		call := svc.Tasks.Insert(tasklistID, task)
		if parent != "" {
			call = call.Parent(parent)
		}
		if previous != "" {
			call = call.Previous(previous)
		}
		created, createErr := call.Do()
		if createErr != nil {
			return createErr
		}
		createdTasks = append(createdTasks, created)
		if previous != "" {
			previous = created.Id
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"tasks": createdTasks,
			"count": len(createdTasks),
		})
	}
	if len(createdTasks) == 1 {
		created := createdTasks[0]
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
		warnTasksDueTime(u, c.Due)
		dueValue, dueErr := normalizeTaskDue(c.Due)
		if dueErr != nil {
			return dueErr
		}
		patch.Due = dueValue
		changed = true
		warnTasksDueTime(u, c.Due)
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
