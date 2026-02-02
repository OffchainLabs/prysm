package cmd

import (
	"fmt"
	"strings"
)

// bashCompletionScript returns the bash completion script for the given binary.
func bashCompletionScript(binaryName string) string {
	// Convert hyphens to underscores for bash function names
	funcName := strings.ReplaceAll(binaryName, "-", "_")
	return fmt.Sprintf(`#!/bin/bash

_%[1]s_completions() {
    local cur prev words cword opts
    COMPREPLY=()

    # Use bash-completion if available, otherwise set variables directly
    if declare -F _init_completion >/dev/null 2>&1; then
        _init_completion -n "=:" || return
    else
        cur="${COMP_WORDS[COMP_CWORD]}"
        prev="${COMP_WORDS[COMP_CWORD-1]}"
        words=("${COMP_WORDS[@]}")
        cword=$COMP_CWORD
    fi

    # Build command array for completion - flag must be at the END
    local -a requestComp
    if [[ "$cur" == "-"* ]]; then
        requestComp=("${COMP_WORDS[@]:0:COMP_CWORD}" "$cur" --generate-bash-completion)
    else
        requestComp=("${COMP_WORDS[@]:0:COMP_CWORD}" --generate-bash-completion)
    fi

    opts=$("${requestComp[@]}" 2>/dev/null)
    COMPREPLY=($(compgen -W "${opts}" -- "${cur}"))
    return 0
}

complete -o bashdefault -o default -o nospace -F _%[1]s_completions %[2]s
`, funcName, binaryName)
}

// zshCompletionScript returns the zsh completion script for the given binary.
func zshCompletionScript(binaryName string) string {
	// Convert hyphens to underscores for zsh function names
	funcName := strings.ReplaceAll(binaryName, "-", "_")
	return fmt.Sprintf(`#compdef %[2]s

_%[1]s() {
    local curcontext="$curcontext" ret=1
    local -a completions

    # Build command array with --generate-bash-completion at the END
    local -a requestComp
    if [[ "${words[CURRENT]}" == -* ]]; then
        requestComp=(${words[1,CURRENT]} --generate-bash-completion)
    else
        requestComp=(${words[1,CURRENT-1]} --generate-bash-completion)
    fi

    completions=($("${requestComp[@]}" 2>/dev/null))

    if [[ ${#completions[@]} -gt 0 ]]; then
        _describe -t commands '%[2]s' completions && ret=0
    fi

    # Fallback to file completion
    _files && ret=0

    return ret
}

compdef _%[1]s %[2]s
`, funcName, binaryName)
}

// fishCompletionScript returns the fish completion script for the given binary.
func fishCompletionScript(binaryName string) string {
	// Convert hyphens to underscores for fish function names
	funcName := strings.ReplaceAll(binaryName, "-", "_")
	return fmt.Sprintf(`# Fish completion for %[2]s

function __fish_%[1]s_complete
    set -l args (commandline -opc)
    set -l cur (commandline -ct)

    # Build command with --generate-bash-completion at the END
    if string match -q -- '-*' "$cur"
        %[2]s $args $cur --generate-bash-completion 2>/dev/null
    else
        %[2]s $args --generate-bash-completion 2>/dev/null
    end
end

complete -c %[2]s -f -a "(__fish_%[1]s_complete)"
`, funcName, binaryName)
}
