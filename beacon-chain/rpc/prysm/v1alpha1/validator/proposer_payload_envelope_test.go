package validator

import (
	"math/big"
	"testing"

	mockp2p "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestExtractExecutionPayloadDeneb(t *testing.T) {
	payload := &enginev1.ExecutionPayloadDeneb{
		ParentHash:    make([]byte, 32),
		FeeRecipient:  make([]byte, 20),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, 256),
		PrevRandao:    make([]byte, 32),
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     make([]byte, 32),
		ExtraData:     make([]byte, 0),
	}
	ed, err := consensusblocks.WrappedExecutionPayloadDeneb(payload)
	require.NoError(t, err)

	local := &consensusblocks.GetPayloadResponse{
		ExecutionData: ed,
		Bid:           big.NewInt(0),
	}

	result := extractExecutionPayloadDeneb(local)
	require.NotNil(t, result)
	require.DeepEqual(t, payload, result)
}

func TestExtractExecutionPayloadDeneb_Nil(t *testing.T) {
	require.Equal(t, true, extractExecutionPayloadDeneb(nil) == nil)
	require.Equal(t, true, extractExecutionPayloadDeneb(&consensusblocks.GetPayloadResponse{}) == nil)
}

func TestSetGetExecutionPayloadEnvelope(t *testing.T) {
	slot := primitives.Slot(42)
	builderIndex := primitives.BuilderIndex(7)

	envelope := &ethpb.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadDeneb{
			ParentHash:    make([]byte, 32),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, 32),
			ReceiptsRoot:  make([]byte, 32),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, 32),
			BaseFeePerGas: make([]byte, 32),
			BlockHash:     make([]byte, 32),
		},
		BuilderIndex:    builderIndex,
		BeaconBlockRoot: make([]byte, 32),
		Slot:            slot,
		StateRoot:       make([]byte, 32),
	}

	vs := &Server{}
	vs.setExecutionPayloadEnvelope(envelope)

	got, found := vs.getExecutionPayloadEnvelope(slot, builderIndex)
	require.Equal(t, true, found)
	require.DeepEqual(t, envelope, got)
}

func TestGetExecutionPayloadEnvelope_SlotMismatch(t *testing.T) {
	envelope := &ethpb.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadDeneb{
			ParentHash:    make([]byte, 32),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, 32),
			ReceiptsRoot:  make([]byte, 32),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, 32),
			BaseFeePerGas: make([]byte, 32),
			BlockHash:     make([]byte, 32),
		},
		BuilderIndex:    primitives.BuilderIndex(7),
		BeaconBlockRoot: make([]byte, 32),
		Slot:            42,
		StateRoot:       make([]byte, 32),
	}

	vs := &Server{}
	vs.setExecutionPayloadEnvelope(envelope)

	_, found := vs.getExecutionPayloadEnvelope(999, primitives.BuilderIndex(7))
	require.Equal(t, false, found)
}

func TestGetExecutionPayloadEnvelope_BuilderMismatch(t *testing.T) {
	envelope := &ethpb.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadDeneb{
			ParentHash:    make([]byte, 32),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, 32),
			ReceiptsRoot:  make([]byte, 32),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, 32),
			BaseFeePerGas: make([]byte, 32),
			BlockHash:     make([]byte, 32),
		},
		BuilderIndex:    primitives.BuilderIndex(7),
		BeaconBlockRoot: make([]byte, 32),
		Slot:            42,
		StateRoot:       make([]byte, 32),
	}

	vs := &Server{}
	vs.setExecutionPayloadEnvelope(envelope)

	_, found := vs.getExecutionPayloadEnvelope(42, primitives.BuilderIndex(99))
	require.Equal(t, false, found)
}

func TestGetExecutionPayloadEnvelope_Nil(t *testing.T) {
	vs := &Server{}
	_, found := vs.getExecutionPayloadEnvelope(1, primitives.BuilderIndex(0))
	require.Equal(t, false, found)
}

func TestGetExecutionPayloadEnvelopeRPC_NilRequest(t *testing.T) {
	vs := &Server{}
	_, err := vs.GetExecutionPayloadEnvelope(t.Context(), nil)
	require.ErrorContains(t, "request cannot be nil", err)
}

func TestGetExecutionPayloadEnvelopeRPC_PreFork(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 10
	params.OverrideBeaconConfig(cfg)

	vs := &Server{}
	_, err := vs.GetExecutionPayloadEnvelope(t.Context(), &ethpb.ExecutionPayloadEnvelopeRequest{
		Slot:         0, // epoch 0, before GloasForkEpoch 10
		BuilderIndex: 0,
	})
	require.ErrorContains(t, "not supported before Gloas fork", err)
}

func TestPublishExecutionPayloadEnvelope_NilRequest(t *testing.T) {
	vs := &Server{}
	_, err := vs.PublishExecutionPayloadEnvelope(t.Context(), nil)
	require.ErrorContains(t, "signed envelope cannot be nil", err)

	_, err = vs.PublishExecutionPayloadEnvelope(t.Context(), &ethpb.SignedExecutionPayloadEnvelope{})
	require.ErrorContains(t, "signed envelope cannot be nil", err)
}

func TestPublishExecutionPayloadEnvelope_PreFork(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 10
	params.OverrideBeaconConfig(cfg)

	vs := &Server{}
	_, err := vs.PublishExecutionPayloadEnvelope(t.Context(), &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Slot: 0, // epoch 0, before GloasForkEpoch 10
		},
	})
	require.ErrorContains(t, "not supported before Gloas fork", err)
}

func TestPublishExecutionPayloadEnvelope_Success(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	broadcaster := &mockp2p.MockBroadcaster{}
	vs := &Server{
		P2P: broadcaster,
	}

	req := &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Slot:            1,
			BuilderIndex:    0,
			BeaconBlockRoot: make([]byte, 32),
			StateRoot:       make([]byte, 32),
		},
		Signature: make([]byte, 96),
	}

	resp, err := vs.PublishExecutionPayloadEnvelope(t.Context(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, true, broadcaster.BroadcastCalled.Load())
	require.Equal(t, 1, len(broadcaster.BroadcastMessages))
}
