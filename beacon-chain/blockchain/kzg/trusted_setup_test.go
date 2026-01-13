package kzg

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestStart(t *testing.T) {
	require.NoError(t, Start())
	require.NotNil(t, kzgContext)
}
