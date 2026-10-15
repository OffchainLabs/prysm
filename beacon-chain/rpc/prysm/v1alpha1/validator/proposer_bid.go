package validator

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"math/bits"
	"slices"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/builder"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/io/logs"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// bidSource indicates where the winning execution payload bid came from.
type bidSource int

const (
	bidSourceSelfBuild  bidSource = iota // local self-build, caller caches the envelope
	bidSourceP2P                         // P2P gossip bid, the builder reveals the envelope
	bidSourceBuilderAPI                  // Builder-API bid, caller submits the signed block to the builder
)

func (s bidSource) String() string {
	switch s {
	case bidSourceP2P:
		return "p2p"
	case bidSourceBuilderAPI:
		return "builder-api"
	default:
		return "self-build"
	}
}

func (vs *Server) setExecutionPayloadBid(
	ctx context.Context,
	sBlk interfaces.SignedBeaconBlock,
	head state.BeaconState,
	local *consensusblocks.GetPayloadResponse,
	builderWin *winningBuilderBid,
	builderConfig *ethpb.BuilderConfig,
	selfBuildOnly bool,
) (bidSource, error) {
	_, span := trace.StartSpan(ctx, "ProposerServer.setExecutionPayloadBid")
	defer span.End()

	if local == nil || local.ExecutionData == nil {
		return bidSourceSelfBuild, errors.New("local execution payload is nil")
	}

	if !selfBuildOnly {
		slot := sBlk.Block().Slot()
		p2pBid := vs.cachedP2PBid(ctx, head, slot, bytesutil.ToBytes32(local.ExecutionData.ParentHash()), sBlk.Block().ParentRoot())
		if p2pBid != nil {
			// The bid may have been cached before the builder was blacklisted.
			if msg, err := p2pBid.Bid(); err != nil || vs.BuilderCircuitBreaker.Blacklisted(msg.BuilderIndex(), slots.ToEpoch(slot)) {
				p2pBid = nil
			}
		}
		if bestBid, src, effective := bestBid(head, local, p2pBid, builderWin, builderConfig); bestBid != nil {
			msg, err := bestBid.Bid()
			if err != nil {
				return bidSourceSelfBuild, errors.Wrap(err, "could not get remote execution payload bid")
			}
			if err := sBlk.SetSignedExecutionPayloadBid(bestBid); err != nil {
				return bidSourceSelfBuild, errors.Wrap(err, "could not set remote execution payload bid")
			}
			log.WithFields(logrus.Fields{
				"slot":      sBlk.Block().Slot(),
				"source":    src,
				"builder":   msg.BuilderIndex(),
				"valueGwei": uint64(effective),
			}).Info("Chose payload bid")
			return src, nil
		}
	}

	// Fall back to self-build bid.
	bid, err := vs.createSelfBuildExecutionPayloadBid(local, sBlk.Block())
	if err != nil {
		return bidSourceSelfBuild, errors.Wrap(err, "could not create execution payload bid")
	}

	ro, err := vs.signedSelfBuildBid(ctx, head, bid)
	if err != nil {
		return bidSourceSelfBuild, errors.Wrap(err, "could not wrap signed execution payload bid")
	}
	if err := sBlk.SetSignedExecutionPayloadBid(ro); err != nil {
		return bidSourceSelfBuild, errors.Wrap(err, "could not set signed execution payload bid")
	}

	log.WithFields(logrus.Fields{
		"slot":      sBlk.Block().Slot(),
		"source":    bidSourceSelfBuild,
		"valueGwei": uint64(primitives.WeiToGwei(local.Bid)),
	}).Info("Chose payload bid")
	return bidSourceSelfBuild, nil
}

// winningBuilderBid pairs the best builder-API bid with the entry whose limits it was selected under.
type winningBuilderBid struct {
	bid   interfaces.ROSignedExecutionPayloadBid
	entry *ethpb.BuilderEntry
}

// Returns a nil bid when the local self-build wins, bids compete by boosted
// effective value with ties going local, the returned Gwei is unboosted.
// A nil local means there is no self-build to fall back to: any remote bid beats
// it and the P2P min bid is waived, since any bid beats missing the slot.
func bestBid(
	head state.BeaconState,
	local *consensusblocks.GetPayloadResponse,
	p2pBid interfaces.ROSignedExecutionPayloadBid,
	builderWin *winningBuilderBid,
	builderConfig *ethpb.BuilderConfig,
) (interfaces.ROSignedExecutionPayloadBid, bidSource, primitives.Gwei) {
	var bestBid interfaces.ROSignedExecutionPayloadBid
	var bestEffective, bestBoosted primitives.Gwei
	if local != nil {
		bestBoosted = primitives.WeiToGwei(local.Bid)
	}
	src := bidSourceSelfBuild

	consider := func(bid interfaces.ROSignedExecutionPayloadBid, effective primitives.Gwei, boostFactor uint64, from bidSource) {
		boosted := boostedBidValue(effective, boostFactor)
		if boosted > bestBoosted || (local == nil && bestBid == nil) {
			bestBid, bestEffective, bestBoosted, src = bid, effective, boosted, from
		}
	}

	if p2pBid != nil {
		minBid, boostFactor := primitives.Gwei(0), uint64(proposer.NeutralBuilderBoostFactor)
		if builderConfig != nil {
			minBid, boostFactor = builderConfig.MinBid, builderConfig.BuilderBoostFactor
		}
		effective := effectiveBidValue(p2pBid, p2pExecutionPaymentCap(head, builderConfig, p2pBid))
		if effective >= minBid || local == nil {
			consider(p2pBid, effective, boostFactor, bidSourceP2P)
		}
	}
	if builderWin != nil {
		effective := effectiveBidValue(builderWin.bid, uint64(builderWin.entry.MaxExecutionPayment))
		consider(builderWin.bid, effective, builderWin.entry.BuilderBoostFactor, bidSourceBuilderAPI)
	}

	return bestBid, src, bestEffective
}

// The proposer's total take, the execution payment counts only up to the proposer's max preference.
// A malformed bid has zero value.
func effectiveBidValue(bid interfaces.ROSignedExecutionPayloadBid, maxExecutionPayment uint64) primitives.Gwei {
	msg, err := bid.Bid()
	if err != nil {
		return 0
	}
	payment := msg.ExecutionPayment()
	if uint64(payment) > maxExecutionPayment {
		payment = primitives.Gwei(maxExecutionPayment)
	}
	sum := msg.Value() + payment
	if sum < msg.Value() {
		return primitives.Gwei(math.MaxUint64)
	}
	return sum
}

func boostedBidValue(v primitives.Gwei, factor uint64) primitives.Gwei {
	hi, lo := bits.Mul64(uint64(v), factor)
	if hi >= 100 {
		return primitives.Gwei(math.MaxUint64)
	}
	q, _ := bits.Div64(hi, lo, 100)
	return primitives.Gwei(q)
}

func p2pExecutionPaymentCap(head state.BeaconState, builderConfig *ethpb.BuilderConfig, bid interfaces.ROSignedExecutionPayloadBid) uint64 {
	if bid == nil || builderConfig == nil {
		return 0
	}
	msg, err := bid.Bid()
	if err != nil {
		return 0
	}
	var keyed []*ethpb.BuilderEntry
	for _, e := range builderConfig.GetBuilders() {
		if len(e.GetBuilderPubkeys()) > 0 {
			keyed = append(keyed, e)
		}
	}
	if len(keyed) == 0 {
		return 0
	}
	pk, err := head.BuilderPubkey(msg.BuilderIndex())
	if err != nil {
		return 0
	}
	var maxCap uint64
	for _, e := range keyed {
		if uint64(e.GetMaxExecutionPayment()) <= maxCap {
			continue
		}
		if slices.ContainsFunc(e.GetBuilderPubkeys(), func(bp []byte) bool { return bytes.Equal(bp, pk[:]) }) {
			maxCap = uint64(e.GetMaxExecutionPayment())
		}
	}
	return maxCap
}

// builderBidQuery carries the proposal context builder bids are requested and validated against.
type builderBidQuery struct {
	slot           primitives.Slot
	parentRoot     [32]byte
	parentHash     [32]byte
	pubkey         [fieldparams.BLSPubkeyLength]byte
	feeRecipient   []byte
	parentGasLimit uint64
	targetGasLimit uint64
	entries        []*ethpb.BuilderEntry
}

// newBuilderBidQuery derives the request context from the head state rather than the local payload,
// so builder bids can be requested before the local payload is available.
func (vs *Server) newBuilderBidQuery(
	ctx context.Context,
	head state.BeaconState,
	slot primitives.Slot,
	parentRoot [32]byte,
	proposerIdx primitives.ValidatorIndex,
	parentFull bool,
	entries []*ethpb.BuilderEntry,
) (*builderBidQuery, error) {
	val, err := head.ValidatorAtIndexReadOnly(proposerIdx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get proposer")
	}
	ph, err := vs.getParentBlockHash(ctx, head, slot, parentRoot, parentFull)
	if err != nil {
		return nil, errors.Wrap(err, "could not get parent block hash")
	}
	parentHash := bytesutil.ToBytes32(ph)
	parentGasLimit, err := vs.ForkchoiceFetcher.GasLimit(parentRoot, parentHash)
	if err != nil {
		return nil, errors.Wrap(err, "could not get parent gas limit")
	}
	pref := vs.proposerPreferenceForProposal(ctx, head, slot, proposerIdx)
	feeRecipient := pref.FeeRecipientOrDefault()
	return &builderBidQuery{
		slot:           slot,
		parentRoot:     parentRoot,
		parentHash:     parentHash,
		pubkey:         val.PublicKey(),
		feeRecipient:   feeRecipient[:],
		parentGasLimit: parentGasLimit,
		targetGasLimit: pref.GasLimitOr(parentGasLimit),
		entries:        entries,
	}, nil
}

// pendingBuilderBid is a builder-API bid request in flight alongside the local payload.
// A nil request yields no bid.
type pendingBuilderBid struct {
	cancel context.CancelFunc
	done   chan struct{}
	win    *winningBuilderBid
}

// wait blocks until the request settles and returns its winning bid.
func (p *pendingBuilderBid) wait() *winningBuilderBid {
	if p == nil {
		return nil
	}
	<-p.done
	return p.win
}

// abort cancels the request and waits for it to unwind so nothing reads the head state afterwards.
func (p *pendingBuilderBid) abort() {
	if p == nil {
		return
	}
	p.cancel()
	<-p.done
}

// requestBuilderBid starts the builder-API bid request in the background so BuilderBidTimeout
// runs alongside the local EL round trip instead of after it.
func (vs *Server) requestBuilderBid(
	ctx context.Context,
	blk interfaces.ReadOnlyBeaconBlock,
	head state.BeaconState,
	parentFull bool,
	builderConfig *ethpb.BuilderConfig,
) *pendingBuilderBid {
	// TODO(heze): builder API bid with inclusion_list_bits. The builder API returns Gloas bids, which a Heze block cannot carry.
	if slots.ToEpoch(blk.Slot()) >= params.BeaconConfig().HezeForkEpoch {
		return nil
	}

	entries := builderConfig.GetBuilders()
	if vs.BlockBuilder == nil || len(entries) == 0 {
		return nil
	}
	slot, parentRoot, proposerIdx := blk.Slot(), blk.ParentRoot(), blk.ProposerIndex()
	ctx, cancel := context.WithCancel(ctx)
	p := &pendingBuilderBid{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(p.done)
		q, err := vs.newBuilderBidQuery(ctx, head, slot, parentRoot, proposerIdx, parentFull, entries)
		if err != nil {
			log.WithError(err).Error("Could not build builder bid request")
			return
		}
		p.win = vs.getBuilderExecutionPayloadBid(ctx, head, q)
	}()
	return p
}

func (vs *Server) getBuilderExecutionPayloadBid(ctx context.Context, head state.BeaconState, q *builderBidQuery) *winningBuilderBid {
	if vs.BlockBuilder == nil || len(q.entries) == 0 {
		return nil
	}
	epoch := slots.ToEpoch(q.slot)
	entries := vs.allowedBuilderEntries(q, epoch)
	if len(entries) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, params.BeaconConfig().BuilderBidTimeout)
	defer cancel()

	bids, err := vs.BlockBuilder.GetExecutionPayloadBid(ctx, q.slot, q.parentHash, q.parentRoot, q.pubkey, entries)
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil
	}
	if err != nil {
		builderGetPayloadMissCount.Inc()
		log.WithError(err).Error("Could not get builder execution payload bid")
		return nil
	}

	var (
		best        *winningBuilderBid
		bestBoosted primitives.Gwei
	)
	bidLog := make([]string, 0, len(bids))
	replayed := replayedSignatures(bids)
	for _, pb := range bids {
		if pb.Bid == nil || pb.Entry == nil {
			continue
		}
		url := string(pb.Entry.GetUrl())
		if vs.BuilderCircuitBreaker.Blacklisted(pb.Bid.Message.BuilderIndex, epoch) {
			bidLog = append(bidLog, fmt.Sprintf("%s(builder=%d discarded: blacklisted)", logs.MaskCredentialsLogging(url), pb.Bid.Message.BuilderIndex))
			continue
		}
		if err := vs.validateBuilderBid(head, pb.Bid, q, pb.Entry); err != nil {
			bidLog = append(bidLog, fmt.Sprintf("%s(builder=%d discarded: %v)", logs.MaskCredentialsLogging(url), pb.Bid.Message.BuilderIndex, err))
			continue
		}
		ro, err := consensusblocks.WrappedROSignedExecutionPayloadBid(pb.Bid)
		if err != nil {
			bidLog = append(bidLog, fmt.Sprintf("%s(builder=%d discarded: %v)", logs.MaskCredentialsLogging(url), pb.Bid.Message.BuilderIndex, err))
			continue
		}
		// The signature proves the builder authored the bid, not that this endpoint serves it, so
		// a bid offered verbatim by several endpoints teaches us nothing.
		if !replayed[bytesutil.ToBytes96(pb.Bid.Signature)] {
			vs.BuilderCircuitBreaker.ObserveRelayBid(url, pb.Bid.Message.BuilderIndex, epoch)
		}
		effective := effectiveBidValue(ro, uint64(pb.Entry.MaxExecutionPayment))
		if effective < pb.Entry.MinBid {
			bidLog = append(bidLog, fmt.Sprintf("%s(builder=%d discarded: effective %d below min bid %d)", logs.MaskCredentialsLogging(url), pb.Bid.Message.BuilderIndex, effective, pb.Entry.MinBid))
			continue
		}
		boosted := boostedBidValue(effective, pb.Entry.BuilderBoostFactor)
		bidLog = append(bidLog, fmt.Sprintf("%s(builder=%d value=%d payment=%d effective=%d boosted=%d)",
			logs.MaskCredentialsLogging(url), pb.Bid.Message.BuilderIndex, pb.Bid.Message.Value, pb.Bid.Message.ExecutionPayment, effective, boosted))
		if best == nil || boosted > bestBoosted {
			best, bestBoosted = &winningBuilderBid{bid: ro, entry: pb.Entry}, boosted
		}
	}

	if len(bidLog) > 0 {
		log.WithField("slot", q.slot).Debugf("Builder bids: [%s]", strings.Join(bidLog, " | "))
	}

	if best == nil {
		builderGetPayloadMissCount.Inc()
		return nil
	}
	return best
}

// allowedBuilderEntries drops entries whose endpoint the circuit breaker banned, so a banned
// endpoint is not contacted at all rather than merely having its bids discarded.
func (vs *Server) allowedBuilderEntries(q *builderBidQuery, epoch primitives.Epoch) []*ethpb.BuilderEntry {
	allowed := make([]*ethpb.BuilderEntry, 0, len(q.entries))
	var banned []string
	for _, e := range q.entries {
		url := string(e.GetUrl())
		if vs.BuilderCircuitBreaker.RelayBanned(url, epoch) {
			banned = append(banned, logs.MaskCredentialsLogging(url))
			continue
		}
		allowed = append(allowed, e)
	}
	if len(banned) > 0 {
		log.WithFields(logrus.Fields{
			"slot":    q.slot,
			"relays":  strings.Join(banned, ","),
			"skipped": len(banned),
		}).Debug("Skipping banned builder endpoints")
	}
	return allowed
}

func replayedSignatures(bids []builder.PayloadBid) map[[96]byte]bool {
	seen := make(map[[96]byte]string, len(bids))
	replayed := make(map[[96]byte]bool)
	for _, pb := range bids {
		if pb.Bid == nil || pb.Entry == nil || len(pb.Bid.Signature) != fieldparams.BLSSignatureLength {
			continue
		}
		sig := bytesutil.ToBytes96(pb.Bid.Signature)
		url := string(pb.Entry.GetUrl())
		if prev, ok := seen[sig]; ok && prev != url {
			replayed[sig] = true
			continue
		}
		seen[sig] = url
	}
	return replayed
}

// validateBuilderBid mirrors process_execution_payload_bid so a chosen bid never invalidates the proposer's own block.
func (vs *Server) validateBuilderBid(head state.BeaconState, signed *ethpb.SignedExecutionPayloadBid, q *builderBidQuery, entry *ethpb.BuilderEntry) error {
	if signed == nil || signed.Message == nil {
		return errors.New("nil builder bid")
	}
	bid := signed.Message
	if bytes.Equal(bid.BlockHash, bid.ParentBlockHash) {
		return errors.New("bid block hash equals parent block hash")
	}
	if len(entry.BuilderPubkeys) > 0 {
		pk, err := head.BuilderPubkey(bid.BuilderIndex)
		if err != nil {
			return errors.Wrap(err, "could not get builder pubkey")
		}
		if !slices.ContainsFunc(entry.BuilderPubkeys, func(bp []byte) bool { return bytes.Equal(bp, pk[:]) }) {
			return errors.Errorf("builder %d is not in the entry's builder pubkeys", bid.BuilderIndex)
		}
	}

	if vs.NewExecutionPayloadBidVerifier == nil {
		return errors.New("bid verifier not ready")
	}
	ro, err := consensusblocks.WrappedROSignedExecutionPayloadBid(signed)
	if err != nil {
		return errors.Wrap(err, "could not wrap builder bid")
	}
	v := vs.NewExecutionPayloadBidVerifier(ro, verification.BuilderAPIBidRequirements)
	if err := v.VerifyBidSlotMatches(q.slot); err != nil {
		return err
	}
	if err := v.VerifyParentBlockRootSeen(func(root [32]byte) bool { return root == q.parentRoot }); err != nil {
		return err
	}
	if err := v.VerifyParentBlockHash(func(root, hash [32]byte) bool {
		return root == q.parentRoot && hash == q.parentHash
	}); err != nil {
		return err
	}
	if err := v.VerifyBuilderActive(head); err != nil {
		return err
	}
	if err := v.VerifyBuilderVersion(head); err != nil {
		return err
	}
	if err := v.VerifyBuilderCanCoverBid(head); err != nil {
		return err
	}
	if err := v.VerifyFeeRecipientMatches(q.feeRecipient); err != nil {
		return err
	}
	if err := v.VerifyGasLimitTargetCompatible(q.parentGasLimit, q.targetGasLimit); err != nil {
		return err
	}
	if err := v.VerifyBlobKzgCommitmentsLimit(); err != nil {
		return err
	}
	if err := v.VerifyPrevRandao(head); err != nil {
		return err
	}
	return v.VerifySignature(head)
}

// Mirrors the preference lookup in getLocalPayloadFromEngine so builder bids are held to the same preferences the EL payload was built with.
func (vs *Server) proposerPreferenceForProposal(ctx context.Context, st state.BeaconState, slot primitives.Slot, idx primitives.ValidatorIndex) cache.ProposerPreference {
	pref := cache.ProposerPreference{ValidatorIndex: idx}
	dependentRoot, err := helpers.ProposerDependentRootOrGenesis(ctx, vs.BeaconDB, st, slot)
	if err != nil {
		log.WithError(err).WithField("slot", slot).Debug("Could not get proposer dependent root, falling back to default proposer preference")
		if def, ok := vs.ProposerPreferencesCache.DefaultFor(idx); ok {
			pref = def
		}
		return pref
	}
	if p, ok := vs.ProposerPreferencesCache.BestFor(dependentRoot, slot, idx); ok {
		pref = p
	}
	return pref
}

// Best-effort and detached from the propose RPC, the builder also learns of the block via P2P.
func (vs *Server) submitBlockToBuilder(block interfaces.ReadOnlySignedBeaconBlock, builderURL string) {
	if vs.BlockBuilder == nil || builderURL == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), params.BeaconConfig().SlotDuration())
	defer cancel()
	if err := vs.BlockBuilder.SubmitSignedBeaconBlock(ctx, builderURL, block); err != nil {
		// Quoted: the url is caller-supplied and may fail validation for containing control bytes.
		log.WithError(err).WithField("builder", strconv.Quote(logs.MaskCredentialsLogging(builderURL))).Error("Could not submit signed beacon block to builder")
	}
}

// setRemoteBidFallback picks the better of the builder-API bid and the cached P2P bid when the
// local EL self-build is unavailable. The circuit breaker is deliberately not consulted for the
// P2P bid: with no local payload at all, a block carrying a possibly-undelivered bid still beats
// missing the slot outright.
func (vs *Server) setRemoteBidFallback(
	ctx context.Context,
	sBlk interfaces.SignedBeaconBlock,
	head state.BeaconState,
	parentFull bool,
	builderWin *winningBuilderBid,
	builderConfig *ethpb.BuilderConfig,
) (bidSource, error) {
	slot := sBlk.Block().Slot()
	var p2pBid interfaces.ROSignedExecutionPayloadBid
	if vs.HighestBidCache != nil {
		parentRoot := sBlk.Block().ParentRoot()
		parentHash, err := vs.getParentBlockHash(ctx, head, slot, parentRoot, parentFull)
		if err != nil {
			return bidSourceSelfBuild, errors.Wrap(err, "could not get parent block hash")
		}
		p2pBid = vs.cachedP2PBid(ctx, head, slot, bytesutil.ToBytes32(parentHash), parentRoot)
	}
	bid, src, effective := bestBid(head, nil, p2pBid, builderWin, builderConfig)
	if bid == nil {
		return bidSourceSelfBuild, errors.New("no builder or cached P2P bid available")
	}
	msg, err := bid.Bid()
	if err != nil {
		return bidSourceSelfBuild, errors.Wrap(err, "could not get fallback execution payload bid")
	}
	if err := sBlk.SetSignedExecutionPayloadBid(bid); err != nil {
		return bidSourceSelfBuild, errors.Wrap(err, "could not set fallback execution payload bid")
	}
	log.WithFields(logrus.Fields{
		"slot":      slot,
		"source":    src,
		"builder":   msg.BuilderIndex(),
		"valueGwei": uint64(effective),
	}).Info("Chose fallback payload bid")
	return src, nil
}

// cachedP2PBid returns the cached P2P bid. At Heze it skips a bid whose inclusion list bits do not include the local view.
func (vs *Server) cachedP2PBid(ctx context.Context, head state.ReadOnlyBeaconState, slot primitives.Slot, parentHash, parentRoot [32]byte) interfaces.ROSignedExecutionPayloadBid {
	if vs.HighestBidCache == nil {
		return nil
	}
	cached, ok := vs.HighestBidCache.Get(slot, parentHash, parentRoot)
	if !ok {
		return nil
	}
	if slots.ToEpoch(slot) < params.BeaconConfig().HezeForkEpoch {
		return cached
	}

	msg, err := cached.Bid()
	if err != nil {
		return nil
	}
	fields := logrus.Fields{"slot": slot, "builder": msg.BuilderIndex()}
	// A Heze block can only carry a Heze bid, which has the inclusion list bits.
	bits := msg.InclusionListBits()
	if bits == nil {
		log.WithFields(fields).Info("Skipping the cached P2P bid because it is not a Heze bid")
		return nil
	}
	ils, err := helpers.NewProposalInclusionLists(ctx, vs.BeaconDB, vs.ForkchoiceFetcher.InclusionListStore(), head, slot)
	inclusive := false
	if err == nil {
		inclusive, err = ils.IsBitsInclusive(ctx, bits)
	}
	if err != nil {
		log.WithError(err).WithFields(fields).Error("Could not check the inclusion list bits of the cached P2P bid, skipping the bid")
		return nil
	}
	if !inclusive {
		log.WithFields(fields).Info("Skipping the cached P2P bid because its inclusion list bits do not include the local inclusion list view")
		return nil
	}
	return cached
}

// signedSelfBuildBid signs the self-build bid with the G2 point at infinity; at Heze it is a Heze bid with the local inclusion list bits.
func (vs *Server) signedSelfBuildBid(ctx context.Context, head state.ReadOnlyBeaconState, bid *ethpb.ExecutionPayloadBid) (interfaces.ROSignedExecutionPayloadBid, error) {
	if slots.ToEpoch(bid.Slot) < params.BeaconConfig().HezeForkEpoch {
		return consensusblocks.WrappedROSignedExecutionPayloadBid(&ethpb.SignedExecutionPayloadBid{
			Message:   bid,
			Signature: common.InfiniteSignature[:],
		})
	}
	ils, err := helpers.NewProposalInclusionLists(ctx, vs.BeaconDB, vs.ForkchoiceFetcher.InclusionListStore(), head, bid.Slot)
	if err != nil {
		return nil, err
	}
	bits, err := ils.Bits(ctx)
	if err != nil {
		return nil, err
	}
	ro, err := consensusblocks.WrappedROExecutionPayloadBid(bid)
	if err != nil {
		return nil, err
	}
	return consensusblocks.WrappedROSignedExecutionPayloadBidHeze(&ethpb.SignedExecutionPayloadBidHeze{
		Message:   consensusblocks.ExecutionPayloadBidHezeProto(ro, bits),
		Signature: common.InfiniteSignature[:],
	})
}

// createSelfBuildExecutionPayloadBid creates an ExecutionPayloadBid for self-building,
// where the proposer acts as its own builder. Per spec, the bid value must be zero
// and the builder index must be BUILDER_INDEX_SELF_BUILD.
func (vs *Server) createSelfBuildExecutionPayloadBid(
	local *consensusblocks.GetPayloadResponse,
	block interfaces.ReadOnlyBeaconBlock,
) (*ethpb.ExecutionPayloadBid, error) {
	ed := local.ExecutionData
	if ed == nil || ed.IsNil() {
		return nil, errors.New("execution data is nil")
	}

	parentBlockRoot := block.ParentRoot()
	executionRequestsRoot, err := local.ExecutionRequestsGloas.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not compute execution requests root")
	}
	return &ethpb.ExecutionPayloadBid{
		ParentBlockHash:       ed.ParentHash(),
		ParentBlockRoot:       bytesutil.SafeCopyBytes(parentBlockRoot[:]),
		BlockHash:             ed.BlockHash(),
		PrevRandao:            ed.PrevRandao(),
		FeeRecipient:          ed.FeeRecipient(),
		GasLimit:              ed.GasLimit(),
		BuilderIndex:          params.BeaconConfig().BuilderIndexSelfBuild,
		Slot:                  block.Slot(),
		Value:                 0,
		ExecutionPayment:      0,
		BlobKzgCommitments:    local.BlobsBundler.GetKzgCommitments(),
		ExecutionRequestsRoot: executionRequestsRoot[:],
	}, nil
}
