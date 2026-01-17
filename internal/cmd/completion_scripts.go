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
