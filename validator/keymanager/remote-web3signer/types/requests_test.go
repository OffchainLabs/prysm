package types_test

import (
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	validatorpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/validator-client"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/validator/keymanager/remote-web3signer/types"
	"github.com/prysmaticlabs/prysm/v5/validator/keymanager/remote-web3signer/types/mock"
)

func TestGetAggregateAndProofSignRequest(t *testing.T) {
	type args struct {
		request               *validatorpb.SignRequest
		genesisValidatorsRoot []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *types.AggregateAndProofSignRequest
		wantErr bool
	}{
		{
			name: "Happy Path Test",
			args: args{
				request:               mock.GetMockSignRequest("AGGREGATE_AND_PROOF"),
				genesisValidatorsRoot: make([]byte, fieldparams.RootLength),
			},
			want:    mock.AggregateAndProofSignRequest(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := types.GetAggregateAndProofSignRequest(tt.args.request, tt.args.genesisValidatorsRoot)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAggregateAndProofSignRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetAggregateAndProofSignRequest() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetAggregationSlotSignRequest(t *testing.T) {
	type args struct {
		request               *validatorpb.SignRequest
		genesisValidatorsRoot []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *types.AggregationSlotSignRequest
		wantErr bool
	}{
		{
			name: "Happy Path Test",
			args: args{
				request:               mock.GetMockSignRequest("AGGREGATION_SLOT"),
				genesisValidatorsRoot: make([]byte, fieldparams.RootLength),
			},
			want:    mock.AggregationSlotSignRequest(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := types.GetAggregationSlotSignRequest(tt.args.request, tt.args.genesisValidatorsRoot)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAggregationSlotSignRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetAggregationSlotSignRequest() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetAttestationSignRequest(t *testing.T) {
	type args struct {
		request               *validatorpb.SignRequest
		genesisValidatorsRoot []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *types.AttestationSignRequest
		wantErr bool
	}{
		{
			name: "Happy Path Test",
			args: args{
				request:               mock.GetMockSignRequest("ATTESTATION"),
				genesisValidatorsRoot: make([]byte, fieldparams.RootLength),
			},
			want: mock.AttestationSignRequest(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := types.GetAttestationSignRequest(tt.args.request, tt.args.genesisValidatorsRoot)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAttestationSignRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetAttestationSignRequest() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetBlockSignRequest(t *testing.T) {
	type args struct {
		request               *validatorpb.SignRequest
		genesisValidatorsRoot []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *types.BlockSignRequest
		wantErr bool
	}{
		{
			name: "Happy Path Test",
			args: args{
				request:               mock.GetMockSignRequest("BLOCK"),
				genesisValidatorsRoot: make([]byte, fieldparams.RootLength),
			},
			want:    mock.BlockSignRequest(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := types.GetBlockSignRequest(tt.args.request, tt.args.genesisValidatorsRoot)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetBlockSignRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetBlockSignRequest() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetBlockV2AltairSignRequest(t *testing.T) {
	type args struct {
		request               *validatorpb.SignRequest
		genesisValidatorsRoot []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *types.BlockAltairSignRequest
		wantErr bool
	}{
		{
			name: "Happy Path Test",
			args: args{
				request:               mock.GetMockSignRequest("BLOCK_V2"),
				genesisValidatorsRoot: make([]byte, fieldparams.RootLength),
			},
			want:    mock.BlockV2AltairSignRequest(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := types.GetBlockAltairSignRequest(tt.args.request, tt.args.genesisValidatorsRoot)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetBlockAltairSignRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetBlockAltairSignRequest() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetRandaoRevealSignRequest(t *testing.T) {
	type args struct {
		request               *validatorpb.SignRequest
		genesisValidatorsRoot []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *types.RandaoRevealSignRequest
		wantErr bool
	}{
		{
			name: "Happy Path Test",
			args: args{
				request:               mock.GetMockSignRequest("RANDAO_REVEAL"),
				genesisValidatorsRoot: make([]byte, fieldparams.RootLength),
			},
			want:    mock.RandaoRevealSignRequest(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := types.GetRandaoRevealSignRequest(tt.args.request, tt.args.genesisValidatorsRoot)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetRandaoRevealSignRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetRandaoRevealSignRequest() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSyncCommitteeContributionAndProofSignRequest(t *testing.T) {
	type args struct {
		request               *validatorpb.SignRequest
		genesisValidatorsRoot []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *types.SyncCommitteeContributionAndProofSignRequest
		wantErr bool
	}{
		{
			name: "Happy Path Test",
			args: args{
				request:               mock.GetMockSignRequest("SYNC_COMMITTEE_CONTRIBUTION_AND_PROOF"),
				genesisValidatorsRoot: make([]byte, fieldparams.RootLength),
			},
			want:    mock.SyncCommitteeContributionAndProofSignRequest(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := types.GetSyncCommitteeContributionAndProofSignRequest(tt.args.request, tt.args.genesisValidatorsRoot)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetSyncCommitteeContributionAndProofSignRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetSyncCommitteeContributionAndProofSignRequest() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSyncCommitteeMessageSignRequest(t *testing.T) {
	type args struct {
		request               *validatorpb.SignRequest
		genesisValidatorsRoot []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *types.SyncCommitteeMessageSignRequest
		wantErr bool
	}{
		{
			name: "Happy Path Test",
			args: args{
				request:               mock.GetMockSignRequest("SYNC_COMMITTEE_MESSAGE"),
				genesisValidatorsRoot: make([]byte, fieldparams.RootLength),
			},
			want:    mock.SyncCommitteeMessageSignRequest(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := types.GetSyncCommitteeMessageSignRequest(tt.args.request, tt.args.genesisValidatorsRoot)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetSyncCommitteeMessageSignRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetSyncCommitteeMessageSignRequest() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSyncCommitteeSelectionProofSignRequest(t *testing.T) {
	type args struct {
		request               *validatorpb.SignRequest
		genesisValidatorsRoot []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *types.SyncCommitteeSelectionProofSignRequest
		wantErr bool
	}{
		{
			name: "Happy Path Test",
			args: args{
				request:               mock.GetMockSignRequest("SYNC_COMMITTEE_SELECTION_PROOF"),
				genesisValidatorsRoot: make([]byte, fieldparams.RootLength),
			},
			want:    mock.SyncCommitteeSelectionProofSignRequest(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := types.GetSyncCommitteeSelectionProofSignRequest(tt.args.request, tt.args.genesisValidatorsRoot)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetSyncCommitteeSelectionProofSignRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetSyncCommitteeSelectionProofSignRequest() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetVoluntaryExitSignRequest(t *testing.T) {
	type args struct {
		request               *validatorpb.SignRequest
		genesisValidatorsRoot []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *types.VoluntaryExitSignRequest
		wantErr bool
	}{
		{
			name: "Happy Path Test",
			args: args{
				request:               mock.GetMockSignRequest("VOLUNTARY_EXIT"),
				genesisValidatorsRoot: make([]byte, fieldparams.RootLength),
			},
			want:    mock.VoluntaryExitSignRequest(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := types.GetVoluntaryExitSignRequest(tt.args.request, tt.args.genesisValidatorsRoot)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetVoluntaryExitSignRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetVoluntaryExitSignRequest() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetBlockV2BlindedSignRequest(t *testing.T) {
	type args struct {
		request               *validatorpb.SignRequest
		genesisValidatorsRoot []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *types.BlockV2BlindedSignRequest
		wantErr bool
	}{
		{
			name: "Happy Path Test non blinded Bellatrix",
			args: args{
				request:               mock.GetMockSignRequest("BLOCK_V2_BELLATRIX"),
				genesisValidatorsRoot: make([]byte, fieldparams.RootLength),
			},
			want: mock.BlockV2BlindedSignRequest(func(t *testing.T) []byte {
				bytevalue, err := hexutil.Decode("0xcd7c49966ebe72b1214e6d4733adf6bf06935c5fbc3b3ad08e84e3085428b82f")
				require.NoError(t, err)
				return bytevalue
			}(t), "BELLATRIX"),
			wantErr: false,
		},
		{
			name: "Happy Path Test blinded Bellatrix",
			args: args{
				request:               mock.GetMockSignRequest("BLOCK_V2_BLINDED_BELLATRIX"),
				genesisValidatorsRoot: make([]byte, fieldparams.RootLength),
			},
			want: mock.BlockV2BlindedSignRequest(func(t *testing.T) []byte {
				bytevalue, err := hexutil.Decode("0xbabb9c2d10dd3f16dc50e31fd6eb270c9c5e95a6dcb5a1eb34389ef28194285b")
				require.NoError(t, err)
				return bytevalue
			}(t), "BELLATRIX"),
			wantErr: false,
		},
		{
			name: "Happy Path Test non blinded Capella",
			args: args{
				request:               mock.GetMockSignRequest("BLOCK_V2_CAPELLA"),
				genesisValidatorsRoot: make([]byte, fieldparams.RootLength),
			},
			want: mock.BlockV2BlindedSignRequest(func(t *testing.T) []byte {
				bytevalue, err := hexutil.Decode("0x74b4bb048d39c75f175fbb2311062eb9867d79b712907f39544fcaf2d7e1b433")
				require.NoError(t, err)
				return bytevalue
			}(t), "CAPELLA"),
			wantErr: false,
		},
		{
			name: "Happy Path Test blinded Capella",
			args: args{
				request:               mock.GetMockSignRequest("BLOCK_V2_BLINDED_CAPELLA"),
				genesisValidatorsRoot: make([]byte, fieldparams.RootLength),
			},
			want: mock.BlockV2BlindedSignRequest(func(t *testing.T) []byte {
				bytevalue, err := hexutil.Decode("0x54797f27f45a91d2cf4d73e509c62e464d648ec34e07ddba946adee742039e76")
				require.NoError(t, err)
				return bytevalue
			}(t), "CAPELLA"),
			wantErr: false,
		},
		{
			name: "Happy Path Test non blinded Deneb",
			args: args{
				request:               mock.GetMockSignRequest("BLOCK_V2_DENEB"),
				genesisValidatorsRoot: make([]byte, fieldparams.RootLength),
			},
			want: mock.BlockV2BlindedSignRequest(func(t *testing.T) []byte {
				bytevalue, err := hexutil.Decode("0xbce73ee2c617851846af2b3ea2287e3b686098e18ae508c7271aaa06ab1d06cd")
				require.NoError(t, err)
				return bytevalue
			}(t), "DENEB"),
			wantErr: false,
		},
		{
			name: "Happy Path Test blinded Deneb",
			args: args{
				request:               mock.GetMockSignRequest("BLOCK_V2_BLINDED_DENEB"),
				genesisValidatorsRoot: make([]byte, fieldparams.RootLength),
			},
			want: mock.BlockV2BlindedSignRequest(func(t *testing.T) []byte {
				bytevalue, err := hexutil.Decode("0xfeb1f7e4f704e72544f4f097b36cb3f3af83043765ad9ad3c3a6cd7fac605055")
				require.NoError(t, err)
				return bytevalue
			}(t), "DENEB"),
			wantErr: false,
		},
		{
			name: "Happy Path Test non blinded Electra",
			args: args{
				request:               mock.GetMockSignRequest("BLOCK_V2_ELECTRA"),
				genesisValidatorsRoot: make([]byte, fieldparams.RootLength),
			},
			want: mock.BlockV2BlindedSignRequest(func(t *testing.T) []byte {
				bytevalue, err := hexutil.Decode("0xca4f98890bc98a59f015d06375a5e00546b8f2ac1e88d31b1774ea28d4b3e7d1")
				require.NoError(t, err)
				return bytevalue
			}(t), "ELECTRA"),
			wantErr: false,
		},
		{
			name: "Happy Path Test blinded Electra",
			args: args{
				request:               mock.GetMockSignRequest("BLOCK_V2_BLINDED_ELECTRA"),
				genesisValidatorsRoot: make([]byte, fieldparams.RootLength),
			},
			want: mock.BlockV2BlindedSignRequest(func(t *testing.T) []byte {
				bytevalue, err := hexutil.Decode("0x60cd4e8a557e64d00f63777b53f18c10cc122997c55f40a37cb19dc2edd3b0c7")
				require.NoError(t, err)
				return bytevalue
			}(t), "ELECTRA"),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := types.GetBlockV2BlindedSignRequest(tt.args.request, tt.args.genesisValidatorsRoot)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetBlockV2BlindedSignRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetBlockV2BlindedSignRequest() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetValidatorRegistrationSignRequest(t *testing.T) {
	type args struct {
		request *validatorpb.SignRequest
	}
	tests := []struct {
		name    string
		args    args
		want    *types.ValidatorRegistrationSignRequest
		wantErr bool
	}{
		{
			name: "Happy Path Test",
			args: args{
				request: mock.GetMockSignRequest("VALIDATOR_REGISTRATION"),
			},
			want:    mock.ValidatorRegistrationSignRequest(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := types.GetValidatorRegistrationSignRequest(tt.args.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetValidatorRegistrationSignRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetValidatorRegistrationSignRequest() got = %v, want %v", got, tt.want)
			}
		})
	}
}
