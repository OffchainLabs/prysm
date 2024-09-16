package client

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/prysmaticlabs/go-bitfield"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/prysmaticlabs/prysm/v5/validator/client/iface"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"go.uber.org/mock/gomock"
)

func TestSubmitAggregateAndProof_GetDutiesRequestFailure(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, _, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			validator.duties = &ethpb.DutiesResponse{CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{}}
			defer finish()

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.SubmitAggregateAndProof(context.Background(), 0, pubKey)

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
			validator.duties = &ethpb.DutiesResponse{
				CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
					{
						PublicKey: validatorKey.PublicKey().Marshal(),
					},
				},
			}

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

			validator.SubmitAggregateAndProof(context.Background(), 0, pubKey)
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
			validator.duties = &ethpb.DutiesResponse{
				CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
					{
						PublicKey: validatorKey.PublicKey().Marshal(),
					},
				},
			}

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

			validator.SubmitAggregateAndProof(context.Background(), 0, pubKey)
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
			validator.duties = &ethpb.DutiesResponse{
				CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
					{
						PublicKey: validatorKey.PublicKey().Marshal(),
					},
				},
			}

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

			validator.SubmitAggregateAndProof(context.Background(), params.BeaconConfig().SlotsPerEpoch.Mul(electraForkEpoch), pubKey)
		})
	}
}

func TestSubmitAggregateAndProof_Distributed(t *testing.T) {
	validatorIdx := primitives.ValidatorIndex(123)
	slot := primitives.Slot(456)
	ctx := context.Background()
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.duties = &ethpb.DutiesResponse{
				CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
					{
						PublicKey:      validatorKey.PublicKey().Marshal(),
						ValidatorIndex: validatorIdx,
						AttesterSlot:   slot,
					},
				},
			}

			validator.distributed = true
			validator.attSelections = make(map[attSelectionKey]iface.BeaconCommitteeSelection)
			validator.attSelections[attSelectionKey{
				slot:  slot,
				index: 123,
			}] = iface.BeaconCommitteeSelection{
				SelectionProof: make([]byte, 96),
				Slot:           slot,
				ValidatorIndex: validatorIdx,
			}

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
			validator.genesisTime = uint64(currentTime.Unix()) - uint64(numOfSlots.Mul(params.BeaconConfig().SecondsPerSlot))
			oneThird := slots.DivideSlotBy(3 /* one third of slot duration */)
			timeToSleep := oneThird + oneThird

			twoThirdTime := currentTime.Add(timeToSleep)
			validator.waitAggregatorDuty(context.Background(), numOfSlots)
			currentTime = time.Now()
			assert.Equal(t, twoThirdTime.Unix(), currentTime.Unix())
		})
	}
}

func TestServer_WaitAggregatorDutyEpbs_CanWait(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.EPBSForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	validator, _, _, finish := setup(t, true)
	defer finish()
	currentTime := time.Now()
	numOfSlots := primitives.Slot(4)
	validator.genesisTime = uint64(currentTime.Unix()) - uint64(numOfSlots.Mul(params.BeaconConfig().SecondsPerSlot))
	delay := slots.DivideSlotBy(int64(params.BeaconConfig().IntervalsPerSlotEPBS))
	timeToSleep := delay + delay

	twoThirdTime := currentTime.Add(timeToSleep)
	validator.waitAggregatorDuty(context.Background(), numOfSlots)
	currentTime = time.Now()
	assert.Equal(t, twoThirdTime.Unix(), currentTime.Unix())
}

func TestWaitForSlotTwoThird_DoneContext_ReturnsImmediately(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			validator, _, _, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			currentTime := time.Now()
			numOfSlots := primitives.Slot(4)
			validator.genesisTime = uint64(currentTime.Unix()) - uint64(numOfSlots.Mul(params.BeaconConfig().SecondsPerSlot))

			expectedTime := time.Now()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			validator.waitAggregatorDuty(ctx, numOfSlots)
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
			sig, err := validator.aggregateAndProofSig(context.Background(), pubKey, agg, 0 /* slot */)
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
			sig, err := validator.aggregateAndProofSig(context.Background(), pubKey, agg, params.BeaconConfig().SlotsPerEpoch.Mul(electraForkEpoch) /* slot */)
			require.NoError(t, err)
			_, err = bls.SignatureFromBytes(sig)
			require.NoError(t, err)
		})
	}
}
