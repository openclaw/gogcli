package cmd

import (
	"context"
	"fmt"
	"os"
)

type CompletionCmd struct {
	Shell string `arg:"" name:"shell" help:"Shell (bash|zsh|fish|powershell)" enum:"bash,zsh,fish,powershell"`
}

func (c *CompletionCmd) Run(_ context.Context) error {
	script, err := completionScript(c.Shell)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(os.Stdout, script)
	return err
}

type CompletionInternalCmd struct {
	Cword int      `name:"cword" help:"Index of the current word" default:"-1"`
	Words []string `arg:"" optional:"" name:"words" help:"Words to complete"`
}

func (c *CompletionInternalCmd) Run(_ context.Context) error {
	items, err := completeWords(c.Cword, c.Words)
	if err != nil {
		return err
	}
	for _, item := range items {
		if _, err := fmt.Fprintln(os.Stdout, item); err != nil {
			return err
		}
	}
	return nil
}
