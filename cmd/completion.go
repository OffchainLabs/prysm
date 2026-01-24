package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

// CompletionCommand returns the completion command for the given binary name.
// The binaryName parameter should be "beacon-chain" or "validator".
func CompletionCommand(binaryName string) *cli.Command {
	return &cli.Command{
		Name:     "completion",
		Category: "completion",
		Usage:    "Generate shell completion scripts",
		Description: fmt.Sprintf(`Generate shell completion scripts for bash, zsh, or fish.

To load completions:

Bash:
  $ source <(%[1]s completion bash)
  # To load completions for each session, execute once:
  $ %[1]s completion bash > /etc/bash_completion.d/%[1]s

Zsh:
  # To load completions for each session, execute once:
  $ %[1]s completion zsh > "${fpath[1]}/_%[1]s"

  # You may need to start a new shell for completions to take effect.

Fish:
  $ %[1]s completion fish | source
  # To load completions for each session, execute once:
  $ %[1]s completion fish > ~/.config/fish/completions/%[1]s.fish
`, binaryName),
		Subcommands: []*cli.Command{
			{
				Name:  "bash",
				Usage: "Generate bash completion script",
				Action: func(_ *cli.Context) error {
					fmt.Println(bashCompletionScript(binaryName))
					return nil
				},
			},
			{
				Name:  "zsh",
				Usage: "Generate zsh completion script",
				Action: func(_ *cli.Context) error {
					fmt.Println(zshCompletionScript(binaryName))
					return nil
				},
			},
			{
				Name:  "fish",
				Usage: "Generate fish completion script",
				Action: func(_ *cli.Context) error {
					fmt.Println(fishCompletionScript(binaryName))
					return nil
				},
			},
		},
	}
}
