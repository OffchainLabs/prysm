package cmd

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/urfave/cli/v2"
)

func TestColorizeHelpTemplate(t *testing.T) {
	t.Run("static headers", func(t *testing.T) {
		actual := colorizeHelpTemplate("NAME:\n   {{.App.Name}}\nGLOBAL OPTIONS:\n")
		require.Equal(t, headerColor+"NAME:"+colorReset+"\n   {{.App.Name}}\n"+headerColor+"GLOBAL OPTIONS:"+colorReset+"\n", actual)
	})

	t.Run("flag group headers", func(t *testing.T) {
		actual := colorizeHelpTemplate("{{range .FlagGroups}}{{.Name}} OPTIONS:\n")
		require.Equal(t, "{{range .FlagGroups}}"+headerColor+"{{.Name}} OPTIONS:"+colorReset+"\n", actual)
	})

	t.Run("no header", func(t *testing.T) {
		const tmpl = "{{if .App.Commands}} command [command options]{{end}}"
		require.Equal(t, tmpl, colorizeHelpTemplate(tmpl))
	})
}

func TestPrysmBlue(t *testing.T) {
	t.Run("24-bit colors", func(t *testing.T) {
		require.Equal(t, "\u001B[38;2;92;184;245m", prysmBlue("truecolor"))
		require.Equal(t, "\u001B[38;2;92;184;245m", prysmBlue("24bit"))
	})

	t.Run("256 colors", func(t *testing.T) {
		require.Equal(t, "\u001B[38;5;75m", prysmBlue(""))
	})
}

func TestBold(t *testing.T) {
	original := useColor
	t.Cleanup(func() { useColor = original })

	t.Run("color", func(t *testing.T) {
		useColor = true
		require.Equal(t, "\u001B[1mPrysm\u001B[0m", bold("Prysm"))
	})

	t.Run("no color", func(t *testing.T) {
		useColor = false
		require.Equal(t, "Prysm", bold("Prysm"))
	})
}

func TestEnableHelpColors(t *testing.T) {
	t.Run("no color", func(t *testing.T) {
		original := useColor
		useColor = false
		t.Cleanup(func() { useColor = original })

		template := cli.AppHelpTemplate
		EnableHelpColors()
		require.Equal(t, template, cli.AppHelpTemplate)
	})
}
