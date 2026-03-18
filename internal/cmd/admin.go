package cmd

// AdminCmd provides Google Workspace admin commands using the Admin SDK Directory API.
// Requires domain-wide delegation with a service account.
type AdminCmd struct {
	Users  AdminUsersCmd  `cmd:"" name:"users" help:"Manage Workspace users"`
	Groups AdminGroupsCmd `cmd:"" name:"groups" help:"Manage Workspace groups"`
}
