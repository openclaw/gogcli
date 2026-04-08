package cmd

import (
	"github.com/steipete/gogcli/internal/googleapi"
)

var newTasksService = googleapi.NewTasks

type TasksCmd struct {
	Lists  TasksListsCmd  `cmd:"" name:"lists" help:"List task lists"`
	List   TasksListCmd   `cmd:"" name:"list" aliases:"ls" help:"List tasks"`
	Get    TasksGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get a task"`
	Add    TasksAddCmd    `cmd:"" name:"add" help:"Add a task" aliases:"create"`
	Update TasksUpdateCmd `cmd:"" name:"update" aliases:"edit,set" help:"Update a task"`
	Done   TasksDoneCmd   `cmd:"" name:"done" help:"Mark task completed" aliases:"complete"`
	Undo   TasksUndoCmd   `cmd:"" name:"undo" help:"Mark task needs action" aliases:"uncomplete,undone"`
	Delete TasksDeleteCmd `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Delete a task"`
	Clear  TasksClearCmd  `cmd:"" name:"clear" help:"Clear completed tasks"`
	Raw    TasksRawCmd    `cmd:"" name:"raw" help:"Dump raw Google Tasks API response as JSON (Tasks.Get; lossless; for scripting and LLM consumption)"`
}
