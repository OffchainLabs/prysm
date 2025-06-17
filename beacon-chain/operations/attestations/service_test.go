package attestations

import (
	"context"
	"errors"
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestStop_OK(t *testing.T) {
	s, err := NewService(t.Context(), &Config{})
	require.NoError(t, err)
	require.NoError(t, s.Stop(), "Unable to stop attestation pool service")
	assert.ErrorContains(t, context.Canceled.Error(), s.ctx.Err(), "Context was not canceled")
}

func TestStatus_Error(t *testing.T) {
	err := errors.New("bad bad bad")
	s := &Service{err: err}
	assert.ErrorContains(t, s.err.Error(), s.Status())
}
