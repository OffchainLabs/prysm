package main

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestAllAnalyzers(t *testing.T) {
	analyzers, err := allAnalyzers()
	require.NoError(t, err)
	require.Equal(t, len(prysmAnalyzers)+len(goAnalyzers)+len(staticcheckChecks), len(analyzers))

	names := make(map[string]bool, len(analyzers))
	for _, a := range analyzers {
		require.Equal(t, false, names[a.Name], "duplicate analyzer %s", a.Name)
		names[a.Name] = true
	}
}
