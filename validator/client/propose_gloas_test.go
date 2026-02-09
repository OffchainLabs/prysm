package client

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/pkg/errors"
	"go.uber.org/mock/gomock"
)

func testExecutionPayloadEnvelope(slot primitives.Slot, builderIndex primitives.BuilderIndex) *ethpb.ExecutionPayloadEnvelope {
	return &ethpb.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadDeneb{
			ParentHash:    make([]byte, 32),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, 32),
			ReceiptsRoot:  make([]byte, 32),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, 32),
			BaseFeePerGas: make([]byte, 32),
			BlockHash:     make([]byte, 32),
			ExtraData:     make([]byte, 0),
		},
		ExecutionRequests: &enginev1.ExecutionRequests{},
		Slot:              slot,
		BuilderIndex:      builderIndex,
		BeaconBlockRoot:   make([]byte, 32),
		StateRoot:         make([]byte, 32),
	}
}

func TestGetExecutionPayloadEnvelope(t *testing.T) {
	validator, m, _, finish := setup(t, false)
	defer finish()

	slot := primitives.Slot(100)
	builderIndex := primitives.BuilderIndex(42)

	expectedEnvelope := testExecutionPayloadEnvelope(slot, builderIndex)

	m.validatorClient.EXPECT().
		GetExecutionPayloadEnvelope(gomock.Any(), slot, builderIndex).
		Return(expectedEnvelope, nil)

	b := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_Gloas{
			Gloas: &ethpb.BeaconBlockGloas{
				Slot: slot,
				Body: &ethpb.BeaconBlockBodyGloas{
					SignedExecutionPayloadBid: &ethpb.SignedExecutionPayloadBid{
						Message: &ethpb.ExecutionPayloadBid{
							BuilderIndex: builderIndex,
						},
						Signature: make([]byte, 96),
					},
				},
			},
		},
	}

	envelope, err := validator.getExecutionPayloadEnvelope(t.Context(), slot, b)
	require.NoError(t, err)
	require.DeepEqual(t, expectedEnvelope, envelope)
}

func TestGetExecutionPayloadEnvelope_NilBlock(t *testing.T) {
	validator, _, _, finish := setup(t, false)
	defer finish()

	b := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_Gloas{},
	}

	_, err := validator.getExecutionPayloadEnvelope(t.Context(), 1, b)
	require.ErrorContains(t, "expected GLOAS block but got nil", err)
}

func TestGetExecutionPayloadEnvelope_MissingBid(t *testing.T) {
	validator, _, _, finish := setup(t, false)
	defer finish()

	b := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_Gloas{
			Gloas: &ethpb.BeaconBlockGloas{
				Slot: 1,
				Body: &ethpb.BeaconBlockBodyGloas{},
			},
		},
	}

	_, err := validator.getExecutionPayloadEnvelope(t.Context(), 1, b)
	require.ErrorContains(t, "block missing signed execution payload bid", err)
}

func TestGetExecutionPayloadEnvelope_ClientError(t *testing.T) {
	validator, m, _, finish := setup(t, false)
	defer finish()

	m.validatorClient.EXPECT().
		GetExecutionPayloadEnvelope(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("connection refused"))

	b := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_Gloas{
			Gloas: &ethpb.BeaconBlockGloas{
				Slot: 1,
				Body: &ethpb.BeaconBlockBodyGloas{
					SignedExecutionPayloadBid: &ethpb.SignedExecutionPayloadBid{
						Message:   &ethpb.ExecutionPayloadBid{BuilderIndex: 1},
						Signature: make([]byte, 96),
					},
				},
			},
		},
	}

	_, err := validator.getExecutionPayloadEnvelope(t.Context(), 1, b)
	require.ErrorContains(t, "connection refused", err)
}

func TestSignExecutionPayloadEnvelope(t *testing.T) {
	validator, m, _, finish := setup(t, false)
	defer finish()

	kp := testKeyFromBytes(t, []byte{1})
	validator.km = newMockKeymanager(t, kp)

	builderDomain := make([]byte, 32)
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&ethpb.DomainResponse{SignatureDomain: builderDomain}, nil)

	envelope := testExecutionPayloadEnvelope(100, 42)

	signed, err := validator.signExecutionPayloadEnvelope(t.Context(), kp.pub, 100, envelope)
	require.NoError(t, err)
	require.NotNil(t, signed)
	require.DeepEqual(t, envelope, signed.Message)
	require.NotNil(t, signed.Signature)

	// Verify the signature was computed with the builder domain.
	expectedRoot, err := signing.ComputeSigningRoot(envelope, builderDomain)
	require.NoError(t, err)
	require.NotEqual(t, [32]byte{}, expectedRoot)
}

func TestSignExecutionPayloadEnvelope_DomainDataError(t *testing.T) {
	validator, m, _, finish := setup(t, false)
	defer finish()

	kp := testKeyFromBytes(t, []byte{1})
	validator.km = newMockKeymanager(t, kp)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("domain data unavailable"))

	envelope := testExecutionPayloadEnvelope(100, 0)

	_, err := validator.signExecutionPayloadEnvelope(t.Context(), kp.pub, 100, envelope)
	require.ErrorContains(t, "could not get domain data", err)
}

func TestSignExecutionPayloadEnvelope_NilDomain(t *testing.T) {
	validator, m, _, finish := setup(t, false)
	defer finish()

	kp := testKeyFromBytes(t, []byte{1})
	validator.km = newMockKeymanager(t, kp)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(nil, nil)

	envelope := testExecutionPayloadEnvelope(100, 0)

	_, err := validator.signExecutionPayloadEnvelope(t.Context(), kp.pub, 100, envelope)
	require.ErrorContains(t, "nil domain data", err)
}

func TestSignExecutionPayloadEnvelope_UsesDomainBeaconBuilder(t *testing.T) {
	validator, m, _, finish := setup(t, false)
	defer finish()

	kp := testKeyFromBytes(t, []byte{1})
	validator.km = newMockKeymanager(t, kp)

	// Verify the correct domain type is requested.
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx any, req *ethpb.DomainRequest) (*ethpb.DomainResponse, error) {
			require.DeepEqual(t, params.BeaconConfig().DomainBeaconBuilder[:], req.Domain)
			return &ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil
		})

	envelope := testExecutionPayloadEnvelope(100, 0)

	_, err := validator.signExecutionPayloadEnvelope(t.Context(), kp.pub, 100, envelope)
	require.NoError(t, err)
}
