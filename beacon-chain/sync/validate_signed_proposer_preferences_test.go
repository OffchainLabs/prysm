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
	"github.com/OffchainLabs/prysm/v7/config/params"
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
	chainService.DependentRootCB = func(_ [32]byte, epoch primitives.Epoch) ([32]byte, error) {
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
		if !called {
			called, gotRoot, gotEpoch = true, root, epoch
		}
		return expectedRoot, nil
	}

	result, err := s.validateSignedProposerPreferencesGossip(ctx, "", msg)
	require.ErrorContains(t, "dependent_root", err)
	require.Equal(t, pubsub.ValidationIgnore, result)
	require.Equal(t, true, called)
	require.Equal(t, [32]byte{}, gotRoot)
	require.Equal(t, primitives.Epoch(1), gotEpoch)
}

func TestValidateSignedProposerPreferencesGossip_PreGloasProposalEpoch(t *testing.T) {
	ctx := context.Background()
	s, msg, _ := setupSignedProposerPreferencesService(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 2
	params.OverrideBeaconConfig(cfg)

	// The proposal slot is in epoch 1, before the fork.
	result, err := s.validateSignedProposerPreferencesGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)
}

func TestValidateSignedProposerPreferencesGossip_DependentBlockAfterShufflingSlot(t *testing.T) {
	ctx := context.Background()
	s, msg, _ := setupSignedProposerPreferencesService(t)

	// The proposal slot is in epoch 1, whose shuffling dependent slot is the genesis slot.
	s.cfg.chain.(*mock.ChainService).BlockSlot = 1

	result, err := s.validateSignedProposerPreferencesGossip(ctx, "", msg)
	require.ErrorContains(t, "after shuffling dependent slot", err)
	require.Equal(t, pubsub.ValidationReject, result)
}

func TestValidateSignedProposerPreferencesGossip_DependentRootOnOtherBranch(t *testing.T) {
	for _, tc := range []struct {
		name       string
		hasChild   bool
		cached     bool
		wantResult pubsub.ValidationResult
		wantError  string
	}{
		{name: "non-head leaf", cached: true, wantResult: pubsub.ValidationIgnore, wantError: "not a possible dependent block"},
		{name: "child across boundary without cached state", hasChild: true, wantResult: pubsub.ValidationIgnore, wantError: "is not cached"},
		{name: "child across boundary with cached state", hasChild: true, cached: true, wantResult: pubsub.ValidationAccept},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			s, _, signedPreferences := setupSignedProposerPreferencesService(t)
			chainService := s.cfg.chain.(*mock.ChainService)
			chainService.HeadStateErr = errors.New("head state should not load")
			tips, _ := chainService.ChainHeads()
			dependentRoot := tips[0]
			signedPreferences.Message.DependentRoot = dependentRoot[:]
			msg := signedProposerPreferencesToPubsub(t, s, s.cfg.p2p, signedPreferences)
			chainService.DependentRootCB = func(root [32]byte, _ primitives.Epoch) ([32]byte, error) {
				if root == dependentRoot || (tc.hasChild && root == tips[1]) {
					return dependentRoot, nil
				}
				return [32]byte{0xbb}, nil
			}
			if tc.cached {
				require.NoError(t, s.cfg.stateGen.SaveState(ctx, dependentRoot, chainService.State))
			}

			result, err := s.validateSignedProposerPreferencesGossip(ctx, "", msg)
			if tc.wantError != "" {
				require.ErrorContains(t, tc.wantError, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantResult, result)
		})
	}
}

// TestValidateSignedProposerPreferencesGossip_SignatureBeforeBoundaryAdvance
// pins the order of the two state-dependent checks when the proposal is two
// epochs past the dependent state, the only case where the validator advances
// the state across an epoch boundary. The signature must be verified against
// the un-advanced state so a forged message is rejected without paying for the
// epoch transition, and the proposal-slot check must still see the advanced
// state when the signature is valid.
func TestValidateSignedProposerPreferencesGossip_SignatureBeforeBoundaryAdvance(t *testing.T) {
	// Head state is at slot 0 (epoch 0); a proposal in epoch 2 makes the
	// validator advance to the epoch 1 boundary at slot 32 before the slot check.
	const proposalSlot = primitives.Slot(64)
	boundarySlot := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)

	t.Run("invalid signature is rejected before the state is advanced", func(t *testing.T) {
		ctx := t.Context()
		s, _, signedPreferences := setupSignedProposerPreferencesService(t)
		newVerifier, verifier := capturingSignedProposerPreferencesVerifier(
			mockSignedProposerPreferencesVerifier{errSignature: errors.New("bad signature")},
		)
		s.newSignedProposerPreferencesVerifier = newVerifier
		signedPreferences.Message.ProposalSlot = proposalSlot
		msg := signedProposerPreferencesToPubsub(t, s, s.cfg.p2p, signedPreferences)

		result, err := s.validateSignedProposerPreferencesGossip(ctx, "", msg)
		require.ErrorContains(t, "bad signature", err)
		require.Equal(t, pubsub.ValidationReject, result)
		require.Equal(t, primitives.Slot(0), verifier.signatureStateSlot, "signature must be checked against the un-advanced state")
		require.Equal(t, false, verifier.proposalSlotCalled, "proposal slot check must not run after a signature failure")
	})

	t.Run("valid signature advances the state for the proposal slot check", func(t *testing.T) {
		ctx := t.Context()
		s, _, signedPreferences := setupSignedProposerPreferencesService(t)
		// The empty test state cannot cross an epoch boundary; use a real genesis state.
		headState, _ := util.DeterministicGenesisStateGloas(t, 64)
		s.cfg.chain.(*mock.ChainService).State = headState
		newVerifier, verifier := capturingSignedProposerPreferencesVerifier(mockSignedProposerPreferencesVerifier{})
		s.newSignedProposerPreferencesVerifier = newVerifier
		signedPreferences.Message.ProposalSlot = proposalSlot
		msg := signedProposerPreferencesToPubsub(t, s, s.cfg.p2p, signedPreferences)

		result, err := s.validateSignedProposerPreferencesGossip(ctx, "", msg)
		require.NoError(t, err)
		require.Equal(t, pubsub.ValidationAccept, result)
		require.Equal(t, primitives.Slot(0), verifier.signatureStateSlot, "signature must be checked against the un-advanced state")
		require.Equal(t, true, verifier.proposalSlotCalled)
		require.Equal(t, boundarySlot, verifier.proposalSlotStateSlot, "proposal slot must be checked against the boundary state")
	})
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

type mockSignedProposerPreferencesVerifier struct {
	errCurrentOrNextEpoch error
	errDependentRootSeen  error
	errValidProposalSlot  error
	errSignature          error

	// Slot of the state each check received, so tests can assert which
	// checks ran against the un-advanced state and which ran after the
	// boundary advance.
	signatureStateSlot    primitives.Slot
	proposalSlotStateSlot primitives.Slot
	proposalSlotCalled    bool
}

var _ verification.SignedProposerPreferencesVerifier = &mockSignedProposerPreferencesVerifier{}

func (m *mockSignedProposerPreferencesVerifier) VerifyCurrentOrNextEpoch() error {
	return m.errCurrentOrNextEpoch
}

func (m *mockSignedProposerPreferencesVerifier) VerifyDependentRootSeen(func([32]byte) bool) error {
	return m.errDependentRootSeen
}

func (m *mockSignedProposerPreferencesVerifier) VerifyValidProposalSlot(st state.ReadOnlyBeaconState) error {
	m.proposalSlotCalled = true
	if st != nil {
		m.proposalSlotStateSlot = st.Slot()
	}
	return m.errValidProposalSlot
}

func (m *mockSignedProposerPreferencesVerifier) VerifySignature(st state.ReadOnlyBeaconState) error {
	if st != nil {
		m.signatureStateSlot = st.Slot()
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

// capturingSignedProposerPreferencesVerifier returns a constructor that hands
// the validator a single mock instance and also returns that instance, so the
// test can inspect which checks ran and with which state after validation.
func capturingSignedProposerPreferencesVerifier(m mockSignedProposerPreferencesVerifier) (verification.NewSignedProposerPreferencesVerifier, *mockSignedProposerPreferencesVerifier) {
	captured := &m
	return func(*ethpb.SignedProposerPreferences, []verification.Requirement) verification.SignedProposerPreferencesVerifier {
		return captured
	}, captured
}

// setupSignedProposerPreferencesService wires a sync Service with a real DB and
// stategen, a saved block whose HashTreeRoot is used as the checkpoint root,
// and a saved post-state for that block — so the gossip validator can resolve
// the checkpoint state.
func setupSignedProposerPreferencesService(t *testing.T) (*Service, *pubsub.Message, *ethpb.SignedProposerPreferences) {
	t.Helper()

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()

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
	digest := s.currentForkDigest()
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*ethpb.SignedProposerPreferences]()]
	topic = s.addDigestToTopic(topic, digest)
	return &pubsub.Message{
		Message: &pb.Message{
			Topic: &topic,
			Data:  buf.Bytes(),
		},
	}
}
