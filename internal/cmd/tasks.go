package cmd

import (
	"github.com/steipete/gogcli/internal/googleapi"
)

var newTasksService = googleapi.NewTasks

type TasksCmd struct {
	Lists  TasksListsCmd  `cmd:"" name:"lists" help:"List task lists"`
	List   TasksListCmd   `cmd:"" name:"list" help:"List tasks"`
	Get    TasksGetCmd    `cmd:"" name:"get" help:"Get a task"`
	Add    TasksAddCmd    `cmd:"" name:"add" help:"Add a task" aliases:"create"`
	Update TasksUpdateCmd `cmd:"" name:"update" help:"Update a task"`
	Done   TasksDoneCmd   `cmd:"" name:"done" help:"Mark task completed" aliases:"complete"`
	Undo   TasksUndoCmd   `cmd:"" name:"undo" help:"Mark task needs action" aliases:"uncomplete,undone"`
	Delete TasksDeleteCmd `cmd:"" name:"delete" help:"Delete a task" aliases:"rm,del"`
	Clear  TasksClearCmd  `cmd:"" name:"clear" help:"Clear completed tasks"`
}
