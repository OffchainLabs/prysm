package pqpool

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq/timing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
)

const (
	// Votes older than this earn too little to be worth a proving job.
	lookbackSlots = 5
	// Rebuilding costs a merge, so a slot proof is only replaced for a real gain in voters.
	rebuildGainPercent = 5
	rescanInterval     = time.Second
)

type Piece struct {
	Slot       primitives.Slot
	Atts       []ethpb.Att
	Proof      []byte
	Validators uint64
	statements []pq.Statement
	covered    bool
}

type pick struct {
	piece *Piece
	score uint64
	fresh uint64
}

type Builder struct {
	Head        ChainHead
	Pool        *Pool
	GenesisTime time.Time
	Local       func(ctx context.Context, st state.ReadOnlyBeaconState, slot primitives.Slot) bool

	// The sidecar proves one job at a time, queueing here keeps the order visible.
	lane   sync.Mutex
	mu     sync.Mutex
	pieces map[primitives.Slot]*Piece
}

func (b *Builder) Slot(ctx context.Context, slot primitives.Slot) {
	cfg := params.BeaconConfig()
	st, err := b.Head.HeadState(ctx)
	if err != nil {
		return
	}
	b.mu.Lock()
	for s := range b.pieces {
		if s+lookbackSlots < slot {
			delete(b.pieces, s)
		}
	}
	b.mu.Unlock()
	local := false
	for s := slot + cfg.MinAttestationInclusionDelayPQ; s <= slot+lookbackSlots; s++ {
		local = local || b.Local(ctx, st, s)
	}
	if !local {
		return
	}
	start, err := slots.StartTime(b.GenesisTime, slot)
	if err != nil {
		return
	}
	end, err := slots.StartTime(b.GenesisTime, slot+cfg.MinAttestationInclusionDelayPQ)
	if err != nil {
		return
	}
	// Aggregators publish in the last third of the slot, later ones are picked up by the rescans.
	next := start.Add(cfg.SlotDuration() * 3 / 4)
	for next.Before(end) && sleepUntil(ctx, next) {
		b.ensure(ctx, slot)
		next = time.Now().Add(rescanInterval)
	}
}

func (b *Builder) piece(slot primitives.Slot) *Piece {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.pieces[slot]
}

type slotVotes struct {
	data       *ethpb.AttestationData
	committees map[primitives.CommitteeIndex]*Aggregate
	validators uint64
}

// A proof cannot be joined with another for the same committee, so the widest aggregate stands for it.
func widestAggregates(aggregates []*Aggregate, voteSlot primitives.Slot) ([]*slotVotes, uint64, error) {
	byData := make(map[[32]byte]*slotVotes)
	for _, aggregate := range aggregates {
		if aggregate.Att.Data.Slot != voteSlot {
			continue
		}
		root, err := aggregate.Att.Data.HashTreeRoot()
		if err != nil {
			return nil, 0, err
		}
		v, ok := byData[root]
		if !ok {
			v = &slotVotes{data: aggregate.Att.Data, committees: make(map[primitives.CommitteeIndex]*Aggregate)}
			byData[root] = v
		}
		ci := aggregate.Att.GetCommitteeIndex()
		if best, ok := v.committees[ci]; !ok || aggregate.Att.AggregationBits.Count() > best.Att.AggregationBits.Count() {
			v.committees[ci] = aggregate
		}
	}
	votes := make([]*slotVotes, 0, len(byData))
	for _, v := range byData {
		for _, aggregate := range v.committees {
			v.validators += aggregate.Att.AggregationBits.Count()
		}
		votes = append(votes, v)
	}
	slices.SortFunc(votes, func(x, y *slotVotes) int {
		if c := cmp.Compare(y.validators, x.validators); c != 0 {
			return c
		}
		return slices.Compare(x.data.BeaconBlockRoot, y.data.BeaconBlockRoot)
	})
	if limit := params.BeaconConfig().MaxAttestationsPQ; uint64(len(votes)) > limit {
		votes = votes[:limit]
	}
	total := uint64(0)
	for _, v := range votes {
		total += v.validators
	}
	return votes, total, nil
}

func (b *Builder) ensure(ctx context.Context, voteSlot primitives.Slot) {
	b.lane.Lock()
	defer b.lane.Unlock()
	votes, validators, err := widestAggregates(b.Pool.Aggregates(), voteSlot)
	if err != nil {
		log.WithError(err).WithField("voteSlot", voteSlot).Debug("Could not select aggregates for a PQ vote slot proof")
		return
	}
	if validators == 0 {
		return
	}
	old := b.piece(voteSlot)
	if old != nil && (validators <= old.Validators || (validators-old.Validators)*100 < old.Validators*rebuildGainPercent) {
		return
	}
	// Committees and keys are fixed for the epoch, so the head state serves without advancing it.
	st, err := b.Head.HeadState(ctx)
	if err != nil {
		return
	}
	piece := &Piece{Slot: voteSlot, Validators: validators}
	var children []pq.ProofChild
	for _, v := range votes {
		indices := make([]primitives.CommitteeIndex, 0, len(v.committees))
		for ci := range v.committees {
			indices = append(indices, ci)
		}
		slices.Sort(indices)
		committeeBits := ethpb.NewCommitteeBitsPQ()
		chosen := make([]*candidate, 0, len(indices))
		for _, ci := range indices {
			committeeBits.SetBitAt(uint64(ci), true)
			chosen = append(chosen, &candidate{bits: v.committees[ci].Att.AggregationBits})
		}
		att := &ethpb.AttestationPQ{AggregationBits: joinBits(chosen), Data: v.data, CommitteeBits: committeeBits}
		statements, err := pq.AttestationStatements(ctx, st, att)
		if err != nil || len(statements) != len(indices) {
			log.WithError(err).WithField("voteSlot", voteSlot).Debug("Could not get the statements of a PQ vote slot proof")
			return
		}
		for i, ci := range indices {
			children = append(children, pq.ProofChild{Statements: statements[i : i+1], Proof: v.committees[ci].Proof})
		}
		piece.Atts = append(piece.Atts, att)
		piece.statements = append(piece.statements, statements...)
	}
	start := time.Now()
	job := timing.NewJob(ctx, "initial", uint64(voteSlot+params.BeaconConfig().MinAttestationInclusionDelayPQ), uint64(voteSlot))
	proof, err := job.Run(map[string]any{"input_proof_count": len(children), "records": len(piece.Atts), "claims": validators}, func() ([]byte, error) {
		return pq.Proofs.Merge(children)
	})
	if err != nil {
		log.WithError(err).WithField("voteSlot", voteSlot).Warn("Could not build a PQ vote slot proof")
		return
	}
	piece.Proof = proof
	b.mu.Lock()
	if b.pieces == nil {
		b.pieces = make(map[primitives.Slot]*Piece)
	}
	b.pieces[voteSlot] = piece
	b.mu.Unlock()
	slotStart, _ := slots.StartTime(b.GenesisTime, voteSlot)
	fields := logrus.Fields{
		"voteSlot":        voteSlot,
		"attestations":    len(piece.Atts),
		"committeeProofs": len(children),
		"validators":      piece.Validators,
		"mergeTime":       time.Since(start),
		"proofSizeBytes":  len(proof),
		"sinceVoteSlot":   time.Since(slotStart),
	}
	if old != nil {
		fields["replacedValidators"] = old.Validators
		log.WithFields(fields).Info("Replaced PQ vote slot proof with a wider one")
		return
	}
	log.WithFields(fields).Info("Built PQ vote slot proof from network aggregates")
}

// The score is the proposer reward of the state transition, counted in flag weights per new voter.
func score(ctx context.Context, st state.ReadOnlyBeaconState, piece *Piece) (uint64, uint64, error) {
	cfg := params.BeaconConfig()
	weights := map[uint8]uint64{
		cfg.TimelySourceFlagIndex: cfg.TimelySourceWeight,
		cfg.TimelyTargetFlagIndex: cfg.TimelyTargetWeight,
		cfg.TimelyHeadFlagIndex:   cfg.TimelyHeadWeight,
	}
	current, err := st.CurrentEpochParticipation()
	if err != nil {
		return 0, 0, err
	}
	previous, err := st.PreviousEpochParticipation()
	if err != nil {
		return 0, 0, err
	}
	var total, fresh uint64
	for _, att := range piece.Atts {
		if err := blocks.VerifyAttestationNoVerifySignature(ctx, st, att); err != nil {
			return 0, 0, err
		}
		data := att.GetData()
		flags, err := altair.AttestationParticipationFlagIndices(st, data, st.Slot()-data.Slot, st.LatestBlockHeader().Slot)
		if err != nil {
			return 0, 0, err
		}
		committees, err := helpers.AttestationCommitteesFromState(ctx, st, att)
		if err != nil {
			return 0, 0, err
		}
		indices, err := attestation.AttestingIndices(att, committees...)
		if err != nil {
			return 0, 0, err
		}
		participation := previous
		if data.Target.Epoch == slots.ToEpoch(st.Slot()) {
			participation = current
		}
		for _, index := range indices {
			if index >= uint64(len(participation)) {
				continue
			}
			earned := uint64(0)
			for flag, set := range flags {
				if set && participation[index]>>flag&1 == 0 {
					earned += weights[flag]
				}
			}
			if earned > 0 {
				fresh++
				total += earned
			}
		}
	}
	return total, fresh, nil
}

func (b *Builder) choose(ctx context.Context, st state.ReadOnlyBeaconState) []pick {
	cfg := params.BeaconConfig()
	b.mu.Lock()
	pieces := make([]*Piece, 0, len(b.pieces))
	for _, piece := range b.pieces {
		pieces = append(pieces, piece)
	}
	b.mu.Unlock()
	var picks []pick
	for _, piece := range pieces {
		if piece.Slot+cfg.MinAttestationInclusionDelayPQ > st.Slot() {
			continue
		}
		total, fresh, err := score(ctx, st, piece)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{"voteSlot": piece.Slot, "proposalSlot": st.Slot()}).Debug("PQ vote slot proof does not fit this block")
			continue
		}
		if total == 0 {
			b.mu.Lock()
			seen := piece.covered
			piece.covered = true
			b.mu.Unlock()
			if !seen {
				log.WithFields(logrus.Fields{
					"voteSlot":     piece.Slot,
					"proposalSlot": st.Slot(),
					"parentSlot":   st.LatestBlockHeader().Slot,
					"validators":   piece.Validators,
				}).Info("Set aside PQ vote slot proof, its votes are on chain")
			}
			continue
		}
		picks = append(picks, pick{piece: piece, score: total, fresh: fresh})
	}
	slices.SortFunc(picks, func(a, c pick) int {
		if d := cmp.Compare(c.score, a.score); d != 0 {
			return d
		}
		return cmp.Compare(c.piece.Slot, a.piece.Slot)
	})
	entries := uint64(0)
	for i, p := range picks {
		entries += uint64(len(p.piece.Atts))
		if entries > cfg.MaxAttestationsPQ {
			log.WithFields(logrus.Fields{"proposalSlot": st.Slot(), "kept": i, "left": len(picks) - i}).Info("PQ block is full, leaving the lowest reward vote slots out")
			picks = picks[:i]
			break
		}
	}
	slices.SortFunc(picks, func(a, c pick) int { return cmp.Compare(a.piece.Slot, c.piece.Slot) })
	return picks
}

func single(slot primitives.Slot, p pick) *Prepared {
	return &Prepared{
		Slot: slot, Atts: p.piece.Atts, Proof: p.piece.Proof, Validators: p.piece.Validators,
		Gain: int(p.fresh), Round: 1, VoteSlots: []primitives.Slot{p.piece.Slot}, Score: p.score, // #nosec G115 -- a voter count
	}
}

func (b *Builder) join(ctx context.Context, slot primitives.Slot, picks []pick) *Prepared {
	b.lane.Lock()
	defer b.lane.Unlock()
	prepared := &Prepared{Slot: slot, Round: 2}
	children := make([]pq.ProofChild, 0, len(picks))
	for _, p := range picks {
		children = append(children, pq.ProofChild{Statements: p.piece.statements, Proof: p.piece.Proof})
		prepared.Atts = append(prepared.Atts, p.piece.Atts...)
		prepared.Validators += p.piece.Validators
		prepared.Gain += int(p.fresh) // #nosec G115 -- a voter count
		prepared.Score += p.score
		prepared.VoteSlots = append(prepared.VoteSlots, p.piece.Slot)
	}
	start := time.Now()
	reference := slot - min(slot, params.BeaconConfig().MinAttestationInclusionDelayPQ)
	job := timing.NewJob(timing.WithRound(ctx, 2), "initial", uint64(slot), uint64(reference))
	proof, err := job.Run(map[string]any{"input_proof_count": len(children), "records": len(prepared.Atts), "claims": prepared.Validators}, func() ([]byte, error) {
		return pq.Proofs.Merge(children)
	})
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{"slot": slot, "voteSlots": prepared.VoteSlots}).Warn("Could not join PQ vote slot proofs")
		return nil
	}
	prepared.Proof = proof
	prepared.JoinTime = time.Since(start)
	return prepared
}

// Votes are checked against the parent state here, so no proof is tied to a parent before the slot.
func (b *Builder) Pack(ctx context.Context, slot primitives.Slot, st state.ReadOnlyBeaconState, deadline time.Time) *Prepared {
	picks := b.choose(ctx, st)
	if len(picks) == 0 {
		return nil
	}
	best := picks[0]
	for _, p := range picks {
		if p.score > best.score {
			best = p
		}
	}
	if len(picks) == 1 {
		return single(slot, best)
	}
	done := make(chan *Prepared, 1)
	go func() { done <- b.join(ctx, slot, picks) }()
	select {
	case prepared := <-done:
		if prepared != nil {
			return prepared
		}
	case <-time.After(time.Until(deadline)):
		log.WithFields(logrus.Fields{"slot": slot, "voteSlot": best.piece.Slot}).Warn("Joining PQ vote slot proofs took too long, packing the best one alone")
	}
	return single(slot, best)
}

func (b *Builder) Has(voteSlot primitives.Slot) bool {
	return b.piece(voteSlot) != nil
}
