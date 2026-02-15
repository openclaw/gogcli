package cmd

// AgentCmd contains helper commands intended to make gog easier to consume from LLM agents.
type AgentCmd struct {
	ExitCodes AgentExitCodesCmd `cmd:"" name:"exit-codes" aliases:"exitcodes,exit-code" help:"Print stable exit codes for automation"`
}
