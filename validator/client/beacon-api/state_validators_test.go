package beacon_api

import (
	"bytes"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/OffchainLabs/prysm/v6/api/apiutil"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/validator/client/beacon-api/mock"
	"github.com/pkg/errors"
	"go.uber.org/mock/gomock"
)

func TestGetStateValidators_Nominal_POST(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	req := &structs.GetValidatorsRequest{
		Ids: []string{
			"12345",
			"0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be13",
			"0x80000e851c0f53c3246ff726d7ff7766661ca5e12a07c45c114d208d54f0f8233d4380b2e9aff759d69795d1df905526",
			"0x424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242",
			"0x800015473bdc3a7f45ef8eb8abc598bc20021e55ad6e6ad1d745aaef9730dd2c28ec08bf42df18451de94dd4a6d24ec5",
		},
		Statuses: []string{"active_ongoing", "active_exiting", "exited_slashed", "exited_unslashed"},
	}
	reqBytes, err := json.Marshal(req)
	require.NoError(t, err)

	stateValidatorsResponseJson := structs.GetValidatorsResponse{}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

	wanted := []*structs.ValidatorContainer{
		{
			Index:  "12345",
			Status: "active_ongoing",
			Validator: &structs.Validator{
				Pubkey: "0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be19",
			},
		},
		{
			Index:  "55293",
			Status: "active_ongoing",
			Validator: &structs.Validator{
				Pubkey: "0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be13",
			},
		},
		{
			Index:  "55294",
			Status: "active_exiting",
			Validator: &structs.Validator{
				Pubkey: "0x80000e851c0f53c3246ff726d7ff7766661ca5e12a07c45c114d208d54f0f8233d4380b2e9aff759d69795d1df905526",
			},
		},
		{
			Index:  "55295",
			Status: "exited_slashed",
			Validator: &structs.Validator{
				Pubkey: "0x800015473bdc3a7f45ef8eb8abc598bc20021e55ad6e6ad1d745aaef9730dd2c28ec08bf42df18451de94dd4a6d24ec5",
			},
		},
	}

	ctx := t.Context()

	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v1/beacon/states/head/validators",
		nil,
		bytes.NewBuffer(reqBytes),
		&stateValidatorsResponseJson,
	).Return(
		nil,
	).SetArg(
		4,
		structs.GetValidatorsResponse{
			Data: wanted,
		},
	).Times(1)

	stateValidatorsProvider := beaconApiStateValidatorsProvider{jsonRestHandler: jsonRestHandler}
	actual, err := stateValidatorsProvider.StateValidators(ctx, []string{
		"0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be13", // active_ongoing
		"0x80000e851c0f53c3246ff726d7ff7766661ca5e12a07c45c114d208d54f0f8233d4380b2e9aff759d69795d1df905526", // active_exiting
		"0x424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242", // does not exist
		"0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be13", // active_ongoing - duplicate
		"0x800015473bdc3a7f45ef8eb8abc598bc20021e55ad6e6ad1d745aaef9730dd2c28ec08bf42df18451de94dd4a6d24ec5", // exited_slashed
	},
		[]primitives.ValidatorIndex{
			12345, // active_ongoing
			12345, // active_ongoing - duplicate
		},
		[]string{"active_ongoing", "active_exiting", "exited_slashed", "exited_unslashed"},
	)
	require.NoError(t, err)
	assert.DeepEqual(t, wanted, actual.Data)
}

func TestGetStateValidators_Nominal_GET(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	req := &structs.GetValidatorsRequest{
		Ids: []string{
			"12345",
			"0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be13",
			"0x80000e851c0f53c3246ff726d7ff7766661ca5e12a07c45c114d208d54f0f8233d4380b2e9aff759d69795d1df905526",
			"0x424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242",
			"0x800015473bdc3a7f45ef8eb8abc598bc20021e55ad6e6ad1d745aaef9730dd2c28ec08bf42df18451de94dd4a6d24ec5",
		},
		Statuses: []string{"active_ongoing", "active_exiting", "exited_slashed", "exited_unslashed"},
	}
	reqBytes, err := json.Marshal(req)
	require.NoError(t, err)

	stateValidatorsResponseJson := structs.GetValidatorsResponse{}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

	wanted := []*structs.ValidatorContainer{
		{
			Index:  "12345",
			Status: "active_ongoing",
			Validator: &structs.Validator{
				Pubkey: "0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be19",
			},
		},
		{
			Index:  "55293",
			Status: "active_ongoing",
			Validator: &structs.Validator{
				Pubkey: "0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be13",
			},
		},
		{
			Index:  "55294",
			Status: "active_exiting",
			Validator: &structs.Validator{
				Pubkey: "0x80000e851c0f53c3246ff726d7ff7766661ca5e12a07c45c114d208d54f0f8233d4380b2e9aff759d69795d1df905526",
			},
		},
		{
			Index:  "55295",
			Status: "exited_slashed",
			Validator: &structs.Validator{
				Pubkey: "0x800015473bdc3a7f45ef8eb8abc598bc20021e55ad6e6ad1d745aaef9730dd2c28ec08bf42df18451de94dd4a6d24ec5",
			},
		},
	}

	ctx := t.Context()

	// First return an error from POST call.
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v1/beacon/states/head/validators",
		nil,
		bytes.NewBuffer(reqBytes),
		&stateValidatorsResponseJson,
	).Return(
		errors.New("an error"),
	).Times(1)

	// Then try the GET call which will be successful.
	queryParams := url.Values{}
	for _, id := range req.Ids {
		queryParams.Add("id", id)
	}
	for _, st := range req.Statuses {
		queryParams.Add("status", st)
	}

	query := apiutil.BuildURL("/eth/v1/beacon/states/head/validators", queryParams)

	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		query,
		&stateValidatorsResponseJson,
	).Return(
		nil,
	).SetArg(
		2,
		structs.GetValidatorsResponse{
			Data: wanted,
		},
	).Times(1)

	stateValidatorsProvider := beaconApiStateValidatorsProvider{jsonRestHandler: jsonRestHandler}
	actual, err := stateValidatorsProvider.StateValidators(ctx, []string{
		"0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be13", // active_ongoing
		"0x80000e851c0f53c3246ff726d7ff7766661ca5e12a07c45c114d208d54f0f8233d4380b2e9aff759d69795d1df905526", // active_exiting
		"0x424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242", // does not exist
		"0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be13", // active_ongoing - duplicate
		"0x800015473bdc3a7f45ef8eb8abc598bc20021e55ad6e6ad1d745aaef9730dd2c28ec08bf42df18451de94dd4a6d24ec5", // exited_slashed
	},
		[]primitives.ValidatorIndex{
			12345, // active_ongoing
			12345, // active_ongoing - duplicate
		},
		[]string{"active_ongoing", "active_exiting", "exited_slashed", "exited_unslashed"},
	)
	require.NoError(t, err)
	assert.DeepEqual(t, wanted, actual.Data)
}

func TestGetStateValidators_GetRestJsonResponseOnError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	req := &structs.GetValidatorsRequest{
		Ids:      []string{"0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be13"},
		Statuses: []string{},
	}
	reqBytes, err := json.Marshal(req)
	require.NoError(t, err)

	stateValidatorsResponseJson := structs.GetValidatorsResponse{}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

	ctx := t.Context()

	// First call POST.
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v1/beacon/states/head/validators",
		nil,
		bytes.NewBuffer(reqBytes),
		&stateValidatorsResponseJson,
	).Return(
		errors.New("an error"),
	).Times(1)

	// Call to GET endpoint upon receiving error from POST call.
	queryParams := url.Values{}
	for _, id := range req.Ids {
		queryParams.Add("id", id)
	}
	for _, st := range req.Statuses {
		queryParams.Add("status", st)
	}

	query := apiutil.BuildURL("/eth/v1/beacon/states/head/validators", queryParams)

	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		query,
		&stateValidatorsResponseJson,
	).Return(
		errors.New("an error"),
	).Times(1)

	stateValidatorsProvider := beaconApiStateValidatorsProvider{jsonRestHandler: jsonRestHandler}
	_, err = stateValidatorsProvider.StateValidators(ctx, []string{
		"0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be13", // active_ongoing
	},
		nil,
		nil,
	)
	assert.ErrorContains(t, "an error", err)
}

func TestGetStateValidators_DataIsNil_POST(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	req := &structs.GetValidatorsRequest{
		Ids:      []string{"0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be13"},
		Statuses: []string{},
	}
	reqBytes, err := json.Marshal(req)
	require.NoError(t, err)

	ctx := t.Context()
	stateValidatorsResponseJson := structs.GetValidatorsResponse{}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v1/beacon/states/head/validators",
		nil, bytes.NewBuffer(reqBytes),
		&stateValidatorsResponseJson,
	).Return(
		nil,
	).SetArg(
		4,
		structs.GetValidatorsResponse{
			Data: nil,
		},
	).Times(1)

	stateValidatorsProvider := beaconApiStateValidatorsProvider{jsonRestHandler: jsonRestHandler}
	_, err = stateValidatorsProvider.StateValidators(ctx, []string{
		"0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be13", // active_ongoing
	},
		nil,
		nil,
	)
	assert.ErrorContains(t, "stateValidatorsJson.Data is nil", err)
}

func TestGetStateValidators_DataIsNil_GET(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	req := &structs.GetValidatorsRequest{
		Ids:      []string{"0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be13"},
		Statuses: []string{},
	}
	reqBytes, err := json.Marshal(req)
	require.NoError(t, err)

	ctx := t.Context()
	stateValidatorsResponseJson := structs.GetValidatorsResponse{}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

	// First call POST which will return an error.
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v1/beacon/states/head/validators",
		nil,
		bytes.NewBuffer(reqBytes),
		&stateValidatorsResponseJson,
	).Return(
		errors.New("an error"),
	).Times(1)

	// Then call GET which returns nil Data.
	queryParams := url.Values{}
	for _, id := range req.Ids {
		queryParams.Add("id", id)
	}
	for _, st := range req.Statuses {
		queryParams.Add("status", st)
	}

	query := apiutil.BuildURL("/eth/v1/beacon/states/head/validators", queryParams)

	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		query,
		&stateValidatorsResponseJson,
	).Return(
		nil,
	).SetArg(
		2,
		structs.GetValidatorsResponse{
			Data: nil,
		},
	).Times(1)

	stateValidatorsProvider := beaconApiStateValidatorsProvider{jsonRestHandler: jsonRestHandler}
	_, err = stateValidatorsProvider.StateValidators(ctx, []string{
		"0x8000091c2ae64ee414a54c1cc1fc67dec663408bc636cb86756e0200e41a75c8f86603f104f02c856983d2783116be13", // active_ongoing
	},
		nil,
		nil,
	)
	assert.ErrorContains(t, "stateValidatorsJson.Data is nil", err)
}
