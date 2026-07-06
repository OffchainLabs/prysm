package client

import (
	"testing"

	grpcutil "github.com/OffchainLabs/prysm/v7/api/grpc"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
)

func TestValidator_connTracker(t *testing.T) {
	t.Run("nil conn generation is zero", func(t *testing.T) {
		v := &validator{}
		require.Equal(t, uint64(0), v.connGeneration())
		require.Equal(t, false, v.connTracker.changed(proposerPrefsPush, v.connGeneration()))
	})

	t.Run("change persists until the push is confirmed", func(t *testing.T) {
		provider := &grpcutil.MockGrpcProvider{MockHosts: []string{"node-a:4000"}}
		conn, err := validatorHelpers.NewNodeConnection(validatorHelpers.WithGRPCProvider(provider))
		require.NoError(t, err)
		v := &validator{conn: conn}

		// Counter starts at 0, matching the zero confirmed generation.
		require.Equal(t, false, v.connTracker.changed(proposerPrefsPush, v.connGeneration()))

		// A fallback switch bumps the counter — detected, and stays detected
		// until a push is confirmed (e.g. the first submission failed).
		provider.ConnCounter = 1
		gen := v.connGeneration()
		require.Equal(t, true, v.connTracker.changed(proposerPrefsPush, gen))
		require.Equal(t, true, v.connTracker.changed(proposerPrefsPush, v.connGeneration()))

		v.connTracker.confirm(proposerPrefsPush, gen)
		require.Equal(t, false, v.connTracker.changed(proposerPrefsPush, v.connGeneration()))

		// A round-robin bounce (host0 → host1 → host0) leaves the host unchanged
		// but advances the counter twice; still detected.
		provider.ConnCounter = 3
		gen = v.connGeneration()
		require.Equal(t, true, v.connTracker.changed(proposerPrefsPush, gen))
		// A stale confirmation from an in-flight push started before the switch
		// must not mask the newer generation.
		v.connTracker.confirm(proposerPrefsPush, 1)
		require.Equal(t, true, v.connTracker.changed(proposerPrefsPush, v.connGeneration()))
		v.connTracker.confirm(proposerPrefsPush, gen)
		require.Equal(t, false, v.connTracker.changed(proposerPrefsPush, v.connGeneration()))
	})
}
