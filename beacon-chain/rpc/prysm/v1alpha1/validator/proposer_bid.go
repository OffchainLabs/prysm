package validator

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// builderBidTimeout bounds the builder bid request so a slow builder cannot eat the slot.
const builderBidTimeout = 500 * time.Millisecond

// bidSource indicates where the winning execution payload bid came from.
type bidSource int

const (
	bidSourceSelfBuild  bidSource = iota // local self-build; caller caches the envelope
	bidSourceP2P                         // P2P gossip bid; the builder reveals the envelope
	bidSourceBuilderAPI                  // Builder-API bid; caller submits the signed block to the builder
)

func (s bidSource) String() string {
	switch s {
	case bidSourceP2P:
		return "p2p"
	case bidSourceBuilderAPI:
		return "builderAPI"
	default:
		return "selfBuild"
	}
}

// setExecutionPayloadBid picks the best of the local, P2P, and Builder-API bids and
// returns where the winning bid came from.
func (vs *Server) setExecutionPayloadBid(
	ctx context.Context,
	sBlk interfaces.SignedBeaconBlock,
	local *consensusblocks.GetPayloadResponse,
	builderBid *ethpb.SignedExecutionPayloadBid,
	selfBuildOnly bool,
) (bidSource, error) {
	_, span := trace.StartSpan(ctx, "ProposerServer.setExecutionPayloadBid")
	defer span.End()

	if local == nil || local.ExecutionData == nil {
		return bidSourceSelfBuild, errors.New("local execution payload is nil")
	}

	if !selfBuildOnly {
		if chosen, src := vs.winningRemoteBid(sBlk, local, builderBid); chosen != nil {
			if err := sBlk.SetSignedExecutionPayloadBid(chosen); err != nil {
				return bidSourceSelfBuild, errors.Wrap(err, "could not set remote execution payload bid")
			}
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

	return bidSourceSelfBuild, nil
}

// winningRemoteBid returns the highest-value remote bid exceeding the local value, or nil.
// builderBid is already validated against the proposer's payment cap, so its full
// execution payment counts toward the comparison.
func (vs *Server) winningRemoteBid(
	sBlk interfaces.SignedBeaconBlock,
	local *consensusblocks.GetPayloadResponse,
	builderBid *ethpb.SignedExecutionPayloadBid,
) (*ethpb.SignedExecutionPayloadBid, bidSource) {
	localValue := primitives.WeiToGwei(local.Bid)

	var chosen *ethpb.SignedExecutionPayloadBid
	src := bidSourceSelfBuild
	var p2pValue primitives.Gwei
	if p2p := vs.winningP2PBid(sBlk, local, localValue); p2p != nil {
		chosen, src, p2pValue = p2p, bidSourceP2P, p2p.Message.Value
	}

	var builderValue primitives.Gwei
	if builderBid != nil {
		builderValue = builderBid.Message.Value + builderBid.Message.ExecutionPayment
		if builderValue > localValue && (chosen == nil || builderValue > p2pValue) {
			chosen, src = builderBid, bidSourceBuilderAPI
		}
	}

	fields := logrus.Fields{
		"slot":          sBlk.Block().Slot(),
		"localValue":    localValue,
		"chosen":        src,
		"hasP2PBid":     chosen != nil && src == bidSourceP2P,
		"hasBuilderBid": builderBid != nil,
	}
	if builderBid != nil {
		fields["builderValue"] = builderValue
	}
	if chosen != nil {
		fields["chosenValue"] = chosen.Message.Value
	}
	log.WithFields(fields).Debug("Selected execution payload bid source")

	return chosen, src
}

// getBuilderExecutionPayloadBid requests a bid from the configured builder, returning nil if none or invalid.
func (vs *Server) getBuilderExecutionPayloadBid(
	ctx context.Context,
	sBlk interfaces.SignedBeaconBlock,
	head state.BeaconState,
	local *consensusblocks.GetPayloadResponse,
	auths []*ethpb.SignedRequestAuthV1,
) *ethpb.SignedExecutionPayloadBid {
	if vs.BlockBuilder == nil || !vs.BlockBuilder.Configured() {
		return nil
	}
	val, err := head.ValidatorAtIndexReadOnly(sBlk.Block().ProposerIndex())
	if err != nil {
		log.WithError(err).Error("Could not get proposer for builder bid request")
		return nil
	}
	pubkey := val.PublicKey()
	parentHash := bytesutil.ToBytes32(local.ExecutionData.ParentHash())
	parentRoot := sBlk.Block().ParentRoot()
	ctx, cancel := context.WithTimeout(ctx, builderBidTimeout)
	defer cancel()
	slot := sBlk.Block().Slot()
	log.WithFields(logrus.Fields{
		"pubkey":   fmt.Sprintf("%#x", pubkey),
		"slot":     slot,
		"numAuths": len(auths),
	}).Debug("Requesting builder execution payload bid")
	bid, err := vs.BlockBuilder.GetExecutionPayloadBid(ctx, slot, parentHash, parentRoot, pubkey, auths)
	if err != nil {
		builderGetPayloadMissCount.Inc()
		log.WithError(err).Error("Could not get builder execution payload bid")
		return nil
	}
	if bid == nil {
		return nil
	}
	var maxPayment uint64
	if v, ok := vs.maxExecutionPayments.Load(pubkey); ok {
		maxPayment, _ = v.(uint64)
	}
	if err := validateBuilderBid(sBlk, bid, parentHash, maxPayment); err != nil {
		log.WithError(err).Warn("Discarding invalid builder execution payload bid")
		return nil
	}
	// Reuse the gossip bid verifier for the active-builder and can-cover-bid spec
	// checks so the proposer never commits to a bid that would fail
	// processExecutionPayloadBid and drop the whole proposal.
	if err := vs.validateBuilderCanCoverBid(head, bid); err != nil {
		log.WithError(err).Warn("Discarding builder execution payload bid the builder cannot back")
		return nil
	}
	return bid
}

// validateBuilderCanCoverBid runs the verifier's active-builder and can-cover-bid
// checks against head, the same checks gossip bid validation applies.
func (vs *Server) validateBuilderCanCoverBid(head state.BeaconState, signed *ethpb.SignedExecutionPayloadBid) error {
	roBid, err := consensusblocks.WrappedROSignedExecutionPayloadBid(signed)
	if err != nil {
		return errors.Wrap(err, "could not wrap builder bid")
	}
	verifier, err := vs.bidVerifierFor(roBid)
	if err != nil {
		return err
	}
	if err := verifier.VerifyBuilderActive(head); err != nil {
		return err
	}
	return verifier.VerifyBuilderCanCoverBid(head)
}

// bidVerifierFor lazily resolves the execution payload bid verifier constructor
// from the post-genesis initializer (cached after the first resolve) and returns
// a verifier for the given bid.
func (vs *Server) bidVerifierFor(b interfaces.ROSignedExecutionPayloadBid) (verification.ExecutionPayloadBidVerifier, error) {
	vs.bidVerifierLock.Lock()
	if vs.bidVerifier == nil {
		if vs.BidVerifierWaiter == nil {
			vs.bidVerifierLock.Unlock()
			return nil, errors.New("execution payload bid verifier unavailable")
		}
		ini, err := vs.BidVerifierWaiter.WaitForInitializer(vs.Ctx)
		if err != nil {
			vs.bidVerifierLock.Unlock()
			return nil, errors.Wrap(err, "could not initialize execution payload bid verifier")
		}
		vs.bidVerifier = func(b interfaces.ROSignedExecutionPayloadBid, reqs []verification.Requirement) verification.ExecutionPayloadBidVerifier {
			return ini.NewExecutionPayloadBidVerifier(b, reqs)
		}
	}
	ctor := vs.bidVerifier
	vs.bidVerifierLock.Unlock()
	return ctor(b, []verification.Requirement{verification.RequireBidBuilderActive, verification.RequireBidBuilderCanCover}), nil
}

// validateBuilderBid pre-filters a Builder-API bid on slot, parent linkage, and payment cap.
func validateBuilderBid(sBlk interfaces.SignedBeaconBlock, signed *ethpb.SignedExecutionPayloadBid, parentHash [32]byte, maxExecutionPayment uint64) error {
	if signed == nil || signed.Message == nil {
		return errors.New("nil builder bid")
	}
	bid := signed.Message
	if bid.Slot != sBlk.Block().Slot() {
		return errors.Errorf("bid slot %d does not match block slot %d", bid.Slot, sBlk.Block().Slot())
	}
	parentRoot := sBlk.Block().ParentRoot()
	if !bytes.Equal(bid.ParentBlockRoot, parentRoot[:]) {
		return errors.New("bid parent block root does not match block parent root")
	}
	if !bytes.Equal(bid.ParentBlockHash, parentHash[:]) {
		return errors.New("bid parent block hash does not match expected parent hash")
	}
	if uint64(bid.ExecutionPayment) > maxExecutionPayment {
		return errors.Errorf("bid execution payment %d exceeds max %d", bid.ExecutionPayment, maxExecutionPayment)
	}
	return nil
}

// recordBidSource remembers where the winning bid for slot came from.
func (vs *Server) recordBidSource(slot primitives.Slot, src bidSource) {
	vs.lastBidLock.Lock()
	defer vs.lastBidLock.Unlock()
	vs.lastBidSlot, vs.lastBidSource = slot, src
}

// bidSourceForSlot returns the recorded bid source for slot, or self-build if the record is for another slot.
func (vs *Server) bidSourceForSlot(slot primitives.Slot) bidSource {
	vs.lastBidLock.Lock()
	defer vs.lastBidLock.Unlock()
	if vs.lastBidSlot != slot {
		return bidSourceSelfBuild
	}
	return vs.lastBidSource
}

// submitBlockToBuilder sends the signed block to the builder so it can reveal the envelope.
// Best-effort and detached from the propose RPC; the builder also learns of the block via P2P.
func (vs *Server) submitBlockToBuilder(block interfaces.ReadOnlySignedBeaconBlock) {
	if vs.BlockBuilder == nil || !vs.BlockBuilder.Configured() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(params.BeaconConfig().SecondsPerSlot)*time.Second)
	defer cancel()
	if err := vs.BlockBuilder.SubmitSignedBeaconBlock(ctx, block); err != nil {
		log.WithError(err).Error("Could not submit signed beacon block to builder")
	}
}

// winningP2PBid returns a cached P2P bid if one exists and exceeds the local EL value.
func (vs *Server) winningP2PBid(
	sBlk interfaces.SignedBeaconBlock,
	local *consensusblocks.GetPayloadResponse,
	localValueGwei primitives.Gwei,
) *ethpb.SignedExecutionPayloadBid {
	if vs.HighestBidCache == nil {
		return nil
	}

	ed := local.ExecutionData
	var parentHash [32]byte
	copy(parentHash[:], ed.ParentHash())
	cached, ok := vs.HighestBidCache.Get(sBlk.Block().Slot(), parentHash, sBlk.Block().ParentRoot())
	if !ok {
		return nil
	}

	builderValueGwei := cached.Message.Value
	if builderValueGwei <= localValueGwei {
		log.WithFields(logrus.Fields{
			"slot":             sBlk.Block().Slot(),
			"builderValueGwei": builderValueGwei,
			"localValueGwei":   localValueGwei,
		}).Info("Local EL value exceeds P2P bid, using self-build")
		return nil
	}

	log.WithFields(logrus.Fields{
		"slot":             sBlk.Block().Slot(),
		"builderIndex":     cached.Message.BuilderIndex,
		"builderValueGwei": builderValueGwei,
		"localValueGwei":   localValueGwei,
	}).Info("Using P2P execution payload bid over self-build")
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
	executionRequestsRoot, err := local.ExecutionRequests.HashTreeRoot()
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
