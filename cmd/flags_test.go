package cmd

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/urfave/cli/v2"
)

func TestLoadFlagsFromConfig(t *testing.T) {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	context := cli.NewContext(&app, set, nil)

	require.NoError(t, os.WriteFile("flags_test.yaml", []byte("testflag: 100"), 0666))

	require.NoError(t, set.Parse([]string{"test-command", "--" + ConfigFileFlag.Name, "flags_test.yaml"}))
	comFlags := WrapFlags([]cli.Flag{
		&cli.StringFlag{
			Name: ConfigFileFlag.Name,
		},
		&cli.IntFlag{
			Name:  "testflag",
			Value: 0,
		},
	})
	command := &cli.Command{
		Name:  "test-command",
		Flags: comFlags,
		Before: func(cliCtx *cli.Context) error {
			return LoadFlagsFromConfig(cliCtx, comFlags)
		},
		Action: func(cliCtx *cli.Context) error {
			require.Equal(t, 100, cliCtx.Int("testflag"))
			return nil
		},
	}
	require.NoError(t, command.Run(context, context.Args().Slice()...))
	require.NoError(t, os.Remove("flags_test.yaml"))
}

func TestValidateNoArgs(t *testing.T) {
	app := &cli.App{
		Before: ValidateNoArgs,
		Action: func(c *cli.Context) error {
			return nil
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name: "foo",
			},
		},
		Commands: []*cli.Command{
			{
				Name: "bar",
				Subcommands: []*cli.Command{
					{
						Name: "subComm1",
						Subcommands: []*cli.Command{
							{
								Name: "subComm3",
							},
						},
					},
					{
						Name: "subComm2",
						Subcommands: []*cli.Command{
							{
								Name: "subComm4",
							},
						},
					},
				},
			},
		},
	}

	// It should not work with a bogus argument
	err := app.Run([]string{"command", "foo"})
	require.ErrorContains(t, "unrecognized argument: foo", err)
	// It should work with registered flags
	err = app.Run([]string{"command", "--foo=bar"})
	require.NoError(t, err)
	// It should work with subcommands.
	err = app.Run([]string{"command", "bar"})
	require.NoError(t, err)
	// It should fail on unregistered flag (default logic in urfave/cli).
	err = app.Run([]string{"command", "bar", "--baz"})
	require.ErrorContains(t, "flag provided but not defined", err)

	// Handle Nested Subcommands

	err = app.Run([]string{"command", "bar", "subComm1"})
	require.NoError(t, err)

	err = app.Run([]string{"command", "bar", "subComm2"})
	require.NoError(t, err)

	// Should fail from unknown subcommands.
	err = app.Run([]string{"command", "bar", "subComm3"})
	require.ErrorContains(t, "unrecognized argument: subComm3", err)

	err = app.Run([]string{"command", "bar", "subComm4"})
	require.ErrorContains(t, "unrecognized argument: subComm4", err)

	// Should fail with invalid double nested subcommands.
	err = app.Run([]string{"command", "bar", "subComm1", "subComm2"})
	require.ErrorContains(t, "unrecognized argument: subComm2", err)

	err = app.Run([]string{"command", "bar", "subComm1", "subComm4"})
	require.ErrorContains(t, "unrecognized argument: subComm4", err)

	err = app.Run([]string{"command", "bar", "subComm2", "subComm1"})
	require.ErrorContains(t, "unrecognized argument: subComm1", err)

	err = app.Run([]string{"command", "bar", "subComm2", "subComm3"})
	require.ErrorContains(t, "unrecognized argument: subComm3", err)

	// Should pass with correct nested double subcommands.
	err = app.Run([]string{"command", "bar", "subComm1", "subComm3"})
	require.NoError(t, err)

	err = app.Run([]string{"command", "bar", "subComm2", "subComm4"})
	require.NoError(t, err)
}

func TestValidateNoArgs_SubcommandFlags(t *testing.T) {
	app := &cli.App{
		Before: ValidateNoArgs,
		Action: func(c *cli.Context) error {
			return nil
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name: "foo",
			},
		},
		Commands: []*cli.Command{
			{
				Name: "bar",
				Subcommands: []*cli.Command{
					{
						Name: "subComm1",
						Subcommands: []*cli.Command{
							{
								Name: "subComm3",
							},
						},
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name: "barfoo2",
							},
							&cli.BoolFlag{
								Name: "barfoo99",
							},
						},
					},
					{
						Name: "subComm2",
						Subcommands: []*cli.Command{
							{
								Name: "subComm4",
							},
						},
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name: "barfoo3",
							},
							&cli.BoolFlag{
								Name: "barfoo100",
							},
						},
					},
				},
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name: "barfoo1",
					},
				},
			},
		},
	}

	// It should not work with a bogus argument
	err := app.Run([]string{"command", "foo"})
	require.ErrorContains(t, "unrecognized argument: foo", err)
	// It should work with registered flags
	err = app.Run([]string{"command", "--foo=bar"})
	require.NoError(t, err)

	// It should work with registered flags with spaces.
	err = app.Run([]string{"command", "--foo", "bar"})
	require.NoError(t, err)

	// Handle Nested Subcommands and its flags

	err = app.Run([]string{"command", "bar", "--barfoo1=xyz"})
	require.NoError(t, err)

	err = app.Run([]string{"command", "bar", "--barfoo1", "xyz"})
	require.NoError(t, err)

	// Should pass with correct nested double subcommands.
	err = app.Run([]string{"command", "bar", "subComm1", "--barfoo2=xyz"})
	require.NoError(t, err)

	err = app.Run([]string{"command", "bar", "subComm1", "--barfoo2", "xyz"})
	require.NoError(t, err)

	err = app.Run([]string{"command", "bar", "subComm2", "--barfoo3=xyz"})
	require.NoError(t, err)

	err = app.Run([]string{"command", "bar", "subComm2", "--barfoo3", "xyz"})
	require.NoError(t, err)

	err = app.Run([]string{"command", "bar", "subComm2", "--barfoo3"})
	require.ErrorContains(t, "flag needs an argument", err)

	err = app.Run([]string{"command", "bar", "subComm1", "--barfoo99"})
	require.NoError(t, err)

	// Test edge case with boolean flags, as they do not require spaced arguments.
	app.CommandNotFound = func(context *cli.Context, s string) {
		require.Equal(t, "garbage", s)
	}
	err = app.Run([]string{"command", "bar", "subComm1", "--barfoo99", "garbage"})
	require.ErrorContains(t, "unrecognized argument: garbage", err)

	err = app.Run([]string{"command", "bar", "subComm1", "--barfoo99", "garbage", "subComm3"})
	require.ErrorContains(t, "unrecognized argument: garbage", err)

	err = app.Run([]string{"command", "bar", "subComm2", "--barfoo100", "garbage"})
	require.ErrorContains(t, "unrecognized argument: garbage", err)

	err = app.Run([]string{"command", "bar", "subComm2", "--barfoo100", "garbage", "subComm4"})
	require.ErrorContains(t, "unrecognized argument: garbage", err)
}

func TestFindUnknownConfigKeys(t *testing.T) {
	// Create a temporary directory for test files
	tempDir := t.TempDir()

	tests := []struct {
		name         string
		configContent string
		flags        []cli.Flag
		wantUnknown  []string
	}{
		{
			name:         "all keys are known",
			configContent: "verbosity: debug\ndatadir: /tmp/data",
			flags: []cli.Flag{
				&cli.StringFlag{Name: "verbosity"},
				&cli.StringFlag{Name: "datadir"},
			},
			wantUnknown: nil,
		},
		{
			name:         "single unknown key",
			configContent: "verbosity: debug\nunknown-flag: true",
			flags: []cli.Flag{
				&cli.StringFlag{Name: "verbosity"},
			},
			wantUnknown: []string{"unknown-flag"},
		},
		{
			name:         "multiple unknown keys",
			configContent: "verbosity: debug\nfake-flag: true\nanother-fake: 123",
			flags: []cli.Flag{
				&cli.StringFlag{Name: "verbosity"},
			},
			wantUnknown: []string{"another-fake", "fake-flag"}, // sorted alphabetically
		},
		{
			name:         "flag alias is recognized",
			configContent: "v: debug\no: /tmp/output",
			flags: []cli.Flag{
				&cli.StringFlag{Name: "verbosity", Aliases: []string{"v"}},
				&cli.StringFlag{Name: "output-file", Aliases: []string{"o"}},
			},
			wantUnknown: nil,
		},
		{
			name:         "empty config file",
			configContent: "",
			flags: []cli.Flag{
				&cli.StringFlag{Name: "verbosity"},
			},
			wantUnknown: nil,
		},
		{
			name:         "no flags defined",
			configContent: "verbosity: debug",
			flags:        []cli.Flag{},
			wantUnknown:  []string{"verbosity"},
		},
		{
			name:         "beacon-only flag in validator config",
			configContent: "verbosity: debug\nsubscribe-all-subnets: true",
			flags: []cli.Flag{
				&cli.StringFlag{Name: "verbosity"},
				// subscribe-all-subnets is NOT in the validator flags
			},
			wantUnknown: []string{"subscribe-all-subnets"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write config file
			configPath := filepath.Join(tempDir, tt.name+".yaml")
			require.NoError(t, os.WriteFile(configPath, []byte(tt.configContent), 0600))

			// Find unknown keys
			unknownKeys := findUnknownConfigKeys(configPath, tt.flags)

			// Verify results
			if len(tt.wantUnknown) == 0 {
				require.Equal(t, 0, len(unknownKeys), "expected no unknown keys but got: %v", unknownKeys)
			} else {
				require.DeepEqual(t, tt.wantUnknown, unknownKeys)
			}
		})
	}
}

func TestFindUnknownConfigKeys_FileErrors(t *testing.T) {
	// Test with non-existent file
	unknownKeys := findUnknownConfigKeys("/nonexistent/path/config.yaml", []cli.Flag{
		&cli.StringFlag{Name: "verbosity"},
	})
	require.Equal(t, 0, len(unknownKeys), "should return nil for non-existent file")

	// Test with invalid YAML
	tempDir := t.TempDir()
	invalidYamlPath := filepath.Join(tempDir, "invalid.yaml")
	require.NoError(t, os.WriteFile(invalidYamlPath, []byte("invalid: yaml: content: ["), 0600))

	unknownKeys = findUnknownConfigKeys(invalidYamlPath, []cli.Flag{
		&cli.StringFlag{Name: "verbosity"},
	})
	require.Equal(t, 0, len(unknownKeys), "should return nil for invalid YAML")
}

func TestLoadFlagsFromConfig_WarnsOnUnknownKeys(t *testing.T) {
	// This test verifies the integration - that LoadFlagsFromConfig properly
	// calls the unknown key detection and still loads valid flags correctly.
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-config.yaml")

	// Config with one valid flag and one unknown flag
	configContent := "testflag: 100\nunknown-option: true"
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0600))

	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	context := cli.NewContext(&app, set, nil)

	require.NoError(t, set.Parse([]string{"test-command", "--" + ConfigFileFlag.Name, configPath}))

	comFlags := WrapFlags([]cli.Flag{
		&cli.StringFlag{
			Name: ConfigFileFlag.Name,
		},
		&cli.IntFlag{
			Name:  "testflag",
			Value: 0,
		},
	})

	command := &cli.Command{
		Name:  "test-command",
		Flags: comFlags,
		Before: func(cliCtx *cli.Context) error {
			return LoadFlagsFromConfig(cliCtx, comFlags)
		},
		Action: func(cliCtx *cli.Context) error {
			// Valid flag should still be loaded correctly
			require.Equal(t, 100, cliCtx.Int("testflag"))
			return nil
		},
	}

	// The command should succeed even with unknown keys (they're just warnings)
	require.NoError(t, command.Run(context, context.Args().Slice()...))
}
