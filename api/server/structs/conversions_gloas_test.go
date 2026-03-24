package structs

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestSignedProposerPreferences_ToConsensus_NilMessage(t *testing.T) {
	s := &SignedProposerPreferences{Message: nil, Signature: ""}
	_, err := s.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestSignedProposerPreferences_ToConsensus(t *testing.T) {
	feeRecipient := bytesutil.PadTo([]byte{0xaa, 0xbb}, 20)
	sig := bytesutil.PadTo([]byte{0xcc}, 96)

	s := &SignedProposerPreferences{
		Message: &ProposerPreferences{
			ProposalSlot:   "32",
			ValidatorIndex: "5",
			FeeRecipient:   hexutil.Encode(feeRecipient),
			GasLimit:       "30000000",
		},
		Signature: hexutil.Encode(sig),
	}

	result, err := s.ToConsensus()
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(32), result.Message.ProposalSlot)
	assert.Equal(t, primitives.ValidatorIndex(5), result.Message.ValidatorIndex)
	assert.DeepEqual(t, feeRecipient, result.Message.FeeRecipient)
	assert.Equal(t, uint64(30000000), result.Message.GasLimit)
	assert.DeepEqual(t, sig, result.Signature)
}

func TestProposerPreferences_ToConsensus_InvalidSlot(t *testing.T) {
	p := &ProposerPreferences{
		ProposalSlot:   "not_a_number",
		ValidatorIndex: "5",
		FeeRecipient:   hexutil.Encode(make([]byte, 20)),
		GasLimit:       "30000000",
	}
	_, err := p.ToConsensus()
	require.ErrorContains(t, "ProposalSlot", err)
}

func TestProposerPreferences_ToConsensus_InvalidFeeRecipient(t *testing.T) {
	p := &ProposerPreferences{
		ProposalSlot:   "32",
		ValidatorIndex: "5",
		FeeRecipient:   "0xinvalid",
		GasLimit:       "30000000",
	}
	_, err := p.ToConsensus()
	require.ErrorContains(t, "FeeRecipient", err)
}

func TestSignedProposerPreferencesFromConsensus(t *testing.T) {
	feeRecipient := bytesutil.PadTo([]byte{0xaa, 0xbb}, 20)
	sig := bytesutil.PadTo([]byte{0xcc}, 96)

	sp := &eth.SignedProposerPreferences{
		Message: &eth.ProposerPreferences{
			ProposalSlot:   32,
			ValidatorIndex: 5,
			FeeRecipient:   feeRecipient,
			GasLimit:       30000000,
		},
		Signature: sig,
	}

	result := SignedProposerPreferencesFromConsensus(sp)
	assert.Equal(t, "32", result.Message.ProposalSlot)
	assert.Equal(t, "5", result.Message.ValidatorIndex)
	assert.Equal(t, hexutil.Encode(feeRecipient), result.Message.FeeRecipient)
	assert.Equal(t, "30000000", result.Message.GasLimit)
	assert.Equal(t, hexutil.Encode(sig), result.Signature)
}

func TestSignedProposerPreferences_RoundTrip(t *testing.T) {
	feeRecipient := bytesutil.PadTo([]byte{0xaa, 0xbb}, 20)
	sig := bytesutil.PadTo([]byte{0xcc}, 96)

	original := &eth.SignedProposerPreferences{
		Message: &eth.ProposerPreferences{
			ProposalSlot:   32,
			ValidatorIndex: 5,
			FeeRecipient:   feeRecipient,
			GasLimit:       30000000,
		},
		Signature: sig,
	}

	jsonStruct := SignedProposerPreferencesFromConsensus(original)
	roundTripped, err := jsonStruct.ToConsensus()
	require.NoError(t, err)

	assert.Equal(t, original.Message.ProposalSlot, roundTripped.Message.ProposalSlot)
	assert.Equal(t, original.Message.ValidatorIndex, roundTripped.Message.ValidatorIndex)
	assert.DeepEqual(t, original.Message.FeeRecipient, roundTripped.Message.FeeRecipient)
	assert.Equal(t, original.Message.GasLimit, roundTripped.Message.GasLimit)
	assert.DeepEqual(t, original.Signature, roundTripped.Signature)
}
