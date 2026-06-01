package sync

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
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
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
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

func TestValidateExecutionPayloadBidGossip_AlreadySeenBuilder(t *testing.T) {
	ctx := context.Background()
	s, msg, signedBid := setupExecutionPayloadBidService(t)
	s.newExecutionPayloadBidVerifier = testNewExecutionPayloadBidVerifier(mockExecutionPayloadBidVerifier{})

	key := executionPayloadBidBuilderKey(signedBid.Message.Slot, signedBid.Message.BuilderIndex)
	s.setSeenExecutionPayloadBidBuilder(signedBid.Message.Slot, key)
	result, err := s.validateExecutionPayloadBidGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)
}

// Dedup must short-circuit before every later check; duplicates pay only the cache lookup.
func TestValidateExecutionPayloadBidGossip_DedupShortCircuitsAllLaterChecks(t *testing.T) {
	ctx := context.Background()
	s, msg, signedBid := setupExecutionPayloadBidService(t)
	key := executionPayloadBidBuilderKey(signedBid.Message.Slot, signedBid.Message.BuilderIndex)
	s.setSeenExecutionPayloadBidBuilder(signedBid.Message.Slot, key)
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
			result:    pubsub.ValidationReject,
			wantError: true,
		},
		{
			name:      "gas limit incompatible",
			verifier:  mockExecutionPayloadBidVerifier{errGasLimitIncompatible: errors.New("incompatible gas limit")},
			result:    pubsub.ValidationIgnore,
			wantError: true,
		},
		{
			name:      "parent root unknown",
			verifier:  mockExecutionPayloadBidVerifier{errParentBlockRootSeen: errors.New("unknown root")},
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

	s.setHighestExecutionPayloadBid(signedBid)

	var err error
	result, err := s.validateExecutionPayloadBidGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)
	builderKey := executionPayloadBidBuilderKey(signedBid.Message.Slot, signedBid.Message.BuilderIndex)
	require.Equal(t, true, s.hasSeenExecutionPayloadBidBuilder(builderKey))
}

func TestValidateExecutionPayloadBidGossip_LowerBidIgnoredStillMarksBuilderSeen(t *testing.T) {
	ctx := context.Background()
	s, msg, signedBid := setupExecutionPayloadBidService(t)
	s.newExecutionPayloadBidVerifier = testNewExecutionPayloadBidVerifier(mockExecutionPayloadBidVerifier{})

	higherBid := proto.Clone(signedBid).(*ethpb.SignedExecutionPayloadBid)
	higherBid.Message.Value = signedBid.Message.Value + 1
	s.setHighestExecutionPayloadBid(higherBid)

	result, err := s.validateExecutionPayloadBidGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)

	// If the lower valid bid did not mark the builder as seen, the same bid would
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
	s.setHighestExecutionPayloadBid(lowerBid)

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

	builderKey := executionPayloadBidBuilderKey(signedBid.Message.Slot, signedBid.Message.BuilderIndex)
	require.Equal(t, true, s.hasSeenExecutionPayloadBidBuilder(builderKey))
	got, ok := msg.ValidatorData.(*ethpb.SignedExecutionPayloadBid)
	require.Equal(t, true, ok)
	require.DeepEqual(t, signedBid, got)
}

func TestValidateExecutionPayloadBidGossip_FeeRecipientMismatch(t *testing.T) {
	ctx := context.Background()
	s, msg, _ := setupExecutionPayloadBidService(t)
	s.newExecutionPayloadBidVerifier = testNewExecutionPayloadBidVerifier(
		mockExecutionPayloadBidVerifier{errFeeRecipientMismatch: verification.ErrBidFeeRecipientMismatch},
	)

	result, err := s.validateExecutionPayloadBidGossip(ctx, "", msg)
	require.NotNil(t, err)
	require.Equal(t, pubsub.ValidationReject, result)
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
	s := &Service{
		highestExecutionPayloadBidCache: cache.NewHighestExecutionPayloadBidCache(),
	}
	signedBid := util.GenerateTestSignedExecutionPayloadBid(1)
	err := s.executionPayloadBidSubscriber(context.Background(), signedBid)
	require.NoError(t, err)
	bid := mustBid(t, signedBid)
	got, ok := s.highestExecutionPayloadBidCache.Get(bid.Slot(), bid.ParentBlockHash(), bid.ParentBlockRoot())
	require.Equal(t, true, ok)
	require.DeepEqual(t, signedBid, got)
}

func TestExecutionPayloadBidSubscriber_NilMessage(t *testing.T) {
	s := &Service{
		highestExecutionPayloadBidCache: cache.NewHighestExecutionPayloadBidCache(),
	}
	err := s.executionPayloadBidSubscriber(context.Background(), &ethpb.SignedExecutionPayloadBid{})
	require.ErrorIs(t, errNilMessage, err)
}

type mockExecutionPayloadBidVerifier struct {
	errCurrentOrNextSlot    error
	errBuilderActive        error
	errExecutionPayment     error
	errFeeRecipientMismatch error
	errGasLimitIncompatible error
	errParentBlockRootSeen  error
	errParentBlockHash      error
	errBuilderCanCoverBid   error
	errSignature            error
}

var _ verification.ExecutionPayloadBidVerifier = &mockExecutionPayloadBidVerifier{}

func (m *mockExecutionPayloadBidVerifier) VerifyCurrentOrNextSlot() error {
	return m.errCurrentOrNextSlot
}

func (m *mockExecutionPayloadBidVerifier) VerifyBuilderActive(state.ReadOnlyBeaconState) error {
	return m.errBuilderActive
}

func (m *mockExecutionPayloadBidVerifier) VerifyExecutionPaymentZero() error {
	return m.errExecutionPayment
}

func (m *mockExecutionPayloadBidVerifier) VerifyFeeRecipientMatches([]byte) error {
	return m.errFeeRecipientMismatch
}

func (m *mockExecutionPayloadBidVerifier) VerifyGasLimitTargetCompatible(uint64, uint64) error {
	return m.errGasLimitIncompatible
}

func (m *mockExecutionPayloadBidVerifier) VerifyParentBlockRootSeen(func([32]byte) bool) error {
	return m.errParentBlockRootSeen
}

func (m *mockExecutionPayloadBidVerifier) VerifyParentBlockHash(func([32]byte) ([32]byte, error)) error {
	return m.errParentBlockHash
}

func (m *mockExecutionPayloadBidVerifier) VerifyBuilderCanCoverBid(state.ReadOnlyBeaconState) error {
	return m.errBuilderCanCoverBid
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

	ctx := context.Background()
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)

	// Save a genesis block so beaconDB.GenesisBlockRoot resolves; bids at slot 1
	// (epoch 0) hit the underflow branch in chain.DependentRootForEpoch which
	// falls back to the genesis block root.
	gb := util.NewBeaconBlock()
	signedGenesis, err := blocks.NewSignedBeaconBlock(gb)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, signedGenesis))
	genesisRoot, err := signedGenesis.Block().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesisRoot))

	state, err := util.NewBeaconStateGloas()
	require.NoError(t, err)
	signedBid := util.GenerateTestSignedExecutionPayloadBid(1)
	signedBid.Message.BuilderIndex = 1
	chainService := &mock.ChainService{
		Genesis:    time.Now(),
		State:      state,
		TargetRoot: genesisRoot,
		ForkchoiceRoots: map[[32]byte]bool{
			[32]byte{0x02}: true,
		},
		ForkchoiceBlockHashes: map[[32]byte][32]byte{[32]byte{0x02}: [32]byte{0x01}},
		ForkchoiceGasLimits:   map[[32]byte]uint64{[32]byte{0x02}: 1},
	}
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
	digest, err := s.currentForkDigest()
	require.NoError(t, err)
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

	wrapped, err := blocks.WrappedROSignedExecutionPayloadBid(signedBid)
	require.NoError(t, err)
	bid, err := wrapped.Bid()
	require.NoError(t, err)
	return bid
}
