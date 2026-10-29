package blocks_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func BenchmarkAttestationSignatureBatch(b *testing.B) {
	for _, n := range []uint64{100_000, 500_000} {
		// All test states share one seed, so a cache entry from the previous size would be stale.
		helpers.ClearCache()
		st, _, att := attestationTestState(b, n, 0.99)
		atts := []ethpb.Att{att}
		ctx := context.Background()

		// Warm the committee cache, its aggregates and the global pubkey cache, as on a live node.
		_, err := blocks.AttestationSignatureBatch(ctx, st, atts)
		require.NoError(b, err)
		require.Eventually(b, func() bool {
			keys, err := helpers.AttestationCommitteeAggregatesFromCache(ctx, st, att)
			return err == nil && keys != nil
		}, time.Minute, time.Millisecond)

		b.Run(fmt.Sprintf("batch/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, err := blocks.AttestationSignatureBatch(ctx, st, atts)
				require.NoError(b, err)
			}
		})

		committees, err := helpers.AttestationCommitteesFromState(ctx, st, att)
		require.NoError(b, err)
		ia, err := attestation.ConvertToIndexed(ctx, att, committees...)
		require.NoError(b, err)

		indices := ia.GetAttestingIndices()
		b.Run(fmt.Sprintf("AggregateKeyFromIndices/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, err := st.AggregateKeyFromIndices(indices)
				require.NoError(b, err)
			}
		})
	}
	helpers.ClearCache()
}
