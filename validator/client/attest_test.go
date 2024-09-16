package client

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/async/event"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	validatorpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/validator-client"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	prysmTime "github.com/prysmaticlabs/prysm/v5/time"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"go.uber.org/mock/gomock"
	"gopkg.in/d4l3k/messagediff.v1"
)

func TestRequestAttestation_ValidatorDutiesRequestFailure(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, _, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			validator.duties = &ethpb.DutiesResponse{CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{}}
			defer finish()

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.SubmitAttestation(context.Background(), 30, pubKey)
			require.LogsContain(t, hook, "Could not fetch validator assignment")
		})
	}
}

func TestAttestToBlockHead_SubmitAttestation_EmptyCommittee(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()

			validator, _, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.duties = &ethpb.DutiesResponse{CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
				{
					PublicKey:      validatorKey.PublicKey().Marshal(),
					CommitteeIndex: 0,
					Committee:      make([]primitives.ValidatorIndex, 0),
					ValidatorIndex: 0,
				}}}
			validator.SubmitAttestation(context.Background(), 0, pubKey)
			require.LogsContain(t, hook, "Empty committee")
		})
	}
}

func TestAttestToBlockHead_SubmitAttestation_RequestFailure(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()

			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			validator.duties = &ethpb.DutiesResponse{CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
				{
					PublicKey:      validatorKey.PublicKey().Marshal(),
					CommitteeIndex: 5,
					Committee:      make([]primitives.ValidatorIndex, 111),
					ValidatorIndex: 0,
				}}}
			m.validatorClient.EXPECT().AttestationData(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.AttestationDataRequest{}),
			).Return(&ethpb.AttestationData{
				BeaconBlockRoot: make([]byte, fieldparams.RootLength),
				Target:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				Source:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
			}, nil)
			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch2
			).Times(2).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)
			m.validatorClient.EXPECT().ProposeAttestation(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.Attestation{}),
			).Return(nil, errors.New("something went wrong"))

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.SubmitAttestation(context.Background(), 30, pubKey)
			require.LogsContain(t, hook, "Could not submit attestation to beacon node")
		})
	}
}

func TestAttestToBlockHead_AttestsCorrectly(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("Phase 0 (SlashingProtectionMinimal:%v)", isSlashingProtectionMinimal), func(t *testing.T) {
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			hook := logTest.NewGlobal()
			validatorIndex := primitives.ValidatorIndex(7)
			committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.duties = &ethpb.DutiesResponse{CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
				{
					PublicKey:      validatorKey.PublicKey().Marshal(),
					CommitteeIndex: 5,
					Committee:      committee,
					ValidatorIndex: validatorIndex,
				},
			}}

			beaconBlockRoot := bytesutil.ToBytes32([]byte("A"))
			targetRoot := bytesutil.ToBytes32([]byte("B"))
			sourceRoot := bytesutil.ToBytes32([]byte("C"))
			m.validatorClient.EXPECT().AttestationData(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.AttestationDataRequest{}),
			).Return(&ethpb.AttestationData{
				BeaconBlockRoot: beaconBlockRoot[:],
				Target:          &ethpb.Checkpoint{Root: targetRoot[:]},
				Source:          &ethpb.Checkpoint{Root: sourceRoot[:], Epoch: 3},
			}, nil)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Times(2).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			var generatedAttestation *ethpb.Attestation
			m.validatorClient.EXPECT().ProposeAttestation(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.Attestation{}),
			).Do(func(_ context.Context, att *ethpb.Attestation) {
				generatedAttestation = att
			}).Return(&ethpb.AttestResponse{}, nil /* error */)

			validator.SubmitAttestation(context.Background(), 30, pubKey)

			aggregationBitfield := bitfield.NewBitlist(uint64(len(committee)))
			aggregationBitfield.SetBitAt(4, true)
			expectedAttestation := &ethpb.Attestation{
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: beaconBlockRoot[:],
					Target:          &ethpb.Checkpoint{Root: targetRoot[:]},
					Source:          &ethpb.Checkpoint{Root: sourceRoot[:], Epoch: 3},
				},
				AggregationBits: aggregationBitfield,
				Signature:       make([]byte, 96),
			}

			root, err := signing.ComputeSigningRoot(expectedAttestation.Data, make([]byte, 32))
			require.NoError(t, err)

			sig, err := validator.km.Sign(context.Background(), &validatorpb.SignRequest{
				PublicKey:   validatorKey.PublicKey().Marshal(),
				SigningRoot: root[:],
			})
			require.NoError(t, err)
			expectedAttestation.Signature = sig.Marshal()
			if !reflect.DeepEqual(generatedAttestation, expectedAttestation) {
				t.Errorf("Incorrectly attested head, wanted %v, received %v", expectedAttestation, generatedAttestation)
				diff, _ := messagediff.PrettyDiff(expectedAttestation, generatedAttestation)
				t.Log(diff)
			}
			require.LogsDoNotContain(t, hook, "Could not")
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
			hook := logTest.NewGlobal()
			validatorIndex := primitives.ValidatorIndex(7)
			committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.duties = &ethpb.DutiesResponse{CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
				{
					PublicKey:      validatorKey.PublicKey().Marshal(),
					CommitteeIndex: 5,
					Committee:      committee,
					ValidatorIndex: validatorIndex,
				},
			}}

			beaconBlockRoot := bytesutil.ToBytes32([]byte("A"))
			targetRoot := bytesutil.ToBytes32([]byte("B"))
			sourceRoot := bytesutil.ToBytes32([]byte("C"))
			m.validatorClient.EXPECT().AttestationData(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.AttestationDataRequest{}),
			).Return(&ethpb.AttestationData{
				BeaconBlockRoot: beaconBlockRoot[:],
				Target:          &ethpb.Checkpoint{Root: targetRoot[:]},
				Source:          &ethpb.Checkpoint{Root: sourceRoot[:], Epoch: 3},
			}, nil)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Times(2).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			var generatedAttestation *ethpb.AttestationElectra
			m.validatorClient.EXPECT().ProposeAttestationElectra(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.AttestationElectra{}),
			).Do(func(_ context.Context, att *ethpb.AttestationElectra) {
				generatedAttestation = att
			}).Return(&ethpb.AttestResponse{}, nil /* error */)

			validator.SubmitAttestation(context.Background(), params.BeaconConfig().SlotsPerEpoch.Mul(electraForkEpoch), pubKey)

			aggregationBitfield := bitfield.NewBitlist(uint64(len(committee)))
			aggregationBitfield.SetBitAt(4, true)
			committeeBits := primitives.NewAttestationCommitteeBits()
			committeeBits.SetBitAt(5, true)
			expectedAttestation := &ethpb.AttestationElectra{
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: beaconBlockRoot[:],
					Target:          &ethpb.Checkpoint{Root: targetRoot[:]},
					Source:          &ethpb.Checkpoint{Root: sourceRoot[:], Epoch: 3},
				},
				AggregationBits: aggregationBitfield,
				CommitteeBits:   committeeBits,
				Signature:       make([]byte, 96),
			}

			root, err := signing.ComputeSigningRoot(expectedAttestation.Data, make([]byte, 32))
			require.NoError(t, err)

			sig, err := validator.km.Sign(context.Background(), &validatorpb.SignRequest{
				PublicKey:   validatorKey.PublicKey().Marshal(),
				SigningRoot: root[:],
			})
			require.NoError(t, err)
			expectedAttestation.Signature = sig.Marshal()
			if !reflect.DeepEqual(generatedAttestation, expectedAttestation) {
				t.Errorf("Incorrectly attested head, wanted %v, received %v", expectedAttestation, generatedAttestation)
				diff, _ := messagediff.PrettyDiff(expectedAttestation, generatedAttestation)
				t.Log(diff)
			}
			require.LogsDoNotContain(t, hook, "Could not")
		})
	}
}

func TestAttestToBlockHead_BlocksDoubleAtt(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			validatorIndex := primitives.ValidatorIndex(7)
			committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.duties = &ethpb.DutiesResponse{CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
				{
					PublicKey:      validatorKey.PublicKey().Marshal(),
					CommitteeIndex: 5,
					Committee:      committee,
					ValidatorIndex: validatorIndex,
				},
			}}
			beaconBlockRoot := bytesutil.ToBytes32([]byte("A"))
			targetRoot := bytesutil.ToBytes32([]byte("B"))
			sourceRoot := bytesutil.ToBytes32([]byte("C"))
			beaconBlockRoot2 := bytesutil.ToBytes32([]byte("D"))

			m.validatorClient.EXPECT().AttestationData(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.AttestationDataRequest{}),
			).Return(&ethpb.AttestationData{
				BeaconBlockRoot: beaconBlockRoot[:],
				Target:          &ethpb.Checkpoint{Root: targetRoot[:], Epoch: 4},
				Source:          &ethpb.Checkpoint{Root: sourceRoot[:], Epoch: 3},
			}, nil)
			m.validatorClient.EXPECT().AttestationData(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.AttestationDataRequest{}),
			).Return(&ethpb.AttestationData{
				BeaconBlockRoot: beaconBlockRoot2[:],
				Target:          &ethpb.Checkpoint{Root: targetRoot[:], Epoch: 4},
				Source:          &ethpb.Checkpoint{Root: sourceRoot[:], Epoch: 3},
			}, nil)
			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Times(4).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().ProposeAttestation(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.Attestation{}),
			).Return(&ethpb.AttestResponse{AttestationDataRoot: make([]byte, 32)}, nil /* error */)

			validator.SubmitAttestation(context.Background(), 30, pubKey)
			validator.SubmitAttestation(context.Background(), 30, pubKey)
			require.LogsContain(t, hook, "Failed attestation slashing protection")
		})
	}
}

func TestAttestToBlockHead_BlocksSurroundAtt(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			validatorIndex := primitives.ValidatorIndex(7)
			committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.duties = &ethpb.DutiesResponse{CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
				{
					PublicKey:      validatorKey.PublicKey().Marshal(),
					CommitteeIndex: 5,
					Committee:      committee,
					ValidatorIndex: validatorIndex,
				},
			}}
			beaconBlockRoot := bytesutil.ToBytes32([]byte("A"))
			targetRoot := bytesutil.ToBytes32([]byte("B"))
			sourceRoot := bytesutil.ToBytes32([]byte("C"))

			m.validatorClient.EXPECT().AttestationData(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.AttestationDataRequest{}),
			).Return(&ethpb.AttestationData{
				BeaconBlockRoot: beaconBlockRoot[:],
				Target:          &ethpb.Checkpoint{Root: targetRoot[:], Epoch: 2},
				Source:          &ethpb.Checkpoint{Root: sourceRoot[:], Epoch: 1},
			}, nil)
			m.validatorClient.EXPECT().AttestationData(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.AttestationDataRequest{}),
			).Return(&ethpb.AttestationData{
				BeaconBlockRoot: beaconBlockRoot[:],
				Target:          &ethpb.Checkpoint{Root: targetRoot[:], Epoch: 3},
				Source:          &ethpb.Checkpoint{Root: sourceRoot[:], Epoch: 0},
			}, nil)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Times(4).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().ProposeAttestation(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.Attestation{}),
			).Return(&ethpb.AttestResponse{}, nil /* error */)

			validator.SubmitAttestation(context.Background(), 30, pubKey)
			validator.SubmitAttestation(context.Background(), 30, pubKey)
			require.LogsContain(t, hook, "Failed attestation slashing protection")
		})
	}
}

func TestAttestToBlockHead_BlocksSurroundedAtt(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			validatorIndex := primitives.ValidatorIndex(7)
			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
			validator.duties = &ethpb.DutiesResponse{CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
				{
					PublicKey:      validatorKey.PublicKey().Marshal(),
					CommitteeIndex: 5,
					Committee:      committee,
					ValidatorIndex: validatorIndex,
				},
			}}
			beaconBlockRoot := bytesutil.ToBytes32([]byte("A"))
			targetRoot := bytesutil.ToBytes32([]byte("B"))
			sourceRoot := bytesutil.ToBytes32([]byte("C"))

			m.validatorClient.EXPECT().AttestationData(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.AttestationDataRequest{}),
			).Return(&ethpb.AttestationData{
				BeaconBlockRoot: beaconBlockRoot[:],
				Target:          &ethpb.Checkpoint{Root: targetRoot[:], Epoch: 3},
				Source:          &ethpb.Checkpoint{Root: sourceRoot[:], Epoch: 0},
			}, nil)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Times(4).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().ProposeAttestation(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.Attestation{}),
			).Return(&ethpb.AttestResponse{}, nil /* error */)

			validator.SubmitAttestation(context.Background(), 30, pubKey)
			require.LogsDoNotContain(t, hook, failedAttLocalProtectionErr)

			m.validatorClient.EXPECT().AttestationData(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.AttestationDataRequest{}),
			).Return(&ethpb.AttestationData{
				BeaconBlockRoot: bytesutil.PadTo([]byte("A"), 32),
				Target:          &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte("B"), 32), Epoch: 2},
				Source:          &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte("C"), 32), Epoch: 1},
			}, nil)

			validator.SubmitAttestation(context.Background(), 30, pubKey)
			require.LogsContain(t, hook, "Failed attestation slashing protection")
		})
	}
}

func TestAttestToBlockHead_DoesNotAttestBeforeDelay(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.genesisTime = uint64(prysmTime.Now().Unix())
			m.validatorClient.EXPECT().Duties(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.DutiesRequest{}),
			).Times(0)

			m.validatorClient.EXPECT().AttestationData(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.AttestationDataRequest{}),
			).Times(0)

			m.validatorClient.EXPECT().ProposeAttestation(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.Attestation{}),
			).Return(&ethpb.AttestResponse{}, nil /* error */).Times(0)

			timer := time.NewTimer(1 * time.Second)
			go validator.SubmitAttestation(context.Background(), 0, pubKey)
			<-timer.C
		})
	}
}

func TestAttestToBlockHead_DoesAttestAfterDelay(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			var wg sync.WaitGroup
			wg.Add(1)
			defer wg.Wait()

			validator.genesisTime = uint64(prysmTime.Now().Unix())
			validatorIndex := primitives.ValidatorIndex(5)
			committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.duties = &ethpb.DutiesResponse{CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
				{
					PublicKey:      validatorKey.PublicKey().Marshal(),
					CommitteeIndex: 5,
					Committee:      committee,
					ValidatorIndex: validatorIndex,
				}}}

			m.validatorClient.EXPECT().AttestationData(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.AttestationDataRequest{}),
			).Return(&ethpb.AttestationData{
				BeaconBlockRoot: bytesutil.PadTo([]byte("A"), 32),
				Target:          &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte("B"), 32)},
				Source:          &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte("C"), 32), Epoch: 3},
			}, nil).Do(func(arg0, arg1 interface{}) {
				wg.Done()
			})

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Times(2).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().ProposeAttestation(
				gomock.Any(), // ctx
				gomock.Any(),
			).Return(&ethpb.AttestResponse{}, nil).Times(1)

			validator.SubmitAttestation(context.Background(), 0, pubKey)
		})
	}
}

func TestAttestToBlockHead_CorrectBitfieldLength(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			validatorIndex := primitives.ValidatorIndex(2)
			committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.duties = &ethpb.DutiesResponse{CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
				{
					PublicKey:      validatorKey.PublicKey().Marshal(),
					CommitteeIndex: 5,
					Committee:      committee,
					ValidatorIndex: validatorIndex,
				}}}
			m.validatorClient.EXPECT().AttestationData(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.AttestationDataRequest{}),
			).Return(&ethpb.AttestationData{
				Target:          &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte("B"), 32)},
				Source:          &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte("C"), 32), Epoch: 3},
				BeaconBlockRoot: make([]byte, fieldparams.RootLength),
			}, nil)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Times(2).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			var generatedAttestation *ethpb.Attestation
			m.validatorClient.EXPECT().ProposeAttestation(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.Attestation{}),
			).Do(func(_ context.Context, att *ethpb.Attestation) {
				generatedAttestation = att
			}).Return(&ethpb.AttestResponse{}, nil /* error */)

			validator.SubmitAttestation(context.Background(), 30, pubKey)

			assert.Equal(t, 2, len(generatedAttestation.AggregationBits))
		})
	}
}

func TestSignAttestation(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			validator, m, _, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			wantedFork := &ethpb.Fork{
				PreviousVersion: []byte{'a', 'b', 'c', 'd'},
				CurrentVersion:  []byte{'d', 'e', 'f', 'f'},
				Epoch:           0,
			}
			genesisValidatorsRoot := [32]byte{0x01, 0x02}
			attesterDomain, err := signing.Domain(wantedFork, 0, params.BeaconConfig().DomainBeaconAttester, genesisValidatorsRoot[:])
			require.NoError(t, err)
			m.validatorClient.EXPECT().
				DomainData(gomock.Any(), gomock.Any()).
				Return(&ethpb.DomainResponse{SignatureDomain: attesterDomain}, nil)
			ctx := context.Background()
			att := util.NewAttestation()
			att.Data.Source.Epoch = 100
			att.Data.Target.Epoch = 200
			att.Data.Slot = 999
			att.Data.BeaconBlockRoot = bytesutil.PadTo([]byte("blockRoot"), 32)

			pk := testKeyFromBytes(t, []byte{1})
			validator.km = newMockKeymanager(t, pk)
			sig, sr, err := validator.signAtt(ctx, pk.pub, att.Data, att.Data.Slot)
			require.NoError(t, err, "%x,%x,%v", sig, sr, err)
			require.Equal(t, "b6a60f8497bd328908be83634d045"+
				"dd7a32f5e246b2c4031fc2f316983f362e36fc27fd3d6d5a2b15"+
				"b4dbff38804ffb10b1719b7ebc54e9cbf3293fd37082bc0fc91f"+
				"79d70ce5b04ff13de3c8e10bb41305bfdbe921a43792c12624f2"+
				"25ee865", hex.EncodeToString(sig))
			// proposer domain
			require.DeepEqual(t, "02bbdb88056d6cbafd6e94575540"+
				"e74b8cf2c0f2c1b79b8e17e7b21ed1694305", hex.EncodeToString(sr[:]))
		})
	}
}

func TestServer_WaitToSlotOneThird_CanWait(t *testing.T) {
	currentTime := uint64(time.Now().Unix())
	currentSlot := primitives.Slot(4)
	genesisTime := currentTime - uint64(currentSlot.Mul(params.BeaconConfig().SecondsPerSlot))

	v := &validator{
		genesisTime: genesisTime,
		slotFeed:    new(event.Feed),
	}

	timeToSleep := params.BeaconConfig().SecondsPerSlot / 3
	oneThird := currentTime + timeToSleep
	v.waitAttesterDuty(context.Background(), currentSlot)

	if oneThird != uint64(time.Now().Unix()) {
		t.Errorf("Wanted %d time for slot one third but got %d", oneThird, currentTime)
	}
}

func TestServer_WaitAttesterDutyEpbs_CanWait(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.EPBSForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	currentTime := uint64(time.Now().Unix())
	currentSlot := primitives.Slot(4)
	genesisTime := currentTime - uint64(currentSlot.Mul(params.BeaconConfig().SecondsPerSlot))

	v := &validator{
		genesisTime: genesisTime,
		slotFeed:    new(event.Feed),
	}

	timeToSleep := params.BeaconConfig().SecondsPerSlot / params.BeaconConfig().IntervalsPerSlotEPBS
	dutyTime := currentTime + timeToSleep
	v.waitAttesterDuty(context.Background(), currentSlot)

	if dutyTime != uint64(time.Now().Unix()) {
		t.Errorf("Wanted %d time for slot one third but got %d", dutyTime, currentTime)
	}
}

func TestServer_WaitToSlotOneThird_SameReqSlot(t *testing.T) {
	currentTime := uint64(time.Now().Unix())
	currentSlot := primitives.Slot(4)
	genesisTime := currentTime - uint64(currentSlot.Mul(params.BeaconConfig().SecondsPerSlot))

	v := &validator{
		genesisTime:      genesisTime,
		slotFeed:         new(event.Feed),
		highestValidSlot: currentSlot,
	}

	v.waitAttesterDuty(context.Background(), currentSlot)

	if currentTime != uint64(time.Now().Unix()) {
		t.Errorf("Wanted %d time for slot one third but got %d", uint64(time.Now().Unix()), currentTime)
	}
}

func TestServer_WaitToSlotOneThird_ReceiveBlockSlot(t *testing.T) {
	resetCfg := features.InitWithReset(&features.Flags{AttestTimely: true})
	defer resetCfg()

	currentTime := uint64(time.Now().Unix())
	currentSlot := primitives.Slot(4)
	genesisTime := currentTime - uint64(currentSlot.Mul(params.BeaconConfig().SecondsPerSlot))

	v := &validator{
		genesisTime: genesisTime,
		slotFeed:    new(event.Feed),
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		time.Sleep(100 * time.Millisecond)
		v.slotFeed.Send(currentSlot)
		wg.Done()
	}()

	v.waitAttesterDuty(context.Background(), currentSlot)

	if currentTime != uint64(time.Now().Unix()) {
		t.Errorf("Wanted %d time for slot one third but got %d", uint64(time.Now().Unix()), currentTime)
	}
}

func Test_slashableAttestationCheck(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			validator, _, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			att := &ethpb.IndexedAttestation{
				AttestingIndices: []uint64{1, 2},
				Data: &ethpb.AttestationData{
					Slot:            5,
					CommitteeIndex:  2,
					BeaconBlockRoot: bytesutil.PadTo([]byte("great block"), 32),
					Source: &ethpb.Checkpoint{
						Epoch: 4,
						Root:  bytesutil.PadTo([]byte("good source"), 32),
					},
					Target: &ethpb.Checkpoint{
						Epoch: 10,
						Root:  bytesutil.PadTo([]byte("good target"), 32),
					},
				},
			}

			err := validator.db.SlashableAttestationCheck(context.Background(), att, pubKey, [32]byte{1}, false, nil)
			require.NoError(t, err, "Expected allowed attestation not to throw error")
		})
	}
}

func Test_slashableAttestationCheck_UpdatesLowestSignedEpochs(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			ctx := context.Background()
			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			att := &ethpb.IndexedAttestation{
				AttestingIndices: []uint64{1, 2},
				Data: &ethpb.AttestationData{
					Slot:            5,
					CommitteeIndex:  2,
					BeaconBlockRoot: bytesutil.PadTo([]byte("great block"), 32),
					Source: &ethpb.Checkpoint{
						Epoch: 4,
						Root:  bytesutil.PadTo([]byte("good source"), 32),
					},
					Target: &ethpb.Checkpoint{
						Epoch: 10,
						Root:  bytesutil.PadTo([]byte("good target"), 32),
					},
				},
			}

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				&ethpb.DomainRequest{Epoch: 10, Domain: []byte{1, 0, 0, 0}},
			).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)
			_, sr, err := validator.domainAndSigningRoot(ctx, att.Data)
			require.NoError(t, err)

			err = validator.db.SlashableAttestationCheck(context.Background(), att, pubKey, sr, false, nil)
			require.NoError(t, err)
			differentSigningRoot := [32]byte{2}

			err = validator.db.SlashableAttestationCheck(context.Background(), att, pubKey, differentSigningRoot, false, nil)
			require.ErrorContains(t, "could not sign attestation", err)

			e, exists, err := validator.db.LowestSignedSourceEpoch(context.Background(), pubKey)
			require.NoError(t, err)
			require.Equal(t, true, exists)
			require.Equal(t, primitives.Epoch(4), e)
			e, exists, err = validator.db.LowestSignedTargetEpoch(context.Background(), pubKey)
			require.NoError(t, err)
			require.Equal(t, true, exists)
			require.Equal(t, primitives.Epoch(10), e)
		})
	}
}

func Test_slashableAttestationCheck_OK(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			ctx := context.Background()
			validator, _, _, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			att := &ethpb.IndexedAttestation{
				AttestingIndices: []uint64{1, 2},
				Data: &ethpb.AttestationData{
					Slot:            5,
					CommitteeIndex:  2,
					BeaconBlockRoot: []byte("great block"),
					Source: &ethpb.Checkpoint{
						Epoch: 4,
						Root:  []byte("good source"),
					},
					Target: &ethpb.Checkpoint{
						Epoch: 10,
						Root:  []byte("good target"),
					},
				},
			}
			sr := [32]byte{1}
			fakePubkey := bytesutil.ToBytes48([]byte("test"))

			err := validator.db.SlashableAttestationCheck(ctx, att, fakePubkey, sr, false, nil)
			require.NoError(t, err, "Expected allowed attestation not to throw error")
		})
	}
}

func Test_slashableAttestationCheck_GenesisEpoch(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			ctx := context.Background()
			validator, _, _, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			att := &ethpb.IndexedAttestation{
				AttestingIndices: []uint64{1, 2},
				Data: &ethpb.AttestationData{
					Slot:            5,
					CommitteeIndex:  2,
					BeaconBlockRoot: bytesutil.PadTo([]byte("great block root"), 32),
					Source: &ethpb.Checkpoint{
						Epoch: 0,
						Root:  bytesutil.PadTo([]byte("great root"), 32),
					},
					Target: &ethpb.Checkpoint{
						Epoch: 0,
						Root:  bytesutil.PadTo([]byte("great root"), 32),
					},
				},
			}

			fakePubkey := bytesutil.ToBytes48([]byte("test"))
			err := validator.db.SlashableAttestationCheck(ctx, att, fakePubkey, [32]byte{}, false, nil)
			require.NoError(t, err, "Expected allowed attestation not to throw error")
			e, exists, err := validator.db.LowestSignedSourceEpoch(context.Background(), fakePubkey)
			require.NoError(t, err)
			require.Equal(t, true, exists)
			require.Equal(t, primitives.Epoch(0), e)
			e, exists, err = validator.db.LowestSignedTargetEpoch(context.Background(), fakePubkey)
			require.NoError(t, err)
			require.Equal(t, true, exists)
			require.Equal(t, primitives.Epoch(0), e)
		})
	}
}
