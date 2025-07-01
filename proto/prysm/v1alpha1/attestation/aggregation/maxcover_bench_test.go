package aggregation_test

import (
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1/attestation/aggregation"
	aggtesting "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1/attestation/aggregation/testing"
	"github.com/prysmaticlabs/go-bitfield"
)

func BenchmarkMaxCoverProblem_MaxCover(b *testing.B) {
	bitlistLen := params.BeaconConfig().MaxValidatorsPerCommittee
	tests := []struct {
		numCandidates uint64
		numMarkedBits uint64
		allowOverlaps bool
	}{
		{
			numCandidates: 32,
			numMarkedBits: 1,
		},
		{
			numCandidates: 128,
			numMarkedBits: 1,
		},
		{
			numCandidates: 256,
			numMarkedBits: 1,
		},
		{
			numCandidates: 32,
			numMarkedBits: 8,
		},
		{
			numCandidates: 1024,
			numMarkedBits: 8,
		},
		{
			numCandidates: 2048,
			numMarkedBits: 8,
		},
		{
			numCandidates: 1024,
			numMarkedBits: 32,
		},
		{
			numCandidates: 2048,
			numMarkedBits: 32,
		},
		{
			numCandidates: 1024,
			numMarkedBits: 128,
		},
		{
			numCandidates: 2048,
			numMarkedBits: 128,
		},
		{
			numCandidates: 1024,
			numMarkedBits: 512,
		},
		{
			numCandidates: 2048,
			numMarkedBits: 512,
		},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%d_aggregations_with_%d_bit(s)_set", tt.numCandidates, tt.numMarkedBits)
		b.Run(fmt.Sprintf("cur_%s", name), func(b *testing.B) {
			b.StopTimer()
			var bitlists []bitfield.Bitlist
			if tt.numMarkedBits == 1 {
				bitlists = aggtesting.BitlistsWithSingleBitSet(tt.numCandidates, bitlistLen)
			} else {
				bitlists = aggtesting.BitlistsWithMultipleBitSet(b, tt.numCandidates, bitlistLen, tt.numMarkedBits)

			}
			b.StartTimer()
			for i := 0; i < b.N; i++ {
				candidates := make([]*aggregation.MaxCoverCandidate, len(bitlists))
				for i := 0; i < len(bitlists); i++ {
					candidates[i] = aggregation.NewMaxCoverCandidate(i, &bitlists[i])
				}
				mc := &aggregation.MaxCoverProblem{Candidates: candidates}
				_, err := mc.Cover(len(bitlists), tt.allowOverlaps)
				_ = err
			}
		})
		b.Run(fmt.Sprintf("new_%s", name), func(b *testing.B) {
			b.StopTimer()
			var bitlists []*bitfield.Bitlist64
			if tt.numMarkedBits == 1 {
				bitlists = aggtesting.Bitlists64WithSingleBitSet(tt.numCandidates, bitlistLen)
			} else {
				bitlists = aggtesting.Bitlists64WithMultipleBitSet(b, tt.numCandidates, bitlistLen, tt.numMarkedBits)

			}
			b.StartTimer()
			for i := 0; i < b.N; i++ {
				_, _, err := aggregation.MaxCover(bitlists, len(bitlists), tt.allowOverlaps)
				_ = err
			}
		})
	}
}
