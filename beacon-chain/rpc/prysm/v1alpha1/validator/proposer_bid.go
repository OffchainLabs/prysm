package validator

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
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
		return "builder-api"
	default:
		return "self-build"
	}
}

// setExecutionPayloadBid picks the best of the local, P2P, and Builder-API bids and
// returns where the winning bid came from.
func (vs *Server) setExecutionPayloadBid(
	ctx context.Context,
	sBlk interfaces.SignedBeaconBlock,
	local *consensusblocks.GetPayloadResponse,
	builderBid *ethpb.SignedExecutionPayloadBid,
	maxExecutionPayment uint64,
	selfBuildOnly bool,
) (bidSource, error) {
	_, span := trace.StartSpan(ctx, "ProposerServer.setExecutionPayloadBid")
	defer span.End()

	if local == nil || local.ExecutionData == nil {
		return bidSourceSelfBuild, errors.New("local execution payload is nil")
	}

	if !selfBuildOnly {
		if chosen, src := vs.winningRemoteBid(sBlk, local, builderBid, maxExecutionPayment); chosen != nil {
			if err := sBlk.SetSignedExecutionPayloadBid(chosen); err != nil {
				return bidSourceSelfBuild, errors.Wrap(err, "could not set remote execution payload bid")
			}
			log.WithFields(logrus.Fields{
				"slot":  sBlk.Block().Slot(),
				"value": uint64(effectiveBuilderBidValue(chosen, maxExecutionPayment)),
			}).Infof("Chose %s execution payload bid", src)
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
		"slot":  sBlk.Block().Slot(),
		"value": uint64(primitives.WeiToGwei(local.Bid)),
	}).Infof("Chose %s execution payload bid", bidSourceSelfBuild)
	return bidSourceSelfBuild, nil
}

// winningRemoteBid returns the highest-value remote bid exceeding the local value, or nil.
func (vs *Server) winningRemoteBid(
	sBlk interfaces.SignedBeaconBlock,
	local *consensusblocks.GetPayloadResponse,
	builderBid *ethpb.SignedExecutionPayloadBid,
	maxExecutionPayment uint64,
) (*ethpb.SignedExecutionPayloadBid, bidSource) {
	var chosen *ethpb.SignedExecutionPayloadBid
	src := bidSourceSelfBuild

	if p2p := vs.winningP2PBid(sBlk, local, false); p2p != nil {
		chosen, src = p2p, bidSourceP2P
	}
	if builderBid != nil {
		builderValue := effectiveBuilderBidValue(builderBid, maxExecutionPayment)
		if builderValue > primitives.WeiToGwei(local.Bid) {
			if chosen == nil || builderValue > chosen.Message.Value {
				chosen, src = builderBid, bidSourceBuilderAPI
			}
		}
	}

	return chosen, src
}

// effectiveBuilderBidValue is the proposer's total take from a builder bid: the bid
// value plus the execution payment capped at the proposer's max preference.
func effectiveBuilderBidValue(bid *ethpb.SignedExecutionPayloadBid, maxExecutionPayment uint64) primitives.Gwei {
	payment := bid.Message.ExecutionPayment
	if uint64(payment) > maxExecutionPayment {
		payment = primitives.Gwei(maxExecutionPayment)
	}
	return bid.Message.Value + payment
}

// getBuilderExecutionPayloadBid queries every builder the proposer signed an auth
// for and returns the highest-value valid bid, the URL it came from, and the
// proposer's max execution payment. Returns a nil bid if none are valid.
func (vs *Server) getBuilderExecutionPayloadBid(
	ctx context.Context,
	sBlk interfaces.SignedBeaconBlock,
	head state.BeaconState,
	local *consensusblocks.GetPayloadResponse,
	auths []*ethpb.SignedRequestAuthV1,
) (*ethpb.SignedExecutionPayloadBid, string, uint64) {
	if vs.BlockBuilder == nil || len(auths) == 0 {
		return nil, "", 0
	}
	val, err := head.ValidatorAtIndexReadOnly(sBlk.Block().ProposerIndex())
	if err != nil {
		log.WithError(err).Error("Could not get proposer for builder bid request")
		return nil, "", 0
	}
	pubkey := val.PublicKey()
	parentHash := bytesutil.ToBytes32(local.ExecutionData.ParentHash())
	parentRoot := sBlk.Block().ParentRoot()
	ctx, cancel := context.WithTimeout(ctx, builderBidTimeout)
	defer cancel()
	slot := sBlk.Block().Slot()
	bids, err := vs.BlockBuilder.GetExecutionPayloadBid(ctx, slot, parentHash, parentRoot, pubkey, auths)
	if err != nil {
		builderGetPayloadMissCount.Inc()
		log.WithError(err).Error("Could not get builder execution payload bid")
		return nil, "", 0
	}
	var maxPayment uint64
	if v, ok := vs.maxExecutionPayments.Load(pubkey); ok {
		maxPayment, _ = v.(uint64)
	}

	var (
		best      *ethpb.SignedExecutionPayloadBid
		bestURL   string
		bestValue primitives.Gwei
	)
	bidLog := make([]string, 0, len(bids))
	for _, pb := range bids {
		if pb.Bid == nil {
			continue
		}
		if err := validateBuilderBid(sBlk, pb.Bid, parentHash, maxPayment); err != nil {
			bidLog = append(bidLog, fmt.Sprintf("%s(builder=%d discarded: %v)", pb.BuilderURL, pb.Bid.Message.BuilderIndex, err))
			continue
		}
		if err := validateBuilderCanCoverBid(head, pb.Bid); err != nil {
			bidLog = append(bidLog, fmt.Sprintf("%s(builder=%d discarded: %v)", pb.BuilderURL, pb.Bid.Message.BuilderIndex, err))
			continue
		}
		value := effectiveBuilderBidValue(pb.Bid, maxPayment)
		bidLog = append(bidLog, fmt.Sprintf("%s(builder=%d value=%d payment=%d effective=%d)",
			pb.BuilderURL, pb.Bid.Message.BuilderIndex, pb.Bid.Message.Value, pb.Bid.Message.ExecutionPayment, value))
		if best == nil || value > bestValue {
			best, bestURL, bestValue = pb.Bid, pb.BuilderURL, value
		}
	}

	selfBuild := primitives.WeiToGwei(local.Bid)
	bestBuilder := "none"
	if best != nil {
		bestBuilder = bestURL
	}
	log.WithFields(logrus.Fields{
		"slot":            slot,
		"selfBuildGwei":   uint64(selfBuild),
		"bestBuilder":     bestBuilder,
		"bestBuilderGwei": uint64(bestValue),
	}).Infof("Received bids: self-build=%d gwei | builders=[%s]", uint64(selfBuild), strings.Join(bidLog, " | "))

	if best == nil {
		builderGetPayloadMissCount.Inc()
		return nil, "", 0
	}
	return best, bestURL, maxPayment
}

// validateBuilderCanCoverBid mirrors the state-transition checks (active builder
// and sufficient balance) so the proposer never commits to a bid that would fail
// processExecutionPayloadBid and cause the whole proposal to be dropped.
func validateBuilderCanCoverBid(head state.BeaconState, signed *ethpb.SignedExecutionPayloadBid) error {
	builderIndex := signed.Message.BuilderIndex
	active, err := head.IsActiveBuilder(builderIndex)
	if err != nil {
		return errors.Wrap(err, "builder active check failed")
	}
	if !active {
		return errors.Errorf("builder %d is not active", builderIndex)
	}
	ok, err := head.CanBuilderCoverBid(builderIndex, primitives.Gwei(signed.Message.Value))
	if err != nil {
		return errors.Wrap(err, "builder balance check failed")
	}
	if !ok {
		return errors.Errorf("builder %d cannot cover bid amount %d", builderIndex, signed.Message.Value)
	}
	return nil
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

// recordBidSource remembers where the winning bid for slot came from, and for a
// Builder-API win, which builder URL it came from so the block can be revealed to it.
func (vs *Server) recordBidSource(slot primitives.Slot, src bidSource, builderURL string) {
	vs.lastBidLock.Lock()
	defer vs.lastBidLock.Unlock()
	vs.lastBidSlot, vs.lastBidSource, vs.lastBidBuilderURL = slot, src, builderURL
}

// bidSourceForSlot returns the recorded bid source and winning builder URL for slot,
// or self-build if the record is for another slot.
func (vs *Server) bidSourceForSlot(slot primitives.Slot) (bidSource, string) {
	vs.lastBidLock.Lock()
	defer vs.lastBidLock.Unlock()
	if vs.lastBidSlot != slot {
		return bidSourceSelfBuild, ""
	}
	return vs.lastBidSource, vs.lastBidBuilderURL
}

// submitBlockToBuilder sends the signed block to the winning builder so it can reveal the envelope.
// Best-effort and detached from the propose RPC; the builder also learns of the block via P2P.
func (vs *Server) submitBlockToBuilder(block interfaces.ReadOnlySignedBeaconBlock, builderURL string) {
	if vs.BlockBuilder == nil || builderURL == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(params.BeaconConfig().SecondsPerSlot)*time.Second)
	defer cancel()
	if err := vs.BlockBuilder.SubmitSignedBeaconBlock(ctx, builderURL, block); err != nil {
		log.WithError(err).Error("Could not submit signed beacon block to builder")
	}
}

// setP2PBidFallback uses a cached P2P bid when the local EL self-build is unavailable.
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

// winningP2PBid returns a cached P2P bid if one exists and exceeds the local EL value.
func (vs *Server) winningP2PBid(
	sBlk interfaces.SignedBeaconBlock,
	local *consensusblocks.GetPayloadResponse,
	selfBuildOnly bool,
) *ethpb.SignedExecutionPayloadBid {
	if selfBuildOnly || vs.HighestBidCache == nil {
		return nil
	}

	ed := local.ExecutionData
	var parentHash [32]byte
	copy(parentHash[:], ed.ParentHash())
	cached, ok := vs.HighestBidCache.Get(sBlk.Block().Slot(), parentHash, sBlk.Block().ParentRoot())
	if !ok {
		return nil
	}

	if cached.Message.Value <= primitives.WeiToGwei(local.Bid) {
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
