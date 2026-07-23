package sync

import (
	"testing"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestSignedProposerPreferencesSubscriber_WrongMessage(t *testing.T) {
	s := &Service{cfg: &config{}}
	err := s.signedProposerPreferencesSubscriber(t.Context(), &ethpb.SignedVoluntaryExit{})
	require.ErrorIs(t, err, errWrongMessage)
}

func TestSignedProposerPreferencesSubscriber_NilMessage(t *testing.T) {
	s := &Service{cfg: &config{}}
	err := s.signedProposerPreferencesSubscriber(t.Context(), &ethpb.SignedProposerPreferences{})
	require.ErrorIs(t, err, errNilMessage)
}

func TestSignedProposerPreferencesSubscriber_Send(t *testing.T) {
	s := &Service{cfg: &config{operationNotifier: &mock.MockOperationNotifier{}}}
	msg := &ethpb.SignedProposerPreferences{
		Message: &ethpb.ProposerPreferences{
			DependentRoot:  make([]byte, fieldparams.RootLength),
			ProposalSlot:   32,
			ValidatorIndex: 7,
			FeeRecipient:   make([]byte, 20),
			TargetGasLimit: 30_000_000,
		},
		Signature: make([]byte, fieldparams.BLSSignatureLength),
	}
	require.NoError(t, s.signedProposerPreferencesSubscriber(t.Context(), msg))
}
