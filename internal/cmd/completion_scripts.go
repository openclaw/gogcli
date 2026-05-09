package cmd

import "fmt"

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

_gog() {
  local -a completions
  completions=("${(@f)$(gog __complete --cword "$((CURRENT - 1))" -- "${words[@]}")}")
  _describe 'values' completions
}

compdef _gog gog
`
}

func fishCompletionScript() string {
	return `function __gog_complete
  set -l words (commandline -opc)
  set -l cur (commandline -ct)

  # Include the current token (partial word being typed) to match bash behavior.
  set words $words $cur

  # cword points to the last word (the one being completed).
  set -l cword (math (count $words) - 1)
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
