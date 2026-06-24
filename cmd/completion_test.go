package cmd

import (
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/urfave/cli/v2"
)

func TestCompletionCommand(t *testing.T) {
	t.Run("creates command with correct name", func(t *testing.T) {
		cmd := CompletionCommand("beacon-chain")
		require.Equal(t, "completion", cmd.Name)
	})

	t.Run("has three subcommands", func(t *testing.T) {
		cmd := CompletionCommand("beacon-chain")
		require.Equal(t, 3, len(cmd.Subcommands))

		names := make([]string, len(cmd.Subcommands))
		for i, sub := range cmd.Subcommands {
			names[i] = sub.Name
		}
		assert.DeepEqual(t, []string{"bash", "zsh", "fish"}, names)
	})

	t.Run("description contains binary name", func(t *testing.T) {
		cmd := CompletionCommand("validator")
		assert.Equal(t, true, strings.Contains(cmd.Description, "validator"))
	})
}

func TestBashCompletionScript(t *testing.T) {
	script := bashCompletionScript("beacon-chain")

	assert.Equal(t, true, strings.Contains(script, "beacon-chain"), "script should contain binary name")
	assert.Equal(t, true, strings.Contains(script, "_beacon_chain_completions"), "script should contain function name with underscores")
	assert.Equal(t, true, strings.Contains(script, "complete -o bashdefault"), "script should contain complete command")
	assert.Equal(t, true, strings.Contains(script, "--generate-bash-completion"), "script should use generate-bash-completion flag")
}

func TestZshCompletionScript(t *testing.T) {
	script := zshCompletionScript("validator")

	assert.Equal(t, true, strings.Contains(script, "#compdef validator"), "script should contain compdef directive")
	assert.Equal(t, true, strings.Contains(script, "_validator"), "script should contain function name")
	assert.Equal(t, true, strings.Contains(script, "--generate-bash-completion"), "script should use generate-bash-completion flag")
}

func TestFishCompletionScript(t *testing.T) {
	script := fishCompletionScript("beacon-chain")

	assert.Equal(t, true, strings.Contains(script, "complete -c beacon-chain"), "script should contain complete command")
	assert.Equal(t, true, strings.Contains(script, "__fish_beacon_chain_complete"), "script should contain function name with underscores")
	assert.Equal(t, true, strings.Contains(script, "--generate-bash-completion"), "script should use generate-bash-completion flag")
}

func TestScriptFunctionNames(t *testing.T) {
	// Test that hyphens are converted to underscores in function names
	bashScript := bashCompletionScript("beacon-chain")
	assert.Equal(t, true, strings.Contains(bashScript, "_beacon_chain_completions"))
	assert.Equal(t, false, strings.Contains(bashScript, "_beacon-chain_completions"))

	zshScript := zshCompletionScript("beacon-chain")
	assert.Equal(t, true, strings.Contains(zshScript, "_beacon_chain"))

	fishScript := fishCompletionScript("beacon-chain")
	assert.Equal(t, true, strings.Contains(fishScript, "__fish_beacon_chain_complete"))
}

func TestCompletionSubcommandActions(t *testing.T) {
	// Test that Action functions execute without errors
	cmd := CompletionCommand("beacon-chain")

	tests := []struct {
		name       string
		subcommand string
	}{
		{"bash action executes", "bash"},
		{"zsh action executes", "zsh"},
		{"fish action executes", "fish"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var subCmd *cli.Command
			for _, sub := range cmd.Subcommands {
				if sub.Name == tt.subcommand {
					subCmd = sub
					break
				}
			}
			require.NotNil(t, subCmd, "subcommand should exist")
			require.NotNil(t, subCmd.Action, "subcommand should have an action")

			// Action should not return an error; use a real cli.Context
			app := &cli.App{}
			ctx := cli.NewContext(app, nil, nil)
			err := subCmd.Action(ctx)
			require.NoError(t, err)
		})
	}
}
