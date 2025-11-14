package interfaces

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/pkg/errors"
)

func TestNewInvalidCastError(t *testing.T) {
	err := NewInvalidCastError(version.Phase0, version.Electra)
	require.Equal(t, true, errors.Is(err, ErrInvalidCast))
}
