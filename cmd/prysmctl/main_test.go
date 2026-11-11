package main

import (
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/urfave/cli/v2"
)

func TestWithoutUsage(t *testing.T) {
	t.Run("default template", func(t *testing.T) {
		tmpl := withoutUsage(cli.AppHelpTemplate)
		require.Equal(t, false, strings.Contains(tmpl, "USAGE:"))
		require.Equal(t, true, strings.Contains(tmpl, "VERSION:"))
	})
}
