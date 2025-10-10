package sync

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/go-bitfield"
)

// Benchmark for processAttestations bucketing logic with various distributions
func BenchmarkProcessAttestationsBucketing(b *testing.B) {
	benchmarks := []struct {
		name             string
		numAtts          int
		numUniqueBuckets int
	}{
		{"10atts_1bucket", 10, 1},
		{"10atts_5buckets", 10, 5},
		{"10atts_10buckets", 10, 10},
		{"100atts_1bucket", 100, 1},
		{"100atts_10buckets", 100, 10},
		{"100atts_50buckets", 100, 50},
		{"1000atts_1bucket", 1000, 1},
		{"1000atts_10buckets", 1000, 10},
		{"1000atts_100buckets", 1000, 100},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Create attestations distributed across buckets
			attestations := make([]ethpb.Att, 0, bm.numAtts)
			root := bytesutil.PadTo([]byte("test-root"), 32)

			for i := 0; i < bm.numAtts; i++ {
				bucketIdx := i % bm.numUniqueBuckets
				aggBits := bitfield.NewBitlist(8)
				aggBits.SetBitAt(uint64(i%8), true)

				att := &ethpb.Attestation{
					Data: &ethpb.AttestationData{
						Slot:            primitives.Slot(bucketIdx),
						BeaconBlockRoot: root,
						Source:          &ethpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
						Target:          &ethpb.Checkpoint{Epoch: 0, Root: root},
						CommitteeIndex:  primitives.CommitteeIndex(bucketIdx),
					},
					AggregationBits: aggBits,
					Signature:       make([]byte, 96),
				}

				attestations = append(attestations, att)
			}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				// Benchmark the old 3-loop implementation
				atts := make([]ethpb.Att, 0, len(attestations))
				for _, att := range attestations {
					atts = append(atts, att)
				}

				_ = bucketAttestationsByData(atts)
			}
		})
	}
}
