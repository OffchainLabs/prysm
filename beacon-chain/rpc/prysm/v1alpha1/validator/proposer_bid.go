package validator

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"math/bits"
	"slices"
	"strings"
	"time"

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

const builderBidTimeout = 300 * time.Millisecond

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
		p2pBid := vs.cachedP2PBid(sBlk, local)
		if bestBid, src, effective := bestBid(head, local, p2pBid, builderWin, builderConfig); bestBid != nil {
			if err := sBlk.SetSignedExecutionPayloadBid(bestBid); err != nil {
				return bidSourceSelfBuild, errors.Wrap(err, "could not set remote execution payload bid")
			}
			log.WithFields(logrus.Fields{
				"slot":      sBlk.Block().Slot(),
				"source":    src,
				"builder":   bestBid.Message.BuilderIndex,
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

	// Per spec, self-build bids must use G2 point-at-infinity as the signature.
	signedBid := &ethpb.SignedExecutionPayloadBid{
		Message:   bid,
		Signature: common.InfiniteSignature[:],
	}
	if err := sBlk.SetSignedExecutionPayloadBid(signedBid); err != nil {
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
	bid   *ethpb.SignedExecutionPayloadBid
	entry *ethpb.BuilderEntry
}

// Returns a nil bid when the local self-build wins, bids compete by boosted
// effective value with ties going local, the returned Gwei is unboosted.
func bestBid(
	head state.BeaconState,
	local *consensusblocks.GetPayloadResponse,
	p2pBid *ethpb.SignedExecutionPayloadBid,
	builderWin *winningBuilderBid,
	builderConfig *ethpb.BuilderConfig,
) (*ethpb.SignedExecutionPayloadBid, bidSource, primitives.Gwei) {
	var bestBid *ethpb.SignedExecutionPayloadBid
	var bestEffective primitives.Gwei
	bestBoosted := primitives.WeiToGwei(local.Bid)
	src := bidSourceSelfBuild

	consider := func(bid *ethpb.SignedExecutionPayloadBid, effective primitives.Gwei, boostFactor uint64, from bidSource) {
		if boosted := boostedBidValue(effective, boostFactor); boosted > bestBoosted {
			bestBid, bestEffective, bestBoosted, src = bid, effective, boosted, from
		}
	}

	if p2pBid != nil {
		minBid, boostFactor := primitives.Gwei(0), uint64(proposer.NeutralBuilderBoostFactor)
		if builderConfig != nil {
			minBid, boostFactor = builderConfig.MinBid, builderConfig.BuilderBoostFactor
		}
		effective := effectiveBidValue(p2pBid, p2pExecutionPaymentCap(head, builderConfig, p2pBid))
		if effective >= minBid {
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
func effectiveBidValue(bid *ethpb.SignedExecutionPayloadBid, maxExecutionPayment uint64) primitives.Gwei {
	payment := bid.Message.ExecutionPayment
	if uint64(payment) > maxExecutionPayment {
		payment = primitives.Gwei(maxExecutionPayment)
	}
	sum := bid.Message.Value + payment
	if sum < bid.Message.Value {
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

func p2pExecutionPaymentCap(head state.BeaconState, builderConfig *ethpb.BuilderConfig, bid *ethpb.SignedExecutionPayloadBid) uint64 {
	if bid == nil || bid.Message == nil || builderConfig == nil {
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
	pk, err := head.BuilderPubkey(bid.Message.BuilderIndex)
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

func (vs *Server) getBuilderExecutionPayloadBid(ctx context.Context, head state.BeaconState, q *builderBidQuery) *winningBuilderBid {
	if vs.BlockBuilder == nil || len(q.entries) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, builderBidTimeout)
	defer cancel()
	bids, err := vs.BlockBuilder.GetExecutionPayloadBid(ctx, q.slot, q.parentHash, q.parentRoot, q.pubkey, q.entries)
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
	epoch := slots.ToEpoch(q.slot)
	for _, pb := range bids {
		if pb.Bid == nil || pb.Entry == nil {
			continue
		}
		url := pb.Entry.GetUrl()
		if vs.BuilderCircuitBreaker.Blacklisted(pb.Bid.Message.BuilderIndex, epoch) {
			bidLog = append(bidLog, fmt.Sprintf("%s(builder=%d discarded: blacklisted)", logs.MaskCredentialsLogging(url), pb.Bid.Message.BuilderIndex))
			continue
		}
		if err := vs.validateBuilderBid(head, pb.Bid, q, pb.Entry); err != nil {
			bidLog = append(bidLog, fmt.Sprintf("%s(builder=%d discarded: %v)", logs.MaskCredentialsLogging(url), pb.Bid.Message.BuilderIndex, err))
			continue
		}
		effective := effectiveBidValue(pb.Bid, uint64(pb.Entry.MaxExecutionPayment))
		if effective < pb.Entry.MinBid {
			bidLog = append(bidLog, fmt.Sprintf("%s(builder=%d discarded: effective %d below min bid %d)", logs.MaskCredentialsLogging(url), pb.Bid.Message.BuilderIndex, effective, pb.Entry.MinBid))
			continue
		}
		boosted := boostedBidValue(effective, pb.Entry.BuilderBoostFactor)
		bidLog = append(bidLog, fmt.Sprintf("%s(builder=%d value=%d payment=%d effective=%d boosted=%d)",
			logs.MaskCredentialsLogging(url), pb.Bid.Message.BuilderIndex, pb.Bid.Message.Value, pb.Bid.Message.ExecutionPayment, effective, boosted))
		if best == nil || boosted > bestBoosted {
			best, bestBoosted = &winningBuilderBid{bid: pb.Bid, entry: pb.Entry}, boosted
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

// validateBuilderBid mirrors process_execution_payload_bid so a chosen bid never invalidates the proposer's own block.
func (vs *Server) validateBuilderBid(head state.BeaconState, signed *ethpb.SignedExecutionPayloadBid, q *builderBidQuery, entry *ethpb.BuilderEntry) error {
	if signed == nil || signed.Message == nil {
		return errors.New("nil builder bid")
	}
	bid := signed.Message
	if bid.ExecutionPayment > entry.MaxExecutionPayment {
		return errors.Errorf("bid execution payment %d exceeds max %d", bid.ExecutionPayment, entry.MaxExecutionPayment)
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
		log.WithError(err).Error("Could not submit signed beacon block to builder")
	}
}

// setP2PBidFallback uses a cached P2P bid when the local EL self-build is unavailable.
// The circuit breaker is deliberately not consulted here: with no local payload at all, a block
// carrying a possibly-undelivered bid still beats missing the slot outright.
func (vs *Server) setP2PBidFallback(ctx context.Context, sBlk interfaces.SignedBeaconBlock, head state.BeaconState, parentFull bool) error {
	if vs.HighestBidCache == nil {
		return errors.New("highest bid cache is nil")
	}
	slot := sBlk.Block().Slot()
	parentRoot := sBlk.Block().ParentRoot()
	parentHash, err := vs.getParentBlockHash(ctx, head, slot, parentRoot, parentFull)
	if err != nil {
		return errors.Wrap(err, "could not get parent block hash")
	}
	cached, ok := vs.HighestBidCache.Get(slot, bytesutil.ToBytes32(parentHash), parentRoot)
	if !ok {
		return errors.New("no cached P2P bid available")
	}
	if err := sBlk.SetSignedExecutionPayloadBid(cached); err != nil {
		return errors.Wrap(err, "could not set cached P2P execution payload bid")
	}
	return nil
}

func (vs *Server) cachedP2PBid(sBlk interfaces.SignedBeaconBlock, local *consensusblocks.GetPayloadResponse) *ethpb.SignedExecutionPayloadBid {
	if vs.HighestBidCache == nil {
		return nil
	}
	var parentHash [32]byte
	copy(parentHash[:], local.ExecutionData.ParentHash())
	cached, ok := vs.HighestBidCache.Get(sBlk.Block().Slot(), parentHash, sBlk.Block().ParentRoot())
	if !ok {
		return nil
	}
	// The bid may have been cached before the builder was blacklisted.
	if vs.BuilderCircuitBreaker.Blacklisted(cached.Message.BuilderIndex, slots.ToEpoch(sBlk.Block().Slot())) {
		return nil
	}
	return cached
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
