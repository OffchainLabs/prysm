package beacon_api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
	"go.uber.org/mock/gomock"
)

func TestDecodeBlockV4Response_Heze(t *testing.T) {
	block := util.NewBeaconBlockHeze().Block
	block.Slot = 3
	block.Body.SignedExecutionPayloadBid.Message.InclusionListBits = []byte{0x01, 0x00}

	t.Run("ssz", func(t *testing.T) {
		data, err := block.MarshalSSZ()
		require.NoError(t, err)
		header := http.Header{}
		header.Set("Content-Type", api.OctetStreamMediaType)
		header.Set(api.VersionHeader, version.String(version.Heze))

		got, contents, err := decodeBlockV4Response(data, header, "url")
		require.NoError(t, err)
		require.IsNil(t, contents)
		require.DeepEqual(t, block, got.GetHeze())
	})

	t.Run("json", func(t *testing.T) {
		jsonBlock, err := structs.BeaconBlockHezeFromConsensus(block)
		require.NoError(t, err)
		raw, err := json.Marshal(jsonBlock)
		require.NoError(t, err)
		data, err := json.Marshal(structs.ProduceBlockV4Response{Version: "heze", Data: raw})
		require.NoError(t, err)
		header := http.Header{}
		header.Set("Content-Type", api.JsonMediaType)
		header.Set(api.VersionHeader, version.String(version.Heze))

		got, _, err := decodeBlockV4Response(data, header, "url")
		require.NoError(t, err)
		require.DeepEqual(t, block, got.GetHeze())
	})

	t.Run("payload included is unsupported", func(t *testing.T) {
		header := http.Header{}
		header.Set("Content-Type", api.OctetStreamMediaType)
		header.Set(api.VersionHeader, version.String(version.Heze))
		header.Set(api.ExecutionPayloadIncludedHeader, "true")

		_, _, err := decodeBlockV4Response(nil, header, "url")
		require.ErrorContains(t, "not supported for heze", err)
	})
}

func TestProposeBeaconBlock_Heze(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signed := util.NewBeaconBlockHeze()
	wantRoot, err := signed.Block.HashTreeRoot()
	require.NoError(t, err)

	handler := mock.NewMockHandler(ctrl)
	expectPostSSZWithFallback(handler)
	handler.EXPECT().PostSSZ(
		gomock.Any(),
		"/eth/v2/beacon/blocks",
		map[string]string{"Eth-Consensus-Version": "heze"},
		gomock.Any(),
	).Return(nil).Times(1)

	validatorClient := &beaconApiValidatorClient{handler: handler}
	resp, err := validatorClient.proposeBeaconBlock(t.Context(), &ethpb.GenericSignedBeaconBlock{Block: &ethpb.GenericSignedBeaconBlock_Heze{Heze: signed}})
	require.NoError(t, err)
	require.DeepEqual(t, wantRoot[:], resp.BlockRoot)
}
