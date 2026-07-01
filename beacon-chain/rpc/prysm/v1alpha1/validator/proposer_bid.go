package validator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
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

// builderBidTimeout times out the builder bid request
const builderBidTimeout = 300 * time.Millisecond

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
		p2pBid := vs.bestP2PBid(sBlk, local, false)
		if bestBid, src := bestBid(local, p2pBid, builderBid, maxExecutionPayment); bestBid != nil {
			if err := sBlk.SetSignedExecutionPayloadBid(bestBid); err != nil {
				return bidSourceSelfBuild, errors.Wrap(err, "could not set remote execution payload bid")
			}
			log.WithFields(logrus.Fields{
				"slot":  sBlk.Block().Slot(),
				"value": uint64(effectiveBidValue(bestBid, maxExecutionPayment)),
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

// bestBid returns the highest-value bid among the local self-build, P2P, and Builder-API
// candidates, with its source. Returns nil when the local self-build wins.
func bestBid(
	local *consensusblocks.GetPayloadResponse,
	p2pBid *ethpb.SignedExecutionPayloadBid,
	builderBid *ethpb.SignedExecutionPayloadBid,
	maxExecutionPayment uint64,
) (*ethpb.SignedExecutionPayloadBid, bidSource) {
	var bestBid *ethpb.SignedExecutionPayloadBid
	bestValue := primitives.WeiToGwei(local.Bid)
	src := bidSourceSelfBuild

	consider := func(bid *ethpb.SignedExecutionPayloadBid, from bidSource) {
		if bid == nil {
			return
		}
		if value := effectiveBidValue(bid, maxExecutionPayment); value > bestValue {
			bestBid, bestValue, src = bid, value, from
		}
	}

	consider(p2pBid, bidSourceP2P)
	consider(builderBid, bidSourceBuilderAPI)

	return bestBid, src
}

// effectiveBidValue is the proposer's total take from a bid: the bid value plus the
// execution payment capped at the proposer's max preference (zero for P2P bids).
func effectiveBidValue(bid *ethpb.SignedExecutionPayloadBid, maxExecutionPayment uint64) primitives.Gwei {
	payment := bid.Message.ExecutionPayment
	if uint64(payment) > maxExecutionPayment {
		payment = primitives.Gwei(maxExecutionPayment)
	}
	return bid.Message.Value + payment
}

// getBuilderExecutionPayloadBid queries every builder the proposer signed an auth
// for and returns the highest-value valid bid and the URL it came from, or a nil
// bid if none are valid.
func (vs *Server) getBuilderExecutionPayloadBid(
	ctx context.Context,
	head state.BeaconState,
	slot primitives.Slot,
	parentRoot [32]byte,
	parentHash [32]byte,
	pubkey [fieldparams.BLSPubkeyLength]byte,
	maxPayment uint64,
	auths []*ethpb.SignedRequestAuthV1,
) (*ethpb.SignedExecutionPayloadBid, string) {
	if vs.BlockBuilder == nil || len(auths) == 0 {
		return nil, ""
	}
	ctx, cancel := context.WithTimeout(ctx, builderBidTimeout)
	defer cancel()
	bids, err := vs.BlockBuilder.GetExecutionPayloadBid(ctx, slot, parentHash, parentRoot, pubkey, auths)
	if err != nil {
		builderGetPayloadMissCount.Inc()
		log.WithError(err).Error("Could not get builder execution payload bid")
		return nil, ""
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
		if err := vs.validateBuilderBid(head, pb.Bid, slot, parentRoot, parentHash, maxPayment); err != nil {
			bidLog = append(bidLog, fmt.Sprintf("%s(builder=%d discarded: %v)", pb.BuilderURL, pb.Bid.Message.BuilderIndex, err))
			continue
		}
		value := effectiveBidValue(pb.Bid, maxPayment)
		bidLog = append(bidLog, fmt.Sprintf("%s(builder=%d value=%d payment=%d effective=%d)",
			pb.BuilderURL, pb.Bid.Message.BuilderIndex, pb.Bid.Message.Value, pb.Bid.Message.ExecutionPayment, value))
		if best == nil || value > bestValue {
			best, bestURL, bestValue = pb.Bid, pb.BuilderURL, value
		}
	}

	log.WithFields(logrus.Fields{
		"slot":            slot,
		"bestBuilder":     bestURL,
		"bestBuilderGwei": uint64(bestValue),
	}).Infof("Received builder bids: [%s]", strings.Join(bidLog, " | "))

	if best == nil {
		builderGetPayloadMissCount.Inc()
		return nil, ""
	}
	return best, bestURL
}

// validateBuilderBid mirrors process_execution_payload_bid so a chosen bid never invalidates the proposer's own block.
func (vs *Server) validateBuilderBid(head state.BeaconState, signed *ethpb.SignedExecutionPayloadBid, slot primitives.Slot, parentRoot [32]byte, parentHash [32]byte, maxExecutionPayment uint64) error {
	if signed == nil || signed.Message == nil {
		return errors.New("nil builder bid")
	}
	bid := signed.Message
	if uint64(bid.ExecutionPayment) > maxExecutionPayment {
		return errors.Errorf("bid execution payment %d exceeds max %d", bid.ExecutionPayment, maxExecutionPayment)
	}

	if vs.NewExecutionPayloadBidVerifier == nil {
		return errors.New("bid verifier not ready")
	}
	ro, err := consensusblocks.WrappedROSignedExecutionPayloadBid(signed)
	if err != nil {
		return errors.Wrap(err, "could not wrap builder bid")
	}
	v := vs.NewExecutionPayloadBidVerifier(ro, verification.BuilderAPIBidRequirements)
	if err := v.VerifyBidSlotMatches(slot); err != nil {
		return err
	}
	if err := v.VerifyParentBlockRootSeen(func(root [32]byte) bool { return root == parentRoot }); err != nil {
		return err
	}
	if err := v.VerifyParentBlockHash(func([32]byte) ([32]byte, error) { return parentHash, nil }); err != nil {
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
	if err := v.VerifyBlobKzgCommitmentsLimit(); err != nil {
		return err
	}
	if err := v.VerifyPrevRandao(head); err != nil {
		return err
	}
	return v.VerifySignature(head)
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

// bestP2PBid returns a cached P2P bid if one exists and exceeds the local EL value.
func (vs *Server) bestP2PBid(
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
