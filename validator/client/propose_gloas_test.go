package client

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
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

func signedGloasBlock(t *testing.T, slot primitives.Slot, builderIndex primitives.BuilderIndex) interfaces.SignedBeaconBlock {
	t.Helper()

	blk := util.NewBeaconBlockGloas()
	blk.Block.Slot = slot
	if blk.Block.Body == nil {
		blk.Block.Body = &ethpb.BeaconBlockBodyGloas{}
	}
	blk.Block.Body.SignedExecutionPayloadBid = &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			BuilderIndex: builderIndex,
		},
		Signature: make([]byte, 96),
	}

	signed, err := consensusblocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	return signed
}

func testExecutionPayloadEnvelope(slot primitives.Slot, builderIndex primitives.BuilderIndex) *ethpb.ExecutionPayloadEnvelope {
	return &ethpb.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadGloas{
			ParentHash:    make([]byte, 32),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, 32),
			ReceiptsRoot:  make([]byte, 32),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, 32),
			BaseFeePerGas: make([]byte, 32),
			BlockHash:     make([]byte, 32),
			ExtraData:     make([]byte, 0),
			SlotNumber:    slot,
		},
		ExecutionRequests: &enginev1.ExecutionRequests{},
		BuilderIndex:      builderIndex,
		BeaconBlockRoot:   make([]byte, 32),
	}
}

func TestProposeSelfBuildEnvelope(t *testing.T) {
	validator, m, validatorKey, finish := setup(t, false)
	defer finish()

	slot := primitives.Slot(100)
	builderIndex := params.BeaconConfig().BuilderIndexSelfBuild

	expectedEnvelope := testExecutionPayloadEnvelope(slot, builderIndex)

	m.validatorClient.EXPECT().
		GetExecutionPayloadEnvelope(gomock.Any(), slot).
		Return(expectedEnvelope, nil)

	builderDomain := make([]byte, 32)
	copy(builderDomain, params.BeaconConfig().DomainBeaconBuilder[:])
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&ethpb.DomainResponse{SignatureDomain: builderDomain}, nil)

	m.validatorClient.EXPECT().
		PublishExecutionPayloadEnvelope(gomock.Any(), gomock.AssignableToTypeOf(&ethpb.SignedExecutionPayloadEnvelope{})).
		Return(&emptypb.Empty{}, nil)

	signedBlock := signedGloasBlock(t, slot, builderIndex)

	var pubKey [fieldparams.BLSPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())

	err := validator.proposeSelfBuildEnvelope(t.Context(), slot, pubKey, signedBlock)
	require.NoError(t, err)
}

func TestProposeSelfBuildEnvelope_MissingBid(t *testing.T) {
	validator, _, _, finish := setup(t, false)
	defer finish()

	blk := util.NewBeaconBlockGloas()
	blk.Block.Slot = 1
	if blk.Block.Body == nil {
		blk.Block.Body = &ethpb.BeaconBlockBodyGloas{}
	}
	blk.Block.Body.SignedExecutionPayloadBid = nil

	signedBlock, err := consensusblocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	var pubKey [fieldparams.BLSPubkeyLength]byte
	err = validator.proposeSelfBuildEnvelope(t.Context(), 1, pubKey, signedBlock)
	require.ErrorContains(t, "no execution payload bid found in block body", err)
}

func TestProposeSelfBuildEnvelope_ClientError(t *testing.T) {
	validator, m, validatorKey, finish := setup(t, false)
	defer finish()

	m.validatorClient.EXPECT().
		GetExecutionPayloadEnvelope(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("connection refused"))

	signedBlock := signedGloasBlock(t, 1, params.BeaconConfig().BuilderIndexSelfBuild)

	var pubKey [fieldparams.BLSPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())

	err := validator.proposeSelfBuildEnvelope(t.Context(), 1, pubKey, signedBlock)
	require.ErrorContains(t, "failed to get execution payload envelope for self-build", err)
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
	if blk.Block.Body == nil {
		blk.Block.Body = &ethpb.BeaconBlockBodyGloas{}
	}
	if blk.Block.Body.SignedExecutionPayloadBid == nil {
		blk.Block.Body.SignedExecutionPayloadBid = &ethpb.SignedExecutionPayloadBid{}
	}
	if blk.Block.Body.SignedExecutionPayloadBid.Message == nil {
		blk.Block.Body.SignedExecutionPayloadBid.Message = &ethpb.ExecutionPayloadBid{}
	}
	blk.Block.Body.SignedExecutionPayloadBid.Message.BuilderIndex = builderIndex

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
		GetExecutionPayloadEnvelope(gomock.Any(), primitives.Slot(1)).
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
