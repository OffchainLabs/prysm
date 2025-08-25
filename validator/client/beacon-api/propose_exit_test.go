package beacon_api

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/validator/client/beacon-api/mock"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"go.uber.org/mock/gomock"
)

const proposeExitTestEndpoint = "/eth/v1/beacon/pool/voluntary_exits"

func TestProposeExit_Valid(t *testing.T) {
	const signature = "0xd0a030a1d6b4f8217062ccc98088fbd908797f107aaa825f2366f090445fa79a6417789aa1d232c4f9b1e56671165bde25eb5586f94fc5677df593b99369684e8f413b1bfbd3fa6f20615244f9381895c71d4f7136c528092a3d03294a98be2d"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jsonSignedVoluntaryExit := structs.SignedVoluntaryExit{
		Message: &structs.VoluntaryExit{
			Epoch:          "1",
			ValidatorIndex: "2",
		},
		Signature: signature,
	}

	marshalledVoluntaryExit, err := json.Marshal(jsonSignedVoluntaryExit)
	require.NoError(t, err)

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		proposeExitTestEndpoint,
		nil,
		bytes.NewBuffer(marshalledVoluntaryExit),
		nil,
	).Return(
		nil,
	).Times(1)

	decodedSignature, err := hexutil.Decode(signature)
	require.NoError(t, err)

	protoSignedVoluntaryExit := &ethpb.SignedVoluntaryExit{
		Exit: &ethpb.VoluntaryExit{
			Epoch:          1,
			ValidatorIndex: 2,
		},
		Signature: decodedSignature,
	}

	expectedExitRoot, err := protoSignedVoluntaryExit.Exit.HashTreeRoot()
	require.NoError(t, err)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	exitResponse, err := validatorClient.proposeExit(ctx, protoSignedVoluntaryExit)
	require.NoError(t, err)
	assert.DeepEqual(t, expectedExitRoot[:], exitResponse.ExitRoot)
}

func TestProposeExit_NilSignedVoluntaryExit(t *testing.T) {
	validatorClient := &beaconApiValidatorClient{}
	_, err := validatorClient.proposeExit(t.Context(), nil)
	assert.ErrorContains(t, "signed voluntary exit is nil", err)
}

func TestProposeExit_NilExit(t *testing.T) {
	validatorClient := &beaconApiValidatorClient{}
	_, err := validatorClient.proposeExit(t.Context(), &ethpb.SignedVoluntaryExit{})
	assert.ErrorContains(t, "exit is nil", err)
}

func TestProposeExit_BadRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		proposeExitTestEndpoint,
		nil,
		gomock.Any(),
		nil,
	).Return(
		errors.New("foo error"),
	).Times(1)

	protoSignedVoluntaryExit := &ethpb.SignedVoluntaryExit{
		Exit: &ethpb.VoluntaryExit{
			Epoch:          1,
			ValidatorIndex: 2,
		},
		Signature: []byte{3},
	}

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	_, err := validatorClient.proposeExit(ctx, protoSignedVoluntaryExit)
	assert.ErrorContains(t, "foo error", err)
}
