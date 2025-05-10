package kv

import (
	"context"
	"testing"

	testpb "github.com/OffchainLabs/prysm/v6/proto/testing"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func Test_encode_handlesNilFromFunction(t *testing.T) {
	foo := func() *testpb.Puzzle {
		return nil
	}
	_, err := encode(context.Background(), foo())
	require.ErrorContains(t, "cannot encode nil message", err)
}
