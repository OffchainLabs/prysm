package beacon_api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	rpctesting "github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/shared/testing"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/validator/client/beacon-api/mock"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/mock/gomock"
)

// Make sure that AttestationData() returns the same thing as the internal attestationData()
func TestBeaconApiValidatorClient_GetAttestationDataValid(t *testing.T) {
	const slot = primitives.Slot(1)
	const committeeIndex = primitives.CommitteeIndex(2)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

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
		t.Context(),
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

	ctx := t.Context()

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
		t.Context(),
		&ethpb.AttestationDataRequest{Slot: slot, CommitteeIndex: committeeIndex},
	)

	assert.ErrorContains(t, expectedErr.Error(), err)
	assert.DeepEqual(t, expectedResp, resp)
}

func TestBeaconApiValidatorClient_GetFeeRecipientByPubKey(t *testing.T) {
	ctx := t.Context()
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

	ctx := t.Context()

	genesisProvider := mock.NewMockGenesisProvider(ctrl)
	genesisProvider.EXPECT().Genesis(gomock.Any()).Return(
		&structs.Genesis{GenesisValidatorsRoot: genesisValidatorRoot},
		nil,
	).Times(2)

	validatorClient := beaconApiValidatorClient{genesisProvider: genesisProvider}
	resp, err := validatorClient.DomainData(t.Context(), &ethpb.DomainRequest{Epoch: epoch, Domain: domainType})

	domainTypeArray := bytesutil.ToBytes4(domainType)
	expectedResp, expectedErr := validatorClient.domainData(ctx, epoch, domainTypeArray)
	assert.DeepEqual(t, expectedErr, err)
	assert.DeepEqual(t, expectedResp, resp)
}

func TestBeaconApiValidatorClient_DomainDataError(t *testing.T) {
	epoch := params.BeaconConfig().AltairForkEpoch
	domainType := make([]byte, 3)
	validatorClient := beaconApiValidatorClient{}
	_, err := validatorClient.DomainData(t.Context(), &ethpb.DomainRequest{Epoch: epoch, Domain: domainType})
	assert.ErrorContains(t, fmt.Sprintf("invalid domain type: %s", hexutil.Encode(domainType)), err)
}

func TestBeaconApiValidatorClient_ProposeBeaconBlockValid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().PostSSZ(
		gomock.Any(),
		"/eth/v2/beacon/blocks",
		gomock.Any(),
		gomock.Any(),
	).Return(
		nil, nil, nil,
	).Times(1)

	validatorClient := beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	expectedResp, expectedErr := validatorClient.proposeBeaconBlock(
		ctx,
		&ethpb.GenericSignedBeaconBlock{
			Block: generateSignedPhase0Block(),
		},
	)
	require.NoError(t, expectedErr)
	require.NotNil(t, expectedResp)
}

func TestBeaconApiValidatorClient_ProposeBeaconBlockError_ThenPass(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().PostSSZ(
		gomock.Any(),
		"/eth/v2/beacon/blocks",
		gomock.Any(),
		gomock.Any(),
	).Return(
		nil, nil, &httputil.DefaultJsonError{
			Code:    http.StatusNotAcceptable,
			Message: "SSZ not supported",
		},
	).Times(1)

	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		"/eth/v2/beacon/blocks",
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		nil,
	).Times(1)

	validatorClient := beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	expectedResp, expectedErr := validatorClient.proposeBeaconBlock(
		ctx,
		&ethpb.GenericSignedBeaconBlock{
			Block: generateSignedPhase0Block(),
		},
	)
	require.NoError(t, expectedErr)
	require.NotNil(t, expectedResp)
}

func TestBeaconApiValidatorClient_ProposeBeaconBlockAllTypes(t *testing.T) {
	tests := []struct {
		name         string
		block        *ethpb.GenericSignedBeaconBlock
		expectedPath string
		wantErr      bool
		errorMessage string
	}{
		{
			name: "Phase0 block",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedPhase0Block(),
			},
			expectedPath: "/eth/v2/beacon/blocks",
		},
		{
			name: "Altair block",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedAltairBlock(),
			},
			expectedPath: "/eth/v2/beacon/blocks",
		},
		{
			name: "Bellatrix block",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedBellatrixBlock(),
			},
			expectedPath: "/eth/v2/beacon/blocks",
		},
		{
			name: "Capella block",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedCapellaBlock(),
			},
			expectedPath: "/eth/v2/beacon/blocks",
		},
		{
			name: "Blinded Bellatrix block",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedBlindedBellatrixBlock(),
			},
			expectedPath: "/eth/v2/beacon/blinded_blocks",
		},
		{
			name: "Blinded Capella block",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedBlindedCapellaBlock(),
			},
			expectedPath: "/eth/v2/beacon/blinded_blocks",
		},
		{
			name: "Deneb block",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedDenebBlock(),
			},
			expectedPath: "/eth/v2/beacon/blocks",
		},
		{
			name: "Blinded Deneb block",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedBlindedDenebBlock(),
			},
			expectedPath: "/eth/v2/beacon/blinded_blocks",
		},
		{
			name: "Electra block",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedElectraBlock(),
			},
			expectedPath: "/eth/v2/beacon/blocks",
		},
		{
			name: "Blinded Electra block",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedBlindedElectraBlock(),
			},
			expectedPath: "/eth/v2/beacon/blinded_blocks",
		},
		{
			name: "Fulu block",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedFuluBlock(),
			},
			expectedPath: "/eth/v2/beacon/blocks",
		},
		{
			name: "Blinded Fulu block",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedBlindedFuluBlock(),
			},
			expectedPath: "/eth/v2/beacon/blinded_blocks",
		},
		{
			name: "Unsupported block type",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: nil,
			},
			wantErr:      true,
			errorMessage: "unsupported block type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := t.Context()
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

			if !tt.wantErr {
				jsonRestHandler.EXPECT().PostSSZ(
					gomock.Any(),
					tt.expectedPath,
					gomock.Any(),
					gomock.Any(),
				).Return(nil, nil, nil).Times(1)
			}

			validatorClient := beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
			resp, err := validatorClient.proposeBeaconBlock(ctx, tt.block)

			if tt.wantErr {
				require.ErrorContains(t, tt.errorMessage, err)
				assert.Equal(t, (*ethpb.ProposeResponse)(nil), resp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
			}
		})
	}
}

func TestBeaconApiValidatorClient_ProposeBeaconBlockHTTPErrors(t *testing.T) {
	tests := []struct {
		name         string
		sszError     error
		expectJSON   bool
		errorMessage string
	}{
		{
			name: "HTTP 202 Accepted - block broadcast but failed validation",
			sszError: &httputil.DefaultJsonError{
				Code:    http.StatusAccepted,
				Message: "block broadcast but failed validation",
			},
			expectJSON:   false, // No fallback for non-406 errors
			errorMessage: "failed to submit block ssz",
		},
		{
			name: "Other HTTP error",
			sszError: &httputil.DefaultJsonError{
				Code:    http.StatusBadRequest,
				Message: "bad request",
			},
			expectJSON:   false, // No fallback for non-406 errors
			errorMessage: "failed to submit block ssz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := t.Context()
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

			jsonRestHandler.EXPECT().PostSSZ(
				gomock.Any(),
				"/eth/v2/beacon/blocks",
				gomock.Any(),
				gomock.Any(),
			).Return(nil, nil, tt.sszError).Times(1)

			if tt.expectJSON {
				// When SSZ fails, it falls back to JSON
				jsonRestHandler.EXPECT().Post(
					gomock.Any(),
					"/eth/v2/beacon/blocks",
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).Return(tt.sszError).Times(1)
			}

			validatorClient := beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
			_, err := validatorClient.proposeBeaconBlock(
				ctx,
				&ethpb.GenericSignedBeaconBlock{
					Block: generateSignedPhase0Block(),
				},
			)
			require.ErrorContains(t, tt.errorMessage, err)
		})
	}
}

func TestBeaconApiValidatorClient_ProposeBeaconBlockJSONFallback(t *testing.T) {
	tests := []struct {
		name         string
		block        *ethpb.GenericSignedBeaconBlock
		expectedPath string
		jsonError    error
		wantErr      bool
	}{
		{
			name: "Phase0 block JSON fallback success",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedPhase0Block(),
			},
			expectedPath: "/eth/v2/beacon/blocks",
		},
		{
			name: "Altair block JSON fallback success",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedAltairBlock(),
			},
			expectedPath: "/eth/v2/beacon/blocks",
		},
		{
			name: "Bellatrix block JSON fallback success",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedBellatrixBlock(),
			},
			expectedPath: "/eth/v2/beacon/blocks",
		},
		{
			name: "Capella block JSON fallback success",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedCapellaBlock(),
			},
			expectedPath: "/eth/v2/beacon/blocks",
		},
		{
			name: "Blinded Bellatrix block JSON fallback success",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedBlindedBellatrixBlock(),
			},
			expectedPath: "/eth/v2/beacon/blinded_blocks",
		},
		{
			name: "Blinded Capella block JSON fallback success",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedBlindedCapellaBlock(),
			},
			expectedPath: "/eth/v2/beacon/blinded_blocks",
		},
		{
			name: "Deneb block JSON fallback success",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedDenebBlock(),
			},
			expectedPath: "/eth/v2/beacon/blocks",
		},
		{
			name: "Blinded Deneb block JSON fallback success",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedBlindedDenebBlock(),
			},
			expectedPath: "/eth/v2/beacon/blinded_blocks",
		},
		{
			name: "Electra block JSON fallback success",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedElectraBlock(),
			},
			expectedPath: "/eth/v2/beacon/blocks",
		},
		{
			name: "Blinded Electra block JSON fallback success",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedBlindedElectraBlock(),
			},
			expectedPath: "/eth/v2/beacon/blinded_blocks",
		},
		{
			name: "Fulu block JSON fallback success",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedFuluBlock(),
			},
			expectedPath: "/eth/v2/beacon/blocks",
		},
		{
			name: "Blinded Fulu block JSON fallback success",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedBlindedFuluBlock(),
			},
			expectedPath: "/eth/v2/beacon/blinded_blocks",
		},
		{
			name: "JSON fallback fails",
			block: &ethpb.GenericSignedBeaconBlock{
				Block: generateSignedPhase0Block(),
			},
			expectedPath: "/eth/v2/beacon/blocks",
			jsonError:    errors.New("json post failed"),
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := t.Context()
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

			// SSZ call fails with 406 to trigger JSON fallback
			jsonRestHandler.EXPECT().PostSSZ(
				gomock.Any(),
				tt.expectedPath,
				gomock.Any(),
				gomock.Any(),
			).Return(nil, nil, &httputil.DefaultJsonError{
				Code:    http.StatusNotAcceptable,
				Message: "SSZ not supported",
			}).Times(1)

			// JSON fallback
			jsonRestHandler.EXPECT().Post(
				gomock.Any(),
				tt.expectedPath,
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
			).Return(tt.jsonError).Times(1)

			validatorClient := beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
			resp, err := validatorClient.proposeBeaconBlock(ctx, tt.block)

			if tt.wantErr {
				assert.NotNil(t, err)
				assert.Equal(t, (*ethpb.ProposeResponse)(nil), resp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
			}
		})
	}
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

// Helper functions for generating test blocks for newer consensus versions
func generateSignedDenebBlock() *ethpb.GenericSignedBeaconBlock_Deneb {
	var blockContents structs.SignedBeaconBlockContentsDeneb
	if err := json.Unmarshal([]byte(rpctesting.DenebBlockContents), &blockContents); err != nil {
		panic(err)
	}
	genericBlock, err := blockContents.ToGeneric()
	if err != nil {
		panic(err)
	}
	return &ethpb.GenericSignedBeaconBlock_Deneb{
		Deneb: genericBlock.GetDeneb(),
	}
}

func generateSignedBlindedDenebBlock() *ethpb.GenericSignedBeaconBlock_BlindedDeneb {
	var blindedBlock structs.SignedBlindedBeaconBlockDeneb
	if err := json.Unmarshal([]byte(rpctesting.BlindedDenebBlock), &blindedBlock); err != nil {
		panic(err)
	}
	genericBlock, err := blindedBlock.ToGeneric()
	if err != nil {
		panic(err)
	}
	return &ethpb.GenericSignedBeaconBlock_BlindedDeneb{
		BlindedDeneb: genericBlock.GetBlindedDeneb(),
	}
}

func generateSignedElectraBlock() *ethpb.GenericSignedBeaconBlock_Electra {
	var blockContents structs.SignedBeaconBlockContentsElectra
	if err := json.Unmarshal([]byte(rpctesting.ElectraBlockContents), &blockContents); err != nil {
		panic(err)
	}
	genericBlock, err := blockContents.ToGeneric()
	if err != nil {
		panic(err)
	}
	return &ethpb.GenericSignedBeaconBlock_Electra{
		Electra: genericBlock.GetElectra(),
	}
}

func generateSignedBlindedElectraBlock() *ethpb.GenericSignedBeaconBlock_BlindedElectra {
	var blindedBlock structs.SignedBlindedBeaconBlockElectra
	if err := json.Unmarshal([]byte(rpctesting.BlindedElectraBlock), &blindedBlock); err != nil {
		panic(err)
	}
	genericBlock, err := blindedBlock.ToGeneric()
	if err != nil {
		panic(err)
	}
	return &ethpb.GenericSignedBeaconBlock_BlindedElectra{
		BlindedElectra: genericBlock.GetBlindedElectra(),
	}
}

func generateSignedFuluBlock() *ethpb.GenericSignedBeaconBlock_Fulu {
	var blockContents structs.SignedBeaconBlockContentsFulu
	if err := json.Unmarshal([]byte(rpctesting.FuluBlockContents), &blockContents); err != nil {
		panic(err)
	}
	genericBlock, err := blockContents.ToGeneric()
	if err != nil {
		panic(err)
	}
	return &ethpb.GenericSignedBeaconBlock_Fulu{
		Fulu: genericBlock.GetFulu(),
	}
}

func generateSignedBlindedFuluBlock() *ethpb.GenericSignedBeaconBlock_BlindedFulu {
	var blindedBlock structs.SignedBlindedBeaconBlockFulu
	if err := json.Unmarshal([]byte(rpctesting.BlindedFuluBlock), &blindedBlock); err != nil {
		panic(err)
	}
	genericBlock, err := blindedBlock.ToGeneric()
	if err != nil {
		panic(err)
	}
	return &ethpb.GenericSignedBeaconBlock_BlindedFulu{
		BlindedFulu: genericBlock.GetBlindedFulu(),
	}
}
