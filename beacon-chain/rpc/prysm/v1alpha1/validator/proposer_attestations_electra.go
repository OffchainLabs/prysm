package validator

import (
	"cmp"
	"container/heap"
	"context"
	"slices"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	coretime "github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
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
			return cmp.Compare(a.GetCommitteeIndex(), b.GetCommitteeIndex())
		})

		sigs := make([]bls.Signature, len(aggs))
		cb := primitives.NewAttestationCommitteeBits()
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
			cb.SetBitAt(uint64(a.GetCommitteeIndex()), true)

			aggBitsOffset += a.GetAggregationBits().Len()
		}

		aggregationBits := bitfield.NewBitlist(aggBitsOffset)
		for _, bi := range aggBitsIndices {
			aggregationBits.SetBitAt(bi, true)
		}

		att := &ethpb.AttestationElectra{
			AggregationBits: aggregationBits,
			Data:            aggs[0].GetData(),
			CommitteeBits:   cb,
			Signature:       bls.AggregateSignatures(sigs).Marshal(),
		}
		result = append(result, att)
	}

	return result, nil
}

type flagReward struct {
	position uint8
	weight   uint64
}

type attCandidate struct {
	att              ethpb.Att
	attestingIndices []uint64
	flags            []flagReward
	flagMask         uint8
	targetsCurrEpoch bool
	score            uint64
	scoredRound      int
}

type candidateHeap []*attCandidate

func (h candidateHeap) Len() int           { return len(h) }
func (h candidateHeap) Less(i, j int) bool { return h[i].score > h[j].score }
func (h candidateHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *candidateHeap) Push(x any)        { *h = append(*h, x.(*attCandidate)) }
func (h *candidateHeap) Pop() any {
	old := *h
	n := len(old)
	c := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return c
}

// Marginal reward never grows as more attestations are selected, so a score computed in an
// earlier round is an upper bound on the current one. The loop below depends on that to leave
// stale scores in the heap and to drop candidates that reach zero.
func (a proposerAtts) selectByMarginalReward(ctx context.Context, st state.ReadOnlyBeaconState, limit uint64) (proposerAtts, error) {
	if len(a) == 0 || limit == 0 {
		return proposerAtts{}, nil
	}

	totalBalance, err := helpers.TotalActiveBalance(ctx, st)
	if err != nil {
		return nil, errors.Wrap(err, "could not get total active balance")
	}
	// Unlike the ReadOnly variants, these return copies that are safe to mutate.
	currParticipation, err := st.CurrentEpochParticipation()
	if err != nil {
		return nil, errors.Wrap(err, "could not get current epoch participation")
	}
	prevParticipation, err := st.PreviousEpochParticipation()
	if err != nil {
		return nil, errors.Wrap(err, "could not get previous epoch participation")
	}

	candidates, err := newAttCandidates(ctx, st, a)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return proposerAtts{}, nil
	}

	participationFor := func(c *attCandidate) []byte {
		if c.targetsCurrEpoch {
			return currParticipation
		}
		return prevParticipation
	}

	h := candidateHeap(candidates)
	for _, c := range h {
		c.score, err = c.marginalReward(st, participationFor(c), totalBalance)
		if err != nil {
			return nil, err
		}
		c.scoredRound = 1
	}
	heap.Init(&h)

	selected := make(proposerAtts, 0, limit)
	for round := 1; uint64(len(selected)) < limit && h.Len() > 0; round++ {
		var best *attCandidate
		for h.Len() > 0 {
			c := heap.Pop(&h).(*attCandidate)
			if c.scoredRound != round {
				c.score, err = c.marginalReward(st, participationFor(c), totalBalance)
				if err != nil {
					return nil, err
				}
				c.scoredRound = round
			}
			if c.score == 0 {
				continue
			}
			if h.Len() == 0 || c.score >= h[0].score {
				best = c
				break
			}
			heap.Push(&h, c)
		}
		if best == nil {
			break
		}
		selected = append(selected, best.att)
		if err := best.markCovered(participationFor(best)); err != nil {
			return nil, err
		}
	}

	return selected, nil
}

func newAttCandidates(ctx context.Context, st state.ReadOnlyBeaconState, atts proposerAtts) ([]*attCandidate, error) {
	cfg := params.BeaconConfig()
	weights := []flagReward{
		{cfg.TimelySourceFlagIndex, cfg.TimelySourceWeight},
		{cfg.TimelyTargetFlagIndex, cfg.TimelyTargetWeight},
		{cfg.TimelyHeadFlagIndex, cfg.TimelyHeadWeight},
	}
	currentEpoch := coretime.CurrentEpoch(st)

	committeesBySlot := make(map[primitives.Slot][][]primitives.ValidatorIndex)
	flagsByData := make(map[[32]byte][]flagReward)

	candidates := make([]*attCandidate, 0, len(atts))
	for _, att := range atts {
		data := att.GetData()

		delay, err := st.Slot().SafeSubSlot(data.Slot)
		if err != nil {
			log.WithFields(attestationFields(att)).Debug("Skipping attestation from a future slot")
			continue
		}

		dataRoot, err := data.HashTreeRoot()
		if err != nil {
			return nil, errors.Wrap(err, "could not hash attestation data")
		}
		flags, ok := flagsByData[dataRoot]
		if !ok {
			participated, err := altair.AttestationParticipationFlagIndices(st, data, delay)
			if err != nil {
				log.WithFields(attestationFields(att)).WithError(err).Debug("Could not get participation flag indices")
				flagsByData[dataRoot] = nil
				continue
			}
			for _, w := range weights {
				if participated[w.position] {
					flags = append(flags, w)
				}
			}
			flagsByData[dataRoot] = flags
		}
		if len(flags) == 0 {
			continue
		}

		slotCommittees, ok := committeesBySlot[data.Slot]
		if !ok {
			slotCommittees, err = helpers.BeaconCommittees(ctx, st, data.Slot)
			if err != nil {
				return nil, errors.Wrap(err, "could not get beacon committees")
			}
			committeesBySlot[data.Slot] = slotCommittees
		}

		committees, err := attCommittees(att, slotCommittees)
		if err != nil {
			log.WithFields(attestationFields(att)).WithError(err).Debug("Could not get attestation committees")
			continue
		}
		indices, err := attestation.AttestingIndices(att, committees...)
		if err != nil {
			log.WithFields(attestationFields(att)).WithError(err).Debug("Could not get attesting indices")
			continue
		}

		c := &attCandidate{
			att:              att,
			attestingIndices: indices,
			flags:            flags,
			targetsCurrEpoch: data.Target.Epoch == currentEpoch,
		}
		for _, f := range flags {
			c.flagMask |= 1 << f.position
		}
		candidates = append(candidates, c)
	}

	return candidates, nil
}

func attCommittees(att ethpb.Att, slotCommittees [][]primitives.ValidatorIndex) ([][]primitives.ValidatorIndex, error) {
	if att.Version() < version.Electra {
		return nil, errors.Errorf("attestation version %s does not have committee bits", version.String(att.Version()))
	}

	indices := att.CommitteeBitsVal().BitIndices()
	committees := make([][]primitives.ValidatorIndex, len(indices))
	for i, ci := range indices {
		if ci >= len(slotCommittees) {
			return nil, errors.Errorf("committee index %d exceeds committee count %d", ci, len(slotCommittees))
		}
		committees[i] = slotCommittees[ci]
	}
	return committees, nil
}

// Mirrors electra.GetProposerRewardNumerator, but scored against participation as the block
// being built would leave it rather than against the untouched pre-block state.
func (c *attCandidate) marginalReward(st state.ReadOnlyBeaconState, participation []byte, totalBalance uint64) (uint64, error) {
	var numerator uint64
	for _, index := range c.attestingIndices {
		if index >= uint64(len(participation)) {
			return 0, errors.Errorf("index %d exceeds participation length %d", index, len(participation))
		}
		missing := c.flagMask &^ participation[index]
		if missing == 0 {
			continue
		}
		baseReward, err := altair.BaseRewardWithTotalBalance(st, primitives.ValidatorIndex(index), totalBalance)
		if err != nil {
			return 0, errors.Wrap(err, "could not get base reward")
		}
		for _, f := range c.flags {
			if missing&(1<<f.position) != 0 {
				numerator += baseReward * f.weight
			}
		}
	}
	return numerator, nil
}

func (c *attCandidate) markCovered(participation []byte) error {
	for _, index := range c.attestingIndices {
		if index >= uint64(len(participation)) {
			return errors.Errorf("index %d exceeds participation length %d", index, len(participation))
		}
		participation[index] |= c.flagMask
	}
	return nil
}
