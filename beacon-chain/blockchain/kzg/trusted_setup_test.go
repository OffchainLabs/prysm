package kzg

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestStart(t *testing.T) {
	require.NoError(t, Start())
	require.NotNil(t, kzgContext)
}
