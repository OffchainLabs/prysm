package client

import (
	"context"
	"fmt"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/pkg/errors"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestSubmitPayloadAttestation_PayloadAttestationDataFailure(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			m.validatorClient.EXPECT().
				PayloadAttestationData(gomock.Any(), gomock.Any()).
				Return(nil, errors.New("request failed"))

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.SubmitPayloadAttestation(t.Context(), 1, pubKey)
			require.LogsContain(t, hook, "Could not request payload attestation data")
		})
	}
}

func TestSubmitPayloadAttestation_ValidatorDutiesRequestFailure(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			validator.duties = &ethpb.ValidatorDutiesContainer{CurrentEpochDuties: []*ethpb.ValidatorDuty{}}
			defer finish()

			m.validatorClient.EXPECT().
				PayloadAttestationData(gomock.Any(), gomock.Any()).
				Return(&ethpb.PayloadAttestationData{
					BeaconBlockRoot: bytesutil.PadTo([]byte{'a'}, 32),
					Slot:            1,
					PayloadPresent:  true,
				}, nil)

			m.validatorClient.EXPECT().
				DomainData(gomock.Any(), gomock.Any()).
				Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.SubmitPayloadAttestation(t.Context(), 1, pubKey)
			require.LogsContain(t, hook, "Could not fetch validator assignment")
		})
	}
}

func TestSubmitPayloadAttestation_BadDomainData(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			validatorIndex := primitives.ValidatorIndex(7)
			validator.duties = &ethpb.ValidatorDutiesContainer{CurrentEpochDuties: []*ethpb.ValidatorDuty{
				{
					PublicKey:      validatorKey.PublicKey().Marshal(),
					ValidatorIndex: validatorIndex,
				},
			}}

			m.validatorClient.EXPECT().
				PayloadAttestationData(gomock.Any(), gomock.Any()).
				Return(&ethpb.PayloadAttestationData{
					BeaconBlockRoot: bytesutil.PadTo([]byte{'a'}, 32),
					Slot:            1,
					PayloadPresent:  true,
				}, nil)

			m.validatorClient.EXPECT().
				DomainData(gomock.Any(), gomock.Any()).
				Return(nil, errors.New("uh oh"))

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.SubmitPayloadAttestation(t.Context(), 1, pubKey)
			require.LogsContain(t, hook, "Could not get PTC attester domain data")
		})
	}
}

func TestSubmitPayloadAttestation_CouldNotSubmit(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			validatorIndex := primitives.ValidatorIndex(7)
			validator.duties = &ethpb.ValidatorDutiesContainer{CurrentEpochDuties: []*ethpb.ValidatorDuty{
				{
					PublicKey:      validatorKey.PublicKey().Marshal(),
					ValidatorIndex: validatorIndex,
				},
			}}

			m.validatorClient.EXPECT().
				PayloadAttestationData(gomock.Any(), gomock.Any()).
				Return(&ethpb.PayloadAttestationData{
					BeaconBlockRoot: bytesutil.PadTo([]byte{'a'}, 32),
					Slot:            1,
					PayloadPresent:  true,
				}, nil)

			m.validatorClient.EXPECT().
				DomainData(gomock.Any(), gomock.Any()).
				Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

			m.validatorClient.EXPECT().
				SubmitPayloadAttestation(gomock.Any(), gomock.AssignableToTypeOf(&ethpb.PayloadAttestationMessage{})).
				Return(&emptypb.Empty{}, errors.New("submit failed"))

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.SubmitPayloadAttestation(t.Context(), 1, pubKey)
			require.LogsContain(t, hook, "Could not submit payload attestation")
		})
	}
}

func TestSubmitPayloadAttestation_OK(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			validatorIndex := primitives.ValidatorIndex(7)
			validator.duties = &ethpb.ValidatorDutiesContainer{CurrentEpochDuties: []*ethpb.ValidatorDuty{
				{
					PublicKey:      validatorKey.PublicKey().Marshal(),
					ValidatorIndex: validatorIndex,
				},
			}}

			blockRoot := bytesutil.PadTo([]byte{'b'}, 32)
			m.validatorClient.EXPECT().
				PayloadAttestationData(gomock.Any(), gomock.Any()).
				Return(&ethpb.PayloadAttestationData{
					BeaconBlockRoot: blockRoot,
					Slot:            1,
					PayloadPresent:  true,
				}, nil)

			m.validatorClient.EXPECT().
				DomainData(gomock.Any(), gomock.Any()).
				Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

			var generatedMsg *ethpb.PayloadAttestationMessage
			m.validatorClient.EXPECT().
				SubmitPayloadAttestation(gomock.Any(), gomock.AssignableToTypeOf(&ethpb.PayloadAttestationMessage{})).
				Do(func(_ context.Context, msg *ethpb.PayloadAttestationMessage) {
					generatedMsg = msg
				}).
				Return(&emptypb.Empty{}, nil)

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.SubmitPayloadAttestation(t.Context(), 1, pubKey)

			require.LogsDoNotContain(t, hook, "Could not")
			require.LogsContain(t, hook, "Submitted new payload attestation")
			require.Equal(t, validatorIndex, generatedMsg.ValidatorIndex)
			require.DeepEqual(t, blockRoot, generatedMsg.Data.BeaconBlockRoot)
			require.Equal(t, true, generatedMsg.Data.PayloadPresent)
			require.Equal(t, 96, len(generatedMsg.Signature))
		})
	}
}
