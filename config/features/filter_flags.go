package features

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/urfave/cli/v2"
)

// ActiveFlags returns all of the flags that are not Hidden.
func ActiveFlags(flags []cli.Flag) []cli.Flag {
	visible := make([]cli.Flag, 0, len(flags))
	for _, flag := range flags {
		field := flagValue(flag).FieldByName("Hidden")
		if !field.IsValid() || !field.Bool() {
			visible = append(visible, flag)
		}
	}
	return visible
}

// WithoutFlags returns all of the flags that are not in excluded.
func WithoutFlags(flags, excluded []cli.Flag) []cli.Flag {
	kept := make([]cli.Flag, 0, len(flags))
	for _, flag := range flags {
		if !slices.Contains(excluded, flag) {
			kept = append(kept, flag)
		}
	}
	return kept
}

// SplitAliases returns a copy of each flag without its deprecated aliases, and one flag per deprecated alias.
// Each deprecated alias flag is a copy of its original flag, renamed after the alias.
// The returned flags are only meant to be displayed in the help, not to be registered.
func SplitAliases(flags []cli.Flag, deprecated []string) (withoutAliases, aliases []cli.Flag) {
	withoutAliases = make([]cli.Flag, 0, len(flags))
	for _, flag := range flags {
		name, flagAliases := flag.Names()[0], flag.Names()[1:]

		var kept, moved []string
		for _, alias := range flagAliases {
			if slices.Contains(deprecated, alias) {
				moved = append(moved, alias)
				continue
			}
			kept = append(kept, alias)
		}

		if len(moved) == 0 {
			withoutAliases = append(withoutAliases, flag)
			continue
		}

		withoutAlias, ok := copyFlag(flag, name, kept, "")
		if !ok {
			withoutAliases = append(withoutAliases, flag)
			continue
		}

		withoutAliases = append(withoutAliases, withoutAlias)
		for _, alias := range moved {
			aliasFlag, _ := copyFlag(flag, alias, nil, fmt.Sprintf("Deprecated alias of --%s.", name))
			aliases = append(aliases, aliasFlag)
		}
	}
	return withoutAliases, aliases
}

// copyFlag returns a copy of the flag, with the given name and aliases.
// If usage is not empty, it replaces the usage of the copy.
func copyFlag(flag cli.Flag, name string, aliases []string, usage string) (cli.Flag, bool) {
	original := reflect.ValueOf(flag)
	if original.Kind() != reflect.Pointer || original.Elem().Kind() != reflect.Struct {
		return nil, false
	}

	copied := reflect.New(original.Elem().Type())
	copied.Elem().Set(original.Elem())

	nameField := copied.Elem().FieldByName("Name")
	aliasesField := copied.Elem().FieldByName("Aliases")
	usageField := copied.Elem().FieldByName("Usage")
	if !nameField.CanSet() || !aliasesField.CanSet() || !usageField.CanSet() {
		return nil, false
	}

	nameField.SetString(name)
	aliasesField.Set(reflect.ValueOf(aliases))
	if usage != "" {
		usageField.SetString(usage)
	}

	copiedFlag, ok := copied.Interface().(cli.Flag)
	return copiedFlag, ok
}

func flagValue(f cli.Flag) reflect.Value {
	fv := reflect.ValueOf(f)
	for fv.Kind() == reflect.Pointer {
		fv = reflect.Indirect(fv)
	}
	return fv
}
