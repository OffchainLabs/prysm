package rpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	validatormock "github.com/prysmaticlabs/prysm/v5/testing/validator-mock"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestGetBeaconStatus_NotConnected(t *testing.T) {
	ctrl := gomock.NewController(t)
	nodeClient := validatormock.NewMockNodeClient(ctrl)
	nodeClient.EXPECT().SyncStatus(
		gomock.Any(), // ctx
		gomock.Any(),
	).Return(nil /*response*/, errors.New("uh oh"))
	srv := &Server{
		nodeClient: nodeClient,
	}
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v2/validator/beacon/status"), nil)
	wr := httptest.NewRecorder()
	wr.Body = &bytes.Buffer{}
	srv.GetBeaconStatus(wr, req)
	require.Equal(t, http.StatusOK, wr.Code)
	resp := &BeaconStatusResponse{}
	require.NoError(t, json.Unmarshal(wr.Body.Bytes(), resp))
	want := &BeaconStatusResponse{
		BeaconNodeEndpoint: "",
		Connected:          false,
		Syncing:            false,
	}
	assert.DeepEqual(t, want, resp)
}

func TestGetBeaconStatus_OK(t *testing.T) {
	ctrl := gomock.NewController(t)
	nodeClient := validatormock.NewMockNodeClient(ctrl)
	chainClient := validatormock.NewMockChainClient(ctrl)
	nodeClient.EXPECT().SyncStatus(
		gomock.Any(), // ctx
		gomock.Any(),
	).Return(&ethpb.SyncStatus{Syncing: true}, nil)
	timeStamp := timestamppb.New(time.Unix(0, 0))
	nodeClient.EXPECT().Genesis(
		gomock.Any(), // ctx
		gomock.Any(),
	).Return(&ethpb.Genesis{
		GenesisTime:            timeStamp,
		DepositContractAddress: []byte("hello"),
	}, nil)
	chainClient.EXPECT().ChainHead(
		gomock.Any(), // ctx
		gomock.Any(),
	).Return(&ethpb.ChainHead{
		HeadEpoch: 1,
	}, nil)
	srv := &Server{
		nodeClient:  nodeClient,
		chainClient: chainClient,
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v2/validator/beacon/status"), nil)
	wr := httptest.NewRecorder()
	wr.Body = &bytes.Buffer{}
	srv.GetBeaconStatus(wr, req)
	require.Equal(t, http.StatusOK, wr.Code)
	resp := &BeaconStatusResponse{}
	require.NoError(t, json.Unmarshal(wr.Body.Bytes(), resp))

	want := &BeaconStatusResponse{
		BeaconNodeEndpoint:     "",
		Connected:              true,
		Syncing:                true,
		GenesisTime:            fmt.Sprintf("%d", time.Unix(0, 0).Unix()),
		DepositContractAddress: "0x68656c6c6f",
		ChainHead: &ChainHead{
			HeadSlot:                   "0",
			HeadEpoch:                  "1",
			HeadBlockRoot:              "0x",
			FinalizedSlot:              "0",
			FinalizedEpoch:             "0",
			FinalizedBlockRoot:         "0x",
			JustifiedSlot:              "0",
			JustifiedEpoch:             "0",
			JustifiedBlockRoot:         "0x",
			PreviousJustifiedSlot:      "0",
			PreviousJustifiedEpoch:     "0",
			PreviousJustifiedBlockRoot: "0x",
			OptimisticStatus:           false,
		},
	}
	assert.DeepEqual(t, want, resp)
}

func TestServer_GetValidators(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		expectedReq *ethpb.ListValidatorsRequest
		chainResp   *ethpb.Validators
		want        *ValidatorsResponse
		wantCode    int
		wantErr     string
	}{
		{
			name:     "happypath on page_size, page_token, public_keys",
			wantCode: http.StatusOK,
			query:    "page_size=4&page_token=0&public_keys=0x855ae9c6184d6edd46351b375f16f541b2d33b0ed0da9be4571b13938588aee840ba606a946f0e8023ae3a4b2a43b4d4",
			expectedReq: func() *ethpb.ListValidatorsRequest {
				b, err := hexutil.Decode("0x855ae9c6184d6edd46351b375f16f541b2d33b0ed0da9be4571b13938588aee840ba606a946f0e8023ae3a4b2a43b4d4")
				require.NoError(t, err)
				pubkeys := [][]byte{b}
				return &ethpb.ListValidatorsRequest{
					PublicKeys: pubkeys,
					PageSize:   int32(4),
					PageToken:  "0",
				}
			}(),
			chainResp: func() *ethpb.Validators {
				b, err := hexutil.Decode("0x855ae9c6184d6edd46351b375f16f541b2d33b0ed0da9be4571b13938588aee840ba606a946f0e8023ae3a4b2a43b4d4")
				require.NoError(t, err)
				return &ethpb.Validators{
					Epoch: 0,
					ValidatorList: []*ethpb.Validators_ValidatorContainer{
						{
							Index: 0,
							Validator: &ethpb.Validator{
								PublicKey: b,
							},
						},
					},
					NextPageToken: "0",
					TotalSize:     0,
				}
			}(),
			want: &ValidatorsResponse{
				Epoch: 0,
				ValidatorList: []*ValidatorContainer{
					{
						Index: 0,
						Validator: &Validator{
							PublicKey:                  "0x855ae9c6184d6edd46351b375f16f541b2d33b0ed0da9be4571b13938588aee840ba606a946f0e8023ae3a4b2a43b4d4",
							WithdrawalCredentials:      "0x",
							EffectiveBalance:           0,
							Slashed:                    false,
							ActivationEligibilityEpoch: 0,
							ActivationEpoch:            0,
							ExitEpoch:                  0,
							WithdrawableEpoch:          0,
						},
					},
				},
				NextPageToken: "0",
				TotalSize:     0,
			},
		},
		{
			name:     "extra public key that's empty still returns correct response",
			wantCode: http.StatusOK,
			query:    "page_size=4&page_token=0&public_keys=0x855ae9c6184d6edd46351b375f16f541b2d33b0ed0da9be4571b13938588aee840ba606a946f0e8023ae3a4b2a43b4d4&public_keys=",
			expectedReq: func() *ethpb.ListValidatorsRequest {
				b, err := hexutil.Decode("0x855ae9c6184d6edd46351b375f16f541b2d33b0ed0da9be4571b13938588aee840ba606a946f0e8023ae3a4b2a43b4d4")
				require.NoError(t, err)
				pubkeys := [][]byte{b}
				return &ethpb.ListValidatorsRequest{
					PublicKeys: pubkeys,
					PageSize:   int32(4),
					PageToken:  "0",
				}
			}(),
			chainResp: func() *ethpb.Validators {
				b, err := hexutil.Decode("0x855ae9c6184d6edd46351b375f16f541b2d33b0ed0da9be4571b13938588aee840ba606a946f0e8023ae3a4b2a43b4d4")
				require.NoError(t, err)
				return &ethpb.Validators{
					Epoch: 0,
					ValidatorList: []*ethpb.Validators_ValidatorContainer{
						{
							Index: 0,
							Validator: &ethpb.Validator{
								PublicKey: b,
							},
						},
					},
					NextPageToken: "0",
					TotalSize:     0,
				}
			}(),
			want: &ValidatorsResponse{
				Epoch: 0,
				ValidatorList: []*ValidatorContainer{
					{
						Index: 0,
						Validator: &Validator{
							PublicKey:                  "0x855ae9c6184d6edd46351b375f16f541b2d33b0ed0da9be4571b13938588aee840ba606a946f0e8023ae3a4b2a43b4d4",
							WithdrawalCredentials:      "0x",
							EffectiveBalance:           0,
							Slashed:                    false,
							ActivationEligibilityEpoch: 0,
							ActivationEpoch:            0,
							ExitEpoch:                  0,
							WithdrawableEpoch:          0,
						},
					},
				},
				NextPageToken: "0",
				TotalSize:     0,
			},
		},
		{
			name:     "no public keys passed results in error",
			wantCode: http.StatusBadRequest,
			query:    "page_size=4&page_token=0&public_keys=",
			wantErr:  "no pubkeys provided",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			beaconChainClient := validatormock.NewMockChainClient(ctrl)
			if tt.wantErr == "" {
				beaconChainClient.EXPECT().Validators(
					gomock.Any(), // ctx
					tt.expectedReq,
				).Return(tt.chainResp, nil)
			}
			s := &Server{
				chainClient: beaconChainClient,
			}
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v2/validator/beacon/validators?%s", tt.query), http.NoBody)
			wr := httptest.NewRecorder()
			wr.Body = &bytes.Buffer{}
			s.GetValidators(wr, req)
			require.Equal(t, tt.wantCode, wr.Code)
			if tt.wantErr != "" {
				require.StringContains(t, tt.wantErr, string(wr.Body.Bytes()))
			} else {
				resp := &ValidatorsResponse{}
				require.NoError(t, json.Unmarshal(wr.Body.Bytes(), resp))

				require.DeepEqual(t, resp, tt.want)
			}
		})
	}
}
