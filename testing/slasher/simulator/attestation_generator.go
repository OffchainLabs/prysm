package simulator

import (
	"bytes"
	"context"
	"math"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/crypto/rand"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func (s *Simulator) generateAttestationsForSlot(ctx context.Context, ver int, slot primitives.Slot) ([]ethpb.IndexedAtt, []ethpb.AttSlashing, error) {
	attestations := make([]ethpb.IndexedAtt, 0)
	slashings := make([]ethpb.AttSlashing, 0)
	currentEpoch := slots.ToEpoch(slot)

	committeesPerSlot := helpers.SlotCommitteeCount(s.srvConfig.Params.NumValidators)
	valsPerCommittee := s.srvConfig.Params.NumValidators /
		(committeesPerSlot * uint64(s.srvConfig.Params.SlotsPerEpoch))
	valsPerSlot := committeesPerSlot * valsPerCommittee

	if currentEpoch < 2 {
		return nil, nil, nil
	}
	sourceEpoch := currentEpoch - 1

	var slashedIndices []uint64
	startIdx := valsPerSlot * uint64(slot%s.srvConfig.Params.SlotsPerEpoch)
	endIdx := startIdx + valsPerCommittee
	for c := primitives.CommitteeIndex(0); uint64(c) < committeesPerSlot; c++ {
		attData := &ethpb.AttestationData{
			Slot:            slot,
			CommitteeIndex:  c,
			BeaconBlockRoot: bytesutil.PadTo([]byte("block"), 32),
			Source: &ethpb.Checkpoint{
				Epoch: sourceEpoch,
				Root:  bytesutil.PadTo([]byte("source"), 32),
			},
			Target: &ethpb.Checkpoint{
				Epoch: currentEpoch,
				Root:  bytesutil.PadTo([]byte("target"), 32),
			},
		}

		valsPerAttestation := uint64(math.Floor(s.srvConfig.Params.AggregationPercent * float64(valsPerCommittee)))
		for i := startIdx; i < endIdx; i += valsPerAttestation {
			attEndIdx := min(i+valsPerAttestation, endIdx)
			indices := make([]uint64, 0, valsPerAttestation)
			for idx := i; idx < attEndIdx; idx++ {
				indices = append(indices, idx)
			}

			var att ethpb.IndexedAtt
			if ver >= version.Electra {
				att = &ethpb.IndexedAttestationElectra{
					AttestingIndices: indices,
					Data:             attData,
					Signature:        params.BeaconConfig().EmptySignature[:],
				}
			} else {
				att = &ethpb.IndexedAttestation{
					AttestingIndices: indices,
					Data:             attData,
					Signature:        params.BeaconConfig().EmptySignature[:],
				}
			}

			data := att.GetData()
			if data == nil || data.Target == nil {
				return nil, nil, errors.New("attestation data is nil")
			}
			beaconState, err := s.srvConfig.AttestationStateFetcher.AttestationTargetState(ctx, data.Target)
			if err != nil {
				return nil, nil, err
			}

			// Sign the attestation with a valid signature.
			aggSig, err := s.aggregateSigForAttestation(beaconState, att)
			if err != nil {
				return nil, nil, err
			}

			if ver >= version.Electra {
				att.(*ethpb.IndexedAttestationElectra).Signature = aggSig.Marshal()
			} else {
				att.(*ethpb.IndexedAttestation).Signature = aggSig.Marshal()
			}

			attestations = append(attestations, att)
			if rand.NewGenerator().Float64() < s.srvConfig.Params.AttesterSlashingProbab {
				slashableAtt := makeSlashableFromAtt(att, []uint64{indices[0]})
				aggSig, err := s.aggregateSigForAttestation(beaconState, slashableAtt)
				if err != nil {
					return nil, nil, err
				}

				if ver >= version.Electra {
					slashableAtt.(*ethpb.IndexedAttestationElectra).Signature = aggSig.Marshal()
				} else {
					slashableAtt.(*ethpb.IndexedAttestation).Signature = aggSig.Marshal()
				}

				slashedIndices = append(slashedIndices, slashableAtt.GetAttestingIndices()...)

				attDataRoot, err := data.HashTreeRoot()
				if err != nil {
					return nil, nil, errors.Wrap(err, "cannot compte `att` hash tree root")
				}

				slashableAttDataRoot, err := slashableAtt.GetData().HashTreeRoot()
				if err != nil {
					return nil, nil, errors.Wrap(err, "cannot compte `slashableAtt` hash tree root")
				}

				var slashing ethpb.AttSlashing
				if ver >= version.Electra {
					slashing = &ethpb.AttesterSlashingElectra{
						Attestation_1: att.(*ethpb.IndexedAttestationElectra),
						Attestation_2: slashableAtt.(*ethpb.IndexedAttestationElectra),
					}
				} else {
					slashing = &ethpb.AttesterSlashing{
						Attestation_1: att.(*ethpb.IndexedAttestation),
						Attestation_2: slashableAtt.(*ethpb.IndexedAttestation),
					}
				}

				// Ensure the attestation with the lower data root is the first attestation.
				if bytes.Compare(attDataRoot[:], slashableAttDataRoot[:]) > 0 {
					if ver >= version.Electra {
						slashing = &ethpb.AttesterSlashingElectra{
							Attestation_1: slashableAtt.(*ethpb.IndexedAttestationElectra),
							Attestation_2: att.(*ethpb.IndexedAttestationElectra),
						}
					} else {
						slashing = &ethpb.AttesterSlashing{
							Attestation_1: slashableAtt.(*ethpb.IndexedAttestation),
							Attestation_2: att.(*ethpb.IndexedAttestation),
						}
					}
				}

				slashings = append(slashings, slashing)
				attestations = append(attestations, slashableAtt)
			}
		}
		startIdx += valsPerCommittee
		endIdx += valsPerCommittee
	}
	if len(slashedIndices) > 0 {
		log.WithFields(logrus.Fields{
			"amount":  len(slashedIndices),
			"indices": slashedIndices,
		}).Infof("Slashable attestation made")
	}
	return attestations, slashings, nil
}

func (s *Simulator) aggregateSigForAttestation(
	beaconState state.ReadOnlyBeaconState, att ethpb.IndexedAtt,
) (bls.Signature, error) {
	data := att.GetData()
	if data == nil || data.Target == nil {
		return nil, errors.New("attestation data is nil")
	}
	domain, err := signing.Domain(
		beaconState.Fork(),
		data.Target.Epoch,
		params.BeaconConfig().DomainBeaconAttester,
		beaconState.GenesisValidatorsRoot(),
	)
	if err != nil {
		return nil, err
	}
	signingRoot, err := signing.ComputeSigningRoot(data, domain)
	if err != nil {
		return nil, err
	}
	sigs := make([]bls.Signature, len(att.GetAttestingIndices()))
	for i, validatorIndex := range att.GetAttestingIndices() {
		privKey := s.srvConfig.PrivateKeysByValidatorIndex[primitives.ValidatorIndex(validatorIndex)]
		if privKey == nil {
			return nil, errors.New("validator private key is nil")
		}
		sigs[i] = privKey.Sign(signingRoot[:])
	}
	aggSig := bls.AggregateSignatures(sigs)
	if aggSig == nil {
		return nil, errors.New("aggregate signature is nil")
	}
	return aggSig, nil
}

func makeSlashableFromAtt(att ethpb.IndexedAtt, indices []uint64) ethpb.IndexedAtt {
	data := att.GetData()
	if data == nil || data.Source == nil {
		return att
	}
	if data.Source.Epoch <= 2 {
		return makeDoubleVoteFromAtt(att, indices)
	}
	if data.Target == nil {
		return att
	}
	attData := &ethpb.AttestationData{
		Slot:            data.Slot,
		CommitteeIndex:  data.CommitteeIndex,
		BeaconBlockRoot: data.BeaconBlockRoot,
		Source: &ethpb.Checkpoint{
			Epoch: data.Source.Epoch - 3,
			Root:  data.Source.Root,
		},
		Target: &ethpb.Checkpoint{
			Epoch: data.Target.Epoch,
			Root:  data.Target.Root,
		},
	}

	if att.Version() >= version.Electra {
		return &ethpb.IndexedAttestationElectra{
			AttestingIndices: indices,
			Data:             attData,
			Signature:        params.BeaconConfig().EmptySignature[:],
		}
	}

	return &ethpb.IndexedAttestation{
		AttestingIndices: indices,
		Data:             attData,
		Signature:        params.BeaconConfig().EmptySignature[:],
	}
}

func makeDoubleVoteFromAtt(att ethpb.IndexedAtt, indices []uint64) ethpb.IndexedAtt {
	data := att.GetData()
	if data == nil || data.Source == nil || data.Target == nil {
		return att
	}
	attData := &ethpb.AttestationData{
		Slot:            data.Slot,
		CommitteeIndex:  data.CommitteeIndex,
		BeaconBlockRoot: bytesutil.PadTo([]byte("slash me"), 32),
		Source: &ethpb.Checkpoint{
			Epoch: data.Source.Epoch,
			Root:  data.Source.Root,
		},
		Target: &ethpb.Checkpoint{
			Epoch: data.Target.Epoch,
			Root:  data.Target.Root,
		},
	}

	if att.Version() >= version.Electra {
		return &ethpb.IndexedAttestationElectra{
			AttestingIndices: indices,
			Data:             attData,
			Signature:        params.BeaconConfig().EmptySignature[:],
		}
	}

	return &ethpb.IndexedAttestation{
		AttestingIndices: indices,
		Data:             attData,
		Signature:        params.BeaconConfig().EmptySignature[:],
	}
}
