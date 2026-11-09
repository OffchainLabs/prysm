package valid_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/internal/valid"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// counter reports a fixed length and panics on a nil receiver, so the tests
// prove that Len never reaches the method when the pointer is nil.
type counter struct{ n int }

func (c *counter) Len() int { return c.n }

func TestLen(t *testing.T) {
	var nilCounter *counter
	require.Equal(t, 0, valid.Len(nilCounter))
	require.Equal(t, 0, valid.Len(&counter{}))
	require.Equal(t, 3, valid.Len(&counter{n: 3}))
}
