package sync

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/methodical-ssz/ssz"
	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

func TestValidateExecutionPayloadBidGossip_InvalidTopic(t *testing.T) {
	ctx := context.Background()
	p := p2ptest.NewTestP2P(t)
	s := &Service{cfg: &config{p2p: p, initialSync: &mockSync.Sync{}}}

	result, err := s.validateExecutionPayloadBidGossip(ctx, "", &pubsub.Message{Message: &pb.Message{}})
	require.ErrorIs(t, p2p.ErrInvalidTopic, err)
	require.Equal(t, pubsub.ValidationReject, result)
}

func TestValidateExecutionPayloadBidGossip_BlockHashEqualsParent(t *testing.T) {
	ctx := context.Background()
	s, _, signedBid := setupExecutionPayloadBidService(t)
	s.newExecutionPayloadBidVerifier = testNewExecutionPayloadBidVerifier(mockExecutionPayloadBidVerifier{})

	signedBid.Message.BlockHash = signedBid.Message.ParentBlockHash
	msg := executionPayloadBidToPubsub(t, s, s.cfg.p2p, signedBid)
	result, err := s.validateExecutionPayloadBidGossip(ctx, "", msg)
	require.ErrorContains(t, "bid block hash equals parent block hash", err)
	require.Equal(t, pubsub.ValidationReject, result)

	// The rejected bid must not be cached as seen.
	require.Equal(t, false, s.hasSeenExecutionPayloadBid(executionPayloadBidTupleKey(mustBid(t, signedBid))))
}

func TestValidateExecutionPayloadBidGossip_AlreadySeenTuple(t *testing.T) {
	ctx := context.Background()
	s, msg, signedBid := setupExecutionPayloadBidService(t)
	s.newExecutionPayloadBidVerifier = testNewExecutionPayloadBidVerifier(mockExecutionPayloadBidVerifier{})

	key := executionPayloadBidTupleKey(mustBid(t, signedBid))
	s.setSeenExecutionPayloadBid(signedBid.Message.Slot, key)
	result, err := s.validateExecutionPayloadBidGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)
}

func TestValidateExecutionPayloadBidGossip_SameBuilderDifferentParentAccepted(t *testing.T) {
	ctx := context.Background()
	s, msg, signedBid := setupExecutionPayloadBidService(t)
	s.newExecutionPayloadBidVerifier = testNewExecutionPayloadBidVerifier(mockExecutionPayloadBidVerifier{})

	result, err := s.validateExecutionPayloadBidGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, result)

	other := proto.Clone(signedBid).(*ethpb.SignedExecutionPayloadBid)
	other.Message.ParentBlockHash = bytesutil.PadTo([]byte{0x09}, 32)
	msg = executionPayloadBidToPubsub(t, s, s.cfg.p2p, other)
	result, err = s.validateExecutionPayloadBidGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, result)
}

// Dedup must short-circuit before every later check; duplicates pay only the cache lookup.
func TestValidateExecutionPayloadBidGossip_DedupShortCircuitsAllLaterChecks(t *testing.T) {
	ctx := context.Background()
	s, msg, signedBid := setupExecutionPayloadBidService(t)
	key := executionPayloadBidTupleKey(mustBid(t, signedBid))
	s.setSeenExecutionPayloadBid(signedBid.Message.Slot, key)
	// Every subsequent verifier method would Reject/Ignore if it ran; the cache hit must skip them all.
	s.newExecutionPayloadBidVerifier = testNewExecutionPayloadBidVerifier(mockExecutionPayloadBidVerifier{
		errCurrentOrNextSlot:    errors.New("slot"),
		errBuilderActive:        errors.New("builder"),
		errExecutionPayment:     errors.New("payment"),
		errFeeRecipientMismatch: errors.New("fee"),
		errSignature:            errors.New("sig"),
	})

	result, err := s.validateExecutionPayloadBidGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)
}

func TestValidateExecutionPayloadBidGossip_ProposerPreferencesUnseen(t *testing.T) {
	ctx := context.Background()
	s, msg, _ := setupExecutionPayloadBidService(t)
	s.proposerPreferencesCache.Clear()
	s.newExecutionPayloadBidVerifier = testNewExecutionPayloadBidVerifier(mockExecutionPayloadBidVerifier{})

	result, err := s.validateExecutionPayloadBidGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)
}

func TestValidateExecutionPayloadBidGossip_InitialSync(t *testing.T) {
	ctx := context.Background()
	p := p2ptest.NewTestP2P(t)
	s := &Service{
		cfg: &config{
			p2p:         p,
			initialSync: &mockSync.Sync{IsSyncing: true},
		},
	}

	result, err := s.validateExecutionPayloadBidGossip(ctx, "", &pubsub.Message{})
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)
}

func TestValidateExecutionPayloadBidGossip_ErrorPathsWithMock(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		verifier  mockExecutionPayloadBidVerifier
		result    pubsub.ValidationResult
		wantError bool
	}{
		{
			name:      "slot out of range",
			verifier:  mockExecutionPayloadBidVerifier{errCurrentOrNextSlot: errors.New("wrong slot")},
			result:    pubsub.ValidationIgnore,
			wantError: true,
		},
		{
			name:      "non-zero execution payment",
			verifier:  mockExecutionPayloadBidVerifier{errExecutionPayment: errors.New("non-zero payment")},
			result:    pubsub.ValidationReject,
			wantError: true,
		},
		{
			name:      "fee recipient mismatch",
			verifier:  mockExecutionPayloadBidVerifier{errFeeRecipientMismatch: errors.New("wrong fee recipient")},
			result:    pubsub.ValidationIgnore,
			wantError: true,
		},
		{
			name:      "gas limit incompatible",
			verifier:  mockExecutionPayloadBidVerifier{errGasLimitIncompatible: errors.New("incompatible gas limit")},
			result:    pubsub.ValidationIgnore,
			wantError: true,
		},
		{
			name:      "bid not compatible with head",
			verifier:  mockExecutionPayloadBidVerifier{errBidCompatibleWithHead: errors.New("incompatible branch")},
			result:    pubsub.ValidationIgnore,
			wantError: true,
		},
		{
			name:      "inactive builder",
			verifier:  mockExecutionPayloadBidVerifier{errBuilderActive: errors.New("inactive builder")},
			result:    pubsub.ValidationReject,
			wantError: true,
		},
		{
			name:      "builder wrong version",
			verifier:  mockExecutionPayloadBidVerifier{errBuilderVersion: errors.New("not a payload builder")},
			result:    pubsub.ValidationReject,
			wantError: true,
		},
		{
			name:      "too many blob commitments",
			verifier:  mockExecutionPayloadBidVerifier{errBlobKzgCommitments: errors.New("too many commitments")},
			result:    pubsub.ValidationReject,
			wantError: true,
		},
		{
			name:      "wrong prev randao",
			verifier:  mockExecutionPayloadBidVerifier{errPrevRandao: errors.New("wrong prev randao")},
			result:    pubsub.ValidationReject,
			wantError: true,
		},
		{
			name:      "slot not higher than parent",
			verifier:  mockExecutionPayloadBidVerifier{errSlotHigherThanParent: errors.New("slot not higher than parent")},
			result:    pubsub.ValidationReject,
			wantError: true,
		},
		{
			name:      "parent hash mismatch",
			verifier:  mockExecutionPayloadBidVerifier{errParentBlockHash: errors.New("wrong hash")},
			result:    pubsub.ValidationIgnore,
			wantError: true,
		},
		{
			name:      "builder cannot cover",
			verifier:  mockExecutionPayloadBidVerifier{errBuilderCanCoverBid: errors.New("cannot cover")},
			result:    pubsub.ValidationIgnore,
			wantError: true,
		},
		{
			name:      "builder exited by parent payload",
			verifier:  mockExecutionPayloadBidVerifier{errBuilderNotExiting: errors.New("builder may exit")},
			result:    pubsub.ValidationIgnore,
			wantError: true,
		},
		{
			name:      "invalid signature",
			verifier:  mockExecutionPayloadBidVerifier{errSignature: errors.New("bad signature")},
			result:    pubsub.ValidationReject,
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, msg, _ := setupExecutionPayloadBidService(t)
			s.newExecutionPayloadBidVerifier = testNewExecutionPayloadBidVerifier(tc.verifier)

			result, err := s.validateExecutionPayloadBidGossip(ctx, "", msg)
			if tc.wantError {
				require.NotNil(t, err)
			}
			require.Equal(t, tc.result, result)
		})
	}
}

func TestValidateExecutionPayloadBidGossip_LowerOrEqualBidIgnored(t *testing.T) {
	ctx := context.Background()
	s, msg, signedBid := setupExecutionPayloadBidService(t)
	s.newExecutionPayloadBidVerifier = testNewExecutionPayloadBidVerifier(mockExecutionPayloadBidVerifier{})

	require.Equal(t, true, s.highestExecutionPayloadBidCache.SetIfHigher(mustWrapBid(t, signedBid)))

	var err error
	result, err := s.validateExecutionPayloadBidGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)
	require.Equal(t, true, s.hasSeenExecutionPayloadBid(executionPayloadBidTupleKey(mustBid(t, signedBid))))
}

func TestValidateExecutionPayloadBidGossip_LowerBidIgnoredStillMarksTupleSeen(t *testing.T) {
	ctx := context.Background()
	s, msg, signedBid := setupExecutionPayloadBidService(t)
	s.newExecutionPayloadBidVerifier = testNewExecutionPayloadBidVerifier(mockExecutionPayloadBidVerifier{})

	higherBid := proto.Clone(signedBid).(*ethpb.SignedExecutionPayloadBid)
	higherBid.Message.Value = signedBid.Message.Value + 1
	require.Equal(t, true, s.highestExecutionPayloadBidCache.SetIfHigher(mustWrapBid(t, higherBid)))

	result, err := s.validateExecutionPayloadBidGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)

	// If the lower valid bid did not mark the tuple as seen, the same bid would
	// be accepted once the highest-bid cache is cleared.
	s.highestExecutionPayloadBidCache = cache.NewHighestExecutionPayloadBidCache()
	msg = executionPayloadBidToPubsub(t, s, s.cfg.p2p, signedBid)

	result, err = s.validateExecutionPayloadBidGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)
}

func TestValidateExecutionPayloadBidGossip_HigherBidAccepted(t *testing.T) {
	ctx := context.Background()
	s, msg, signedBid := setupExecutionPayloadBidService(t)
	s.newExecutionPayloadBidVerifier = testNewExecutionPayloadBidVerifier(mockExecutionPayloadBidVerifier{})

	wrapped, err := blocks.WrappedROSignedExecutionPayloadBid(signedBid)
	require.NoError(t, err)
	bid, err := wrapped.Bid()
	require.NoError(t, err)
	lowerBid := proto.Clone(signedBid).(*ethpb.SignedExecutionPayloadBid)
	lowerBid.Message.Value = bid.Value() - 1
	require.Equal(t, true, s.highestExecutionPayloadBidCache.SetIfHigher(mustWrapBid(t, lowerBid)))

	result, err := s.validateExecutionPayloadBidGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, result)
}

func TestValidateExecutionPayloadBidGossip_HappyPath(t *testing.T) {
	ctx := context.Background()
	s, msg, signedBid := setupExecutionPayloadBidService(t)
	s.newExecutionPayloadBidVerifier = testNewExecutionPayloadBidVerifier(mockExecutionPayloadBidVerifier{})

	result, err := s.validateExecutionPayloadBidGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, result)

	require.Equal(t, true, s.hasSeenExecutionPayloadBid(executionPayloadBidTupleKey(mustBid(t, signedBid))))
	got, ok := msg.ValidatorData.(*ethpb.SignedExecutionPayloadBid)
	require.Equal(t, true, ok)
	require.DeepEqual(t, signedBid, got)
}

// A blacklisted builder must be ignored before any verification runs, and must not be cached.
func TestValidateExecutionPayloadBidGossip_BlacklistedBuilderIgnored(t *testing.T) {
	ctx := context.Background()
	s, msg, signedBid := setupExecutionPayloadBidService(t)
	s.builderCircuitBreaker = cache.NewBuilderCircuitBreaker()
	// The bid is at slot 1, so the failure has to be charged in epoch 0.
	require.Equal(t, true, s.builderCircuitBreaker.RecordFailure(signedBid.Message.BuilderIndex, [32]byte{0xff}, 0).Blacklisted)
	// Every verifier method would reject if reached.
	s.newExecutionPayloadBidVerifier = testNewExecutionPayloadBidVerifier(mockExecutionPayloadBidVerifier{
		errCurrentOrNextSlot: errors.New("slot"),
		errBuilderActive:     errors.New("builder"),
		errSignature:         errors.New("sig"),
	})

	result, err := s.validateExecutionPayloadBidGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)

	require.Equal(t, false, s.hasSeenExecutionPayloadBid(executionPayloadBidTupleKey(mustBid(t, signedBid))))
	_, cached := s.highestExecutionPayloadBidCache.Get(
		signedBid.Message.Slot,
		bytesutil.ToBytes32(signedBid.Message.ParentBlockHash),
		bytesutil.ToBytes32(signedBid.Message.ParentBlockRoot))
	require.Equal(t, false, cached)
}

// A bid from a builder that is not blacklisted is unaffected by the circuit breaker.
func TestValidateExecutionPayloadBidGossip_OtherBuilderNotBlacklisted(t *testing.T) {
	ctx := context.Background()
	s, msg, signedBid := setupExecutionPayloadBidService(t)
	s.builderCircuitBreaker = cache.NewBuilderCircuitBreaker()
	require.Equal(t, true, s.builderCircuitBreaker.RecordFailure(signedBid.Message.BuilderIndex+1, [32]byte{0xff}, 0).Blacklisted)
	s.newExecutionPayloadBidVerifier = testNewExecutionPayloadBidVerifier(mockExecutionPayloadBidVerifier{})

	result, err := s.validateExecutionPayloadBidGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, result)
}

func TestValidateExecutionPayloadBidGossip_NextSlotStateMissIgnored(t *testing.T) {
	ctx := context.Background()
	s, msg, _ := setupExecutionPayloadBidService(t)
	// errPrevRandao would reject if reached, but a next-slot-cache miss for the bid's parent must ignore first.
	s.newExecutionPayloadBidVerifier = testNewExecutionPayloadBidVerifier(mockExecutionPayloadBidVerifier{
		errPrevRandao: errors.New("wrong prev randao"),
	})
	// Evict the bid's parent (0x02) from the next-slot cache.
	other, err := util.NewBeaconStateGloas()
	require.NoError(t, err)
	require.NoError(t, transition.UpdateNextSlotCache(ctx, bytesutil.PadTo([]byte{0x07}, 32), other))
	require.NoError(t, transition.UpdateNextSlotCache(ctx, bytesutil.PadTo([]byte{0x08}, 32), other))

	result, err := s.validateExecutionPayloadBidGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)
}

func TestValidateExecutionPayloadBidGossip_FeeRecipientMismatch(t *testing.T) {
	ctx := context.Background()
	s, msg, _ := setupExecutionPayloadBidService(t)
	s.newExecutionPayloadBidVerifier = testNewExecutionPayloadBidVerifier(
		mockExecutionPayloadBidVerifier{errFeeRecipientMismatch: verification.ErrBidFeeRecipientMismatch},
	)

	result, err := s.validateExecutionPayloadBidGossip(ctx, "", msg)
	require.NotNil(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)
	require.ErrorIs(t, err, verification.ErrBidFeeRecipientMismatch)
}

func TestValidateExecutionPayloadBidGossip_GasLimitIncompatible(t *testing.T) {
	ctx := context.Background()
	s, msg, _ := setupExecutionPayloadBidService(t)
	s.newExecutionPayloadBidVerifier = testNewExecutionPayloadBidVerifier(
		mockExecutionPayloadBidVerifier{errGasLimitIncompatible: verification.ErrBidGasLimitIncompatible},
	)

	result, err := s.validateExecutionPayloadBidGossip(ctx, "", msg)
	require.NotNil(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)
	require.ErrorIs(t, err, verification.ErrBidGasLimitIncompatible)
}

func TestExecutionPayloadBidSubscriber_WrongMessage(t *testing.T) {
	s := &Service{}
	err := s.executionPayloadBidSubscriber(context.Background(), &ethpb.BeaconBlock{})
	require.ErrorIs(t, errWrongMessage, err)
}

func TestExecutionPayloadBidSubscriber_HappyPath(t *testing.T) {
	gloasBid := util.GenerateTestSignedExecutionPayloadBid(1)
	hezeBid := hezeBidFromGloas(util.GenerateTestSignedExecutionPayloadBid(1))
	for _, tc := range []struct {
		name      string
		msg       proto.Message
		wrapped   interfaces.ROSignedExecutionPayloadBid
		eventData any
	}{
		{name: "gloas", msg: gloasBid, wrapped: mustWrapBid(t, gloasBid), eventData: &opfeed.ExecutionPayloadBidReceivedData{Bid: gloasBid}},
		{name: "heze", msg: hezeBid, wrapped: mustWrapBidHeze(t, hezeBid), eventData: &opfeed.ExecutionPayloadBidHezeReceivedData{Bid: hezeBid}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			notifier := &mock.MockOperationNotifier{}
			s := &Service{
				cfg:                             &config{operationNotifier: notifier},
				highestExecutionPayloadBidCache: cache.NewHighestExecutionPayloadBidCache(),
			}
			events := make(chan *feed.Event, 1)
			sub := notifier.OperationFeed().Subscribe(events)
			defer sub.Unsubscribe()

			require.NoError(t, s.executionPayloadBidSubscriber(t.Context(), tc.msg))
			bid, err := tc.wrapped.Bid()
			require.NoError(t, err)
			got, ok := s.highestExecutionPayloadBidCache.Get(bid.Slot(), bid.ParentBlockHash(), bid.ParentBlockRoot())
			require.Equal(t, true, ok)
			require.DeepEqual(t, tc.wrapped, got)
			require.DeepEqual(t, tc.eventData, (<-events).Data)
		})
	}
}

// TestProposerDependentRoot_DelegatesToForkchoice asserts that the helper
// queries the chain's DependentRootForEpoch at epoch-1 anchored to
// parentBlockRoot, and returns whatever root forkchoice gives back.
func TestProposerDependentRoot_DelegatesToForkchoice(t *testing.T) {
	parentRoot := [32]byte{0xaa}
	expectedDepRoot := [32]byte{0xbb}
	bidSlot := 2*params.BeaconConfig().SlotsPerEpoch + 6
	expectedEpoch := slots.ToEpoch(bidSlot).Sub(1)

	var gotRoot [32]byte
	var gotEpoch primitives.Epoch
	chainService := &mock.ChainService{
		DependentRootCB: func(root [32]byte, epoch primitives.Epoch) ([32]byte, error) {
			gotRoot = root
			gotEpoch = epoch
			return expectedDepRoot, nil
		},
	}
	s := &Service{cfg: &config{chain: chainService}}

	got, err := s.proposerDependentRoot(parentRoot, bidSlot)
	require.NoError(t, err)
	require.Equal(t, expectedDepRoot, got)
	require.Equal(t, parentRoot, gotRoot)
	require.Equal(t, expectedEpoch, gotEpoch)
}

// TestProposerDependentRoot_UnderflowClampsToZero asserts that proposal epochs
// below 2 query DependentRootForEpoch at epoch 0 (which the chain maps to the
// origin block root) rather than underflowing epoch-1.
func TestProposerDependentRoot_UnderflowClampsToZero(t *testing.T) {
	parentRoot := [32]byte{0xaa}
	originRoot := [32]byte{0xcc}

	for _, slot := range []primitives.Slot{0, 1, params.BeaconConfig().SlotsPerEpoch + 1} {
		t.Run(fmt.Sprintf("slot %d", slot), func(t *testing.T) {
			var gotEpoch primitives.Epoch
			chainService := &mock.ChainService{
				DependentRootCB: func(_ [32]byte, epoch primitives.Epoch) ([32]byte, error) {
					gotEpoch = epoch
					return originRoot, nil
				},
			}
			s := &Service{cfg: &config{chain: chainService}}

			got, err := s.proposerDependentRoot(parentRoot, slot)
			require.NoError(t, err)
			require.Equal(t, originRoot, got)
			require.Equal(t, primitives.Epoch(0), gotEpoch)
		})
	}
}

func TestExecutionPayloadBidSubscriber_NilMessage(t *testing.T) {
	s := &Service{
		highestExecutionPayloadBidCache: cache.NewHighestExecutionPayloadBidCache(),
	}
	err := s.executionPayloadBidSubscriber(context.Background(), &ethpb.SignedExecutionPayloadBid{})
	require.ErrorIs(t, errNilMessage, err)
	err = s.executionPayloadBidSubscriber(context.Background(), &ethpb.SignedExecutionPayloadBidHeze{})
	require.ErrorIs(t, errNilMessage, err)
}

// setupExecutionPayloadBidServiceHeze returns the bid service with Heze at hezeEpoch and a next-slot state with validators.
func setupExecutionPayloadBidServiceHeze(t *testing.T, hezeEpoch primitives.Epoch) (*Service, *ethpb.SignedExecutionPayloadBidHeze) {
	t.Helper()
	s, _, gloasBid := setupExecutionPayloadBidService(t)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = hezeEpoch
	params.OverrideBeaconConfig(cfg)
	s.newExecutionPayloadBidVerifier = testNewExecutionPayloadBidVerifier(mockExecutionPayloadBidVerifier{})

	st, _ := util.DeterministicGenesisStateGloas(t, 64)
	chain, ok := s.cfg.chain.(*mock.ChainService)
	require.Equal(t, true, ok)
	require.NoError(t, transition.UpdateNextSlotCache(t.Context(), chain.Root, st))
	return s, hezeBidFromGloas(gloasBid)
}

func hezeBidFromGloas(g *ethpb.SignedExecutionPayloadBid) *ethpb.SignedExecutionPayloadBidHeze {
	m := g.Message
	return &ethpb.SignedExecutionPayloadBidHeze{
		Message: &ethpb.ExecutionPayloadBidHeze{
			ParentBlockHash:       m.ParentBlockHash,
			ParentBlockRoot:       m.ParentBlockRoot,
			BlockHash:             m.BlockHash,
			PrevRandao:            m.PrevRandao,
			FeeRecipient:          m.FeeRecipient,
			GasLimit:              m.GasLimit,
			BuilderIndex:          m.BuilderIndex,
			Slot:                  m.Slot,
			Value:                 m.Value,
			ExecutionPayment:      m.ExecutionPayment,
			BlobKzgCommitments:    m.BlobKzgCommitments,
			ExecutionRequestsRoot: m.ExecutionRequestsRoot,
			InclusionListBits:     bitfield.NewBitvector16(),
		},
		Signature: g.Signature,
	}
}

func bidToPubsubAtEpoch(t *testing.T, s *Service, bid ssz.Marshaler, epoch primitives.Epoch) *pubsub.Message {
	t.Helper()
	buf := new(bytes.Buffer)
	_, err := s.cfg.p2p.Encoding().EncodeGossip(buf, bid)
	require.NoError(t, err)
	topic := s.addDigestToTopic(p2p.ExecutionPayloadBidTopicFormat, params.ForkDigest(epoch))
	return &pubsub.Message{Message: &pb.Message{Data: buf.Bytes(), Topic: &topic}}
}

// storeTimelyInclusionList stores a timely list from the first inclusion list committee member of slot 0 and returns the bits the store reports.
func storeTimelyInclusionList(t *testing.T, s *Service) bitfield.Bitvector16 {
	return storeInclusionList(t, s, true)
}

// storeInclusionList stores a list from the first inclusion list committee member of slot 0 and returns the bits of every stored list.
func storeInclusionList(t *testing.T, s *Service, timely bool) bitfield.Bitvector16 {
	t.Helper()
	chain, ok := s.cfg.chain.(*mock.ChainService)
	require.Equal(t, true, ok)
	st := transition.NextSlotStateReadOnly(chain.Root, 1)
	require.NotNil(t, st)
	committee, err := helpers.InclusionListCommittee(t.Context(), st, 0)
	require.NoError(t, err)
	txs, err := enginev1.NewProgressiveTransactionList([][]byte{{0x02}})
	require.NoError(t, err)
	store := chain.InclusionListStore()
	store.Process(&ethpb.SignedInclusionList{
		Message: &ethpb.InclusionList{
			Slot:           0,
			ValidatorIndex: committee[0],
			DependentRoot:  chain.TargetRoot[:],
			Transactions:   txs,
		},
		Signature: make([]byte, 96),
	}, timely)
	bits := store.Bits(committee, 0, chain.TargetRoot, false)
	require.NotEqual(t, uint64(0), bits.Count())
	return bits
}

func TestValidateExecutionPayloadBidGossip_Heze(t *testing.T) {
	t.Run("accepted with empty store", func(t *testing.T) {
		s, bid := setupExecutionPayloadBidServiceHeze(t, 0)
		msg := bidToPubsubAtEpoch(t, s, bid, 0)

		result, err := s.validateExecutionPayloadBidGossip(t.Context(), "", msg)
		require.NoError(t, err)
		require.Equal(t, pubsub.ValidationAccept, result)
		got, ok := msg.ValidatorData.(*ethpb.SignedExecutionPayloadBidHeze)
		require.Equal(t, true, ok)
		require.DeepEqual(t, bid, got)
	})
	t.Run("bits not inclusive is ignored", func(t *testing.T) {
		s, bid := setupExecutionPayloadBidServiceHeze(t, 0)
		storeTimelyInclusionList(t, s)
		msg := bidToPubsubAtEpoch(t, s, bid, 0)

		result, err := s.validateExecutionPayloadBidGossip(t.Context(), "", msg)
		require.ErrorContains(t, "not inclusive", err)
		require.Equal(t, pubsub.ValidationIgnore, result)
		// An ignored bid is not marked as seen, so an inclusive bid from the same builder can follow.
		b, err := blocks.WrappedROSignedExecutionPayloadBidHeze(bid)
		require.NoError(t, err)
		ro, err := b.Bid()
		require.NoError(t, err)
		require.Equal(t, false, s.hasSeenExecutionPayloadBid(executionPayloadBidTupleKey(ro)))
	})
	t.Run("inclusive bits are accepted", func(t *testing.T) {
		s, bid := setupExecutionPayloadBidServiceHeze(t, 0)
		bid.Message.InclusionListBits = storeTimelyInclusionList(t, s)
		msg := bidToPubsubAtEpoch(t, s, bid, 0)

		result, err := s.validateExecutionPayloadBidGossip(t.Context(), "", msg)
		require.NoError(t, err)
		require.Equal(t, pubsub.ValidationAccept, result)
	})
	t.Run("an untimely list is not required in the bits", func(t *testing.T) {
		s, bid := setupExecutionPayloadBidServiceHeze(t, 0)
		storeInclusionList(t, s, false)
		msg := bidToPubsubAtEpoch(t, s, bid, 0)

		result, err := s.validateExecutionPayloadBidGossip(t.Context(), "", msg)
		require.NoError(t, err)
		require.Equal(t, pubsub.ValidationAccept, result)
	})
	t.Run("bits check runs before the signature check", func(t *testing.T) {
		s, bid := setupExecutionPayloadBidServiceHeze(t, 0)
		s.newExecutionPayloadBidVerifier = testNewExecutionPayloadBidVerifier(mockExecutionPayloadBidVerifier{errSignature: errors.New("bad signature")})
		storeTimelyInclusionList(t, s)

		result, err := s.validateExecutionPayloadBidGossip(t.Context(), "", bidToPubsubAtEpoch(t, s, bid, 0))
		require.ErrorContains(t, "not inclusive", err)
		require.Equal(t, pubsub.ValidationIgnore, result)
	})
	t.Run("lower Heze bid than the cached Heze bid is ignored", func(t *testing.T) {
		s, bid := setupExecutionPayloadBidServiceHeze(t, 0)
		higher := proto.Clone(bid).(*ethpb.SignedExecutionPayloadBidHeze)
		higher.Message.Value = bid.Message.Value + 1
		require.Equal(t, true, s.highestExecutionPayloadBidCache.SetIfHigher(mustWrapBidHeze(t, higher)))

		result, err := s.validateExecutionPayloadBidGossip(t.Context(), "", bidToPubsubAtEpoch(t, s, bid, 0))
		require.NoError(t, err)
		require.Equal(t, pubsub.ValidationIgnore, result)
	})
	t.Run("Gloas bid for a Heze slot is rejected", func(t *testing.T) {
		s, bid := setupExecutionPayloadBidServiceHeze(t, 1)
		gloasBid := util.GenerateTestSignedExecutionPayloadBid(bid.Message.Slot + params.BeaconConfig().SlotsPerEpoch)
		// The Gloas digest decodes a Gloas bid.
		result, err := s.validateExecutionPayloadBidGossip(t.Context(), "", bidToPubsubAtEpoch(t, s, gloasBid, 0))
		require.ErrorIs(t, err, errWrongMessage)
		require.Equal(t, pubsub.ValidationReject, result)
	})
	t.Run("Heze bid for a Gloas slot is rejected", func(t *testing.T) {
		s, bid := setupExecutionPayloadBidServiceHeze(t, 1)
		// The Heze digest decodes a Heze bid, but bid.slot 1 is before the Heze fork.
		result, err := s.validateExecutionPayloadBidGossip(t.Context(), "", bidToPubsubAtEpoch(t, s, bid, 1))
		require.ErrorIs(t, err, errWrongMessage)
		require.Equal(t, pubsub.ValidationReject, result)
	})
	t.Run("Heze bid at the first Heze slot is accepted", func(t *testing.T) {
		s, bid := setupExecutionPayloadBidServiceHeze(t, 1)
		// The inclusion lists of bid.slot - 1 are in a Gloas slot, so there is no committee to check.
		bid.Message.Slot = params.BeaconConfig().SlotsPerEpoch
		chain, ok := s.cfg.chain.(*mock.ChainService)
		require.Equal(t, true, ok)
		st, _ := util.DeterministicGenesisStateGloas(t, 64)
		require.NoError(t, st.SetSlot(bid.Message.Slot-1))
		require.NoError(t, transition.UpdateNextSlotCache(t.Context(), chain.Root, st))
		require.Equal(t, true, s.proposerPreferencesCache.Add(cache.ProposerPreference{
			DependentRoot:  chain.TargetRoot,
			FeeRecipient:   bytesutil.ToBytes20(bid.Message.FeeRecipient),
			TargetGasLimit: bid.Message.GasLimit,
		}, bid.Message.Slot))
		result, err := s.validateExecutionPayloadBidGossip(t.Context(), "", bidToPubsubAtEpoch(t, s, bid, 1))
		require.NoError(t, err)
		require.Equal(t, pubsub.ValidationAccept, result)
	})
	t.Run("Gloas bid before Heze is accepted", func(t *testing.T) {
		s, bid := setupExecutionPayloadBidServiceHeze(t, 1)
		// The bid.slot - 1 inclusion lists are before Heze, so the bits check does not compute a committee.
		gloasMsg := bidToPubsubAtEpoch(t, s, util.GenerateTestSignedExecutionPayloadBid(bid.Message.Slot), 0)
		result, err := s.validateExecutionPayloadBidGossip(t.Context(), "", gloasMsg)
		require.NoError(t, err)
		require.Equal(t, pubsub.ValidationAccept, result)
	})
}

type mockExecutionPayloadBidVerifier struct {
	errCurrentOrNextSlot     error
	errSlotMatches           error
	errBuilderActive         error
	errBuilderVersion        error
	errExecutionPayment      error
	errFeeRecipientMismatch  error
	errBlobKzgCommitments    error
	errPrevRandao            error
	errGasLimitIncompatible  error
	errParentBlockRootSeen   error
	errBidCompatibleWithHead error
	errSlotHigherThanParent  error
	errParentBlockHash       error
	errBuilderCanCoverBid    error
	errBuilderNotExiting     error
	errSignature             error
}

var _ verification.ExecutionPayloadBidVerifier = &mockExecutionPayloadBidVerifier{}

func (m *mockExecutionPayloadBidVerifier) VerifyCurrentOrNextSlot() error {
	return m.errCurrentOrNextSlot
}

func (m *mockExecutionPayloadBidVerifier) VerifyBidSlotMatches(primitives.Slot) error {
	return m.errSlotMatches
}

func (m *mockExecutionPayloadBidVerifier) VerifyBuilderActive(state.ReadOnlyBeaconState) error {
	return m.errBuilderActive
}

func (m *mockExecutionPayloadBidVerifier) VerifyBuilderVersion(state.ReadOnlyBeaconState) error {
	return m.errBuilderVersion
}

func (m *mockExecutionPayloadBidVerifier) VerifyExecutionPaymentZero() error {
	return m.errExecutionPayment
}

func (m *mockExecutionPayloadBidVerifier) VerifyFeeRecipientMatches([]byte) error {
	return m.errFeeRecipientMismatch
}

func (m *mockExecutionPayloadBidVerifier) VerifyBlobKzgCommitmentsLimit() error {
	return m.errBlobKzgCommitments
}

func (m *mockExecutionPayloadBidVerifier) VerifyPrevRandao(state.ReadOnlyBeaconState) error {
	return m.errPrevRandao
}

func (m *mockExecutionPayloadBidVerifier) VerifyGasLimitTargetCompatible(uint64, uint64) error {
	return m.errGasLimitIncompatible
}

func (m *mockExecutionPayloadBidVerifier) VerifyParentBlockRootSeen(func([32]byte) bool) error {
	return m.errParentBlockRootSeen
}

func (m *mockExecutionPayloadBidVerifier) VerifyBidCompatibleWithHead(func(interfaces.ROExecutionPayloadBid) bool) error {
	return m.errBidCompatibleWithHead
}

func (m *mockExecutionPayloadBidVerifier) VerifyBidSlotHigherThanParent(primitives.Slot) error {
	return m.errSlotHigherThanParent
}

func (m *mockExecutionPayloadBidVerifier) VerifyParentBlockHash(func([32]byte, [32]byte) bool) error {
	return m.errParentBlockHash
}

func (m *mockExecutionPayloadBidVerifier) VerifyBuilderCanCoverBid(state.ReadOnlyBeaconState) error {
	return m.errBuilderCanCoverBid
}

func (m *mockExecutionPayloadBidVerifier) VerifyBuilderNotExiting(state.ReadOnlyBeaconState, func([32]byte) ([]*enginev1.BuilderExitRequest, error)) error {
	return m.errBuilderNotExiting
}

func (m *mockExecutionPayloadBidVerifier) VerifySignature(state.ReadOnlyBeaconState) error {
	return m.errSignature
}

func (*mockExecutionPayloadBidVerifier) SatisfyRequirement(verification.Requirement) {}

func testNewExecutionPayloadBidVerifier(m mockExecutionPayloadBidVerifier) verification.NewExecutionPayloadBidVerifier {
	return func(interfaces.ROSignedExecutionPayloadBid, []verification.Requirement) verification.ExecutionPayloadBidVerifier {
		clone := m
		return &clone
	}
}

func setupExecutionPayloadBidService(t *testing.T) (*Service, *pubsub.Message, *ethpb.SignedExecutionPayloadBid) {
	t.Helper()

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)

	// The bid is at slot 1 (epoch 0); the mock chain maps that to its TargetRoot
	// via DependentRootForEpoch, so the proposer preference is keyed on it below.
	genesisRoot := [32]byte{0x01}

	state, err := util.NewBeaconStateGloas()
	require.NoError(t, err)
	signedBid := util.GenerateTestSignedExecutionPayloadBid(1)
	signedBid.Message.BuilderIndex = 1
	chainService := &mock.ChainService{
		Genesis:    time.Now(),
		State:      state,
		TargetRoot: genesisRoot,
		Root:       bytesutil.PadTo([]byte{0x02}, 32),
		ForkchoiceRoots: map[[32]byte]bool{
			[32]byte{0x02}: true,
		},
		ForkchoiceBlockHashes: map[[32]byte][32]byte{[32]byte{0x02}: [32]byte{0x01}},
		ForkchoiceGasLimits:   map[[32]byte]uint64{[32]byte{0x02}: 1},
	}
	require.NoError(t, transition.UpdateNextSlotCache(context.Background(), chainService.Root, state))
	s := &Service{
		seenExecutionPayloadBidCache:    newSlotAwareCache(10),
		highestExecutionPayloadBidCache: cache.NewHighestExecutionPayloadBidCache(),
		proposerPreferencesCache:        cache.NewProposerPreferencesCache(),
		cfg: &config{
			p2p:         p,
			initialSync: &mockSync.Sync{},
			chain:       chainService,
			beaconDB:    db,
			clock:       startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
		},
	}
	// The Gloas test state has a zero-filled proposer lookahead, so the
	// proposer for any slot is validator index 0.
	require.Equal(t, true, s.proposerPreferencesCache.Add(cache.ProposerPreference{
		DependentRoot:  genesisRoot,
		ValidatorIndex: 0,
		FeeRecipient:   bytesutil.ToBytes20(signedBid.Message.FeeRecipient),
		TargetGasLimit: signedBid.Message.GasLimit,
	}, signedBid.Message.Slot))
	msg := executionPayloadBidToPubsub(t, s, p, signedBid)
	return s, msg, signedBid
}

func executionPayloadBidToPubsub(t *testing.T, s *Service, p p2p.P2P, bid *ethpb.SignedExecutionPayloadBid) *pubsub.Message {
	t.Helper()

	buf := new(bytes.Buffer)
	_, err := p.Encoding().EncodeGossip(buf, bid)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeFor[*ethpb.SignedExecutionPayloadBid]()]
	digest := s.currentForkDigest()
	topic = s.addDigestToTopic(topic, digest)

	return &pubsub.Message{
		Message: &pb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
}

func mustBid(t *testing.T, signedBid *ethpb.SignedExecutionPayloadBid) interfaces.ROExecutionPayloadBid {
	t.Helper()

	bid, err := mustWrapBid(t, signedBid).Bid()
	require.NoError(t, err)
	return bid
}

func mustWrapBid(t *testing.T, signedBid *ethpb.SignedExecutionPayloadBid) interfaces.ROSignedExecutionPayloadBid {
	t.Helper()

	wrapped, err := blocks.WrappedROSignedExecutionPayloadBid(signedBid)
	require.NoError(t, err)
	return wrapped
}

func mustWrapBidHeze(t *testing.T, signedBid *ethpb.SignedExecutionPayloadBidHeze) interfaces.ROSignedExecutionPayloadBid {
	t.Helper()

	wrapped, err := blocks.WrappedROSignedExecutionPayloadBidHeze(signedBid)
	require.NoError(t, err)
	return wrapped
}
