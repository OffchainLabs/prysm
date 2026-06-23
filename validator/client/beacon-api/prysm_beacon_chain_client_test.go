package beacon_api

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"go.uber.org/mock/gomock"
)

func Test_beaconApiBeaconChainClient_GetValidatorPerformance(t *testing.T) {
	const nodeVersion = "prysm/v0.0.1"
	publicKeys := [][48]byte{
		bytesutil.ToBytes48([]byte{1}),
		bytesutil.ToBytes48([]byte{2}),
		bytesutil.ToBytes48([]byte{3}),
	}

	ctx := t.Context()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	request, err := json.Marshal(structs.GetValidatorPerformanceRequest{
		PublicKeys: [][]byte{publicKeys[0][:], publicKeys[2][:], publicKeys[1][:]},
	})
	require.NoError(t, err)
	handler := mock.NewMockHandler(ctrl)
	// Expect node version endpoint call.
	var nodeVersionResponse structs.GetVersionResponse
	handler.EXPECT().Get(
		gomock.Any(),
		"/eth/v1/node/version",
		&nodeVersionResponse,
	).Return(
		nil,
	).SetArg(
		2,
		structs.GetVersionResponse{
			Data: &structs.Version{Version: nodeVersion},
		},
	)

	wantResponse := &structs.GetValidatorPerformanceResponse{}
	want := &ethpb.ValidatorPerformanceResponse{}

	handler.EXPECT().Post(
		gomock.Any(),
		"/prysm/validators/performance",
		nil,
		bytes.NewBuffer(request),
		wantResponse,
	).Return(
		nil,
	)

	var client iface.PrysmChainClient = &prysmChainClient{
		nodeClient: &beaconApiNodeClient{handler: handler},
		handler:    handler,
	}

	got, err := client.ValidatorPerformance(ctx, &ethpb.ValidatorPerformanceRequest{
		PublicKeys: [][]byte{publicKeys[0][:], publicKeys[2][:], publicKeys[1][:]},
	})
	require.NoError(t, err)
	require.DeepEqual(t, want.PublicKeys, got.PublicKeys)
}
