package validator

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/io/logs"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
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
	local *consensusblocks.GetPayloadResponse,
	builderBid *ethpb.SignedExecutionPayloadBid,
	builderEff primitives.Gwei,
	selfBuildOnly bool,
) (bidSource, error) {
	_, span := trace.StartSpan(ctx, "ProposerServer.setExecutionPayloadBid")
	defer span.End()

	if local == nil || local.ExecutionData == nil {
		return bidSourceSelfBuild, errors.New("local execution payload is nil")
	}

	if !selfBuildOnly {
		p2pBid := vs.cachedP2PBid(sBlk, local)
		if winner, winnerValue, src := bestBid(local, p2pBid, builderBid, builderEff); winner != nil {
			if err := sBlk.SetSignedExecutionPayloadBid(winner); err != nil {
				return bidSourceSelfBuild, errors.Wrap(err, "could not set remote execution payload bid")
			}
			log.WithFields(logrus.Fields{
				"slot":      sBlk.Block().Slot(),
				"source":    src,
				"builder":   winner.Message.BuilderIndex,
				"valueGwei": uint64(winnerValue),
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

// The 100% factor. Callers default a missing preference to this so an explicit
// 0 keeps its reserved prefer-local meaning.
const neutralBuilderBoostFactor = 100

// bidPreferences carries the proposer's per-key builder bid-selection preferences.
type bidPreferences struct {
	maxPayment  uint64
	minBid      uint64
	boostFactor uint64
	pubkey      []byte
}

// Returns a nil bid when the local self-build wins. The builder-API bid arrives
// already gated under its own entry's preferences; the P2P bid is outside entry
// governance and competes on raw value.
func bestBid(
	local *consensusblocks.GetPayloadResponse,
	p2pBid *ethpb.SignedExecutionPayloadBid,
	builderBid *ethpb.SignedExecutionPayloadBid,
	builderEff primitives.Gwei,
) (*ethpb.SignedExecutionPayloadBid, primitives.Gwei, bidSource) {
	var winner *ethpb.SignedExecutionPayloadBid
	var winnerValue primitives.Gwei
	localValue := primitives.WeiToGwei(local.Bid)
	src := bidSourceSelfBuild

	if p2pBid != nil {
		if v := p2pBid.Message.Value; v > localValue {
			winner, winnerValue, src = p2pBid, v, bidSourceP2P
		}
	}
	if builderBid != nil && (winner == nil || builderEff > winnerValue) {
		winner, winnerValue, src = builderBid, builderEff, bidSourceBuilderAPI
	}
	return winner, winnerValue, src
}

// builderBeatsLocal reports whether a boosted non-local bid beats the local bid.
// Reserved factors: 0 always prefers local, max always prefers the builder.
func builderBeatsLocal(builderValue, localValue primitives.Gwei, boostFactor uint64) bool {
	switch boostFactor {
	case 0:
		return false
	case math.MaxUint64:
		return true
	}
	lhs := new(big.Int).Mul(new(big.Int).SetUint64(uint64(builderValue)), new(big.Int).SetUint64(boostFactor))
	rhs := new(big.Int).Mul(new(big.Int).SetUint64(uint64(localValue)), big.NewInt(100))
	return lhs.Cmp(rhs) > 0
}

func validBuilderURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// safeURLForLog never echoes an unparseable value: MaskCredentialsLogging falls
// back to the raw input on parse failure, which could leak embedded credentials.
func safeURLForLog(raw string) string {
	if _, err := url.Parse(raw); err != nil {
		return "<unparseable>"
	}
	return logs.MaskCredentialsLogging(raw)
}

// The proposer's total take, the execution payment counts only up to the proposer's max preference.
// Saturates instead of wrapping: a malicious bid must not rank below honest ones by overflowing.
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

// builderBidQuery carries the proposal context builder bids are requested and validated against.
type builderBidQuery struct {
	slot           primitives.Slot
	parentRoot     [32]byte
	parentHash     [32]byte
	pubkey         [fieldparams.BLSPubkeyLength]byte
	localValue     primitives.Gwei
	feeRecipient   []byte
	parentGasLimit uint64
	targetGasLimit uint64
	prefsByURL     map[string]builderPref
}

// builderPref pairs one builder's signed auth with its own bid preferences.
type builderPref struct {
	auth  *ethpb.SignedRequestAuthV1
	prefs bidPreferences
}

// builderBidQualifies applies one entry's floor and boost gate to its bid.
func builderBidQualifies(eff, localValue primitives.Gwei, p bidPreferences) bool {
	return uint64(eff) >= p.minBid && builderBeatsLocal(eff, localValue, p.boostFactor)
}

func (vs *Server) getBuilderExecutionPayloadBid(ctx context.Context, head state.BeaconState, q *builderBidQuery) (*ethpb.SignedExecutionPayloadBid, string, primitives.Gwei) {
	if vs.BlockBuilder == nil || len(q.prefsByURL) == 0 {
		return nil, "", 0
	}
	authsByURL := make(map[string]*ethpb.SignedRequestAuthV1, len(q.prefsByURL))
	for u, p := range q.prefsByURL {
		authsByURL[u] = p.auth
	}
	ctx, cancel := context.WithTimeout(ctx, builderBidTimeout)
	defer cancel()
	bids, err := vs.BlockBuilder.GetExecutionPayloadBid(ctx, q.slot, q.parentHash, q.parentRoot, q.pubkey, authsByURL)
	if err != nil {
		builderGetPayloadMissCount.Inc()
		log.WithError(err).Error("Could not get builder execution payload bid")
		return nil, "", 0
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
		// Each bid is judged under its own entry's preferences.
		pref, ok := q.prefsByURL[pb.BuilderURL]
		if !ok {
			continue
		}
		if err := vs.validateBuilderBid(head, pb.Bid, q, pref.prefs); err != nil {
			bidLog = append(bidLog, fmt.Sprintf("%s(builder=%d discarded: %v)", logs.MaskCredentialsLogging(pb.BuilderURL), pb.Bid.Message.BuilderIndex, err))
			continue
		}
		value := effectiveBidValue(pb.Bid, pref.prefs.maxPayment)
		if !builderBidQualifies(value, q.localValue, pref.prefs) {
			bidLog = append(bidLog, fmt.Sprintf("%s(builder=%d below floor or boost gate: effective=%d)",
				logs.MaskCredentialsLogging(pb.BuilderURL), pb.Bid.Message.BuilderIndex, value))
			continue
		}
		bidLog = append(bidLog, fmt.Sprintf("%s(builder=%d value=%d payment=%d effective=%d)",
			logs.MaskCredentialsLogging(pb.BuilderURL), pb.Bid.Message.BuilderIndex, pb.Bid.Message.Value, pb.Bid.Message.ExecutionPayment, value))
		if best == nil || value > bestValue {
			best, bestURL, bestValue = pb.Bid, pb.BuilderURL, value
		}
	}

	if len(bidLog) > 0 {
		log.WithField("slot", q.slot).Debugf("Builder bids: [%s]", strings.Join(bidLog, " | "))
	}

	if best == nil {
		builderGetPayloadMissCount.Inc()
		return nil, "", 0
	}
	return best, bestURL, bestValue
}

// validateBuilderBid mirrors process_execution_payload_bid so a chosen bid never invalidates the proposer's own block.
func (vs *Server) validateBuilderBid(head state.BeaconState, signed *ethpb.SignedExecutionPayloadBid, q *builderBidQuery, prefs bidPreferences) error {
	if signed == nil || signed.Message == nil {
		return errors.New("nil builder bid")
	}
	bid := signed.Message
	if uint64(bid.ExecutionPayment) > prefs.maxPayment {
		return errors.Errorf("bid execution payment %d exceeds max %d", bid.ExecutionPayment, prefs.maxPayment)
	}
	if len(prefs.pubkey) > 0 {
		builderPubkey, err := head.BuilderPubkey(bid.BuilderIndex)
		if err != nil {
			return errors.Wrap(err, "could not get builder pubkey")
		}
		if !bytes.Equal(builderPubkey[:], prefs.pubkey) {
			return errors.New("bid is not signed by the configured builder pubkey")
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
	if err := v.VerifyParentBlockHash(func([32]byte) ([32]byte, error) { return q.parentHash, nil }); err != nil {
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
