package backfill

import (
	"bytes"
	"context"
	"maps"
	"slices"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/iface"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/kv"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/sync"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/logging"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Terminal skip reasons for slots whose envelope could not be backfilled.
const (
	envSkipSigUnverifiable = "sig_unverifiable"
	envSkipPeerExhausted   = "peer_exhausted"
	envSkipTailUnresolved  = "tail_unresolved"
	envSkipDBFailed        = "db_failed"
)

const (
	// envelopePageSize is the number of slots covered by a single envelopes-by-range request.
	envelopePageSize = primitives.Slot(16)
	// envelopeMaxPageAttempts caps the total RPC attempts for an unresolved page.
	envelopeMaxPageAttempts = 3
	// envelopeLocalRetries caps local (non-network) retries.
	envelopeLocalRetries = 3
)

var (
	errInvalidEnvelopeResponse = errors.New("invalid execution payload envelope response")
	// envelopeSkipLogger rate limits the per-batch summary of skipped envelope slots.
	envelopeSkipLogger = logging.NewIntervalLogger(log, 30)
)

// envelopeFetcher requests execution payload envelopes by range from the given peer.
type envelopeFetcher func(ctx context.Context, pid peer.ID, req *ethpb.ExecutionPayloadEnvelopesByRangeRequest) ([]*ethpb.SignedExecutionPayloadEnvelope, error)

// envelopeVerifier verifies backfilled envelope signatures statelessly, using key material
// snapshotted from the checkpoint origin state.
type envelopeVerifier struct {
	keys     [][fieldparams.BLSPubkeyLength]byte
	builders []*ethpb.Builder
	domain   *domainCache
}

func newEnvelopeVerifier(vr []byte, keys [][fieldparams.BLSPubkeyLength]byte, builders []*ethpb.Builder) (*envelopeVerifier, error) {
	dc, err := newDomainCache(vr, params.BeaconConfig().DomainBeaconBuilder)
	if err != nil {
		return nil, err
	}
	return &envelopeVerifier{keys: keys, builders: builders, domain: dc}, nil
}

// envelopeExpectation is a slot that expects an envelope: the block/bid it must bind to and the
// key its signature must verify against.
type envelopeExpectation struct {
	block  blocks.ROBlock
	bid    interfaces.ROExecutionPayloadBid
	pubkey bls.PublicKey
	// required is false only for the batch-tail slot while its payload fullness is still
	// unclassified; it is resolved at import time.
	required bool
	// skipReason marks an unclassified tail whose signature can never be verified; never fetched.
	skipReason string
}

// expectation resolves the verification key for the block's envelope: the proposer key for
// self-built payloads, otherwise the builder registry snapshot. Unverifiable slots are skipped
// without a request.
func (v *envelopeVerifier) expectation(b blocks.ROBlock) (*envelopeExpectation, string, error) {
	sb, err := b.Block().Body().SignedExecutionPayloadBid()
	if err != nil {
		return nil, "", errors.Wrap(err, "signed execution payload bid")
	}
	if sb == nil || sb.IsNil() {
		return nil, "", errors.New("nil execution payload bid in verified block")
	}
	bid := sb.Bid()
	var key []byte
	if bid.BuilderIndex() == params.BeaconConfig().BuilderIndexSelfBuild {
		pidx := b.Block().ProposerIndex()
		if uint64(pidx) >= uint64(len(v.keys)) {
			return nil, envSkipSigUnverifiable, nil
		}
		key = v.keys[pidx][:]
	} else {
		idx := uint64(bid.BuilderIndex())
		if idx >= uint64(len(v.builders)) || v.builders[idx] == nil {
			return nil, envSkipSigUnverifiable, nil
		}
		bldr := v.builders[idx]
		// Builder indices are reused: the snapshot occupant provably held the index at the
		// envelope's epoch only if it deposited strictly before that epoch.
		if bldr.DepositEpoch >= slots.ToEpoch(b.Block().Slot()) {
			return nil, envSkipSigUnverifiable, nil
		}
		key = bldr.Pubkey
	}
	pub, err := bls.PublicKeyFromBytes(key)
	if err != nil {
		return nil, envSkipSigUnverifiable, nil
	}
	return &envelopeExpectation{block: b, bid: bid, pubkey: pub}, "", nil
}

// signatureBatch builds the aggregate verification entry for one envelope over DOMAIN_BEACON_BUILDER.
func (v *envelopeVerifier) signatureBatch(env *ethpb.SignedExecutionPayloadEnvelope, exp *envelopeExpectation) (*bls.SignatureBatch, error) {
	dom, err := v.domain.forEpoch(slots.ToEpoch(exp.block.Block().Slot()))
	if err != nil {
		return nil, err
	}
	root, err := signing.ComputeSigningRoot(env.Message, dom)
	if err != nil {
		return nil, err
	}
	return &bls.SignatureBatch{
		Signatures:   [][]byte{env.Signature},
		PublicKeys:   []bls.PublicKey{exp.pubkey},
		Messages:     [][32]byte{root},
		Descriptions: []string{"backfill execution payload envelope signature"},
	}, nil
}

// bindEnvelope checks the envelope against the expected block and its signed bid.
func bindEnvelope(env *ethpb.SignedExecutionPayloadEnvelope, exp *envelopeExpectation) error {
	m := env.Message
	root := exp.block.Root()
	if !bytes.Equal(m.BeaconBlockRoot, root[:]) {
		return errors.Wrapf(errInvalidEnvelopeResponse, "beacon_block_root=%#x does not match block root=%#x", m.BeaconBlockRoot, root)
	}
	parentRoot := exp.block.Block().ParentRoot()
	if !bytes.Equal(m.ParentBeaconBlockRoot, parentRoot[:]) {
		return errors.Wrapf(errInvalidEnvelopeResponse, "parent_beacon_block_root=%#x does not match block parent root=%#x", m.ParentBeaconBlockRoot, parentRoot)
	}
	if primitives.Slot(m.Payload.SlotNumber) != exp.block.Block().Slot() {
		return errors.Wrapf(errInvalidEnvelopeResponse, "payload slot=%d does not match block slot=%d", m.Payload.SlotNumber, exp.block.Block().Slot())
	}
	if m.BuilderIndex != exp.bid.BuilderIndex() {
		return errors.Wrapf(errInvalidEnvelopeResponse, "builder_index=%d does not match bid builder_index=%d", m.BuilderIndex, exp.bid.BuilderIndex())
	}
	if bytesutil.ToBytes32(m.Payload.BlockHash) != exp.bid.BlockHash() {
		return errors.Wrapf(errInvalidEnvelopeResponse, "payload block_hash=%#x does not match bid block_hash=%#x", m.Payload.BlockHash, exp.bid.BlockHash())
	}
	if bytesutil.ToBytes32(m.Payload.ParentHash) != exp.bid.ParentBlockHash() {
		return errors.Wrapf(errInvalidEnvelopeResponse, "payload parent_hash=%#x does not match bid parent_block_hash=%#x", m.Payload.ParentHash, exp.bid.ParentBlockHash())
	}
	reqRoot, err := m.ExecutionRequests.HashTreeRoot()
	if err != nil {
		return errors.Wrapf(errInvalidEnvelopeResponse, "execution_requests hash tree root: %v", err)
	}
	if reqRoot != exp.bid.ExecutionRequestsRoot() {
		return errors.Wrapf(errInvalidEnvelopeResponse, "execution_requests root=%#x does not match bid execution_requests_root=%#x", reqRoot, exp.bid.ExecutionRequestsRoot())
	}
	return nil
}

// envelopePage tracks the attempt and elapsed-time budget for one contiguous chunk of expected
// slots; empty responses are protocol-legal, so the budget is what bounds retries.
type envelopePage struct {
	lo, hi   primitives.Slot // half-open [lo, hi)
	attempts int
	deadline time.Time // zero until the first attempt
}

// envelopeSync tracks a batch's envelope expectations through fetch and verification; verified
// envelopes are held blinded until finalize persists them at import time.
type envelopeSync struct {
	cfg     *envelopeSyncCfg
	peer    peer.ID
	pending map[primitives.Slot]*envelopeExpectation
	held    map[primitives.Slot]*ethpb.SignedBlindedExecutionPayloadEnvelope
	// tail is the batch-tail expectation while its payload fullness is unclassified.
	tail      *envelopeExpectation
	pages     []*envelopePage
	skips     map[string][]primitives.Slot
	finalized bool
	// published is set once the skip counters are emitted, after the batch's status write.
	published bool
	now       func() time.Time
}

type envelopeSyncCfg struct {
	verifier      *envelopeVerifier
	hasEnvelope   func(ctx context.Context, root [32]byte) bool
	boundaryChild childLookup
	currentNeeds  func() das.CurrentNeeds
	downscore     peerDownscorer
	maxAttempts   int
	attemptBudget time.Duration
	pace          time.Duration
	localDelay    time.Duration
}

func (cfg *envelopeSyncCfg) applyDefaults() {
	if cfg.maxAttempts == 0 {
		cfg.maxAttempts = envelopeMaxPageAttempts
	}
	if cfg.attemptBudget == 0 {
		cfg.attemptBudget = time.Duration(envelopeMaxPageAttempts) * params.BeaconConfig().RespTimeoutDuration()
	}
	if cfg.pace == 0 {
		cfg.pace = minReqInterval
	}
	if cfg.localDelay == 0 {
		cfg.localDelay = 250 * time.Millisecond
	}
}

// newEnvelopeSync computes the envelope expectations for a batch: Gloas slots inside the Env
// window whose payload was revealed and whose envelope is not already stored. The batch tail's
// child is outside the batch, so its classification may be deferred to import time.
func newEnvelopeSync(ctx context.Context, vbs verifiedROBlocks, cfg *envelopeSyncCfg) (*envelopeSync, error) {
	if cfg == nil || cfg.verifier == nil || cfg.hasEnvelope == nil {
		return nil, nil
	}
	cfg.applyDefaults()
	es := &envelopeSync{
		cfg:     cfg,
		pending: make(map[primitives.Slot]*envelopeExpectation),
		held:    make(map[primitives.Slot]*ethpb.SignedBlindedExecutionPayloadEnvelope),
		skips:   make(map[string][]primitives.Slot),
		now:     time.Now,
	}
	window := cfg.currentNeeds().Env
	for i := range vbs {
		b := vbs[i]
		if b.Block().Version() < version.Gloas {
			continue
		}
		slot := b.Block().Slot()
		if !window.At(slot) {
			continue
		}
		isTail := i == len(vbs)-1
		if !isTail {
			// A non-tail block's child is the next block in the batch; the child's bid commits
			// the parent's payload fullness.
			revealed, err := blocks.BlockBuiltOnParentPayload(b.Block(), vbs[i+1].Block())
			if err != nil {
				return nil, errors.Wrap(err, "block built on parent payload")
			}
			if !revealed {
				continue
			}
		}
		if cfg.hasEnvelope(ctx, b.Root()) {
			continue
		}
		exp, skip, err := cfg.verifier.expectation(b)
		if err != nil {
			return nil, err
		}
		if !isTail {
			if skip != "" {
				es.skip(slot, skip)
				continue
			}
			exp.required = true
			es.pending[slot] = exp
			continue
		}
		// The tail's fullness is unknown until its child is available.
		if child := es.tryBoundaryChild(ctx, b.Root()); child != nil {
			revealed, err := blocks.BlockBuiltOnParentPayload(b.Block(), child)
			if err != nil {
				return nil, errors.Wrap(err, "tail built on parent payload")
			}
			if !revealed {
				continue
			}
			if skip != "" {
				es.skip(slot, skip)
				continue
			}
			exp.required = true
			es.pending[slot] = exp
			continue
		}
		if skip != "" {
			// Unverifiable tail: never fetched, classification deferred to import time.
			es.tail = &envelopeExpectation{block: b, skipReason: skip}
			continue
		}
		es.tail = exp
		es.pending[slot] = exp
	}
	es.buildPages()
	return es, nil
}

// tryBoundaryChild returns the tail's already-imported child, or nil to defer classification.
func (es *envelopeSync) tryBoundaryChild(ctx context.Context, tailRoot [32]byte) interfaces.ReadOnlyBeaconBlock {
	if es.cfg.boundaryChild == nil {
		return nil
	}
	child, err := es.cfg.boundaryChild(ctx, tailRoot)
	if err != nil {
		return nil
	}
	return child
}

func (es *envelopeSync) buildPages() {
	slotList := es.pendingSlots()
	for _, s := range slotList {
		if n := len(es.pages); n > 0 && s < es.pages[n-1].hi {
			continue
		}
		es.pages = append(es.pages, &envelopePage{lo: s, hi: s + envelopePageSize})
	}
}

func (es *envelopeSync) pendingSlots() []primitives.Slot {
	return slices.Sorted(maps.Keys(es.pending))
}

func (es *envelopeSync) pendingIn(pg *envelopePage) []primitives.Slot {
	out := make([]primitives.Slot, 0)
	for _, s := range es.pendingSlots() {
		if s >= pg.lo && s < pg.hi {
			out = append(out, s)
		}
	}
	return out
}

// unresolved returns the number of slots that still need a fetch attempt.
func (es *envelopeSync) unresolved() int {
	if es == nil {
		return 0
	}
	return len(es.pending)
}

// skip records a terminal skip for the slot; the counter is emitted later by publishSkips.
func (es *envelopeSync) skip(slot primitives.Slot, reason string) {
	delete(es.pending, slot)
	if es.tail != nil && es.tail.block.Block().Slot() == slot {
		es.tail = nil
	}
	es.skips[reason] = append(es.skips[reason], slot)
}

// publishSkips emits the skip counters. A skip claims a permanent gap, so it is published only
// after the batch's status write succeeds; a retried batch must not double-count.
func (es *envelopeSync) publishSkips() {
	if es == nil || es.published {
		return
	}
	es.published = true
	for reason, slots := range es.skips {
		envelopeSlotsSkipped.WithLabelValues(reason).Add(float64(len(slots)))
	}
}

// retireTailFetch removes the tail from the fetch set, keeping it for import-time classification.
func (es *envelopeSync) retireTailFetch(slot primitives.Slot) {
	delete(es.pending, slot)
}

// pruneExpiredWindow drops pending slots that have left the Env retention window.
func (es *envelopeSync) pruneExpiredWindow() {
	window := es.cfg.currentNeeds().Env
	for slot := range es.pending {
		if window.At(slot) {
			continue
		}
		delete(es.pending, slot)
		if es.tail != nil && es.tail.block.Block().Slot() == slot {
			es.tail = nil
		}
	}
}

// fetchPass makes at most one RPC attempt per unresolved page against the assigned peer; pages
// that exhaust their budget expire into terminal outcomes.
func (es *envelopeSync) fetchPass(ctx context.Context, pid peer.ID, fetch envelopeFetcher) {
	es.peer = pid
	es.pruneExpiredWindow()
	first := true
	for _, pg := range es.pages {
		if ctx.Err() != nil {
			return
		}
		slotsIn := es.pendingIn(pg)
		if len(slotsIn) == 0 {
			continue
		}
		if pg.attempts >= es.cfg.maxAttempts || (!pg.deadline.IsZero() && es.now().After(pg.deadline)) {
			continue // expired below
		}
		if !first {
			select {
			case <-ctx.Done():
				return
			case <-time.After(es.cfg.pace):
			}
		}
		first = false
		pg.attempts++
		if pg.deadline.IsZero() {
			// Anchored at the first attempt; waiting for a new peer does not extend the budget.
			pg.deadline = es.now().Add(es.cfg.attemptBudget)
		}
		lo := slotsIn[0]
		hi := slotsIn[len(slotsIn)-1] + 1
		envs, err := fetch(ctx, pid, &ethpb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: lo, Count: uint64(hi.FlooredSubSlot(lo))})
		if err != nil {
			// Only invalid content is downscored; empty and short responses are protocol-legal.
			if errors.Is(err, sync.ErrInvalidFetchedData) {
				es.cfg.downscore(pid, "invalid ExecutionPayloadEnvelope range response", err)
			}
			log.WithError(err).WithFields(logrus.Fields{"peer": pid, "startSlot": lo, "count": uint64(hi - lo)}).
				Debug("Failed to request execution payload envelopes by range")
			continue
		}
		envelopeDownloadCount.Add(float64(len(envs)))
		es.processResponse(pid, envs)
	}
	es.expireExhaustedPages()
}

// expirePending applies window pruning and page expiry outside a worker dispatch and reports
// whether any fetch work remains; the pool routes finished batches onward without a peer.
func (es *envelopeSync) expirePending() (done bool) {
	if es == nil {
		return true
	}
	es.pruneExpiredWindow()
	es.expireExhaustedPages()
	return es.unresolved() == 0
}

// expireExhaustedPages converts out-of-budget pages into terminal outcomes: required slots skip
// as peer_exhausted, the tail defers to import-time classification.
func (es *envelopeSync) expireExhaustedPages() {
	for _, pg := range es.pages {
		slotsIn := es.pendingIn(pg)
		if len(slotsIn) == 0 {
			continue
		}
		if pg.attempts < es.cfg.maxAttempts && (pg.deadline.IsZero() || !es.now().After(pg.deadline)) {
			continue
		}
		for _, slot := range slotsIn {
			exp := es.pending[slot]
			if exp.required {
				es.skip(slot, envSkipPeerExhausted)
			} else {
				es.retireTailFetch(slot)
			}
		}
	}
}

// processResponse verifies expected envelopes (block/bid binding, aggregate BLS, local block
// hash against the bid) and holds them blinded for persistence at import.
func (es *envelopeSync) processResponse(pid peer.ID, envs []*ethpb.SignedExecutionPayloadEnvelope) {
	type candidate struct {
		env  *ethpb.SignedExecutionPayloadEnvelope
		exp  *envelopeExpectation
		slot primitives.Slot
	}
	cands := make([]candidate, 0, len(envs))
	for _, env := range envs {
		// The RPC helper rejects structurally invalid envelopes before they get here.
		if env == nil || env.Message == nil || env.Message.Payload == nil || env.Message.ExecutionRequests == nil {
			continue
		}
		slot := env.Message.Payload.SlotNumber
		exp, ok := es.pending[slot]
		if !ok {
			// Peers legitimately serve every canonical envelope in range; ignore unexpected slots.
			continue
		}
		cands = append(cands, candidate{env: env, exp: exp, slot: slot})
	}
	if len(cands) == 0 {
		return
	}

	set := bls.NewSet()
	for _, c := range cands {
		if err := bindEnvelope(c.env, c.exp); err != nil {
			es.cfg.downscore(pid, "invalid ExecutionPayloadEnvelope batch rpc response", err)
			return
		}
		sb, err := es.cfg.verifier.signatureBatch(c.env, c.exp)
		if err != nil {
			// A domain/signing-root failure is local, not a peer offense.
			log.WithError(err).WithField("slot", c.slot).Debug("Could not build envelope signature batch")
			es.skip(c.slot, envSkipSigUnverifiable)
			return
		}
		set.Join(sb)
	}
	valid, err := set.Verify()
	if err != nil || !valid {
		if err == nil {
			err = errors.Wrap(errInvalidEnvelopeResponse, "envelope batch signature verification failed")
		}
		es.cfg.downscore(pid, "invalid ExecutionPayloadEnvelope batch signature", err)
		return
	}

	for _, c := range cands {
		parentRoot := c.exp.block.Block().ParentRoot()
		computed, err := executionBlockHashGloas(c.env.Message.Payload, c.env.Message.ExecutionRequests, parentRoot)
		if err != nil {
			// A decode failure or a mismatch below is a peer offense; the slot stays unresolved.
			es.cfg.downscore(pid, "undecodable ExecutionPayloadEnvelope payload", errors.Wrap(errInvalidEnvelopeResponse, err.Error()))
			continue
		}
		if computed != c.exp.bid.BlockHash() {
			es.cfg.downscore(pid, "envelope payload does not match bid block hash", errors.Wrapf(errInvalidEnvelopeResponse,
				"computed block hash=%#x, bid block hash=%#x", computed, c.exp.bid.BlockHash()))
			continue
		}
		delete(es.pending, c.slot)
		es.held[c.slot] = kv.BlindEnvelope(c.env)
		envelopeVerifiedCount.Inc()
	}
}

// finalize runs at import time: it classifies the tail via the boundary child, persists held
// envelopes blinded, and reduces leftovers to terminal outcomes. It never fails the batch.
func (es *envelopeSync) finalize(ctx context.Context, db BeaconDB, lowRoot [32]byte) {
	if es == nil || es.finalized {
		return
	}
	es.finalized = true

	if es.tail != nil {
		exp := es.tail
		es.tail = nil
		tailSlot := exp.block.Block().Slot()
		held := es.held[tailSlot]
		delete(es.held, tailSlot)
		child := es.boundaryChildWithRetries(ctx, db, lowRoot, exp.block.Root())
		if child == nil {
			es.skip(tailSlot, envSkipTailUnresolved)
		} else {
			revealed, err := blocks.BlockBuiltOnParentPayload(exp.block.Block(), child)
			switch {
			case err != nil:
				log.WithError(err).WithField("slot", tailSlot).Debug("Could not classify batch tail payload fullness")
				es.skip(tailSlot, envSkipTailUnresolved)
			case !revealed:
				// Withheld tail: expects nothing, even if it could not have been verified.
			case exp.skipReason != "":
				es.skip(tailSlot, exp.skipReason)
			case held != nil:
				es.held[tailSlot] = held
			default:
				es.skip(tailSlot, envSkipPeerExhausted)
			}
		}
	}

	for _, slot := range es.heldSlots() {
		h := es.held[slot]
		outcome, err := es.saveWithRetries(ctx, db, h)
		if err != nil {
			log.WithError(err).WithField("slot", slot).Debug("Could not persist backfilled execution payload envelope")
			es.skip(slot, envSkipDBFailed)
			continue
		}
		if outcome == iface.EnvelopeSaveConflict {
			// The stored envelope came from an already verified save path; keep it.
			envelopeConflictsKept.Inc()
			envelopeSkipLogger.WithField("slot", slot).WithField("blockRoot", bytesutil.Trunc(h.Message.BeaconBlockRoot)).
				Warn("Backfilled envelope conflicts with stored envelope; kept existing")
		}
	}
	es.logSkipSummary()
}

func (es *envelopeSync) heldSlots() []primitives.Slot {
	return slices.Sorted(maps.Keys(es.held))
}

// boundaryChildWithRetries looks up the tail's imported child via the status low root, retrying
// transient errors; a missing or non-linking block returns nil immediately.
func (es *envelopeSync) boundaryChildWithRetries(ctx context.Context, db BeaconDB, lowRoot, tailRoot [32]byte) interfaces.ReadOnlyBeaconBlock {
	for i := 0; i < envelopeLocalRetries; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(es.cfg.localDelay):
			}
		}
		child, err := db.Block(ctx, lowRoot)
		if err != nil {
			continue
		}
		if child == nil || child.Block() == nil || child.Block().ParentRoot() != tailRoot {
			return nil
		}
		return child.Block()
	}
	return nil
}

func (es *envelopeSync) saveWithRetries(ctx context.Context, db BeaconDB, env *ethpb.SignedBlindedExecutionPayloadEnvelope) (iface.EnvelopeSaveOutcome, error) {
	var outcome iface.EnvelopeSaveOutcome
	var err error
	for i := 0; i < envelopeLocalRetries; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return outcome, ctx.Err()
			case <-time.After(es.cfg.localDelay):
			}
		}
		outcome, err = db.SaveBlindedExecutionPayloadEnvelope(ctx, env)
		if err == nil {
			return outcome, nil
		}
	}
	return outcome, err
}

func (es *envelopeSync) logSkipSummary() {
	if len(es.skips) == 0 {
		return
	}
	fields := logrus.Fields{}
	total := 0
	for reason, skipped := range es.skips {
		fields[reason] = len(skipped)
		total += len(skipped)
	}
	fields["total"] = total
	envelopeSkipLogger.WithFields(fields).Warn("Skipped execution payload envelope slots during backfill")
}
