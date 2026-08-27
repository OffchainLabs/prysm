package backfill

import (
	"bytes"
	"context"
	"sort"
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
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Terminal skip reasons for slots whose envelope could not be backfilled. Skipped slots are
// never retried after the batch completes, and are counted once the batch is durable.
const (
	envSkipSigUnverifiable = "sig_unverifiable"
	envSkipELFailed        = "el_failed"
	envSkipPeerExhausted   = "peer_exhausted"
	envSkipTailUnresolved  = "tail_unresolved"
	envSkipDBFailed        = "db_failed"
)

const (
	// envelopePageSize is the number of slots covered by a single envelopes-by-range request.
	envelopePageSize = primitives.Slot(16)
	// envelopeMaxPageAttempts caps the total RPC attempts for an unresolved page.
	envelopeMaxPageAttempts = 3
	// envelopeLocalRetries caps local (non-network) retries: EL reconstruction calls,
	// the boundary child lookup, and envelope db writes.
	envelopeLocalRetries = 3
)

var (
	errInvalidEnvelopeResponse = errors.New("invalid execution payload envelope response")
	// envelopeSkipLogger rate limits the per-batch summary of skipped envelope slots.
	envelopeSkipLogger = newIntervalLogger(log, 30)
)

// EnvelopeReconstructor reconstructs full Gloas execution payloads from the execution client,
// keyed by execution block hash. It is satisfied by execution.Reconstructor.
type EnvelopeReconstructor interface {
	ReconstructFullGloasExecutionPayloadsByHash(ctx context.Context, blockHashes [][32]byte) (map[[32]byte]*enginev1.ExecutionPayloadGloas, error)
}

// envelopeFetcher requests execution payload envelopes by range from the given peer.
// It is a var so tests can inject responses without a p2p stack.
type envelopeFetcher func(ctx context.Context, pid peer.ID, req *ethpb.ExecutionPayloadEnvelopesByRangeRequest) ([]*ethpb.SignedExecutionPayloadEnvelope, error)

// envelopeVerifier verifies backfilled envelope signatures statelessly, using key material
// snapshotted from the checkpoint origin state. Backfill only descends below the origin, so a
// pre-Gloas origin implies no envelope expectations at all, and a Gloas origin's registries are
// cumulative for everything below it.
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

// envelopeExpectation captures a slot that expects an envelope, along with the block/bid it
// must bind to and the key its signature must verify against.
type envelopeExpectation struct {
	block  blocks.ROBlock
	bid    *ethpb.ExecutionPayloadBid
	pubkey bls.PublicKey
	// required is false only for the batch-tail slot while its payload fullness is still
	// unclassified; it is resolved via the boundary child at import time.
	required bool
	// skipReason is set on an unclassified batch-tail expectation whose signature can never be
	// verified. The slot is never fetched; import-time classification decides whether the skip
	// is recorded (revealed) or the slot simply expects nothing (withheld).
	skipReason string
}

// expectation resolves the verification key for the given block's envelope. The key is decided
// before any fetch: the historical block's proposer key for self-built payloads, otherwise the
// builder registry snapshot. A missing builder index or one whose occupant deposited after the
// envelope's epoch means the signature is unverifiable (builder indices are reused, so the
// current occupant could validly sign a historical envelope and a passing signature would prove
// nothing); those slots are skipped without a request.
func (v *envelopeVerifier) expectation(b blocks.ROBlock) (*envelopeExpectation, string, error) {
	sb, err := b.Block().Body().SignedExecutionPayloadBid()
	if err != nil {
		return nil, "", errors.Wrap(err, "signed execution payload bid")
	}
	if sb == nil || sb.Message == nil {
		return nil, "", errors.New("nil execution payload bid in verified block")
	}
	bid := sb.Message
	var key []byte
	if bid.BuilderIndex == params.BeaconConfig().BuilderIndexSelfBuild {
		pidx := b.Block().ProposerIndex()
		if uint64(pidx) >= uint64(len(v.keys)) {
			return nil, envSkipSigUnverifiable, nil
		}
		key = v.keys[pidx][:]
	} else {
		idx := uint64(bid.BuilderIndex)
		if idx >= uint64(len(v.builders)) || v.builders[idx] == nil {
			return nil, envSkipSigUnverifiable, nil
		}
		bldr := v.builders[idx]
		// A builder can only be active in epochs strictly after its deposit epoch, so the
		// snapshot occupant provably held the index at the envelope's epoch only when
		// deposit_epoch < envelope_epoch; anything else could be a later occupant of a
		// reused index.
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

// signatureBatch builds the aggregate verification entry for one envelope, deriving the
// DOMAIN_BEACON_BUILDER domain for the envelope's epoch from the fork schedule.
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

// bindEnvelope checks every envelope field retained by blinding against the expected block, and
// the payload commitments against the block's signed bid. Any mismatch means the peer served an
// envelope that cannot belong to the canonical block at that slot.
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
	if m.BuilderIndex != exp.bid.BuilderIndex {
		return errors.Wrapf(errInvalidEnvelopeResponse, "builder_index=%d does not match bid builder_index=%d", m.BuilderIndex, exp.bid.BuilderIndex)
	}
	if !bytes.Equal(m.Payload.BlockHash, exp.bid.BlockHash) {
		return errors.Wrapf(errInvalidEnvelopeResponse, "payload block_hash=%#x does not match bid block_hash=%#x", m.Payload.BlockHash, exp.bid.BlockHash)
	}
	if !bytes.Equal(m.Payload.ParentHash, exp.bid.ParentBlockHash) {
		return errors.Wrapf(errInvalidEnvelopeResponse, "payload parent_hash=%#x does not match bid parent_block_hash=%#x", m.Payload.ParentHash, exp.bid.ParentBlockHash)
	}
	reqRoot, err := m.ExecutionRequests.HashTreeRoot()
	if err != nil {
		return errors.Wrapf(errInvalidEnvelopeResponse, "execution_requests hash tree root: %v", err)
	}
	if !bytes.Equal(reqRoot[:], exp.bid.ExecutionRequestsRoot) {
		return errors.Wrapf(errInvalidEnvelopeResponse, "execution_requests root=%#x does not match bid execution_requests_root=%#x", reqRoot, exp.bid.ExecutionRequestsRoot)
	}
	return nil
}

// envelopePage tracks the request attempt budget for one contiguous chunk of expected slots.
// An empty or short response is protocol-legal and never downscored, so pages are bounded by
// both a total attempt count and an elapsed budget anchored at the first attempt.
type envelopePage struct {
	lo, hi   primitives.Slot // half-open [lo, hi)
	attempts int
	deadline time.Time // zero until the first attempt
}

type heldEnvelope struct {
	exp     *envelopeExpectation
	blinded *ethpb.SignedBlindedExecutionPayloadEnvelope
}

// envelopeSync tracks per-batch envelope expectations through fetch and verification. Verified
// envelopes are held in blinded form until the batch is imported, at which point finalize
// classifies the batch-tail slot via the boundary child and persists everything.
type envelopeSync struct {
	cfg     *envelopeSyncCfg
	peer    peer.ID
	pending map[primitives.Slot]*envelopeExpectation
	held    map[primitives.Slot]*heldEnvelope
	// tail is the batch-tail expectation while its fullness is unclassified. Its slot may leave
	// pending (fetched or budget exhausted) while classification still needs to happen at import.
	tail      *envelopeExpectation
	pages     []*envelopePage
	skips     map[string][]primitives.Slot
	finalized bool
	// published is set once the skip counters have been emitted, which happens only after the
	// batch's status write succeeds.
	published bool
	now       func() time.Time
}

type envelopeSyncCfg struct {
	verifier      *envelopeVerifier
	reconstructor EnvelopeReconstructor
	hasEnvelope   func(ctx context.Context, root [32]byte) bool
	boundaryChild func(ctx context.Context, tailRoot [32]byte) (interfaces.ReadOnlyBeaconBlock, error)
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

// newEnvelopeSync computes the envelope expectations for a batch of verified blocks. A slot
// expects an envelope iff the block is Gloas+, the slot is within the Env retention window, the
// payload was revealed (committed by the child block's bid), and no envelope is already stored.
// Withheld and skipped slots expect nothing. The batch-tail slot's child is outside the batch;
// if the boundary child is already imported it is classified immediately, otherwise
// classification is deferred to import time.
func newEnvelopeSync(ctx context.Context, vbs verifiedROBlocks, cfg *envelopeSyncCfg) (*envelopeSync, error) {
	if cfg == nil || cfg.verifier == nil || cfg.reconstructor == nil || cfg.hasEnvelope == nil {
		return nil, nil
	}
	cfg.applyDefaults()
	es := &envelopeSync{
		cfg:     cfg,
		pending: make(map[primitives.Slot]*envelopeExpectation),
		held:    make(map[primitives.Slot]*heldEnvelope),
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
			// Every non-tail block's direct child is the next block in the batch, because batch
			// verification enforces the parent-root chain. The child's bid commits the parent's
			// payload fullness.
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
		// The batch tail's fullness is unknown until its child is available. Skips are only
		// recorded once the slot is known to be revealed; a withheld slot expects nothing.
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
			// Unverifiable: never fetched, classification deferred to import time.
			es.tail = &envelopeExpectation{block: b, skipReason: skip}
			continue
		}
		es.tail = exp
		es.pending[slot] = exp
	}
	es.buildPages()
	return es, nil
}

// tryBoundaryChild returns the already-imported child of the batch tail if it is available and
// links to the tail, or nil when classification must wait for import time.
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
	out := make([]primitives.Slot, 0, len(es.pending))
	for s := range es.pending {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
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

// unresolved returns the number of slots that still need a fetch attempt. The batch leaves the
// envelope sync state once this reaches zero; every expectation then has a held envelope or a
// terminal skip (or, for the tail, a pending import-time classification).
func (es *envelopeSync) unresolved() int {
	if es == nil {
		return 0
	}
	return len(es.pending)
}

// elFailed records an EL cross-check failure, which is never a peer offense. A required slot
// takes the terminal skip immediately. The batch tail may still be unclassified, and a withheld
// tail must stay silent, so its reason is deferred to import-time classification rather than
// dropped — otherwise an expired tail is reported as a peer drought instead of an EL failure.
func (es *envelopeSync) elFailed(exp *envelopeExpectation, slot primitives.Slot) {
	if exp.required {
		es.skip(slot, envSkipELFailed)
		return
	}
	exp.skipReason = envSkipELFailed
}

// skip records a terminal skip for the slot: the envelope stays absent and is never retried
// after the batch completes. The counter is emitted later, by publishSkips.
func (es *envelopeSync) skip(slot primitives.Slot, reason string) {
	delete(es.pending, slot)
	if es.tail != nil && es.tail.block.Block().Slot() == slot {
		es.tail = nil
	}
	es.skips[reason] = append(es.skips[reason], slot)
}

// publishSkips emits the skip counters. A skip claims a permanent gap, so it is only published
// once the batch's backfill status write has succeeded: a batch that is retried or expires
// before import would otherwise leave phantom or double-counted gaps behind. Activity counters
// (downloads, verifications) are deliberately not gated this way — they measure work done, not
// final state.
func (es *envelopeSync) publishSkips() {
	if es == nil || es.published {
		return
	}
	es.published = true
	for reason, slots := range es.skips {
		envelopeSlotsSkipped.WithLabelValues(reason).Add(float64(len(slots)))
	}
}

// retireTailFetch removes the tail slot from the fetch set while keeping the expectation for
// import-time classification (which decides between nothing, persist, and peer_exhausted).
func (es *envelopeSync) retireTailFetch(slot primitives.Slot) {
	delete(es.pending, slot)
}

// pruneExpiredWindow drops pending slots that have fallen out of the Env retention window since
// the expectations were computed.
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

// fetchPass makes at most one RPC attempt per unresolved page against the assigned peer,
// verifying and holding whatever it can. Remaining unresolved pages either still have attempt
// budget (the batch stays in the envelope sync state and is reassigned, preferring a different
// peer) or are expired into terminal skips.
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
			// Anchor the elapsed budget at the first attempt; waiting for a new peer does not extend it.
			pg.deadline = es.now().Add(es.cfg.attemptBudget)
		}
		lo := slotsIn[0]
		hi := slotsIn[len(slotsIn)-1] + 1
		envs, err := fetch(ctx, pid, &ethpb.ExecutionPayloadEnvelopesByRangeRequest{StartSlot: lo, Count: uint64(hi.FlooredSubSlot(lo))})
		if err != nil {
			// Empty and short responses are protocol-legal and never downscored; only invalid content is.
			if errors.Is(err, sync.ErrInvalidFetchedData) {
				es.cfg.downscore(pid, "invalid ExecutionPayloadEnvelope range response", err)
			}
			log.WithError(err).WithFields(logrus.Fields{"peer": pid, "startSlot": lo, "count": uint64(hi - lo)}).
				Debug("Failed to request execution payload envelopes by range")
			continue
		}
		envelopeDownloadCount.Add(float64(len(envs)))
		es.processResponse(ctx, pid, envs)
	}
	es.expireExhaustedPages()
}

// expirePending applies window pruning and page expiry outside a worker dispatch and reports
// whether any fetch work remains. Deadlines are anchored at each page's first attempt, so once
// the elapsed budget lapses the batch has only local bookkeeping left; the pool uses this to
// route such batches onward instead of holding them until a peer can be assigned.
func (es *envelopeSync) expirePending() (done bool) {
	if es == nil {
		return true
	}
	es.pruneExpiredWindow()
	es.expireExhaustedPages()
	return es.unresolved() == 0
}

// expireExhaustedPages converts pages that are out of attempts or elapsed budget into terminal
// outcomes: required slots become peer_exhausted skips, and the tail slot leaves the fetch set
// with classification deferred to import.
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

// processResponse runs the staged verification over the envelopes we actually expect:
// block/bid binding, one aggregate BLS verification for the page, then the EL content
// cross-check. Verified envelopes are blinded and held for persistence at import time.
func (es *envelopeSync) processResponse(ctx context.Context, pid peer.ID, envs []*ethpb.SignedExecutionPayloadEnvelope) {
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
			// Peers legitimately serve every canonical envelope in the requested range; ignore
			// slots we don't expect (withheld, already stored, or terminally skipped).
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
			// A domain/signing-root failure is local, not a peer offense: the signature can
			// never be checked, so the slot takes the unverifiable skip.
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

	hashes := make([][32]byte, 0, len(cands))
	for _, c := range cands {
		hashes = append(hashes, bytesutil.ToBytes32(c.exp.bid.BlockHash))
	}
	payloads, err := es.reconstructWithRetries(ctx, hashes)
	if err != nil {
		// The batched engine call fails as a whole when any single body is unavailable, so a
		// batch-level error must not discard every otherwise-reconstructable envelope in the
		// page: isolate the failure by falling back to per-hash reconstruction.
		payloads = es.reconstructIndividually(ctx, hashes)
	}
	for _, c := range cands {
		recon := payloads[bytesutil.ToBytes32(c.exp.bid.BlockHash)]
		if recon == nil {
			es.elFailed(c.exp, c.slot)
			continue
		}
		fetchedRoot, htrErr := c.env.Message.Payload.HashTreeRoot()
		if htrErr != nil {
			es.elFailed(c.exp, c.slot)
			continue
		}
		reconRoot, htrErr := recon.HashTreeRoot()
		if htrErr != nil {
			es.elFailed(c.exp, c.slot)
			continue
		}
		if fetchedRoot != reconRoot {
			// The reconstruction succeeded, so the peer served payload bytes that do not match
			// the proposer-committed block hash. That is a peer offense; leave the slot
			// unresolved so it can be retried from another peer.
			es.cfg.downscore(pid, "envelope payload does not match EL reconstruction", errors.Wrapf(errInvalidEnvelopeResponse,
				"payload htr=%#x, reconstructed htr=%#x", fetchedRoot, reconRoot))
			continue
		}
		delete(es.pending, c.slot)
		// A later attempt resolved the slot, so drop any EL failure deferred by an earlier one;
		// finalize prefers a deferred reason over a held envelope.
		c.exp.skipReason = ""
		es.held[c.slot] = &heldEnvelope{exp: c.exp, blinded: kv.BlindEnvelope(c.env)}
		envelopeVerifiedCount.Inc()
	}
}

// reconstructIndividually reconstructs each payload with its own engine call, so that hashes the
// EL cannot serve resolve to missing entries instead of failing the entire set. It is only used
// after the batched call (with its bounded retries) has failed, so each hash gets one attempt.
func (es *envelopeSync) reconstructIndividually(ctx context.Context, hashes [][32]byte) map[[32]byte]*enginev1.ExecutionPayloadGloas {
	out := make(map[[32]byte]*enginev1.ExecutionPayloadGloas, len(hashes))
	for _, h := range hashes {
		if ctx.Err() != nil {
			return out
		}
		payloads, err := es.cfg.reconstructor.ReconstructFullGloasExecutionPayloadsByHash(ctx, [][32]byte{h})
		if err != nil {
			continue
		}
		out[h] = payloads[h]
	}
	return out
}

func (es *envelopeSync) reconstructWithRetries(ctx context.Context, hashes [][32]byte) (map[[32]byte]*enginev1.ExecutionPayloadGloas, error) {
	var lastErr error
	for i := 0; i < envelopeLocalRetries; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(es.cfg.localDelay):
			}
		}
		payloads, err := es.cfg.reconstructor.ReconstructFullGloasExecutionPayloadsByHash(ctx, hashes)
		if err == nil {
			return payloads, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// finalize runs at import time, after the batch has been connected to the already-imported chain
// (so the boundary child of the tail is the block behind status.LowRoot). It classifies the tail
// definitively, persists every verified envelope in blinded form, and reduces every leftover to
// an explicit terminal outcome. It never fails the batch: block import always proceeds.
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
		outcome, err := es.saveWithRetries(ctx, db, h.blinded)
		if err != nil {
			log.WithError(err).WithField("slot", slot).Debug("Could not persist backfilled execution payload envelope")
			es.skip(slot, envSkipDBFailed)
			continue
		}
		if outcome == iface.EnvelopeSaveConflict {
			// A conflicting stored envelope came from an already verified save path; keep it.
			envelopeConflictsKept.Inc()
			root := h.exp.block.Root()
			envelopeSkipLogger.WithField("slot", slot).WithField("blockRoot", bytesutil.Trunc(root[:])).
				Warn("Backfilled envelope conflicts with stored envelope; kept existing")
		}
	}
	es.logSkipSummary()
}

func (es *envelopeSync) heldSlots() []primitives.Slot {
	out := make([]primitives.Slot, 0, len(es.held))
	for s := range es.held {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// boundaryChildWithRetries looks up the already-imported child of the batch tail via the
// backfill status low root, retrying transient lookup errors. A missing block or one that does
// not link to the tail is a data condition, not a transient error, and returns nil immediately.
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
