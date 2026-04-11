package sync

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
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

func TestValidateSignedProposerPreferencesGossip_ErrorPathsWithMock(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		verifier  mockSignedProposerPreferencesVerifier
		result    pubsub.ValidationResult
		wantError bool
	}{
		{
			name:      "not next epoch",
			verifier:  mockSignedProposerPreferencesVerifier{errNextEpoch: errors.New("wrong epoch")},
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

func TestValidateSignedProposerPreferencesGossip_AlreadySeenSlot(t *testing.T) {
	ctx := context.Background()
	s, msg, signedPreferences := setupSignedProposerPreferencesService(t)
	s.newSignedProposerPreferencesVerifier = testNewSignedProposerPreferencesVerifier(mockSignedProposerPreferencesVerifier{})

	require.Equal(t, true, s.proposerPreferencesCache.Add(signedPreferences.Message.ProposalSlot, []byte{0x01}, 10))
	result, err := s.validateSignedProposerPreferencesGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, result)
}

func TestValidateSignedProposerPreferencesGossip_HappyPath(t *testing.T) {
	ctx := context.Background()
	s, msg, signedPreferences := setupSignedProposerPreferencesService(t)
	s.newSignedProposerPreferencesVerifier = testNewSignedProposerPreferencesVerifier(mockSignedProposerPreferencesVerifier{})

	s.proposerPreferencesCache.Clear()
	result, err := s.validateSignedProposerPreferencesGossip(ctx, "", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, result)

	got, ok := s.proposerPreferencesCache.Get(signedPreferences.Message.ProposalSlot)
	require.Equal(t, true, ok)
	require.DeepEqual(t, signedPreferences.Message.FeeRecipient, got.FeeRecipient)
	require.Equal(t, signedPreferences.Message.GasLimit, got.GasLimit)
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
	s := &Service{}
	err := s.signedProposerPreferencesSubscriber(context.Background(), &ethpb.SignedProposerPreferences{})
	require.NoError(t, err)
}

type mockSignedProposerPreferencesVerifier struct {
	errNextEpoch         error
	errValidProposalSlot error
	errSignature         error
}

var _ verification.SignedProposerPreferencesVerifier = &mockSignedProposerPreferencesVerifier{}

func (m *mockSignedProposerPreferencesVerifier) VerifyNextEpoch(state.ReadOnlyBeaconState) error {
	return m.errNextEpoch
}

func (m *mockSignedProposerPreferencesVerifier) VerifyValidProposalSlot(state.ReadOnlyBeaconState) error {
	return m.errValidProposalSlot
}

func (m *mockSignedProposerPreferencesVerifier) VerifySignature(state.ReadOnlyBeaconState) error {
	return m.errSignature
}

func (*mockSignedProposerPreferencesVerifier) SatisfyRequirement(verification.Requirement) {}

func testNewSignedProposerPreferencesVerifier(m mockSignedProposerPreferencesVerifier) verification.NewSignedProposerPreferencesVerifier {
	return func(*ethpb.SignedProposerPreferences, []verification.Requirement) verification.SignedProposerPreferencesVerifier {
		clone := m
		return &clone
	}
}

func setupSignedProposerPreferencesService(t *testing.T) (*Service, *pubsub.Message, *ethpb.SignedProposerPreferences) {
	t.Helper()

	p := p2ptest.NewTestP2P(t)
	st, err := util.NewBeaconStateGloas()
	require.NoError(t, err)
	chainService := &mock.ChainService{
		Genesis: time.Now(),
		State:   st,
	}
	s := &Service{
		proposerPreferencesCache: cache.NewProposerPreferencesCache(),
		cfg: &config{
			p2p:         p,
			initialSync: &mockSync.Sync{},
			chain:       chainService,
			clock:       startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
		},
	}
	signedPreferences := &ethpb.SignedProposerPreferences{
		Message: &ethpb.ProposerPreferences{
			ProposalSlot:   primitives.Slot(1),
			ValidatorIndex: 2,
			FeeRecipient:   bytes.Repeat([]byte{0x01}, 20),
			GasLimit:       30_000_000,
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

	topic := p2p.GossipTypeMapping[reflect.TypeFor[*ethpb.SignedProposerPreferences]()]
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
