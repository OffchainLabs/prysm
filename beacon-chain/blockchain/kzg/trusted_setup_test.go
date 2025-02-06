package kzg

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v6/testing/require"
)

func TestStart(t *testing.T) {
	require.NoError(t, Start())
	require.NotNil(t, kzgContext)
}
