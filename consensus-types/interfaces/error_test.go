package interfaces

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestNewInvalidCastError(t *testing.T) {
	err := NewInvalidCastError(version.Phase0, version.Electra)
	require.Equal(t, true, errors.Is(err, ErrInvalidCast))
}
