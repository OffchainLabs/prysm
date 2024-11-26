package validator

import (
	"slices"

	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// computeOnChainAggregate constructs a final aggregate form a list of network aggregates with equal attestation data.
// It assumes that each network aggregate has exactly one committee bit set.
//
// Our implementation allows to pass aggregates for different attestation data, in which case the function will return
// one final aggregate per attestation data.
//
// Spec definition:
//
//	def compute_on_chain_aggregate(network_aggregates: Sequence[Attestation]) -> Attestation:
//		aggregates = sorted(network_aggregates, key=lambda a: get_committee_indices(a.committee_bits)[0])
//
//		data = aggregates[0].data
//		aggregation_bits = Bitlist[MAX_VALIDATORS_PER_COMMITTEE * MAX_COMMITTEES_PER_SLOT]()
//		for a in aggregates:
//			for b in a.aggregation_bits:
//				aggregation_bits.append(b)
//
//		signature = bls.Aggregate([a.signature for a in aggregates])
//
//		committee_indices = [get_committee_indices(a.committee_bits)[0] for a in aggregates]
//		committee_flags = [(index in committee_indices) for index in range(0, MAX_COMMITTEES_PER_SLOT)]
//		committee_bits = Bitvector[MAX_COMMITTEES_PER_SLOT](committee_flags)
//
//		return Attestation(
//			aggregation_bits=aggregation_bits,
//			data=data,
//			committee_bits=committee_bits,
//			signature=signature,
//		)
func computeOnChainAggregate(aggregates []ethpb.Att) ([]ethpb.Att, error) {
	aggsByDataRoot := make(map[[32]byte][]ethpb.Att)
	for _, agg := range aggregates {
		key, err := agg.GetData().HashTreeRoot()
		if err != nil {
			return nil, err
		}
		existing, ok := aggsByDataRoot[key]
		if ok {
			aggsByDataRoot[key] = append(existing, agg)
		} else {
			aggsByDataRoot[key] = []ethpb.Att{agg}
		}
	}

	result := make([]ethpb.Att, 0)

	for _, aggs := range aggsByDataRoot {
		slices.SortFunc(aggs, func(a, b ethpb.Att) int {
			return a.CommitteeBitsVal().BitIndices()[0] - b.CommitteeBitsVal().BitIndices()[0]
		})

		sigs := make([]bls.Signature, len(aggs))
		committeeIndices := make([]primitives.CommitteeIndex, len(aggs))
		aggBitsIndices := make([]uint64, 0)
		aggBitsOffset := uint64(0)
		var err error
		for i, a := range aggs {
			for _, bi := range a.GetAggregationBits().BitIndices() {
				aggBitsIndices = append(aggBitsIndices, uint64(bi)+aggBitsOffset)
			}
			sigs[i], err = bls.SignatureFromBytes(a.GetSignature())
			if err != nil {
				return nil, err
			}
			committeeIndices[i] = helpers.CommitteeIndices(a.CommitteeBitsVal())[0]

			aggBitsOffset += a.GetAggregationBits().Len()
		}

		aggregationBits := bitfield.NewBitlist(aggBitsOffset)
		for _, bi := range aggBitsIndices {
			aggregationBits.SetBitAt(bi, true)
		}

		cb := primitives.NewAttestationCommitteeBits()
		att := &ethpb.AttestationElectra{
			AggregationBits: aggregationBits,
			Data:            aggs[0].GetData(),
			CommitteeBits:   cb,
			Signature:       bls.AggregateSignatures(sigs).Marshal(),
		}
		for _, ci := range committeeIndices {
			att.CommitteeBits.SetBitAt(uint64(ci), true)
		}
		result = append(result, att)
	}

	return result, nil
}
