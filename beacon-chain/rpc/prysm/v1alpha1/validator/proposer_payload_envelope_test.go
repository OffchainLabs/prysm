//go:build minimal

package validator

import (
	"math/big"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func testGloasBlock(t *testing.T) (*consensusblocks.GetPayloadResponse, interfaces.SignedBeaconBlock) {
	t.Helper()

	payload := &enginev1.ExecutionPayloadGloas{
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
	ed, err := consensusblocks.WrappedExecutionPayloadGloas(payload)
	require.NoError(t, err)

	local := &consensusblocks.GetPayloadResponse{
		ExecutionData:          ed,
		Bid:                    big.NewInt(0),
		ExecutionRequestsGloas: &enginev1.ExecutionRequestsGloas{},
	}

	sBlk, err := consensusblocks.NewSignedBeaconBlock(util.NewBeaconBlockGloas())
	require.NoError(t, err)

	return local, sBlk
}

func TestStoreExecutionPayloadEnvelope(t *testing.T) {
	local, sBlk := testGloasBlock(t)
	envelopeCache := cache.NewExecutionPayloadEnvelopeCache()
	vs := &Server{CoreService: &core.Service{ExecutionPayloadEnvelopeCache: envelopeCache}}

	envelope, err := vs.storeExecutionPayloadEnvelope(sBlk, local)
	require.NoError(t, err)
	require.Equal(t, sBlk.Block().Slot(), envelope.Payload.SlotNumber)

	contents, ok := envelopeCache.Contents()
	require.Equal(t, true, ok)
	require.Equal(t, sBlk.Block().Slot(), contents.Envelope.Payload.SlotNumber)
}

func TestExtractExecutionPayloadGloas(t *testing.T) {
	payload := &enginev1.ExecutionPayloadGloas{
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
	ed, err := consensusblocks.WrappedExecutionPayloadGloas(payload)
	require.NoError(t, err)

	result := extractExecutionPayloadGloas(&consensusblocks.GetPayloadResponse{
		ExecutionData: ed,
		Bid:           big.NewInt(0),
	})
	require.NotNil(t, result)
	require.DeepEqual(t, payload, result)
}

func TestExtractExecutionPayloadGloas_Nil(t *testing.T) {
	require.Equal(t, true, extractExecutionPayloadGloas(nil) == nil)
	require.Equal(t, true, extractExecutionPayloadGloas(&consensusblocks.GetPayloadResponse{}) == nil)
}

func TestGetExecutionPayloadEnvelopeGRPC(t *testing.T) {
	t.Run("invalid argument", func(t *testing.T) {
		vs := &Server{CoreService: &core.Service{}}
		_, err := vs.GetExecutionPayloadEnvelope(t.Context(), nil)
		require.ErrorContains(t, "request cannot be nil", err)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("not found", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)
		vs := &Server{CoreService: &core.Service{ExecutionPayloadEnvelopeCache: cache.NewExecutionPayloadEnvelopeCache()}}

		_, err := vs.GetExecutionPayloadEnvelope(t.Context(), &ethpb.ExecutionPayloadEnvelopeRequest{Slot: 1})
		require.ErrorContains(t, "not found for slot 1", err)
		require.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("success", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)
		local, sBlk := testGloasBlock(t)
		vs := &Server{CoreService: &core.Service{ExecutionPayloadEnvelopeCache: cache.NewExecutionPayloadEnvelopeCache()}}
		want, err := vs.storeExecutionPayloadEnvelope(sBlk, local)
		require.NoError(t, err)

		resp, err := vs.GetExecutionPayloadEnvelope(t.Context(), &ethpb.ExecutionPayloadEnvelopeRequest{Slot: sBlk.Block().Slot()})
		require.NoError(t, err)
		require.Equal(t, want, resp.Envelope)
	})
}

func TestPublishExecutionPayloadEnvelopeGRPC_ErrorMapping(t *testing.T) {
	t.Run("invalid argument", func(t *testing.T) {
		vs := &Server{CoreService: &core.Service{}}
		_, err := vs.PublishExecutionPayloadEnvelope(t.Context(), nil)
		require.ErrorContains(t, "must set contents or signed_envelope", err)
		require.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = vs.PublishExecutionPayloadEnvelope(t.Context(), &ethpb.GenericSignedExecutionPayloadEnvelope{
			Envelope: &ethpb.GenericSignedExecutionPayloadEnvelope_Contents{
				Contents: &ethpb.SignedExecutionPayloadEnvelopeContents{SignedExecutionPayloadEnvelope: &ethpb.SignedExecutionPayloadEnvelope{}},
			},
		})
		require.ErrorContains(t, "signed envelope or payload cannot be nil", err)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("failed precondition", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)
		vs := &Server{CoreService: &core.Service{ExecutionPayloadEnvelopeCache: cache.NewExecutionPayloadEnvelopeCache()}}
		_, err := vs.PublishExecutionPayloadEnvelope(t.Context(), &ethpb.GenericSignedExecutionPayloadEnvelope{
			Envelope: &ethpb.GenericSignedExecutionPayloadEnvelope_SignedEnvelope{
				SignedEnvelope: &ethpb.SignedExecutionPayloadEnvelope{
					Message: &ethpb.ExecutionPayloadEnvelope{Payload: &enginev1.ExecutionPayloadGloas{SlotNumber: 1}},
				},
			},
		})
		require.ErrorContains(t, "no cached blobs and KZG proofs", err)
		require.Equal(t, codes.FailedPrecondition, status.Code(err))
	})
}
