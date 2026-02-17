package client

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/pkg/errors"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/emptypb"
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

func TestExecutionPayloadEnvelope(t *testing.T) {
	validator, m, _, finish := setup(t, false)
	defer finish()

	slot := primitives.Slot(100)
	builderIndex := params.BeaconConfig().BuilderIndexSelfBuild

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

	envelope, err := validator.getSelfBuildExecutionPayloadEnvelope(t.Context(), slot, b)
	require.NoError(t, err)
	require.DeepEqual(t, expectedEnvelope, envelope)
}

func TestExecutionPayloadEnvelope_NilBlock(t *testing.T) {
	validator, _, _, finish := setup(t, false)
	defer finish()

	b := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_Gloas{},
	}

	_, err := validator.getSelfBuildExecutionPayloadEnvelope(t.Context(), 1, b)
	require.ErrorContains(t, "expected Gloas block but got nil", err)
}

func TestExecutionPayloadEnvelope_MissingBid(t *testing.T) {
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

	_, err := validator.getSelfBuildExecutionPayloadEnvelope(t.Context(), 1, b)
	require.ErrorContains(t, "block missing signed execution payload bid", err)
}

func TestExecutionPayloadEnvelope_ClientError(t *testing.T) {
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

	_, err := validator.getSelfBuildExecutionPayloadEnvelope(t.Context(), 1, b)
	require.ErrorContains(t, "connection refused", err)
}

func TestSignExecutionPayloadEnvelope(t *testing.T) {
	validator, m, _, finish := setup(t, false)
	defer finish()

	kp := testKeyFromBytes(t, []byte{1})
	validator.km = newMockKeymanager(t, kp)

	builderDomain := make([]byte, 32)
	copy(builderDomain, params.BeaconConfig().DomainBeaconBuilder[:])
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

func TestSignExecutionPayloadEnvelope_VerifySignature(t *testing.T) {
	validator, m, _, finish := setup(t, false)
	defer finish()

	kp := testKeyFromBytes(t, []byte{1})
	validator.km = newMockKeymanager(t, kp)

	builderDomain := make([]byte, 32)
	copy(builderDomain, params.BeaconConfig().DomainBeaconBuilder[:])
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&ethpb.DomainResponse{SignatureDomain: builderDomain}, nil)

	envelope := testExecutionPayloadEnvelope(100, 42)

	signed, err := validator.signExecutionPayloadEnvelope(t.Context(), kp.pub, 100, envelope)
	require.NoError(t, err)

	// Compute the expected signing root and verify the signature.
	signingRoot, err := signing.ComputeSigningRoot(envelope, builderDomain)
	require.NoError(t, err)

	sig, err := bls.SignatureFromBytes(signed.Signature)
	require.NoError(t, err)
	require.Equal(t, true, sig.Verify(kp.pri.PublicKey(), signingRoot[:]))
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

// TestProposeBlock_Gloas_EnvelopeAfterBlock verifies that the Gloas propose flow
// submits the block first, then retrieves, signs, and publishes the envelope.
// The envelope's state root is lazily computed by the beacon node from the
// post-block state, so this ordering is critical.
func TestProposeBlock_Gloas_EnvelopeAfterBlock(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t, false)
	defer finish()

	var pubKey [fieldparams.BLSPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())

	blk := util.NewBeaconBlockGloas()
	builderIndex := params.BeaconConfig().BuilderIndexSelfBuild

	gloasBlock := &ethpb.GenericBeaconBlock{
		Block: &ethpb.GenericBeaconBlock_Gloas{
			Gloas: blk.Block,
		},
	}

	envelope := testExecutionPayloadEnvelope(1, builderIndex)

	// DomainData for randao signing.
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

	// BeaconBlock returns a Gloas block.
	m.validatorClient.EXPECT().
		BeaconBlock(gomock.Any(), gomock.AssignableToTypeOf(&ethpb.BlockRequest{})).
		Return(gloasBlock, nil)

	// DomainData for block signing.
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

	// Critical ordering: ProposeBeaconBlock must be called BEFORE ExecutionPayloadEnvelope.
	proposeCall := m.validatorClient.EXPECT().
		ProposeBeaconBlock(gomock.Any(), gomock.AssignableToTypeOf(&ethpb.GenericSignedBeaconBlock{})).
		Return(&ethpb.ProposeResponse{BlockRoot: make([]byte, 32)}, nil)

	getEnvelopeCall := m.validatorClient.EXPECT().
		GetExecutionPayloadEnvelope(gomock.Any(), primitives.Slot(1), builderIndex).
		Return(envelope, nil).
		After(proposeCall)

	// DomainData for envelope signing.
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil).
		After(getEnvelopeCall)

	m.validatorClient.EXPECT().
		PublishExecutionPayloadEnvelope(gomock.Any(), gomock.AssignableToTypeOf(&ethpb.SignedExecutionPayloadEnvelope{})).
		Return(&emptypb.Empty{}, nil)

	validator.ProposeBlock(t.Context(), 1, pubKey)
	require.LogsContain(t, hook, "Submitted new block")
}
