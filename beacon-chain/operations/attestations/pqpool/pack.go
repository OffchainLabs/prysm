package pqpool

import (
	"cmp"
	"context"
	"math"
	"slices"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq/timing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
)

// Spec: MAX_STARK_PROOF_SIZE
const maxProofSize = 1 << 20

type group struct {
	data       *ethpb.AttestationData
	signatures map[primitives.CommitteeIndex]map[primitives.ValidatorIndex][]byte
	aggregates map[primitives.CommitteeIndex][]*Aggregate
}

type candidate struct {
	bits       bitfield.Bitlist
	signatures [][]byte
	proof      []byte
	fresh      int
	size       int
}

type pendingProof struct {
	child      int
	signatures [][]byte
}

type Packing struct {
	Atts       []ethpb.Att
	Validators uint64
	// Voters whose target vote is not on chain yet, the measure one packing beats another by.
	Gain      int
	slot      primitives.Slot
	reference primitives.Slot
	children  []pq.ProofChild
	pending   []pendingProof
}

func Pack(ctx context.Context, st state.ReadOnlyBeaconState, singles []*ethpb.SingleAttestationPQ, aggregates []*Aggregate) ([]ethpb.Att, []byte, error) {
	packing, err := Select(ctx, st, singles, aggregates)
	if err != nil {
		return nil, nil, err
	}
	proof, err := packing.Merge(ctx)
	if err != nil {
		return nil, nil, err
	}
	return packing.Atts, proof, nil
}

func Select(ctx context.Context, st state.ReadOnlyBeaconState, singles []*ethpb.SingleAttestationPQ, aggregates []*Aggregate) (*Packing, error) {
	cfg := params.BeaconConfig()
	slot := st.Slot()
	currentEpoch := slots.ToEpoch(slot)
	previousEpoch := currentEpoch
	if currentEpoch > 0 {
		previousEpoch = currentEpoch - 1
	}
	currentParticipation, err := st.CurrentEpochParticipation()
	if err != nil {
		return nil, err
	}
	previousParticipation, err := st.PreviousEpochParticipation()
	if err != nil {
		return nil, err
	}
	participation := func(data *ethpb.AttestationData) []byte {
		if data.Slot+cfg.MinAttestationInclusionDelayPQ > slot {
			return nil
		}
		switch data.Target.Epoch {
		case currentEpoch:
			return currentParticipation
		case previousEpoch:
			return previousParticipation
		}
		return nil
	}
	// A validator whose target vote is on chain earns nothing from another inclusion.
	fresh := func(p []byte, index primitives.ValidatorIndex) bool {
		return uint64(index) < uint64(len(p)) && p[index]>>cfg.TimelyTargetFlagIndex&1 == 0
	}

	groups := make(map[[32]byte]*group)
	groupFor := func(data *ethpb.AttestationData) (*group, error) {
		root, err := data.HashTreeRoot()
		if err != nil {
			return nil, err
		}
		g, ok := groups[root]
		if !ok {
			g = &group{
				data:       data,
				signatures: make(map[primitives.CommitteeIndex]map[primitives.ValidatorIndex][]byte),
				aggregates: make(map[primitives.CommitteeIndex][]*Aggregate),
			}
			groups[root] = g
		}
		return g, nil
	}
	for _, single := range singles {
		p := participation(single.Data)
		if p == nil || !fresh(p, single.AttesterIndex) {
			continue
		}
		g, err := groupFor(single.Data)
		if err != nil {
			return nil, err
		}
		if g.signatures[single.CommitteeId] == nil {
			g.signatures[single.CommitteeId] = make(map[primitives.ValidatorIndex][]byte)
		}
		g.signatures[single.CommitteeId][single.AttesterIndex] = single.Signature
	}
	for _, aggregate := range aggregates {
		if participation(aggregate.Att.Data) == nil {
			continue
		}
		g, err := groupFor(aggregate.Att.Data)
		if err != nil {
			return nil, err
		}
		ci := aggregate.Att.GetCommitteeIndex()
		g.aggregates[ci] = append(g.aggregates[ci], aggregate)
	}

	ordered := make([]*group, 0, len(groups))
	for _, g := range groups {
		ordered = append(ordered, g)
	}
	// Oldest first, they are the closest to expiring.
	slices.SortFunc(ordered, func(a, b *group) int {
		if c := cmp.Compare(a.data.Slot, b.data.Slot); c != 0 {
			return c
		}
		return slices.Compare(a.data.BeaconBlockRoot, b.data.BeaconBlockRoot)
	})

	reference := primitives.Slot(0)
	if slot > cfg.MinAttestationInclusionDelayPQ {
		reference = slot - cfg.MinAttestationInclusionDelayPQ
	}
	budget := maxProofSize
	if pq.Proofs.Succinct() {
		budget = math.MaxInt
	}
	var atts []ethpb.Att
	var proofs []pq.ProofChild
	var validators uint64
	var pending []pendingProof
	var gain int
	for _, g := range ordered {
		if uint64(len(atts)) >= cfg.MaxAttestationsPQ || budget == 0 {
			break
		}
		p := participation(g.data)
		committeeIndices := make([]primitives.CommitteeIndex, 0, len(g.signatures)+len(g.aggregates))
		for ci := range g.signatures {
			committeeIndices = append(committeeIndices, ci)
		}
		for ci := range g.aggregates {
			if _, ok := g.signatures[ci]; !ok {
				committeeIndices = append(committeeIndices, ci)
			}
		}
		slices.Sort(committeeIndices)

		committeeBits := ethpb.NewCommitteeBitsPQ()
		var chosen []*candidate
		used := 0
		for _, ci := range committeeIndices {
			committee, err := helpers.BeaconCommitteeFromState(ctx, st, g.data.Slot, ci)
			if err != nil {
				continue
			}
			best := bestCandidate(committee, g.signatures[ci], g.aggregates[ci], func(index primitives.ValidatorIndex) bool {
				return fresh(p, index)
			})
			if best.fresh == 0 || best.size > budget-used {
				continue
			}
			committeeBits.SetBitAt(uint64(ci), true)
			chosen = append(chosen, best)
			used += best.size
		}
		if len(chosen) == 0 {
			continue
		}
		att := &ethpb.AttestationPQ{AggregationBits: joinBits(chosen), Data: g.data, CommitteeBits: committeeBits}
		if err := blocks.VerifyAttestationNoVerifySignature(ctx, st, att); err != nil {
			log.WithError(err).WithField("slot", g.data.Slot).Debug("Skipping attestation that does not fit the block")
			continue
		}
		statements, err := pq.AttestationStatements(ctx, st, att)
		if err != nil {
			return nil, err
		}
		for i, c := range chosen {
			validators += c.bits.Count()
			gain += c.fresh
			if c.proof == nil {
				pending = append(pending, pendingProof{child: len(proofs), signatures: c.signatures})
			}
			proofs = append(proofs, pq.ProofChild{Statements: statements[i : i+1], Proof: c.proof})
		}
		atts = append(atts, att)
		budget -= used
	}

	return &Packing{Atts: atts, Validators: validators, Gain: gain, slot: slot, reference: reference, children: proofs, pending: pending}, nil
}

func (p *Packing) Merge(ctx context.Context) ([]byte, error) {
	var proveTime time.Duration
	for _, pending := range p.pending {
		child := &p.children[pending.child]
		if child.Proof != nil {
			continue
		}
		start := time.Now()
		job := timing.NewJob(ctx, "fallback", uint64(p.slot), uint64(p.reference))
		proof, err := job.Run(map[string]any{"signatures": len(pending.signatures)}, func() ([]byte, error) {
			return pq.Proofs.Prove(child.Statements, [][][]byte{pending.signatures})
		})
		if err != nil {
			return nil, err
		}
		child.Proof = proof
		proveTime += time.Since(start)
	}
	start := time.Now()
	job := timing.NewJob(ctx, "initial", uint64(p.slot), uint64(p.reference))
	proof, err := job.Run(map[string]any{"input_proof_count": len(p.children), "records": len(p.Atts), "claims": p.Validators}, func() ([]byte, error) {
		return pq.Proofs.Merge(p.children)
	})
	if err != nil {
		return nil, err
	}
	log.WithFields(logrus.Fields{
		"slot":                p.slot,
		"attestations":        len(p.Atts),
		"committeeProofs":     len(p.children),
		"validators":          p.Validators,
		"provedByProposer":    len(p.pending),
		"proposerProveTime":   proveTime,
		"mergeTime":           time.Since(start),
		"blockProofSizeBytes": len(proof),
	}).Info("Merged attestation proofs for PQ block")
	return proof, nil
}

func bestCandidate(committee []primitives.ValidatorIndex, signatures map[primitives.ValidatorIndex][]byte, aggregates []*Aggregate, fresh func(primitives.ValidatorIndex) bool) *candidate {
	best := localCandidate(committee, signatures)
	for _, aggregate := range aggregates {
		bits := aggregate.Att.AggregationBits
		if bits.Len() != uint64(len(committee)) {
			continue
		}
		c := &candidate{bits: bits, proof: aggregate.Proof, size: len(aggregate.Proof)}
		for _, i := range bits.BitIndices() {
			if fresh(committee[i]) {
				c.fresh++
			}
		}
		// A network aggregate wins a tie, it is already proven.
		if c.fresh >= best.fresh {
			best = c
		}
	}
	return best
}

func joinBits(chosen []*candidate) bitfield.Bitlist {
	total := uint64(0)
	for _, c := range chosen {
		total += c.bits.Len()
	}
	out := bitfield.NewBitlist(total)
	offset := uint64(0)
	for _, c := range chosen {
		for _, i := range c.bits.BitIndices() {
			out.SetBitAt(offset+uint64(i), true)
		}
		offset += c.bits.Len()
	}
	return out
}

func localCandidate(committee []primitives.ValidatorIndex, signatures map[primitives.ValidatorIndex][]byte) *candidate {
	c := &candidate{bits: bitfield.NewBitlist(uint64(len(committee)))}
	for i, index := range committee {
		if sig, ok := signatures[index]; ok {
			c.bits.SetBitAt(uint64(i), true)
			c.signatures = append(c.signatures, sig)
		}
	}
	c.fresh = len(c.signatures)
	c.size = len(c.signatures) * pq.XMSSSignatureLength
	return c
}

// BuildAggregate proves the largest set of matching single attestations of one committee, unless covering has an aggregate with all of them.
func BuildAggregate(ctx context.Context, st state.ReadOnlyBeaconState, singles []*ethpb.SingleAttestationPQ, slot primitives.Slot, index primitives.CommitteeIndex, covering func(*ethpb.AttestationPQ) (*Aggregate, error)) (*Aggregate, error) {
	type votes struct {
		data       *ethpb.AttestationData
		signatures map[primitives.ValidatorIndex][]byte
	}
	byRoot := make(map[[32]byte]*votes)
	var best *votes
	for _, single := range singles {
		if single.Data.Slot != slot || single.CommitteeId != index {
			continue
		}
		root, err := single.Data.HashTreeRoot()
		if err != nil {
			return nil, err
		}
		v, ok := byRoot[root]
		if !ok {
			v = &votes{data: single.Data, signatures: make(map[primitives.ValidatorIndex][]byte)}
			byRoot[root] = v
		}
		v.signatures[single.AttesterIndex] = single.Signature
		if best == nil || len(v.signatures) > len(best.signatures) {
			best = v
		}
	}
	if best == nil {
		return nil, nil
	}
	committee, err := helpers.BeaconCommitteeFromState(ctx, st, slot, index)
	if err != nil {
		return nil, err
	}
	c := localCandidate(committee, best.signatures)
	if c.fresh == 0 {
		return nil, nil
	}
	committeeBits := ethpb.NewCommitteeBitsPQ()
	committeeBits.SetBitAt(uint64(index), true)
	att := &ethpb.AttestationPQ{AggregationBits: c.bits, Data: best.data, CommitteeBits: committeeBits}
	if covering != nil {
		covered, err := covering(att)
		if err != nil {
			return nil, err
		}
		if covered != nil {
			return covered, nil
		}
	}
	statements, err := pq.AttestationStatements(ctx, st, att)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	job := timing.NewJob(ctx, "committee", uint64(slot), uint64(slot))
	proof, err := job.Run(map[string]any{"committee": uint64(index), "signatures": len(c.signatures), "committee_size": len(committee)}, func() ([]byte, error) {
		return pq.Proofs.Prove(statements, [][][]byte{c.signatures})
	})
	if err != nil {
		return nil, err
	}
	observeProof("committee", start, float64(len(c.signatures)))
	log.WithFields(logrus.Fields{
		"slot":           slot,
		"committeeIndex": index,
		"signatures":     len(c.signatures),
		"committeeSize":  len(committee),
		"proveTime":      time.Since(start),
		"proofSizeBytes": len(proof),
	}).Info("Aggregated PQ attestation signatures")
	return &Aggregate{Att: att, Proof: proof}, nil
}
