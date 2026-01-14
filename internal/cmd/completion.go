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

func completionScript(shell string) (string, error) {
	switch shell {
	case "bash":
		return bashCompletionScript(), nil
	case "zsh":
		return zshCompletionScript(), nil
	case "fish":
		return fishCompletionScript(), nil
	case "powershell":
		return powerShellCompletionScript(), nil
	default:
		return "", fmt.Errorf("unsupported shell: %s", shell)
	}
}

func bashCompletionScript() string {
	return `#!/usr/bin/env bash

_gog_complete() {
  local IFS=$'\n'
  local completions
  completions=$(gog __complete --cword "$COMP_CWORD" -- "${COMP_WORDS[@]}")
  COMPREPLY=()
  if [[ -n "$completions" ]]; then
    COMPREPLY=( $completions )
  fi
}

complete -F _gog_complete gog
`
}

func zshCompletionScript() string {
	return `#compdef gog

autoload -Uz bashcompinit
bashcompinit
` + bashCompletionScript()
}

func fishCompletionScript() string {
	return `function __gog_complete
  set -l words (commandline -opc)
  set -l cur (commandline -ct)
  set -l cword (count $words)
  if test -n "$cur"
    set cword (math $cword - 1)
  end
  gog __complete --cword $cword -- $words
end

complete -c gog -f -a "(__gog_complete)"
`
}

func powerShellCompletionScript() string {
	return `Register-ArgumentCompleter -CommandName gog -ScriptBlock {
  param($commandName, $wordToComplete, $cursorPosition, $commandAst, $fakeBoundParameter)
  $elements = $commandAst.CommandElements | ForEach-Object { $_.ToString() }
  $cword = $elements.Count - 1
  $completions = gog __complete --cword $cword -- $elements
  foreach ($completion in $completions) {
    [System.Management.Automation.CompletionResult]::new($completion, $completion, 'ParameterValue', $completion)
  }
}
`
}
