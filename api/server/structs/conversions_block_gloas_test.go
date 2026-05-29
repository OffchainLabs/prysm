package structs

import (
	"bytes"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
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

func testWireBlindedProto() *eth.WireBlindedExecutionPayloadEnvelope {
	return &eth.WireBlindedExecutionPayloadEnvelope{
		PayloadRoot:           fillByteSlice(32, 0x55),
		ExecutionRequests:     &enginev1.ExecutionRequests{},
		BuilderIndex:          7,
		BeaconBlockRoot:       fillByteSlice(32, 0x33),
		ParentBeaconBlockRoot: fillByteSlice(32, 0x44),
	}
}

// HTR(blinded) must equal HTR(full) so the validator signature stays valid against either form.
func TestWireBlindedHTRMatchesFull(t *testing.T) {
	full := &eth.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadGloas{
			ParentHash:    fillByteSlice(32, 0x01),
			FeeRecipient:  fillByteSlice(20, 0x02),
			StateRoot:     fillByteSlice(32, 0x03),
			ReceiptsRoot:  fillByteSlice(32, 0x04),
			LogsBloom:     fillByteSlice(256, 0x05),
			PrevRandao:    fillByteSlice(32, 0x06),
			BaseFeePerGas: fillByteSlice(32, 0x07),
			BlockHash:     fillByteSlice(32, 0x08),
			Transactions:  [][]byte{[]byte("tx1"), []byte("tx2")},
			Withdrawals:   []*enginev1.Withdrawal{},
			SlotNumber:    primitives.Slot(100),
		},
		ExecutionRequests:     &enginev1.ExecutionRequests{},
		BuilderIndex:          primitives.BuilderIndex(42),
		BeaconBlockRoot:       fillByteSlice(32, 0x09),
		ParentBeaconBlockRoot: fillByteSlice(32, 0x0a),
	}

	blinded, err := WireBlindedFromFull(full)
	require.NoError(t, err)
	fullHTR, err := full.HashTreeRoot()
	require.NoError(t, err)
	blindedHTR, err := blinded.HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, fullHTR, blindedHTR)

	// SSZ roundtrip.
	enc, err := blinded.MarshalSSZ()
	require.NoError(t, err)
	decoded := &eth.WireBlindedExecutionPayloadEnvelope{}
	require.NoError(t, decoded.UnmarshalSSZ(enc))
	rtHTR, err := decoded.HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, fullHTR, rtHTR)

	// Signed wrapper SSZ roundtrip.
	signedBlinded, err := SignedWireBlindedFromFull(&eth.SignedExecutionPayloadEnvelope{
		Message:   full,
		Signature: fillByteSlice(96, 0x0b),
	})
	require.NoError(t, err)
	signedEnc, err := signedBlinded.MarshalSSZ()
	require.NoError(t, err)
	decodedSigned := &eth.SignedWireBlindedExecutionPayloadEnvelope{}
	require.NoError(t, decodedSigned.UnmarshalSSZ(signedEnc))
	rtBlindedHTR, err := decodedSigned.Message.HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, fullHTR, rtBlindedHTR)
}

func TestBlindedExecutionPayloadEnvelopeFromConsensus(t *testing.T) {
	b := testWireBlindedProto()
	result, err := BlindedExecutionPayloadEnvelopeFromConsensus(b)
	require.NoError(t, err)
	require.Equal(t, hexutil.Encode(b.PayloadRoot), result.PayloadRoot)
	require.Equal(t, "7", result.BuilderIndex)
	require.Equal(t, hexutil.Encode(b.BeaconBlockRoot), result.BeaconBlockRoot)
	require.Equal(t, hexutil.Encode(b.ParentBeaconBlockRoot), result.ParentBeaconBlockRoot)
	require.NotNil(t, result.ExecutionRequests)
}

func TestBlindedExecutionPayloadEnvelopeFromConsensus_Nil(t *testing.T) {
	_, err := BlindedExecutionPayloadEnvelopeFromConsensus(nil)
	require.NotNil(t, err)
}

func TestBlindedExecutionPayloadEnvelope_ToConsensusRoundTrip(t *testing.T) {
	b := testWireBlindedProto()
	api, err := BlindedExecutionPayloadEnvelopeFromConsensus(b)
	require.NoError(t, err)
	back, err := api.ToConsensus()
	require.NoError(t, err)
	require.DeepEqual(t, b.PayloadRoot, back.PayloadRoot)
	require.Equal(t, b.BuilderIndex, back.BuilderIndex)
	require.DeepEqual(t, b.BeaconBlockRoot, back.BeaconBlockRoot)
	require.DeepEqual(t, b.ParentBeaconBlockRoot, back.ParentBeaconBlockRoot)
	require.NotNil(t, back.ExecutionRequests)
}

func TestSignedBlindedExecutionPayloadEnvelope_ToConsensus(t *testing.T) {
	msg, err := BlindedExecutionPayloadEnvelopeFromConsensus(testWireBlindedProto())
	require.NoError(t, err)
	sig := fillByteSlice(96, 0x66)
	signed := &SignedBlindedExecutionPayloadEnvelope{Message: msg, Signature: hexutil.Encode(sig)}
	result, err := signed.ToConsensus()
	require.NoError(t, err)
	require.NotNil(t, result.Message)
	require.DeepEqual(t, sig, result.Signature)
}

func TestSignedBlindedExecutionPayloadEnvelope_ToConsensus_BadSignature(t *testing.T) {
	msg, err := BlindedExecutionPayloadEnvelopeFromConsensus(testWireBlindedProto())
	require.NoError(t, err)
	signed := &SignedBlindedExecutionPayloadEnvelope{Message: msg, Signature: "0xdead"}
	_, err = signed.ToConsensus()
	require.NotNil(t, err)
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
