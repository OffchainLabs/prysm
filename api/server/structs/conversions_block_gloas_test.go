package structs

import (
	"bytes"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func testEnvelopeProto() *eth.ExecutionPayloadEnvelope {
	return &eth.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadGloas{
			ParentHash:    fillByteSlice(common.HashLength, 0xaa),
			FeeRecipient:  fillByteSlice(20, 0xbb),
			StateRoot:     fillByteSlice(32, 0xcc),
			ReceiptsRoot:  fillByteSlice(32, 0xdd),
			LogsBloom:     fillByteSlice(256, 0xee),
			PrevRandao:    fillByteSlice(32, 0xff),
			BaseFeePerGas: fillByteSlice(32, 0x11),
			BlockHash:     fillByteSlice(common.HashLength, 0x22),
			SlotNumber:    42,
		},
		ExecutionRequests:     &enginev1.ExecutionRequests{},
		BuilderIndex:          7,
		BeaconBlockRoot:       fillByteSlice(32, 0x33),
		ParentBeaconBlockRoot: fillByteSlice(32, 0x44),
	}
}

func TestExecutionPayloadEnvelopeFromConsensus(t *testing.T) {
	env := testEnvelopeProto()
	result, err := ExecutionPayloadEnvelopeFromConsensus(env)
	require.NoError(t, err)
	require.NotNil(t, result.Payload)
	require.Equal(t, hexutil.Encode(env.Payload.ParentHash), result.Payload.ParentHash)
	require.Equal(t, "7", result.BuilderIndex)
	require.Equal(t, hexutil.Encode(env.BeaconBlockRoot), result.BeaconBlockRoot)
	require.Equal(t, hexutil.Encode(env.ParentBeaconBlockRoot), result.ParentBeaconBlockRoot)
	require.Equal(t, "42", result.Payload.SlotNumber)
	require.NotNil(t, result.ExecutionRequests)
}

func TestExecutionPayloadEnvelopeFromConsensus_NilRequests(t *testing.T) {
	env := testEnvelopeProto()
	env.ExecutionRequests = nil
	result, err := ExecutionPayloadEnvelopeFromConsensus(env)
	require.NoError(t, err)
	require.Equal(t, (*ExecutionRequests)(nil), result.ExecutionRequests)
}

func TestBlockContentsGloasFromConsensus(t *testing.T) {
	block := util.NewBeaconBlockGloas().Block
	env := testEnvelopeProto()
	proofs := [][]byte{bytes.Repeat([]byte{0x11}, 48)}
	blobs := [][]byte{bytes.Repeat([]byte{0x22}, fieldparams.BlobSize)}

	result, err := BlockContentsGloasFromConsensus(block, env, proofs, blobs)
	require.NoError(t, err)
	require.NotNil(t, result.Block)
	require.NotNil(t, result.Block.Body)
	require.NotNil(t, result.ExecutionPayloadEnvelope)
	require.Equal(t, hexutil.Encode(env.BeaconBlockRoot), result.ExecutionPayloadEnvelope.BeaconBlockRoot)
	require.Equal(t, 1, len(result.KzgProofs))
	require.Equal(t, hexutil.Encode(proofs[0]), result.KzgProofs[0])
	require.Equal(t, 1, len(result.Blobs))
	require.Equal(t, hexutil.Encode(blobs[0]), result.Blobs[0])
}

func validProposerPreferences() *ProposerPreferences {
	return &ProposerPreferences{
		DependentRoot:  hexutil.Encode(bytes.Repeat([]byte{0xcc}, fieldparams.RootLength)),
		ProposalSlot:   "32",
		ValidatorIndex: "2",
		FeeRecipient:   hexutil.Encode(bytes.Repeat([]byte{0xab}, 20)),
		TargetGasLimit: "30000000",
	}
}

func TestSignedProposerPreferences_ToConsensus_NilMessage(t *testing.T) {
	s := &SignedProposerPreferences{Message: nil, Signature: ""}
	_, err := s.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestSignedProposerPreferences_ToConsensus_NilReceiver(t *testing.T) {
	var s *SignedProposerPreferences
	_, err := s.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestSignedProposerPreferences_ToConsensus_BadSignature(t *testing.T) {
	s := &SignedProposerPreferences{Message: validProposerPreferences(), Signature: "0xnothex"}
	_, err := s.ToConsensus()
	require.ErrorContains(t, "Signature", err)
}

func TestSignedProposerPreferences_ToConsensus_OK(t *testing.T) {
	sig := hexutil.Encode(bytes.Repeat([]byte{0x01}, fieldparams.BLSSignatureLength))
	s := &SignedProposerPreferences{Message: validProposerPreferences(), Signature: sig}
	out, err := s.ToConsensus()
	require.NoError(t, err)
	require.Equal(t, uint64(30_000_000), out.Message.TargetGasLimit)
	require.Equal(t, uint64(32), uint64(out.Message.ProposalSlot))
	require.Equal(t, uint64(2), uint64(out.Message.ValidatorIndex))
	require.Equal(t, fieldparams.BLSSignatureLength, len(out.Signature))
	require.Equal(t, 20, len(out.Message.FeeRecipient))
	require.Equal(t, fieldparams.RootLength, len(out.Message.DependentRoot))
}

func TestProposerPreferences_ToConsensus_BadDependentRootHex(t *testing.T) {
	p := validProposerPreferences()
	p.DependentRoot = "0xnothex"
	_, err := p.ToConsensus()
	require.ErrorContains(t, "DependentRoot", err)
}

func TestProposerPreferences_ToConsensus_ShortDependentRoot(t *testing.T) {
	p := validProposerPreferences()
	p.DependentRoot = "0xcc"
	_, err := p.ToConsensus()
	require.ErrorContains(t, "DependentRoot", err)
}

func TestProposerPreferences_ToConsensus_BadProposalSlot(t *testing.T) {
	p := validProposerPreferences()
	p.ProposalSlot = "nope"
	_, err := p.ToConsensus()
	require.ErrorContains(t, "ProposalSlot", err)
}

func TestProposerPreferences_ToConsensus_BadValidatorIndex(t *testing.T) {
	p := validProposerPreferences()
	p.ValidatorIndex = "nope"
	_, err := p.ToConsensus()
	require.ErrorContains(t, "ValidatorIndex", err)
}

func TestProposerPreferences_ToConsensus_BadFeeRecipientHex(t *testing.T) {
	p := validProposerPreferences()
	p.FeeRecipient = "0xnothex"
	_, err := p.ToConsensus()
	require.ErrorContains(t, "FeeRecipient", err)
}

func TestProposerPreferences_ToConsensus_ShortFeeRecipient(t *testing.T) {
	p := validProposerPreferences()
	p.FeeRecipient = "0xab"
	_, err := p.ToConsensus()
	require.ErrorContains(t, "FeeRecipient", err)
}

func TestProposerPreferences_ToConsensus_BadTargetGasLimit(t *testing.T) {
	p := validProposerPreferences()
	p.TargetGasLimit = "nope"
	_, err := p.ToConsensus()
	require.ErrorContains(t, "TargetGasLimit", err)
}
