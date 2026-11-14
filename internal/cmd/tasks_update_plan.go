package cmd

import (
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/tasks/v1"
)

type tasksUpdateFields struct {
	Title  bool
	Notes  bool
	Due    bool
	Status bool
}

func tasksUpdateFieldsFromContext(kctx *kong.Context) tasksUpdateFields {
	return tasksUpdateFields{
		Title:  flagProvided(kctx, "title"),
		Notes:  flagProvided(kctx, "notes"),
		Due:    flagProvided(kctx, "due"),
		Status: flagProvided(kctx, "status"),
	}
}

type tasksUpdateInput struct {
	TasklistID string
	TaskID     string
	Title      string
	Notes      string
	Due        string
	Status     string
}

type tasksUpdatePlan struct {
	TasklistID string
	TaskID     string
	Patch      *tasks.Task
	WarnDue    string
}

func newTasksUpdatePlan(input tasksUpdateInput, fields tasksUpdateFields) (tasksUpdatePlan, error) {
	plan := tasksUpdatePlan{
		TasklistID: strings.TrimSpace(input.TasklistID),
		TaskID:     strings.TrimSpace(input.TaskID),
		Patch:      &tasks.Task{},
	}
	if plan.TasklistID == "" {
		return tasksUpdatePlan{}, usage("empty tasklistId")
	}
	if plan.TaskID == "" {
		return tasksUpdatePlan{}, usage("empty taskId")
	}

	changed := false
	// Title and Notes are omitempty in the API struct; force an explicit empty
	// value into the PATCH so "set empty to clear" clears instead of sending {}.
	if fields.Title {
		plan.Patch.Title = strings.TrimSpace(input.Title)
		if plan.Patch.Title == "" {
			plan.Patch.ForceSendFields = append(plan.Patch.ForceSendFields, "Title")
		}
		changed = true
	}
	if fields.Notes {
		plan.Patch.Notes = strings.TrimSpace(input.Notes)
		if plan.Patch.Notes == "" {
			plan.Patch.ForceSendFields = append(plan.Patch.ForceSendFields, "Notes")
		}
		changed = true
	}
	if fields.Due {
		dueValue, err := normalizeTaskDue(input.Due)
		if err != nil {
			return tasksUpdatePlan{}, err
		}
		if dueValue == "" {
			plan.Patch.NullFields = append(plan.Patch.NullFields, "Due")
		} else {
			plan.Patch.Due = dueValue
			plan.WarnDue = input.Due
		}
		changed = true
	}
	if fields.Status {
		plan.Patch.Status = strings.TrimSpace(input.Status)
		changed = true
	}
	if !changed {
		return tasksUpdatePlan{}, usage("no fields to update (set at least one of: --title, --notes, --due, --status)")
	}
	// A task is always needsAction or completed; there is no empty status to set.
	if fields.Status && plan.Patch.Status != taskStatusNeedsAction && plan.Patch.Status != taskStatusCompleted {
		return tasksUpdatePlan{}, usage("invalid --status (expected needsAction or completed)")
	}
	return plan, nil
}

func (p tasksUpdatePlan) dryRunPayload() map[string]any {
	return map[string]any{
		"tasklist_id": p.TasklistID,
		"task_id":     p.TaskID,
		"patch":       p.Patch,
	}
}
