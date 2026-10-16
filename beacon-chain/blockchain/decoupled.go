package blockchain

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/decoupled"
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	payloadattribute "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attribute"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	prysmTime "github.com/OffchainLabs/prysm/v7/time"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

var errDecoupledUnjoined = errors.New("decoupled fork choice has not joined")

// Spec: ac_proposal, one evaluation per slot shared by the parent and the vote lists.
type decoupledProposal struct {
	slot           primitives.Slot
	parent         [32]byte
	votes, support []decoupled.Vote
	ok             bool
}

type decoupledFCU struct {
	head, safe, finalized [32]byte
	attr                  payloadattribute.Attributer
}

// The duty reads captured at their scheduled instants, the vote head at Δ and the FG source at the action time, so an RPC never recomputes them under later locks.
type decoupledDuty struct {
	voteSlot     primitives.Slot
	vote         [32]byte
	voteOK       bool
	actionSlot   primitives.Slot
	source, head [32]byte
	finality     *ethpb.FinalityAttestationData
}

type DecoupledReceiver interface {
	ReceiveAvailableAttestation(context.Context, *ethpb.AvailableAttestationMessage) error
}

// Spec: Figure 2's order. Every entry runs the phases scheduled between the last entry and now at their own instants, the slot start prune, the Δ tick then the vote read, the 2Δ tick then the source read, and ticks to now last. Stamps are exact, so a phase run late gives its scheduled result and a read sees the state its own tick left, never a later lock. Nothing is captured before the restart at sync. The caller holds decoupledLock and the store exists.
func (s *Service) decoupledAdvance(now time.Time) {
	store := s.decoupledStore
	locks := store.Locks()
	cfg := params.BeaconConfig()
	slotDuration, delta := cfg.SlotDuration(), cfg.SlotDuration()/4
	slotAt := func(t time.Time) primitives.Slot {
		if t.Before(s.genesisTime) {
			return 0
		}
		return primitives.Slot(t.Sub(s.genesisTime) / slotDuration)
	}
	from := s.decoupledProcessed
	for sl := slotAt(from); sl <= slotAt(now); sl++ {
		start := s.genesisTime.Add(time.Duration(sl) * slotDuration)
		for phase, t := range [3]time.Time{start, start.Add(delta), start.Add(2 * delta)} {
			if !t.After(from) || t.After(now) {
				continue
			}
			switch phase {
			case 0:
				store.NewSlot(sl)
				if s.decoupledSynced && !s.decoupledJoined && store.Joined(sl) {
					s.decoupledJoined = true
					log.WithField("slot", sl).Info("Joined the decoupled fork choice")
				}
			case 1:
				store.OnTick(t)
				if s.decoupledSynced {
					n := store.VoteHead(sl, t)
					s.decoupledDuty.voteSlot, s.decoupledDuty.voteOK = sl, n != nil
					if n != nil {
						s.decoupledDuty.vote = n.Root
					}
				}
			case 2:
				store.OnTick(t)
				if s.decoupledSynced && cfg.DecoupledActionSlot(sl) {
					source, head := store.Source(t)
					d := &s.decoupledDuty
					d.actionSlot, d.finality = sl, decoupledFinalityData(source, head)
					if d.finality != nil {
						d.source, d.head = source.Root, head.Root
					}
				}
			}
		}
	}
	store.OnTick(now)
	s.decoupledProcessed = now
	logDecoupledLocks(locks, store.Locks())
}

// A release is routine expiry, a lock moved by unlock shows as released and acquired with one deadline.
func logDecoupledLocks(before, after []decoupled.Lock) {
	for _, l := range after {
		if !slices.Contains(before, l) {
			log.WithFields(logrus.Fields{"block": blockRef(l.Root, l.Slot), "deadline": l.Deadline.Format(time.RFC3339)}).Info("Decoupled lock acquired")
		}
	}
	for _, l := range before {
		if !slices.Contains(after, l) {
			log.WithFields(logrus.Fields{"block": blockRef(l.Root, l.Slot), "deadline": l.Deadline.Format(time.RFC3339)}).Debug("Decoupled lock released")
		}
	}
}

// The σ transitions the block's state took, read from the summaries so regen and replay stay quiet.
func logDecoupledFG(parent *decoupled.Node, b decoupled.Block) {
	if parent == nil {
		return
	}
	if b.FG.Target.Height > parent.FG.Target.Height {
		reason := "progress quorum"
		if b.FG.Justified.Height > parent.FG.Justified.Height {
			reason = "target quorum"
		}
		log.WithFields(logrus.Fields{"slot": b.Slot, "height": b.FG.Target.Height, "entry": rootRef(b.FG.Target.Root), "reason": reason}).Info("Decoupled height advanced")
	}
	if b.FG.Justified.Height > parent.FG.Justified.Height {
		log.WithFields(logrus.Fields{"slot": b.Slot, "height": b.FG.Justified.Height, "root": rootRef(b.FG.Justified.Root)}).Info("Decoupled height justified")
	}
	if b.FG.Finalized.Height > parent.FG.Finalized.Height {
		log.WithFields(logrus.Fields{"slot": b.Slot, "height": b.FG.Finalized.Height, "root": rootRef(b.FG.Finalized.Root)}).Info("Decoupled height finalized")
	}
}

func rootRef(root [32]byte) string { return fmt.Sprintf("%#x", bytesutil.Trunc(root[:])) }

func blockRef(root [32]byte, slot primitives.Slot) string {
	return fmt.Sprintf("%s@%d", rootRef(root), slot)
}

func nodeRef(n *decoupled.Node) string {
	if n == nil {
		return "none"
	}
	return blockRef(n.Root, n.Slot)
}

// Spec: fg_attest's inputs, (h_c, T_c) from the source state and (h_j, J, h_F) from the head state, read from the engine summaries. Nil when there is no source.
func decoupledFinalityData(source, head *decoupled.Node) *ethpb.FinalityAttestationData {
	if source == nil || head == nil {
		return nil
	}
	return &ethpb.FinalityAttestationData{
		Height:          source.FG.Target.Height,
		Entry:           bytesutil.SafeCopyBytes(source.FG.Target.Root[:]),
		JustifiedHeight: head.FG.Justified.Height,
		Justified:       bytesutil.SafeCopyBytes(head.FG.Justified.Root[:]),
		FinalizedHeight: head.FG.Finalized.Height,
	}
}

// Spec: ac_restart at live observation. The ticker calls it once the node is synced, before that no duty decision is captured and no proposal is offered, so nothing is served from the engine the sync built.
func (s *Service) restartDecoupled() {
	s.decoupledLock.Lock()
	defer s.decoupledLock.Unlock()
	s.decoupledSynced, s.decoupledJoined = true, false
	if s.decoupledStore == nil {
		return
	}
	now := prysmTime.Now()
	s.decoupledStore.Restart(now)
	s.decoupledProcessed = now
	s.decoupledProposal, s.decoupledDuty = decoupledProposal{}, decoupledDuty{}
	log.WithField("slot", s.CurrentSlot()).Info("Restarted the decoupled fork choice at sync")
}

// Spec: on_ac_message, the engine stamps the vote on receipt, the pool keeps the signed copy for the next proposer.
func (s *Service) ReceiveAvailableAttestation(_ context.Context, m *ethpb.AvailableAttestationMessage) error {
	if m == nil || m.Data == nil {
		return errors.New("nil available attestation")
	}
	root := bytesutil.ToBytes32(m.Data.BeaconBlockRoot)
	s.decoupledLock.Lock()
	if s.decoupledStore != nil {
		now := prysmTime.Now()
		s.decoupledAdvance(now)
		s.decoupledStore.ProcessVote([]primitives.ValidatorIndex{m.ValidatorIndex}, root, m.Data.Slot, now)
	}
	s.decoupledLock.Unlock()
	if s.cfg.DecoupledVotePool != nil {
		s.cfg.DecoupledVotePool.Insert(m.ValidatorIndex, m.Data.Slot, root, m.Signature)
	}
	return nil
}

// The vote head captured at the slot's Δ, false for any other slot or when the node is unjoined.
func (s *Service) DecoupledVote(slot primitives.Slot) ([32]byte, bool) {
	s.decoupledLock.Lock()
	defer s.decoupledLock.Unlock()
	if s.decoupledStore == nil {
		return [32]byte{}, false
	}
	s.decoupledAdvance(prysmTime.Now())
	d := s.decoupledDuty
	if d.voteSlot != slot || !d.voteOK {
		return [32]byte{}, false
	}
	return d.vote, true
}

// The FG inputs captured at the slot's action time, false for any other slot or when there is no source.
func (s *Service) DecoupledFinalityData(slot primitives.Slot) (*ethpb.FinalityAttestationData, bool) {
	s.decoupledLock.Lock()
	defer s.decoupledLock.Unlock()
	if s.decoupledStore == nil {
		return nil, false
	}
	s.decoupledAdvance(prysmTime.Now())
	d := s.decoupledDuty
	if d.actionSlot != slot || d.finality == nil {
		return nil, false
	}
	return d.finality, true
}

func (s *Service) prepareDecoupledBlock(ctx context.Context, st state.BeaconState, roblock consensusblocks.ROBlock) ([]decoupled.Block, error) {
	b, err := decoupledBlock(st, roblock)
	if err != nil {
		return nil, err
	}
	return s.prepareDecoupled(ctx, b.FG.Finalized, uint64(st.NumValidators()), b)
}

// Spec: ac_on_block after fg_on_block. Everything fallible happens here, the lookups for the missing ancestors, with no engine mutation beyond creating it, so an import that fails afterwards leaves the engine as it was. claimed is the finalized pair of the first block's post-state. The caller holds the legacy fork choice lock.
func (s *Service) prepareDecoupled(ctx context.Context, claimed decoupled.Pair, validators uint64, blocks ...decoupled.Block) ([]decoupled.Block, error) {
	s.decoupledLock.Lock()
	defer s.decoupledLock.Unlock()
	if s.decoupledStore == nil {
		if err := s.startDecoupled(ctx, claimed, validators, prysmTime.Now()); err != nil {
			return nil, err
		}
	}
	prepared := make(map[[32]byte]bool, len(blocks))
	var out []decoupled.Block
	for _, b := range blocks {
		ancestors, err := s.missingDecoupledAncestors(ctx, b.Parent, prepared)
		if err != nil {
			return nil, err
		}
		for _, a := range ancestors {
			prepared[a.Root] = true
		}
		prepared[b.Root] = true
		out = append(append(out, ancestors...), b)
	}
	return out, nil
}

// Prepared blocks go in oldest first with every parent present by construction, then the engine's J and F reach the legacy store, which holds the same blocks by now. The context cannot cancel once mutation starts, a half pruned legacy tree is worse than a late import. Any failure left is a broken invariant between two stores that just accepted the same blocks, not a retryable import.
func (s *Service) commitDecoupled(ctx context.Context, blocks []decoupled.Block) {
	ctx = context.WithoutCancel(ctx)
	s.decoupledLock.Lock()
	defer s.decoupledLock.Unlock()
	now := prysmTime.Now()
	s.decoupledAdvance(now)
	for _, b := range blocks {
		if err := s.decoupledStore.InsertNode(b, now); err != nil {
			log.WithError(err).WithField("root", fmt.Sprintf("%#x", b.Root)).Fatal("Decoupled fork choice rejected a prepared block")
		}
		if s.inRegularSync() {
			logDecoupledFG(s.decoupledStore.Node(b.Parent), b)
		}
	}
	if err := s.syncDecoupledCheckpoints(ctx); err != nil {
		log.WithError(err).Fatal("Legacy fork choice could not follow the decoupled checkpoints")
	}
}

// Spec: the fg_on_block tail, every included row joins the pool for other branches, and rows below the accepted finalized height leave with it.
func (s *Service) poolDecoupledAttestations(blocks ...consensusblocks.ROBlock) {
	pool := s.cfg.DecoupledFGPool
	if pool == nil {
		return
	}
	for _, b := range blocks {
		if atts, err := b.Block().Body().AttestationsDecoupled(); err == nil {
			pool.InsertAggregates(atts)
		}
	}
	s.decoupledLock.Lock()
	finalized := s.decoupledStore.FinalizedHeight()
	s.decoupledLock.Unlock()
	pool.Prune(finalized)
}

// Spec: Store.init at F with its own slot and target, the accepted finalization pair seeds J, the genesis σ when F predates the fork.
// TODO(DC): K_s is the registry at creation, it does not follow registry growth.
// TODO(DC): sampled committees need a per-slot denominator with seat multiplicity, not only a container swap.
func (s *Service) startDecoupled(ctx context.Context, claimed decoupled.Pair, validators uint64, now time.Time) error {
	// The later of two finalizations by slot. At activation the legacy store still holds the last Gloas checkpoint it applied before external mode, while the upgraded state carries the final Gloas advancement, after a restart the database's F is the latest and the re-imported block claims an older pair. The claimed pair is exact, a post-fork legacy F is the entry of its own state height.
	root := s.ensureRootNotZeros(s.cfg.ForkChoiceStore.FinalizedCheckpoint().Root)
	blk, err := s.getBlock(ctx, root)
	if err != nil {
		return errors.Wrapf(err, "could not get finalized block %#x", root)
	}
	finalized := decoupled.Pair{Root: root}
	if blk.Version() >= version.Decoupled {
		fst, err := s.cfg.StateGen.StateByRoot(ctx, root)
		if err != nil {
			return errors.Wrapf(err, "could not get finalized state %#x", root)
		}
		if finalized.Height, err = fst.Height(); err != nil {
			return err
		}
	}
	if c := s.ensureRootNotZeros(claimed.Root); c != root {
		cblk, err := s.getBlock(ctx, c)
		if err != nil {
			return errors.Wrapf(err, "could not get claimed finalized block %#x", c)
		}
		if cblk.Block().Slot() > blk.Block().Slot() {
			root, blk, finalized = c, cblk, decoupled.Pair{Height: claimed.Height, Root: c}
		}
	}
	anchor := decoupled.Block{Root: root, Slot: blk.Block().Slot(), FG: decoupled.FGSummary{Target: decoupled.Pair{Height: 1, Root: root}}}
	if blk.Version() >= version.Decoupled {
		fst, err := s.cfg.StateGen.StateByRoot(ctx, root)
		if err != nil {
			return errors.Wrapf(err, "could not get finalized state %#x", root)
		}
		if anchor.FG, err = decoupledSummary(fst, root); err != nil {
			return err
		}
	}
	anchor.FG.Finalized = finalized
	// TODO(DC): locks with their deadlines and latest_confirmed are not persisted, a restarted node comes back unlocked. Persist them and attach them as their blocks re-import.
	// TODO(DC): the FG pool is not persisted, singles received since the last block are lost at restart.
	cfg := params.BeaconConfig()
	if cfg.DecoupledSlotsPerRound < 2 {
		return errors.New("decoupled slots per round must be at least 2")
	}
	forkSlot, err := slots.EpochStart(cfg.DecoupledForkEpoch)
	if err != nil {
		return err
	}
	store, err := decoupled.New(decoupled.Config{Delta: cfg.SlotDuration() / 4, LockStreak: cfg.DecoupledLockStreak, CommitteeSize: validators, ForkSlot: forkSlot}, s.genesisTime, anchor, now)
	if err != nil {
		return err
	}
	s.decoupledStore = store
	s.decoupledProcessed = now
	s.cfg.ForkChoiceStore.SetExternalCheckpoints()
	return nil
}

// Spec: the engine's accepted J and F are the legacy tree's checkpoints, so it prunes where the engine prunes and a same epoch advance still moves it. Both locks are held.
func (s *Service) syncDecoupledCheckpoints(ctx context.Context) error {
	j, f := s.decoupledStore.Justified(), s.decoupledStore.Finalized()
	legacy := s.cfg.ForkChoiceStore
	if legacy.JustifiedCheckpoint().Root == j.Root && legacy.FinalizedCheckpoint().Root == f.Root {
		return nil
	}
	jc := &forkchoicetypes.Checkpoint{Epoch: slots.ToEpoch(j.Slot), Root: j.Root}
	fc := &forkchoicetypes.Checkpoint{Epoch: slots.ToEpoch(f.Slot), Root: f.Root}
	return legacy.SetCheckpointsByRoot(ctx, jc, fc)
}

// Ancestors missing from the engine, oldest first, back to a known or already prepared node, the engine's own F bounds the walk. Pre-fork blocks carry the root's σ, post-fork ones read theirs from their saved state.
func (s *Service) missingDecoupledAncestors(ctx context.Context, root [32]byte, prepared map[[32]byte]bool) ([]decoupled.Block, error) {
	store := s.decoupledStore
	var chain []consensusblocks.ROBlock
	for !store.HasNode(root) && !prepared[root] {
		blk, err := s.getBlock(ctx, root)
		if err != nil {
			return nil, errors.Wrapf(err, "could not get ancestor %#x", root)
		}
		if blk.Block().Slot() <= store.Finalized().Slot {
			return nil, errors.Errorf("ancestor %#x does not descend from the fork choice root", root)
		}
		ro, err := consensusblocks.NewROBlockWithRoot(blk, root)
		if err != nil {
			return nil, err
		}
		chain = append(chain, ro)
		root = blk.Block().ParentRoot()
	}
	slices.Reverse(chain)
	out := make([]decoupled.Block, 0, len(chain))
	for _, ro := range chain {
		b := decoupled.Block{Root: ro.Root(), Parent: ro.Block().ParentRoot(), Slot: ro.Block().Slot(), FG: store.Finalized().FG}
		if ro.Version() >= version.Decoupled {
			st, err := s.cfg.StateGen.StateByRoot(ctx, ro.Root())
			if err != nil {
				return nil, errors.Wrapf(err, "could not get state for ancestor %#x", ro.Root())
			}
			if b, err = decoupledBlock(st, ro); err != nil {
				return nil, err
			}
		}
		out = append(out, b)
	}
	return out, nil
}

// Blocks the execution layer invalidated leave the engine too, exactly the set the legacy store removed. Losing F is not survivable. The caller holds the legacy fork choice lock.
func (s *Service) removeDecoupled(ctx context.Context, roots [][32]byte) error {
	s.decoupledLock.Lock()
	defer s.decoupledLock.Unlock()
	store := s.decoupledStore
	if store == nil {
		return nil
	}
	// Due phases read the tree as it stood at their instant, so they run before the removal. A captured duty whose block left the tree is withdrawn.
	s.decoupledAdvance(prysmTime.Now())
	if err := store.Remove(roots); err != nil {
		log.WithError(err).Fatal("Decoupled fork choice lost its finalized block")
	}
	s.decoupledProposal = decoupledProposal{}
	d := &s.decoupledDuty
	if d.voteOK && !store.HasNode(d.vote) {
		d.voteOK = false
	}
	if d.finality != nil && (!store.HasNode(d.source) || !store.HasNode(d.head)) {
		d.finality = nil
	}
	return s.syncDecoupledCheckpoints(ctx)
}

func decoupledBlock(st state.ReadOnlyBeaconState, roblock consensusblocks.ROBlock) (decoupled.Block, error) {
	fg, err := decoupledSummary(st, roblock.Root())
	if err != nil {
		return decoupled.Block{}, err
	}
	votes, support, err := decoupledVotes(roblock.Block().Body())
	if err != nil {
		return decoupled.Block{}, err
	}
	return decoupled.Block{
		Root:         roblock.Root(),
		Parent:       roblock.Block().ParentRoot(),
		Slot:         roblock.Block().Slot(),
		BlockVotes:   votes,
		SupportVotes: support,
		FG:           fg,
	}, nil
}

// A block that advanced the height is its own entry, the state only learns that root at the next slot.
func decoupledSummary(st state.ReadOnlyBeaconState, root [32]byte) (decoupled.FGSummary, error) {
	var fg decoupled.FGSummary
	height, err := st.Height()
	if err != nil {
		return fg, err
	}
	entry, err := st.Entry()
	if err != nil {
		return fg, err
	}
	fg.Target = decoupled.Pair{Height: height, Root: bytesutil.ToBytes32(entry.Root)}
	if bytesutil.ZeroRoot(entry.Root) {
		fg.Target.Root = root
	}
	if fg.Justified.Height, err = st.JustifiedHeight(); err != nil {
		return fg, err
	}
	justified, err := st.Justified()
	if err != nil {
		return fg, err
	}
	fg.Justified.Root = bytesutil.ToBytes32(justified.Root)
	if fg.Finalized.Height, err = st.FinalizedHeight(); err != nil {
		return fg, err
	}
	finalized, err := st.Finalized()
	if err != nil {
		return fg, err
	}
	fg.Finalized.Root = bytesutil.ToBytes32(finalized.Root)
	return fg, nil
}

// Spec: B.votes and B.support_votes. Bit i of an aggregate is validator i, an aggregate's support bit marks all of its votes.
func decoupledVotes(body interfaces.ReadOnlyBeaconBlockBody) (votes, support []decoupled.Vote, err error) {
	atts, err := body.AvailableAttestations()
	if err != nil {
		return nil, nil, err
	}
	marks, err := body.SupportVotes()
	if err != nil {
		return nil, nil, err
	}
	for i, att := range atts {
		if att.Data == nil {
			continue
		}
		root := bytesutil.ToBytes32(att.Data.BeaconBlockRoot)
		start := len(votes)
		for _, v := range att.AggregationBits.BitIndices() {
			votes = append(votes, decoupled.Vote{Validator: primitives.ValidatorIndex(v), Slot: att.Data.Slot, Root: root})
		}
		if marks.BitAt(uint64(i)) {
			support = append(support, votes[start:]...)
		}
	}
	return votes, support, nil
}

// The first decoupled proposal needs an engine before any decoupled block exists, so the last pre-fork slot's 2Δ starts one from the legacy head with the canonical pre-fork chain as its nodes. AC messages begin at the fork, so a node synced by then has observed all of them since that slot's start and joins at the fork, a later activation observes a full slot first.
func (s *Service) activateDecoupled(ctx context.Context, slot primitives.Slot) error {
	cfg := params.BeaconConfig()
	forkSlot, err := slots.EpochStart(cfg.DecoupledForkEpoch)
	if err != nil || slot+1 < forkSlot || s.decoupledActive() {
		return nil
	}
	headRoot, err := s.HeadRoot(ctx)
	if err != nil {
		return err
	}
	headState, err := s.HeadState(ctx)
	if err != nil {
		return err
	}
	// The anchor is the fork state's F, which the fork slot's epoch transition can still move.
	st, err := transition.ProcessSlotsUsingNextSlotCache(ctx, headState, headRoot, forkSlot)
	if err != nil {
		return err
	}
	finalized, err := st.Finalized()
	if err != nil {
		return err
	}
	restart := prysmTime.Now()
	if slot+1 == forkSlot {
		restart = s.genesisTime.Add(time.Duration(slot) * cfg.SlotDuration())
	}
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	s.decoupledLock.Lock()
	defer s.decoupledLock.Unlock()
	if s.decoupledStore != nil {
		return nil
	}
	s.cfg.ForkChoiceStore.SetExternalCheckpoints()
	if err := s.startDecoupled(ctx, decoupled.Pair{Root: bytesutil.ToBytes32(finalized.Root)}, uint64(st.NumValidators()), restart); err != nil {
		return err
	}
	chain, err := s.missingDecoupledAncestors(ctx, bytesutil.ToBytes32(headRoot), nil)
	if err != nil {
		return err
	}
	now := prysmTime.Now()
	for _, b := range chain {
		if err := s.decoupledStore.InsertNode(b, now); err != nil {
			log.WithError(err).WithField("root", fmt.Sprintf("%#x", b.Root)).Fatal("Decoupled fork choice rejected a pre-fork block")
		}
	}
	log.WithFields(logrus.Fields{"head": rootRef(bytesutil.ToBytes32(headRoot)), "finalized": rootRef(bytesutil.ToBytes32(finalized.Root)), "blocks": len(chain)}).Info("Started the decoupled fork choice at the fork")
	return s.syncDecoupledCheckpoints(ctx)
}

// Spec: ac_on_tick at Δ and 2Δ, with the pool pruned at slot start. Wall time drives the engine, the label only picks the pruning point, so an overdue label changes nothing. The head is published after each tick since unlocking at Δ can move the anchor.
// TODO(DC): observation loss is not detected, a machine sleep or a stalled process keeps acting on a stale view until the next process restart.
func (s *Service) spawnDecoupledTicker() {
	go func() {
		if _, err := s.clockWaiter.WaitForClock(s.ctx); err != nil {
			log.WithError(err).Error("Failed to receive genesis data")
			return
		}
		if err := s.waitForSync(); err != nil {
			log.WithError(err).Error("Could not wait to sync")
			return
		}
		s.restartDecoupled()
		delta := params.BeaconConfig().SlotDuration() / 4
		ticker := slots.NewSlotTickerWithIntervals(s.genesisTime, []time.Duration{0, delta, 2 * delta})
		defer ticker.Done()
		for {
			select {
			case <-s.ctx.Done():
				return
			case t := <-ticker.C():
				if t.Interval == 0 && s.cfg.DecoupledVotePool != nil {
					s.cfg.DecoupledVotePool.Prune(t.Slot)
				}
				if t.Interval == 2 {
					if err := s.activateDecoupled(s.ctx, t.Slot); err != nil {
						log.WithError(err).Error("Could not start the decoupled fork choice at the fork")
					}
				}
				if s.tickDecoupled(t.Slot, t.Interval) && t.Interval > 0 {
					s.UpdateHead(s.ctx, t.Slot+1)
				}
				if t.Interval == 2 {
					s.notifyFastConfirmation(t.Slot)
				}
			}
		}
	}()
}

func (s *Service) tickDecoupled(slot primitives.Slot, interval int) bool {
	s.decoupledLock.Lock()
	store := s.decoupledStore
	if store == nil {
		s.decoupledLock.Unlock()
		return false
	}
	now := prysmTime.Now()
	s.decoupledAdvance(now)
	if interval != 2 {
		s.decoupledLock.Unlock()
		return true
	}
	head, confirmed := store.Head(now), store.Confirmed(now)
	justified, finalized := store.JustifiedHeight(), store.FinalizedHeight()
	source, _ := store.Source(now)
	voters, committee, locks := 0, store.CommitteeSize(), len(store.Locks())
	if slot > 0 {
		voters = store.Voters(slot - 1)
	}
	prev := s.decoupledHead
	reorg := prev != [32]byte{} && prev != head.Root && !store.IsAncestor(prev, head.Root)
	s.decoupledLock.Unlock()
	decoupledHeadSlot.Set(float64(head.Slot))
	decoupledConfirmedSlot.Set(float64(confirmed.Slot))
	decoupledJustifiedHeight.Set(float64(justified))
	decoupledFinalizedHeight.Set(float64(finalized))
	log.WithFields(logrus.Fields{
		"slot":      slot,
		"head":      nodeRef(head),
		"confirmed": nodeRef(confirmed),
		"votes":     fmt.Sprintf("%d/%d", voters, committee),
		"height":    head.FG.Target.Height,
		"justified": justified,
		"finalized": finalized,
		"source":    nodeRef(source),
		"locks":     locks,
	}).Info("Decoupled slot summary")
	if reorg {
		log.WithFields(logrus.Fields{"slot": slot, "old": rootRef(prev), "new": nodeRef(head)}).Info("Decoupled head reorg")
	}
	if ok := source != nil; ok != s.decoupledSourceOK {
		s.decoupledSourceOK = ok
		if ok {
			log.WithFields(logrus.Fields{"slot": slot, "source": nodeRef(source)}).Info("Source available")
		} else {
			log.WithField("slot", slot).Info("Source lost")
		}
	}
	if head.Root == s.decoupledHead {
		return true
	}
	s.decoupledHead = head.Root
	s.headLock.RLock()
	legacy := s.headRoot()
	s.headLock.RUnlock()
	if legacy != head.Root {
		log.WithFields(logrus.Fields{"decoupled": fmt.Sprintf("%#x", head.Root), "legacy": fmt.Sprintf("%#x", legacy), "slot": slot}).Debug("Decoupled head differs from the legacy head")
	}
	return true
}

func (s *Service) decoupledActive() bool {
	s.decoupledLock.Lock()
	defer s.decoupledLock.Unlock()
	return s.decoupledStore != nil
}

// Spec: ac_head. ok means the engine exists, not that the node has joined.
func (s *Service) DecoupledHead() ([32]byte, bool) {
	return s.decoupledRead((*decoupled.Store).Head)
}

// Spec: ac_confirmed.
func (s *Service) DecoupledConfirmed() ([32]byte, bool) {
	return s.decoupledRead((*decoupled.Store).Confirmed)
}

// Due ticks run first so a read past a cutoff sees the scheduled state even when it beats the ticker.
func (s *Service) decoupledRead(read func(*decoupled.Store, time.Time) *decoupled.Node) ([32]byte, bool) {
	s.decoupledLock.Lock()
	defer s.decoupledLock.Unlock()
	if s.decoupledStore == nil {
		return [32]byte{}, false
	}
	now := prysmTime.Now()
	s.decoupledAdvance(now)
	n := read(s.decoupledStore, now)
	if n == nil {
		return [32]byte{}, false
	}
	return n.Root, true
}

// Head, confirmed and F from one engine state, so a fork choice update never mixes two.
func (s *Service) decoupledSnapshot() (head, confirmed, finalized [32]byte, ok bool) {
	s.decoupledLock.Lock()
	defer s.decoupledLock.Unlock()
	if s.decoupledStore == nil {
		return
	}
	now := prysmTime.Now()
	s.decoupledAdvance(now)
	return s.decoupledStore.Head(now).Root, s.decoupledStore.Confirmed(now).Root, s.decoupledStore.Finalized().Root, true
}

// No proposal before the restart at sync, the engine built during sync has not observed anything.
func (s *Service) DecoupledProposal(slot primitives.Slot) (parent [32]byte, votes, support []decoupled.Vote, ok bool) {
	s.decoupledLock.Lock()
	defer s.decoupledLock.Unlock()
	if s.decoupledStore == nil || !s.decoupledSynced {
		return
	}
	if m := s.decoupledProposal; m.ok && m.slot == slot {
		return m.parent, m.votes, m.support, true
	}
	now := prysmTime.Now()
	s.decoupledAdvance(now)
	p, v, sp := s.decoupledStore.Proposal(slot, now)
	if p == nil {
		return
	}
	s.decoupledProposal = decoupledProposal{slot: slot, parent: p.Root, votes: v, support: sp, ok: true}
	return p.Root, v, sp, true
}

// Spec: propose_block skips the proposal while unjoined, the validator gets an error instead of a parent.
func (s *Service) ProposerParent(slot primitives.Slot) ([32]byte, error) {
	if !s.decoupledActive() {
		return s.GetProposerHead(), nil
	}
	parent, _, _, ok := s.DecoupledProposal(slot)
	if !ok {
		log.WithField("slot", slot).Warn("Skipped the proposal, the decoupled fork choice has not joined")
		return [32]byte{}, errDecoupledUnjoined
	}
	return parent, nil
}

// The engine chooses the beacon root, the PTC machinery decides full or empty, and the head is loaded by root since it need not be an incoming block. The caller holds the legacy fork choice lock.
func (s *Service) publishDecoupledHead(ctx context.Context, proposingSlot primitives.Slot) ([32]byte, bool) {
	head, confirmed, finalized, ok := s.decoupledSnapshot()
	if !ok {
		return [32]byte{}, false
	}
	hash, full, err := s.cfg.ForkChoiceStore.PayloadContentDecoupled(head)
	if err != nil {
		log.WithError(err).WithField("root", fmt.Sprintf("%#x", head)).Error("Could not resolve the decoupled head payload")
		return head, false
	}
	headState, headBlock, err := s.getStateAndBlock(ctx, head)
	if err != nil {
		log.WithError(err).WithField("root", fmt.Sprintf("%#x", head)).Error("Could not load the decoupled head")
		return head, false
	}
	if s.isNewHead(head, full) {
		if err := s.saveHead(ctx, head, headBlock, headState, full); err != nil {
			log.WithError(err).Error("Could not save head")
		}
	}
	if !s.inRegularSync() || hash == [32]byte{} {
		return head, true
	}
	attr := s.getPayloadAttribute(ctx, headState, proposingSlot, head[:], full)
	safe := s.cfg.ForkChoiceStore.ConfirmedPayloadBlockHash(confirmed)
	final := s.cfg.ForkChoiceStore.ConfirmedPayloadBlockHash(finalized)
	if s.decoupledFCUSent(hash, safe, final, attr) {
		return head, true
	}
	fcs := &enginev1.ForkchoiceState{HeadBlockHash: hash[:], SafeBlockHash: safe[:], FinalizedBlockHash: final[:]}
	go s.fcuDecoupled(headBlock, head, fcs, full, attr, proposingSlot)
	return head, true
}

// Only an exact successful repeat is skipped, two empty heads can share an execution hash while their attributes differ in the parent beacon block root.
func (s *Service) decoupledFCUSent(head, safe, finalized [32]byte, attr payloadattribute.Attributer) bool {
	s.decoupledFCULock.Lock()
	defer s.decoupledFCULock.Unlock()
	last := s.decoupledFCU
	return last.head == head && last.safe == safe && last.finalized == finalized && sameAttributes(last.attr, attr)
}

// An INVALID answer invalidates the head's payload branch like a rejected new payload would, then the engine picks again.
func (s *Service) fcuDecoupled(headBlock interfaces.ReadOnlySignedBeaconBlock, hr [32]byte, fcs *enginev1.ForkchoiceState, full bool, attr payloadattribute.Attributer, proposingSlot primitives.Slot) {
	pid, syncing, err := s.forkchoiceUpdatedGloas(s.ctx, fcs, attr)
	if err != nil {
		log.WithError(err).Error("Could not update forkchoice with engine")
		if !IsInvalidBlock(err) {
			return
		}
		s.cfg.ForkChoiceStore.Lock()
		defer s.cfg.ForkChoiceStore.Unlock()
		parentHash := s.cfg.ForkChoiceStore.ParentHash(hr)
		if err := s.pruneInvalidBlock(s.ctx, hr, headBlock.Block().ParentRoot(), parentHash, InvalidBlockLVH(err)); err != nil && !IsInvalidBlock(err) {
			log.WithError(err).Error("Could not prune the invalid decoupled head")
			return
		}
		s.publishDecoupledHead(s.ctx, proposingSlot)
		return
	}
	if !syncing {
		s.recordDecoupledFCU(fcs, attr)
	}
	if pid == nil {
		if !attr.IsEmpty() {
			log.Warn("Engine did not return a payload ID for the fork choice update with attributes")
		}
		return
	}
	var pId [8]byte
	copy(pId[:], pid[:])
	s.cfg.PayloadIDCache.Set(proposingSlot, hr, full, pId)
	if !attr.IsEmpty() {
		s.firePayloadAttributesEvent(s.cfg.StateNotifier.StateFeed(), headBlock, hr, proposingSlot, attr, fcs.HeadBlockHash)
	}
}

func (s *Service) recordDecoupledFCU(fcs *enginev1.ForkchoiceState, attr payloadattribute.Attributer) {
	s.decoupledFCULock.Lock()
	defer s.decoupledFCULock.Unlock()
	s.decoupledFCU = decoupledFCU{head: bytesutil.ToBytes32(fcs.HeadBlockHash), safe: bytesutil.ToBytes32(fcs.SafeBlockHash), finalized: bytesutil.ToBytes32(fcs.FinalizedBlockHash), attr: attr}
}

// The complete versioned attributes, a changed target gas limit alone is a new build request.
func sameAttributes(a, b payloadattribute.Attributer) bool {
	if a == nil || a.IsEmpty() {
		return b == nil || b.IsEmpty()
	}
	if b == nil || b.IsEmpty() || a.Version() != b.Version() {
		return false
	}
	return proto.Equal(attributesProto(a), attributesProto(b))
}

func attributesProto(a payloadattribute.Attributer) proto.Message {
	if m, err := a.PbV4(); err == nil {
		return m
	}
	if m, err := a.PbV3(); err == nil {
		return m
	}
	if m, err := a.PbV2(); err == nil {
		return m
	}
	m, _ := a.PbV1()
	return m
}

// Spec: the legacy store's checkpoints already follow the engine, the database and its finalized index follow by root here.
func (s *Service) persistDecoupledCheckpoints(ctx context.Context) error {
	j, f := s.cfg.ForkChoiceStore.JustifiedCheckpoint(), s.cfg.ForkChoiceStore.FinalizedCheckpoint()
	saved, err := s.cfg.BeaconDB.JustifiedCheckpoint(ctx)
	if err != nil {
		return err
	}
	if !bytes.Equal(saved.Root, j.Root[:]) {
		if err := s.cfg.BeaconDB.SaveJustifiedCheckpoint(ctx, &ethpb.Checkpoint{Epoch: j.Epoch, Root: j.Root[:]}); err != nil {
			return err
		}
	}
	return s.updateFinalized(ctx, &ethpb.Checkpoint{Epoch: f.Epoch, Root: f.Root[:]})
}

// Canonical means an ancestor of the published head in the engine, or a finalized ancestor below the engine's root.
func (s *Service) decoupledIsCanonical(ctx context.Context, root [32]byte) (bool, error) {
	s.headLock.RLock()
	head := s.headRoot()
	s.headLock.RUnlock()
	s.decoupledLock.Lock()
	known := s.decoupledStore.HasNode(root)
	canonical := known && s.decoupledStore.IsAncestor(root, head)
	s.decoupledLock.Unlock()
	if known {
		return canonical, nil
	}
	return s.cfg.BeaconDB.IsFinalizedAncestor(ctx, root), nil
}

// Full or empty follows the selected chain's successor, never a fresh preference for the block itself. The caller holds the legacy fork choice read lock.
func (s *Service) decoupledCanonicalAtSlot(ctx context.Context, slot primitives.Slot) ([32]byte, bool) {
	s.headLock.RLock()
	head := s.headRoot()
	s.headLock.RUnlock()
	s.decoupledLock.Lock()
	root, ok := s.decoupledStore.AncestorAtSlot(head, slot)
	var successor [32]byte
	if ok && root != head {
		for n := s.decoupledStore.Node(head); n != nil && n.Parent != nil; n = n.Parent {
			if n.Parent.Root == root {
				successor = n.Root
				break
			}
		}
	}
	s.decoupledLock.Unlock()
	if !ok {
		var err error
		if root, err = s.ancestorByDB(ctx, head, slot); err != nil {
			return [32]byte{}, false
		}
		child, err := s.cfg.BeaconDB.FinalizedChildBlock(ctx, root)
		if err != nil || child == nil || child.IsNil() {
			return root, false
		}
		return root, s.builtOnFull(ctx, root, child)
	}
	if rootSlot, err := s.cfg.ForkChoiceStore.Slot(root); err == nil && rootSlot == s.CurrentSlot() {
		return root, false
	}
	if root == head {
		_, full, _ := s.cfg.ForkChoiceStore.PayloadContentDecoupled(root)
		return root, full
	}
	blockHash, err := s.cfg.ForkChoiceStore.BlockHash(root)
	if err != nil {
		return root, false
	}
	return root, s.cfg.ForkChoiceStore.ParentHash(successor) == blockHash
}

func (s *Service) builtOnFull(ctx context.Context, parent [32]byte, child interfaces.ReadOnlySignedBeaconBlock) bool {
	pb, err := s.getBlock(ctx, parent)
	if err != nil {
		return false
	}
	pBid, err := pb.Block().Body().SignedExecutionPayloadBid()
	if err != nil || pBid == nil || pBid.Message == nil {
		return false
	}
	cBid, err := child.Block().Body().SignedExecutionPayloadBid()
	if err != nil || cBid == nil || cBid.Message == nil {
		return false
	}
	return bytes.Equal(cBid.Message.ParentBlockHash, pBid.Message.BlockHash)
}
