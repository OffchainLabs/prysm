package beacon_api

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/mock/gomock"
)

func TestGetDomainData_ValidDomainData(t *testing.T) {
	const genesisValidatorRoot = "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
	forkVersion := params.BeaconConfig().AltairForkVersion
	epoch := params.BeaconConfig().AltairForkEpoch
	domainType := params.BeaconConfig().DomainBeaconProposer

	genesisValidatorRootBytes, err := hexutil.Decode(genesisValidatorRoot)
	require.NoError(t, err)

	expectedForkDataRoot, err := (&ethpb.ForkData{
		CurrentVersion:        forkVersion,
		GenesisValidatorsRoot: genesisValidatorRootBytes,
	}).HashTreeRoot()
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

	// Make sure that Genesis() is called exactly once
	genesisProvider := mock.NewMockGenesisProvider(ctrl)
	genesisProvider.EXPECT().Genesis(gomock.Any()).Return(
		&structs.Genesis{GenesisValidatorsRoot: genesisValidatorRoot},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{genesisProvider: genesisProvider}
	resp, err := validatorClient.domainData(ctx, epoch, domainType)
	assert.NoError(t, err)
	require.NotNil(t, resp)

	var expectedSignatureDomain []byte
	expectedSignatureDomain = append(expectedSignatureDomain, domainType[:]...)
	expectedSignatureDomain = append(expectedSignatureDomain, expectedForkDataRoot[:28]...)

	assert.Equal(t, len(expectedSignatureDomain), len(resp.SignatureDomain))
	assert.DeepEqual(t, expectedSignatureDomain, resp.SignatureDomain)
}

func TestGetDomainData_GenesisError(t *testing.T) {
	epoch := params.BeaconConfig().AltairForkEpoch
	domainType := params.BeaconConfig().DomainBeaconProposer

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

	// Make sure that Genesis() is called exactly once
	genesisProvider := mock.NewMockGenesisProvider(ctrl)
	genesisProvider.EXPECT().Genesis(gomock.Any()).Return(nil, errors.New("foo error")).Times(1)

	validatorClient := &beaconApiValidatorClient{genesisProvider: genesisProvider}
	_, err := validatorClient.domainData(ctx, epoch, domainType)
	assert.ErrorContains(t, "failed to get genesis info", err)
	assert.ErrorContains(t, "foo error", err)
}

func TestGetDomainData_InvalidGenesisRoot(t *testing.T) {
	epoch := params.BeaconConfig().AltairForkEpoch
	domainType := params.BeaconConfig().DomainBeaconProposer

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

	// Make sure that Genesis() is called exactly once
	genesisProvider := mock.NewMockGenesisProvider(ctrl)
	genesisProvider.EXPECT().Genesis(gomock.Any()).Return(
		&structs.Genesis{GenesisValidatorsRoot: "foo"},
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{genesisProvider: genesisProvider}
	_, err := validatorClient.domainData(ctx, epoch, domainType)
	assert.ErrorContains(t, "failed to decode genesis validators root: foo", err)
}

func TestGetDomainData_VoluntaryExitEIP7044(t *testing.T) {
	const genesisValidatorRoot = "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
	gvr, err := hexutil.Decode(genesisValidatorRoot)
	require.NoError(t, err)

	cfg := params.BeaconConfig()
	preDenebGenesis := strconv.FormatInt(time.Now().Unix(), 10) // current epoch 0, before Deneb
	postDenebGenesis := "0"                                     // current epoch far past Deneb

	tests := []struct {
		name        string
		genesisTime string
		epoch       primitives.Epoch
		domainType  [4]byte
		wantVersion []byte
	}{
		{
			name:        "pre-Deneb exit uses fork at exit epoch",
			genesisTime: preDenebGenesis,
			epoch:       cfg.AltairForkEpoch,
			domainType:  cfg.DomainVoluntaryExit,
			wantVersion: cfg.AltairForkVersion,
		},
		{
			name:        "post-Deneb exit uses Capella version",
			genesisTime: postDenebGenesis,
			epoch:       cfg.DenebForkEpoch,
			domainType:  cfg.DomainVoluntaryExit,
			wantVersion: cfg.CapellaForkVersion,
		},
		{
			name:        "post-Deneb exit with pre-Deneb epoch uses Capella version",
			genesisTime: postDenebGenesis,
			epoch:       cfg.AltairForkEpoch,
			domainType:  cfg.DomainVoluntaryExit,
			wantVersion: cfg.CapellaForkVersion,
		},
		{
			name:        "non-exit domain uses fork at epoch",
			genesisTime: postDenebGenesis,
			epoch:       cfg.DenebForkEpoch,
			domainType:  cfg.DomainBeaconProposer,
			wantVersion: cfg.DenebForkVersion,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			genesisProvider := mock.NewMockGenesisProvider(ctrl)
			genesisProvider.EXPECT().Genesis(gomock.Any()).Return(
				&structs.Genesis{GenesisValidatorsRoot: genesisValidatorRoot, GenesisTime: tt.genesisTime},
				nil,
			).Times(1)

			validatorClient := &beaconApiValidatorClient{genesisProvider: genesisProvider}
			resp, err := validatorClient.domainData(t.Context(), tt.epoch, tt.domainType)
			require.NoError(t, err)

			want, err := signing.ComputeDomain(tt.domainType, tt.wantVersion, gvr)
			require.NoError(t, err)
			require.Equal(t, hexutil.Encode(want), hexutil.Encode(resp.SignatureDomain))
		})
	}
}
