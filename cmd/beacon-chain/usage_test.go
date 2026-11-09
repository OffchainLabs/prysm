package main

import (
	"slices"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/urfave/cli/v2"
)

func TestAllFlagsExistInHelp(t *testing.T) {
	// If this test is failing, it is because you've recently added/removed a
	// flag in beacon chain main.go, but did not add/remove it to the usage.go
	// flag grouping (appHelpFlagGroups).

	var helpFlags []cli.Flag
	for _, group := range appHelpFlagGroups {
		helpFlags = append(helpFlags, group.Flags...)
	}
	helpFlags = features.ActiveFlags(helpFlags)
	appFlags = features.ActiveFlags(appFlags)

	// Aliases are displayed as separate flags in the help.
	helpNames := flagNames(helpFlags)
	appNames := flagNames(appFlags)

	for _, name := range appNames {
		if !slices.Contains(helpNames, name) {
			t.Errorf("Flag %s does not exist in help/usage flags.", name)
		}
	}

	for _, name := range helpNames {
		if !slices.Contains(appNames, name) {
			t.Errorf("Flag %s does not exist in main.go, "+
				"but exists in help flags", name)
		}
	}
}

func flagNames(flags []cli.Flag) []string {
	var names []string
	for _, flag := range flags {
		names = append(names, flag.Names()...)
	}

	return names
}
