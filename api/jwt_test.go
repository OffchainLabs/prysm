package api

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestGenerateRandomHexString(t *testing.T) {
	token, err := GenerateRandomHexString()
	require.NoError(t, err)
	require.NoError(t, ValidateAuthToken(token))
}
