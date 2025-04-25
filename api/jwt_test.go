package api

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestGenerateRandomHexString(t *testing.T) {
	token, err := GenerateRandomHexString()
	require.NoError(t, err)
	require.NoError(t, ValidateAuthToken(token))
}
