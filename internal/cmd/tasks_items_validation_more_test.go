package cmd

import (
	"context"
	"io"
	"testing"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/ui"
)

func parseTasksKong(t *testing.T, cmd any, args []string) *kong.Context {
	t.Helper()

	parser, err := kong.New(cmd)
	if err != nil {
		t.Fatalf("kong new: %v", err)
	}
	kctx, err := parser.Parse(args)
	if err != nil {
		t.Fatalf("kong parse: %v", err)
	}
	return kctx
}

func TestTasksValidationErrors(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	if err := (&TasksListCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected list missing tasklistId")
	}
	if err := (&TasksAddCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected add missing tasklistId")
	}
	if err := (&TasksAddCmd{TasklistID: "l1"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected add missing title")
	}
	if err := (&TasksAddCmd{TasklistID: "l1", Title: "Task", Repeat: "daily"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected add repeat missing due")
	}
	if err := (&TasksAddCmd{TasklistID: "l1", Title: "Task", Repeat: "daily", Due: "2025-01-01"}).Run(ctx, flags); err == nil {
		t.Fatalf("expected add repeat missing count/until")
	}
	if err := (&TasksAddCmd{TasklistID: "l1", Title: "Task", RepeatCount: 2}).Run(ctx, flags); err == nil {
		t.Fatalf("expected add repeat-count without repeat")
	}

	{
		cmd := &TasksUpdateCmd{}
		kctx := parseTasksKong(t, cmd, []string{"l1", "t1"})
		if err := cmd.Run(ctx, kctx, flags); err == nil {
			t.Fatalf("expected update no fields error")
		}
	}
	{
		cmd := &TasksUpdateCmd{TasklistID: "l1"}
		kctx := parseTasksKong(t, cmd, []string{"l1", "t1"})
		cmd.TaskID = ""
		if err := cmd.Run(ctx, kctx, flags); err == nil {
			t.Fatalf("expected update missing taskId error")
		}
	}
	{
		cmd := &TasksUpdateCmd{}
		kctx := parseTasksKong(t, cmd, []string{"l1", "t1", "--status", "bad"})
		if err := cmd.Run(ctx, kctx, flags); err == nil {
			t.Fatalf("expected update invalid status error")
		}
	}

	if err := (&TasksDoneCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected done missing tasklistId")
	}
	if err := (&TasksUndoCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected undo missing tasklistId")
	}
	if err := (&TasksDeleteCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected delete missing tasklistId")
	}
	if err := (&TasksClearCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected clear missing tasklistId")
	}
}
