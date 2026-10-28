package cmd

import (
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

// This code was adapted from https://github.com/ethereum/go-ethereum/blob/master/internal/flags/helpers.go

const (
	// colorPrysmBlue is a lighter shade of the Prysm logo blue (#2ba1f0), as a 24-bit color (#5cb8f5).
	colorPrysmBlue = "\u001B[38;2;92;184;245m"
	// colorPrysmBlue256 is the closest 256-color palette entry, for terminals without 24-bit colors.
	colorPrysmBlue256 = "\u001B[38;5;75m"
	colorReset        = "\u001B[0m"
	styleBold         = "\u001B[1m"
)

var (
	// useColor defines whether the CLI help should use colored output.
	useColor = term.IsTerminal(int(os.Stdout.Fd())) && os.Getenv("TERM") != "dumb" && os.Getenv("NO_COLOR") == ""

	// headerColor is the color of the help section headers.
	headerColor = prysmBlue(os.Getenv("COLORTERM"))
)

// Authors are the authors shown in the help of every Prysm binary.
var Authors = []*cli.Author{{Name: "The " + bold("Prysm") + " team by " + bold("Offchain")}}

var (
	helpHeaderRegex      = regexp.MustCompile("[A-Z][A-Z ]*:")
	enableHelpColorsOnce sync.Once
)

// EnableHelpColors colors the section headers of the help templates.
// It does nothing if the standard output is not a terminal.
func EnableHelpColors() {
	if !useColor {
		return
	}

	enableHelpColorsOnce.Do(func() {
		cli.AppHelpTemplate = colorizeHelpTemplate(cli.AppHelpTemplate)
		cli.CommandHelpTemplate = colorizeHelpTemplate(cli.CommandHelpTemplate)
		cli.SubcommandHelpTemplate = colorizeHelpTemplate(cli.SubcommandHelpTemplate)
	})
}

// prysmBlue returns the Prysm blue escape sequence supported by the terminal, given its COLORTERM value.
func prysmBlue(colorTerm string) string {
	if colorTerm == "truecolor" || colorTerm == "24bit" {
		return colorPrysmBlue
	}

	return colorPrysmBlue256
}

// colorizeHelpTemplate wraps the section headers of a help template in Prysm blue.
func colorizeHelpTemplate(tmpl string) string {
	tmpl = helpHeaderRegex.ReplaceAllString(tmpl, headerColor+"$0"+colorReset)

	// Flag group headers are built as `{{.Name}} OPTIONS:`. Start the color before the group name.
	return strings.ReplaceAll(tmpl, "{{.Name}} "+headerColor, headerColor+"{{.Name}} ")
}

// bold makes the text bold if the CLI help uses colored output.
func bold(text string) string {
	if !useColor {
		return text
	}

	return styleBold + text + colorReset
}
