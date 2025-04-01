package validator_api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api/mock"
	testhelpers "github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api/test-helpers"
	"go.uber.org/mock/gomock"
)

// Make sure that AttestationData() returns the same thing as the internal attestationData()
func TestBeaconApiValidatorClient_GetAttestationDataValid(t *testing.T) {
	const slot = primitives.Slot(1)
	const committeeIndex = primitives.CommitteeIndex(2)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	produceAttestationDataResponseJson := structs.GetAttestationDataResponse{}
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		fmt.Sprintf("/eth/v1/validator/attestation_data?committee_index=%d&slot=%d", committeeIndex, slot),
		&produceAttestationDataResponseJson,
	).Return(
		nil,
	).SetArg(
		2,
		generateValidAttestation(uint64(slot), uint64(committeeIndex)),
	).Times(2)

	validatorClient := beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	expectedResp, expectedErr := validatorClient.attestationData(ctx, slot, committeeIndex)

	resp, err := validatorClient.AttestationData(
		context.Background(),
		&ethpb.AttestationDataRequest{Slot: slot, CommitteeIndex: committeeIndex},
	)

	assert.DeepEqual(t, expectedErr, err)
	assert.DeepEqual(t, expectedResp, resp)
}

func TestBeaconApiValidatorClient_GetAttestationDataError(t *testing.T) {
	const slot = primitives.Slot(1)
	const committeeIndex = primitives.CommitteeIndex(2)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	produceAttestationDataResponseJson := structs.GetAttestationDataResponse{}
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		fmt.Sprintf("/eth/v1/validator/attestation_data?committee_index=%d&slot=%d", committeeIndex, slot),
		&produceAttestationDataResponseJson,
	).Return(
		errors.New("some specific json error"),
	).SetArg(
		2,
		generateValidAttestation(uint64(slot), uint64(committeeIndex)),
	).Times(2)

	validatorClient := beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	expectedResp, expectedErr := validatorClient.attestationData(ctx, slot, committeeIndex)

	resp, err := validatorClient.AttestationData(
		context.Background(),
		&ethpb.AttestationDataRequest{Slot: slot, CommitteeIndex: committeeIndex},
	)

	assert.ErrorContains(t, expectedErr.Error(), err)
	assert.DeepEqual(t, expectedResp, resp)
}

func TestBeaconApiValidatorClient_GetFeeRecipientByPubKey(t *testing.T) {
	ctx := context.Background()
	validatorClient := beaconApiValidatorClient{}
	var expected *ethpb.FeeRecipientByPubKeyResponse = nil

	resp, err := validatorClient.FeeRecipientByPubKey(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, expected, resp)
}

func TestBeaconApiValidatorClient_DomainDataValid(t *testing.T) {
	const genesisValidatorRoot = "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
	epoch := params.BeaconConfig().AltairForkEpoch
	domainType := params.BeaconConfig().DomainSyncCommittee[:]

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	genesisProvider := mock.NewMockGenesisProvider(ctrl)
	genesisProvider.EXPECT().Genesis(gomock.Any()).Return(
		&structs.Genesis{GenesisValidatorsRoot: genesisValidatorRoot},
		nil,
	).Times(2)

	validatorClient := beaconApiValidatorClient{genesisProvider: genesisProvider}
	resp, err := validatorClient.DomainData(context.Background(), &ethpb.DomainRequest{Epoch: epoch, Domain: domainType})

	domainTypeArray := bytesutil.ToBytes4(domainType)
	expectedResp, expectedErr := validatorClient.domainData(ctx, epoch, domainTypeArray)
	assert.DeepEqual(t, expectedErr, err)
	assert.DeepEqual(t, expectedResp, resp)
}

func TestBeaconApiValidatorClient_DomainDataError(t *testing.T) {
	epoch := params.BeaconConfig().AltairForkEpoch
	domainType := make([]byte, 3)
	validatorClient := beaconApiValidatorClient{}
	_, err := validatorClient.DomainData(context.Background(), &ethpb.DomainRequest{Epoch: epoch, Domain: domainType})
	assert.ErrorContains(t, fmt.Sprintf("invalid domain type: %s", hexutil.Encode(domainType)), err)
}

func TestBeaconApiValidatorClient_ProposeBeaconBlockValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v2/beacon/blocks",
		map[string]string{"Eth-Consensus-Version": "phase0"},
		gomock.Any(),
		nil,
	).Return(
		nil,
	).Times(2)

	validatorClient := beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	expectedResp, expectedErr := validatorClient.proposeBeaconBlock(
		ctx,
		&ethpb.GenericSignedBeaconBlock{
			Block: generateSignedPhase0Block(),
		},
	)

	resp, err := validatorClient.ProposeBeaconBlock(
		ctx,
		&ethpb.GenericSignedBeaconBlock{
			Block: generateSignedPhase0Block(),
		},
	)

	assert.DeepEqual(t, expectedErr, err)
	assert.DeepEqual(t, expectedResp, resp)
}

func TestBeaconApiValidatorClient_ProposeBeaconBlockError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v2/beacon/blocks",
		map[string]string{"Eth-Consensus-Version": "phase0"},
		gomock.Any(),
		nil,
	).Return(
		errors.New("foo error"),
	).Times(2)

	validatorClient := beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	expectedResp, expectedErr := validatorClient.proposeBeaconBlock(
		ctx,
		&ethpb.GenericSignedBeaconBlock{
			Block: generateSignedPhase0Block(),
		},
	)

	resp, err := validatorClient.ProposeBeaconBlock(
		ctx,
		&ethpb.GenericSignedBeaconBlock{
			Block: generateSignedPhase0Block(),
		},
	)

	assert.ErrorContains(t, expectedErr.Error(), err)
	assert.DeepEqual(t, expectedResp, resp)
}

func TestBeaconApiValidatorClient_Host(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	hosts := []string{"http://localhost:8080", "http://localhost:8081"}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().SetHost(
		hosts[0],
	).Times(1)
	jsonRestHandler.EXPECT().Host().Return(
		hosts[0],
	).Times(1)

	validatorClient := beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	validatorClient.SetHost(hosts[0])
	host := validatorClient.Host()
	require.Equal(t, hosts[0], host)

	jsonRestHandler.EXPECT().SetHost(
		hosts[1],
	).Times(1)
	jsonRestHandler.EXPECT().Host().Return(
		hosts[1],
	).Times(1)
	validatorClient.SetHost(hosts[1])
	host = validatorClient.Host()
	require.Equal(t, hosts[1], host)
}

func TestGetAggregatedSelections(t *testing.T) {
	testcases := []struct {
		name                 string
		req                  []iface.BeaconCommitteeSelection
		res                  []iface.BeaconCommitteeSelection
		endpointError        error
		expectedErrorMessage string
	}{
		{
			name: "valid",
			req: []iface.BeaconCommitteeSelection{
				{
					SelectionProof: testhelpers.FillByteSlice(96, 82),
					Slot:           75,
					ValidatorIndex: 76,
				},
			},
			res: []iface.BeaconCommitteeSelection{
				{
					SelectionProof: testhelpers.FillByteSlice(96, 100),
					Slot:           75,
					ValidatorIndex: 76,
				},
			},
		},
		{
			name: "endpoint error",
			req: []iface.BeaconCommitteeSelection{
				{
					SelectionProof: testhelpers.FillByteSlice(96, 82),
					Slot:           75,
					ValidatorIndex: 76,
				},
			},
			endpointError:        errors.New("bad request"),
			expectedErrorMessage: "bad request",
		},
		{
			name: "no response error",
			req: []iface.BeaconCommitteeSelection{
				{
					SelectionProof: testhelpers.FillByteSlice(96, 82),
					Slot:           75,
					ValidatorIndex: 76,
				},
			},
			expectedErrorMessage: "no aggregated selection returned",
		},
		{
			name: "mismatch response",
			req: []iface.BeaconCommitteeSelection{
				{
					SelectionProof: testhelpers.FillByteSlice(96, 82),
					Slot:           75,
					ValidatorIndex: 76,
				},
				{
					SelectionProof: testhelpers.FillByteSlice(96, 102),
					Slot:           75,
					ValidatorIndex: 79,
				},
			},
			res: []iface.BeaconCommitteeSelection{
				{
					SelectionProof: testhelpers.FillByteSlice(96, 100),
					Slot:           75,
					ValidatorIndex: 76,
				},
			},
			expectedErrorMessage: "mismatching number of selections",
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

			reqBody, err := json.Marshal(test.req)
			require.NoError(t, err)

			ctx := context.Background()
			jsonRestHandler.EXPECT().Post(
				gomock.Any(),
				"/eth/v1/validator/beacon_committee_selections",
				nil,
				bytes.NewBuffer(reqBody),
				&aggregatedSelectionResponse{},
			).SetArg(
				4,
				aggregatedSelectionResponse{Data: test.res},
			).Return(
				test.endpointError,
			).Times(1)

			validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
			res, err := validatorClient.AggregatedSelections(ctx, test.req)
			if test.expectedErrorMessage != "" {
				require.ErrorContains(t, test.expectedErrorMessage, err)
				return
			}

			require.NoError(t, err)
			require.DeepEqual(t, test.res, res)
		})
	}
}
