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
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/pkg/errors"
)

func TestValidateSignedProposerPreferencesGossip_InvalidTopic(t *testing.T) {
	ctx := context.Background()
	p := p2ptest.NewTestP2P(t)
	s := &Service{cfg: &config{p2p: p, initialSync: &mockSync.Sync{}}}

	result, err := s.validateSignedProposerPreferencesGossip(ctx, "", &pubsub.Message{Message: &pb.Message{}})
	require.ErrorIs(t, p2p.ErrInvalidTopic, err)
	require.Equal(t, pubsub.ValidationReject, result)
}

func TestValidateSignedProposerPreferencesGossip_InitialSync(t *testing.T) {
	ctx := context.Background()
	p := p2ptest.NewTestP2P(t)
	s := &Service{
		cfg: &config{
			p2p:         p,
			initialSync: &mockSync.Sync{IsSyncing: true},
		},
	}

	result, err := s.validateSignedProposerPreferencesGossip(ctx, "", &pubsub.Message{})
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)
}

func TestValidateSignedProposerPreferencesGossip_CheckpointBlockNotSeen(t *testing.T) {
	ctx := context.Background()
	s, msg, _ := setupSignedProposerPreferencesService(t)
	s.newSignedProposerPreferencesVerifier = testNewSignedProposerPreferencesVerifier(
		mockSignedProposerPreferencesVerifier{errDependentRootSeen: errors.New("dependent_root block not seen")},
	)

	result, err := s.validateSignedProposerPreferencesGossip(ctx, "", msg)
	require.NotNil(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)
}

func TestValidateSignedProposerPreferencesGossip_ErrorPathsWithMock(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		verifier  mockSignedProposerPreferencesVerifier
		result    pubsub.ValidationResult
		wantError bool
	}{
		{
			name:      "not current or next epoch",
			verifier:  mockSignedProposerPreferencesVerifier{errCurrentOrNextEpoch: errors.New("wrong epoch")},
			result:    pubsub.ValidationIgnore,
			wantError: true,
		},
		{
			name:      "invalid proposer slot",
			verifier:  mockSignedProposerPreferencesVerifier{errValidProposalSlot: errors.New("invalid slot")},
			result:    pubsub.ValidationReject,
			wantError: true,
		},
		{
			name:      "invalid signature",
			verifier:  mockSignedProposerPreferencesVerifier{errSignature: errors.New("bad signature")},
			result:    pubsub.ValidationReject,
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, msg, _ := setupSignedProposerPreferencesService(t)
			s.newSignedProposerPreferencesVerifier = testNewSignedProposerPreferencesVerifier(tc.verifier)

			result, err := s.validateSignedProposerPreferencesGossip(ctx, "", msg)
			if tc.wantError {
				require.NotNil(t, err)
			}
			require.Equal(t, tc.result, result)
		})
	}
}

func TestValidateSignedProposerPreferencesGossip_AlreadySeen(t *testing.T) {
	ctx := context.Background()
	s, msg, signedPreferences := setupSignedProposerPreferencesService(t)
	s.newSignedProposerPreferencesVerifier = testNewSignedProposerPreferencesVerifier(mockSignedProposerPreferencesVerifier{})

	dependentRoot := bytesutil.ToBytes32(signedPreferences.Message.DependentRoot)
	require.Equal(t, true, s.proposerPreferencesCache.Add(cache.ProposerPreference{
		DependentRoot:  dependentRoot,
		ValidatorIndex: signedPreferences.Message.ValidatorIndex,
		FeeRecipient:   primitives.ExecutionAddress{0x01},
		TargetGasLimit: 10,
	}, signedPreferences.Message.ProposalSlot))
	result, err := s.validateSignedProposerPreferencesGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)
}

// TestValidateSignedProposerPreferencesGossip_HeadTooStale exercises the branch
// that returns when the proposal is more than one epoch ahead of the head state
// (and not the +2 boundary edge case). With head state at epoch 0 and proposal
// in epoch 3 the validator must ignore — proposer_lookahead cannot cover it.
func TestValidateSignedProposerPreferencesGossip_HeadTooStale(t *testing.T) {
	ctx := context.Background()
	s, _, signedPreferences := setupSignedProposerPreferencesService(t)
	s.newSignedProposerPreferencesVerifier = testNewSignedProposerPreferencesVerifier(mockSignedProposerPreferencesVerifier{})
	signedPreferences.Message.ProposalSlot = primitives.Slot(96)
	msg := signedProposerPreferencesToPubsub(t, s, s.cfg.p2p, signedPreferences)

	result, err := s.validateSignedProposerPreferencesGossip(ctx, "", msg)
	require.ErrorContains(t, "cannot verify", err)
	require.Equal(t, pubsub.ValidationIgnore, result)
}

func TestValidateSignedProposerPreferencesGossip_DependentRootMismatchSkipsStateLoad(t *testing.T) {
	ctx := context.Background()
	s, _, signedPreferences := setupSignedProposerPreferencesService(t)
	s.newSignedProposerPreferencesVerifier = testNewSignedProposerPreferencesVerifier(mockSignedProposerPreferencesVerifier{})
	msg := signedProposerPreferencesToPubsub(t, s, s.cfg.p2p, signedPreferences)

	chainService := s.cfg.chain.(*mock.ChainService)
	chainService.HeadStateErr = errors.New("head state should not load")
	chainService.DependentRootCB = func(root [32]byte, epoch primitives.Epoch) ([32]byte, error) {
		require.Equal(t, [32]byte{}, root)
		require.Equal(t, primitives.Epoch(0), epoch)
		return [32]byte{0xbb}, nil
	}

	result, err := s.validateSignedProposerPreferencesGossip(ctx, "", msg)
	require.ErrorContains(t, "dependent_root", err)
	require.Equal(t, pubsub.ValidationIgnore, result)
}

func TestValidateSignedProposerPreferencesGossip_EpochPlus2DependentRootMismatch(t *testing.T) {
	ctx := context.Background()
	s, _, signedPreferences := setupSignedProposerPreferencesService(t)
	s.newSignedProposerPreferencesVerifier = testNewSignedProposerPreferencesVerifier(mockSignedProposerPreferencesVerifier{})
	signedPreferences.Message.ProposalSlot = primitives.Slot(64)
	msg := signedProposerPreferencesToPubsub(t, s, s.cfg.p2p, signedPreferences)

	var called bool
	var gotRoot [32]byte
	var gotEpoch primitives.Epoch
	expectedRoot := [32]byte{0xaa}
	s.cfg.chain.(*mock.ChainService).DependentRootCB = func(root [32]byte, epoch primitives.Epoch) ([32]byte, error) {
		called = true
		gotRoot = root
		gotEpoch = epoch
		return expectedRoot, nil
	}

	result, err := s.validateSignedProposerPreferencesGossip(ctx, "", msg)
	require.ErrorContains(t, "dependent_root", err)
	require.Equal(t, pubsub.ValidationIgnore, result)
	require.Equal(t, true, called)
	require.Equal(t, [32]byte{}, gotRoot)
	require.Equal(t, primitives.Epoch(1), gotEpoch)
}

func TestValidateSignedProposerPreferencesGossip_HappyPath(t *testing.T) {
	ctx := context.Background()
	s, msg, signedPreferences := setupSignedProposerPreferencesService(t)
	s.newSignedProposerPreferencesVerifier = testNewSignedProposerPreferencesVerifier(mockSignedProposerPreferencesVerifier{})

	s.proposerPreferencesCache.Clear()
	result, err := s.validateSignedProposerPreferencesGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, result)

	dependentRoot := bytesutil.ToBytes32(signedPreferences.Message.DependentRoot)
	got, ok := s.proposerPreferencesCache.Get(dependentRoot, signedPreferences.Message.ProposalSlot)
	require.Equal(t, true, ok)
	require.DeepEqual(t, signedPreferences.Message.FeeRecipient, got.FeeRecipient[:])
	require.Equal(t, signedPreferences.Message.TargetGasLimit, got.TargetGasLimit)
	validatorData, ok := msg.ValidatorData.(*ethpb.SignedProposerPreferences)
	require.Equal(t, true, ok)
	require.DeepEqual(t, signedPreferences, validatorData)
}

func TestSignedProposerPreferencesSubscriber_WrongMessage(t *testing.T) {
	s := &Service{}
	err := s.signedProposerPreferencesSubscriber(context.Background(), &ethpb.BeaconBlock{})
	require.ErrorIs(t, errWrongMessage, err)
}

func TestSignedProposerPreferencesSubscriber_HappyPath(t *testing.T) {
	s := &Service{
		cfg: &config{operationNotifier: &mock.MockOperationNotifier{}},
	}
	err := s.signedProposerPreferencesSubscriber(context.Background(), &ethpb.SignedProposerPreferences{})
	require.NoError(t, err)
}

type mockSignedProposerPreferencesVerifier struct {
	errCurrentOrNextEpoch error
	errDependentRootSeen  error
	errValidProposalSlot  error
	errSignature          error
	lastStateSlot         primitives.Slot
}

var _ verification.SignedProposerPreferencesVerifier = &mockSignedProposerPreferencesVerifier{}

func (m *mockSignedProposerPreferencesVerifier) VerifyCurrentOrNextEpoch() error {
	return m.errCurrentOrNextEpoch
}

func (m *mockSignedProposerPreferencesVerifier) VerifyDependentRootSeen(func([32]byte) bool) error {
	return m.errDependentRootSeen
}

func (m *mockSignedProposerPreferencesVerifier) VerifyValidProposalSlot(st state.ReadOnlyBeaconState) error {
	if st != nil {
		m.lastStateSlot = st.Slot()
	}
	return m.errValidProposalSlot
}

func (m *mockSignedProposerPreferencesVerifier) VerifySignature(st state.ReadOnlyBeaconState) error {
	if st != nil {
		m.lastStateSlot = st.Slot()
	}
	return m.errSignature
}

func (*mockSignedProposerPreferencesVerifier) SatisfyRequirement(verification.Requirement) {}

func testNewSignedProposerPreferencesVerifier(m mockSignedProposerPreferencesVerifier) verification.NewSignedProposerPreferencesVerifier {
	return func(*ethpb.SignedProposerPreferences, []verification.Requirement) verification.SignedProposerPreferencesVerifier {
		clone := m
		return &clone
	}
}

// setupSignedProposerPreferencesService wires a sync Service with a real DB and
// stategen, a saved block whose HashTreeRoot is used as the checkpoint root,
// and a saved post-state for that block — so the gossip validator can resolve
// the checkpoint state.
func setupSignedProposerPreferencesService(t *testing.T) (*Service, *pubsub.Message, *ethpb.SignedProposerPreferences) {
	t.Helper()

	ctx := context.Background()
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	st, err := util.NewBeaconStateGloas()
	require.NoError(t, err)

	sb := util.NewBeaconBlockGloas()
	signedBlock, err := blocks.NewSignedBeaconBlock(sb)
	require.NoError(t, err)
	dependentRoot, err := signedBlock.Block().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, signedBlock))
	require.NoError(t, db.SaveState(ctx, st, dependentRoot))

	chainService := &mock.ChainService{
		Genesis:    time.Now(),
		DB:         db,
		State:      st,
		TargetRoot: dependentRoot,
		ForkchoiceRoots: map[[32]byte]bool{
			dependentRoot: true,
		},
	}

	stateGen := stategen.New(db, doublylinkedtree.New())

	s := &Service{
		proposerPreferencesCache:             cache.NewProposerPreferencesCache(),
		newSignedProposerPreferencesVerifier: testNewSignedProposerPreferencesVerifier(mockSignedProposerPreferencesVerifier{}),
		cfg: &config{
			p2p:         p,
			initialSync: &mockSync.Sync{},
			chain:       chainService,
			beaconDB:    db,
			stateGen:    stateGen,
			clock:       startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
		},
	}
	// ProposalSlot is in epoch 1 so the gossip validator's checkpoint epoch
	// (epoch(slot)-1) is 0, with boundary at slot 0. With genesis "now" the
	// wall-clock current slot is 0, so the proposal is in the next epoch and
	// has not yet passed.
	signedPreferences := &ethpb.SignedProposerPreferences{
		Message: &ethpb.ProposerPreferences{
			DependentRoot:  dependentRoot[:],
			ProposalSlot:   33,
			ValidatorIndex: 0,
			FeeRecipient:   bytes.Repeat([]byte{0x01}, 20),
			TargetGasLimit: 30_000_000,
		},
		Signature: bytes.Repeat([]byte{0x02}, 96),
	}
	msg := signedProposerPreferencesToPubsub(t, s, p, signedPreferences)
	return s, msg, signedPreferences
}

func signedProposerPreferencesToPubsub(t *testing.T, s *Service, p p2p.P2P, preferences *ethpb.SignedProposerPreferences) *pubsub.Message {
	t.Helper()

	buf := new(bytes.Buffer)
	_, err := p.Encoding().EncodeGossip(buf, preferences)
	require.NoError(t, err)
	digest, err := s.currentForkDigest()
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*ethpb.SignedProposerPreferences]()]
	topic = s.addDigestToTopic(topic, digest)
	return &pubsub.Message{
		Message: &pb.Message{
			Topic: &topic,
			Data:  buf.Bytes(),
		},
	}
}
