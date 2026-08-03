package client

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"go.uber.org/mock/gomock"
)

func TestSubmitAggregateAndProof_GetDutiesRequestFailure(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, _, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			validator.duties = testDutyStore()
			defer finish()

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.SubmitAggregateAndProof(t.Context(), 0, pubKey)

			require.LogsContain(t, hook, "Could not fetch validator assignment")
		})
	}
}

func TestSubmitAggregateAndProof_SignFails(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.duties = testDutyStore(&ethpb.ValidatorDuty{
				PublicKey: validatorKey.PublicKey().Marshal(),
			})

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().SubmitAggregateSelectionProof(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.AggregateSelectionRequest{}),
				gomock.Any(),
				gomock.Any(),
			).Return(&ethpb.AggregateSelectionResponse{
				AggregateAndProof: &ethpb.AggregateAttestationAndProof{
					AggregatorIndex: 0,
					Aggregate: util.HydrateAttestation(&ethpb.Attestation{
						AggregationBits: make([]byte, 1),
					}),
					SelectionProof: make([]byte, 96),
				},
			}, nil)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&ethpb.DomainResponse{SignatureDomain: nil}, errors.New("bad domain root"))

			validator.SubmitAggregateAndProof(t.Context(), 0, pubKey)
		})
	}
}

func TestSubmitAggregateAndProof_Ok(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("Phase 0 (SlashingProtectionMinimal:%v)", isSlashingProtectionMinimal), func(t *testing.T) {
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.duties = testDutyStore(&ethpb.ValidatorDuty{
				PublicKey: validatorKey.PublicKey().Marshal(),
			})

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().SubmitAggregateSelectionProof(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.AggregateSelectionRequest{}),
				gomock.Any(),
				gomock.Any(),
			).Return(&ethpb.AggregateSelectionResponse{
				AggregateAndProof: &ethpb.AggregateAttestationAndProof{
					AggregatorIndex: 0,
					Aggregate: util.HydrateAttestation(&ethpb.Attestation{
						AggregationBits: make([]byte, 1),
					}),
					SelectionProof: make([]byte, 96),
				},
			}, nil)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().SubmitSignedAggregateSelectionProof(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.SignedAggregateSubmitRequest{}),
			).Return(&ethpb.SignedAggregateSubmitResponse{AttestationDataRoot: make([]byte, 32)}, nil)

			validator.SubmitAggregateAndProof(t.Context(), 0, pubKey)
		})
	}
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("Electra (SlashingProtectionMinimal:%v)", isSlashingProtectionMinimal), func(t *testing.T) {
			electraForkEpoch := uint64(1)
			params.SetupTestConfigCleanup(t)
			cfg := params.BeaconConfig().Copy()
			cfg.ElectraForkEpoch = primitives.Epoch(electraForkEpoch)
			params.OverrideBeaconConfig(cfg)

			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.duties = testDutyStore(&ethpb.ValidatorDuty{
				PublicKey: validatorKey.PublicKey().Marshal(),
			})

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().SubmitAggregateSelectionProofElectra(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.AggregateSelectionRequest{}),
				gomock.Any(),
				gomock.Any(),
			).Return(&ethpb.AggregateSelectionElectraResponse{
				AggregateAndProof: &ethpb.AggregateAttestationAndProofElectra{
					AggregatorIndex: 0,
					Aggregate: util.HydrateAttestationElectra(&ethpb.AttestationElectra{
						AggregationBits: make([]byte, 1),
					}),
					SelectionProof: make([]byte, 96),
				},
			}, nil)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().SubmitSignedAggregateSelectionProofElectra(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.SignedAggregateSubmitElectraRequest{}),
			).Return(&ethpb.SignedAggregateSubmitResponse{AttestationDataRoot: make([]byte, 32)}, nil)

			validator.SubmitAggregateAndProof(t.Context(), params.BeaconConfig().SlotsPerEpoch.Mul(electraForkEpoch), pubKey)
		})
	}
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("Gloas (SlashingProtectionMinimal:%v)", isSlashingProtectionMinimal), func(t *testing.T) {
			gloasForkEpoch := uint64(1)
			params.SetupTestConfigCleanup(t)
			cfg := params.BeaconConfig().Copy()
			cfg.ElectraForkEpoch = primitives.Epoch(gloasForkEpoch)
			cfg.FuluForkEpoch = primitives.Epoch(gloasForkEpoch)
			cfg.GloasForkEpoch = primitives.Epoch(gloasForkEpoch)
			params.OverrideBeaconConfig(cfg)

			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.duties = testDutyStore(&ethpb.ValidatorDuty{
				PublicKey: validatorKey.PublicKey().Marshal(),
			})

			m.validatorClient.EXPECT().DomainData(gomock.Any(), gomock.Any()).
				Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

			electraAggregate := &ethpb.AggregateAttestationAndProofElectra{
				AggregatorIndex: 1,
				Aggregate: util.HydrateAttestationElectra(&ethpb.AttestationElectra{
					AggregationBits: bitfield.NewBitlist(1),
				}),
				SelectionProof: make([]byte, 96),
			}
			m.validatorClient.EXPECT().SubmitAggregateSelectionProofElectra(
				gomock.Any(),
				gomock.AssignableToTypeOf(&ethpb.AggregateSelectionRequest{}),
				gomock.Any(),
				gomock.Any(),
			).Return(&ethpb.AggregateSelectionElectraResponse{AggregateAndProof: electraAggregate}, nil)

			m.validatorClient.EXPECT().DomainData(gomock.Any(), gomock.Any()).
				Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

			m.validatorClient.EXPECT().SubmitSignedAggregateSelectionProofElectra(
				gomock.Any(),
				gomock.Any(),
			).DoAndReturn(func(_ context.Context, req *ethpb.SignedAggregateSubmitElectraRequest) (*ethpb.SignedAggregateSubmitResponse, error) {
				require.DeepEqual(t, electraAggregate, req.SignedAggregateAndProof.Message)
				return &ethpb.SignedAggregateSubmitResponse{AttestationDataRoot: make([]byte, 32)}, nil
			})

			validator.SubmitAggregateAndProof(t.Context(), params.BeaconConfig().SlotsPerEpoch.Mul(gloasForkEpoch), pubKey)
		})
	}
}

func TestSubmitAggregateAndProof_Distributed(t *testing.T) {
	validatorIdx := primitives.ValidatorIndex(123)
	slot := primitives.Slot(456)
	ctx := t.Context()
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.duties = testDutyStore(&ethpb.ValidatorDuty{
				PublicKey:      validatorKey.PublicKey().Marshal(),
				ValidatorIndex: validatorIdx,
				AttesterSlot:   slot,
			})

			validator.pubkeyToStatus[pubKey] = &validatorStatus{
				publicKey: validatorKey.PublicKey().Marshal(),
				index:     validatorIdx,
			}
			dvProvider := newDistributedSelector(validator)
			dvProvider.attSelections = map[attSelectionKey]iface.BeaconCommitteeSelection{
				{slot: slot, index: 123}: {
					SelectionProof: make([]byte, 96),
					Slot:           slot,
					ValidatorIndex: validatorIdx,
				},
			}
			validator.aggSelector = dvProvider

			m.validatorClient.EXPECT().SubmitAggregateSelectionProof(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.AggregateSelectionRequest{}),
				gomock.Any(),
				gomock.Any(),
			).Return(&ethpb.AggregateSelectionResponse{
				AggregateAndProof: &ethpb.AggregateAttestationAndProof{
					AggregatorIndex: 0,
					Aggregate: util.HydrateAttestation(&ethpb.Attestation{
						AggregationBits: make([]byte, 1),
					}),
					SelectionProof: make([]byte, 96),
				},
			}, nil)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().SubmitSignedAggregateSelectionProof(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.SignedAggregateSubmitRequest{}),
			).Return(&ethpb.SignedAggregateSubmitResponse{AttestationDataRoot: make([]byte, 32)}, nil)

			validator.SubmitAggregateAndProof(ctx, slot, pubKey)
		})
	}
}

func TestWaitForSlotTwoThird_WaitCorrectly(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			validator, _, _, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			currentTime := time.Now()
			numOfSlots := primitives.Slot(4)
			slotDuration := params.BeaconConfig().SlotDuration()
			validator.genesisTime = currentTime.Add(-slotDuration * time.Duration(numOfSlots))
			timeToSleep := params.BeaconConfig().SlotComponentDuration(params.BeaconConfig().AggregateDueBPS)

			twoThirdTime := currentTime.Add(timeToSleep)
			validator.waitUntilAggregateDue(t.Context(), numOfSlots)
			currentTime = time.Now()
			assert.Equal(t, twoThirdTime.Unix(), currentTime.Unix())
		})
	}
}

func TestWaitForSlotTwoThird_DoneContext_ReturnsImmediately(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			validator, _, _, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			currentTime := time.Now()
			numOfSlots := primitives.Slot(4)
			slotDuration := params.BeaconConfig().SlotDuration()
			validator.genesisTime = currentTime.Add(-slotDuration * time.Duration(numOfSlots))

			expectedTime := time.Now()
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			validator.waitUntilAggregateDue(ctx, numOfSlots)
			currentTime = time.Now()
			assert.Equal(t, expectedTime.Unix(), currentTime.Unix())
		})
	}
}

func TestAggregateAndProofSignature_CanSignValidSignature(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("Phase 0 (SlashingProtectionMinimal:%v)", isSlashingProtectionMinimal), func(t *testing.T) {
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				&ethpb.DomainRequest{Epoch: 0, Domain: params.BeaconConfig().DomainAggregateAndProof[:]},
			).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			agg := &ethpb.AggregateAttestationAndProof{
				AggregatorIndex: 0,
				Aggregate: util.HydrateAttestation(&ethpb.Attestation{
					AggregationBits: bitfield.NewBitlist(1),
				}),
				SelectionProof: make([]byte, 96),
			}
			sig, err := validator.aggregateAndProofSig(t.Context(), pubKey, agg, 0 /* slot */)
			require.NoError(t, err)
			_, err = bls.SignatureFromBytes(sig)
			require.NoError(t, err)
		})
	}
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("Electra (SlashingProtectionMinimal:%v)", isSlashingProtectionMinimal), func(t *testing.T) {
			electraForkEpoch := uint64(1)
			params.SetupTestConfigCleanup(t)
			cfg := params.BeaconConfig().Copy()
			cfg.ElectraForkEpoch = primitives.Epoch(electraForkEpoch)
			params.OverrideBeaconConfig(cfg)

			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				&ethpb.DomainRequest{Epoch: 0, Domain: params.BeaconConfig().DomainAggregateAndProof[:]},
			).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			agg := &ethpb.AggregateAttestationAndProofElectra{
				AggregatorIndex: 0,
				Aggregate: util.HydrateAttestationElectra(&ethpb.AttestationElectra{
					AggregationBits: bitfield.NewBitlist(1),
				}),
				SelectionProof: make([]byte, 96),
			}
			sig, err := validator.aggregateAndProofSig(t.Context(), pubKey, agg, params.BeaconConfig().SlotsPerEpoch.Mul(electraForkEpoch) /* slot */)
			require.NoError(t, err)
			_, err = bls.SignatureFromBytes(sig)
			require.NoError(t, err)
		})
	}
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("Gloas (SlashingProtectionMinimal:%v)", isSlashingProtectionMinimal), func(t *testing.T) {
			gloasForkEpoch := uint64(1)
			params.SetupTestConfigCleanup(t)
			cfg := params.BeaconConfig().Copy()
			cfg.ElectraForkEpoch = primitives.Epoch(gloasForkEpoch)
			cfg.FuluForkEpoch = primitives.Epoch(gloasForkEpoch)
			cfg.GloasForkEpoch = primitives.Epoch(gloasForkEpoch)
			params.OverrideBeaconConfig(cfg)

			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			domain := make([]byte, 32)
			m.validatorClient.EXPECT().DomainData(
				gomock.Any(),
				&ethpb.DomainRequest{Epoch: 0, Domain: params.BeaconConfig().DomainAggregateAndProof[:]},
			).Return(&ethpb.DomainResponse{SignatureDomain: domain}, nil)

			electraAggregate := &ethpb.AggregateAttestationAndProofElectra{
				AggregatorIndex: 1,
				Aggregate: util.HydrateAttestationElectra(&ethpb.AttestationElectra{
					AggregationBits: bitfield.NewBitlist(1),
				}),
				SelectionProof: make([]byte, 96),
			}
			gloasAggregate := ethpb.AggregateAttestationAndProofElectraToGloas(electraAggregate)
			sig, err := validator.aggregateAndProofSig(
				t.Context(),
				pubKey,
				gloasAggregate,
				params.BeaconConfig().SlotsPerEpoch.Mul(gloasForkEpoch),
			)
			require.NoError(t, err)

			parsedSig, err := bls.SignatureFromBytes(sig)
			require.NoError(t, err)
			gloasRoot, err := signing.ComputeSigningRoot(gloasAggregate, domain)
			require.NoError(t, err)
			require.Equal(t, true, parsedSig.Verify(validatorKey.PublicKey(), gloasRoot[:]))

			electraRoot, err := signing.ComputeSigningRoot(electraAggregate, domain)
			require.NoError(t, err)
			if electraRoot != gloasRoot {
				require.Equal(t, false, parsedSig.Verify(validatorKey.PublicKey(), electraRoot[:]))
			}
		})
	}
}
