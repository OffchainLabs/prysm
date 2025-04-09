package beacon_api

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api/mock"
	"go.uber.org/mock/gomock"
)

func TestGetGenesis_ValidGenesis(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	genesisResponseJson := structs.GetGenesisResponse{}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		"/eth/v1/beacon/genesis",
		&genesisResponseJson,
	).Return(
		nil,
	).SetArg(
		2,
		structs.GetGenesisResponse{
			Data: &structs.Genesis{
				GenesisTime:           "1234",
				GenesisValidatorsRoot: "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2",
			},
		},
	).Times(1)

	genesisProvider := &beaconApiGenesisProvider{jsonRestHandler: jsonRestHandler}
	resp, err := genesisProvider.Genesis(ctx)
	assert.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "1234", resp.GenesisTime)
	assert.Equal(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2", resp.GenesisValidatorsRoot)
}

func TestGetGenesis_NilData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	genesisResponseJson := structs.GetGenesisResponse{}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		"/eth/v1/beacon/genesis",
		&genesisResponseJson,
	).Return(
		nil,
	).SetArg(
		2,
		structs.GetGenesisResponse{Data: nil},
	).Times(1)

	genesisProvider := &beaconApiGenesisProvider{jsonRestHandler: jsonRestHandler}
	_, err := genesisProvider.Genesis(ctx)
	assert.ErrorContains(t, "genesis data is nil", err)
}

func TestGetGenesis_EndpointCalledOnlyOnce(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	genesisResponseJson := structs.GetGenesisResponse{}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		"/eth/v1/beacon/genesis",
		&genesisResponseJson,
	).Return(
		nil,
	).SetArg(
		2,
		structs.GetGenesisResponse{
			Data: &structs.Genesis{
				GenesisTime:           "1234",
				GenesisValidatorsRoot: "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2",
			},
		},
	).Times(1)

	genesisProvider := &beaconApiGenesisProvider{jsonRestHandler: jsonRestHandler}
	_, err := genesisProvider.Genesis(ctx)
	assert.NoError(t, err)
	resp, err := genesisProvider.Genesis(ctx)
	assert.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "1234", resp.GenesisTime)
	assert.Equal(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2", resp.GenesisValidatorsRoot)
}

func TestGetGenesis_EndpointCanBeCalledAgainAfterError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	genesisResponseJson := structs.GetGenesisResponse{}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		"/eth/v1/beacon/genesis",
		&genesisResponseJson,
	).Return(
		errors.New("foo"),
	).Times(1)
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		"/eth/v1/beacon/genesis",
		&genesisResponseJson,
	).Return(
		nil,
	).SetArg(
		2,
		structs.GetGenesisResponse{
			Data: &structs.Genesis{
				GenesisTime:           "1234",
				GenesisValidatorsRoot: "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2",
			},
		},
	).Times(1)

	genesisProvider := &beaconApiGenesisProvider{jsonRestHandler: jsonRestHandler}
	_, err := genesisProvider.Genesis(ctx)
	require.ErrorContains(t, "foo", err)
	resp, err := genesisProvider.Genesis(ctx)
	assert.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "1234", resp.GenesisTime)
	assert.Equal(t, "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2", resp.GenesisValidatorsRoot)
}
