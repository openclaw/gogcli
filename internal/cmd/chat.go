package cmd

type ChatCmd struct {
	Spaces   ChatSpacesCmd   `cmd:"" name:"spaces" help:"Chat spaces"`
	Messages ChatMessagesCmd `cmd:"" name:"messages" help:"Chat messages"`
	Threads  ChatThreadsCmd  `cmd:"" name:"threads" help:"Chat threads"`
	DM       ChatDMCmd       `cmd:"" name:"dm" help:"Direct messages"`
}
