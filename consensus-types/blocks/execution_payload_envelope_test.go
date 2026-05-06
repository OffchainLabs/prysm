package blocks_test

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	consensus_types "github.com/OffchainLabs/prysm/v7/consensus-types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func validExecutionPayloadEnvelope() *ethpb.ExecutionPayloadEnvelope {
	payload := &enginev1.ExecutionPayloadGloas{
		ParentHash:    bytes.Repeat([]byte{0x01}, 32),
		FeeRecipient:  bytes.Repeat([]byte{0x02}, 20),
		StateRoot:     bytes.Repeat([]byte{0x03}, 32),
		ReceiptsRoot:  bytes.Repeat([]byte{0x04}, 32),
		LogsBloom:     bytes.Repeat([]byte{0x05}, 256),
		PrevRandao:    bytes.Repeat([]byte{0x06}, 32),
		BlockNumber:   1,
		GasLimit:      2,
		GasUsed:       3,
		Timestamp:     4,
		BaseFeePerGas: bytes.Repeat([]byte{0x07}, 32),
		BlockHash:     bytes.Repeat([]byte{0x08}, 32),
		Transactions:  [][]byte{},
		Withdrawals:   []*enginev1.Withdrawal{},
		BlobGasUsed:   0,
		ExcessBlobGas: 0,
		SlotNumber:    9,
	}

	return &ethpb.ExecutionPayloadEnvelope{
		Payload: payload,
		ExecutionRequests: &enginev1.ExecutionRequests{
			Deposits: []*enginev1.DepositRequest{
				{
					Pubkey:                bytes.Repeat([]byte{0x09}, 48),
					WithdrawalCredentials: bytes.Repeat([]byte{0x0A}, 32),
					Signature:             bytes.Repeat([]byte{0x0B}, 96),
				},
			},
		},
		BuilderIndex:          10,
		BeaconBlockRoot:       bytes.Repeat([]byte{0xAA}, 32),
		ParentBeaconBlockRoot: bytes.Repeat([]byte{0xCC}, 32),
	}
}

func TestWrappedROExecutionPayloadEnvelope(t *testing.T) {
	t.Run("returns error on nil payload", func(t *testing.T) {
		invalid := validExecutionPayloadEnvelope()
		invalid.Payload = nil
		_, err := blocks.WrappedROExecutionPayloadEnvelope(invalid)
		require.Equal(t, consensus_types.ErrNilObjectWrapped, err)
	})

	t.Run("returns error on invalid beacon root length", func(t *testing.T) {
		invalid := validExecutionPayloadEnvelope()
		invalid.BeaconBlockRoot = []byte{0x01}
		_, err := blocks.WrappedROExecutionPayloadEnvelope(invalid)
		require.Equal(t, consensus_types.ErrNilObjectWrapped, err)
	})

	t.Run("wraps and exposes fields", func(t *testing.T) {
		env := validExecutionPayloadEnvelope()
		wrapped, err := blocks.WrappedROExecutionPayloadEnvelope(env)
		require.NoError(t, err)

		require.Equal(t, primitives.BuilderIndex(10), wrapped.BuilderIndex())
		require.Equal(t, primitives.Slot(9), wrapped.Slot())
		assert.DeepEqual(t, [32]byte(bytes.Repeat([]byte{0xAA}, 32)), wrapped.BeaconBlockRoot())

		reqs := wrapped.ExecutionRequests()
		require.NotNil(t, reqs)
		if len(reqs.Deposits) > 0 {
			reqs.Deposits[0].Pubkey[0] = 0xFF
			require.NotEqual(t, reqs.Deposits[0].Pubkey[0], env.ExecutionRequests.Deposits[0].Pubkey[0])
		}

		exec, err := wrapped.Execution()
		require.NoError(t, err)
		assert.DeepEqual(t, env.Payload.ParentHash, exec.ParentHash())

		require.Equal(t, false, wrapped.IsBlinded())
	})
}

func TestWrappedROSignedExecutionPayloadEnvelope(t *testing.T) {
	t.Run("returns error for invalid signature length", func(t *testing.T) {
		signed := &ethpb.SignedExecutionPayloadEnvelope{
			Message:   validExecutionPayloadEnvelope(),
			Signature: bytes.Repeat([]byte{0xAA}, 95),
		}
		_, err := blocks.WrappedROSignedExecutionPayloadEnvelope(signed)
		require.Equal(t, consensus_types.ErrNilObjectWrapped, err)
	})

	t.Run("returns error on nil envelope", func(t *testing.T) {
		_, err := blocks.WrappedROSignedExecutionPayloadEnvelope(nil)
		require.Equal(t, consensus_types.ErrNilObjectWrapped, err)
	})

	t.Run("wraps and provides envelope/signing data", func(t *testing.T) {
		sig := bytes.Repeat([]byte{0xAB}, 96)
		signed := &ethpb.SignedExecutionPayloadEnvelope{
			Message:   validExecutionPayloadEnvelope(),
			Signature: sig,
		}

		wrapped, err := blocks.WrappedROSignedExecutionPayloadEnvelope(signed)
		require.NoError(t, err)

		gotSig := wrapped.Signature()
		assert.DeepEqual(t, [96]byte(sig), gotSig)

		env, err := wrapped.Envelope()
		require.NoError(t, err)
		assert.DeepEqual(t, [32]byte(bytes.Repeat([]byte{0xAA}, 32)), env.BeaconBlockRoot())

		domain := bytes.Repeat([]byte{0xCC}, 32)
		wantRoot, err := signing.ComputeSigningRoot(signed.Message, domain)
		require.NoError(t, err)
		gotRoot, err := wrapped.SigningRoot(domain)
		require.NoError(t, err)
		require.Equal(t, wantRoot, gotRoot)

		require.Equal(t, signed, wrapped.Proto())
	})
}
