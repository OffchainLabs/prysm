package util

import (
	"context"
	"fmt"
	"math"

	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/transition"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/crypto/rand"
	attv1 "github.com/prysmaticlabs/prysm/v5/proto/eth/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	log "github.com/sirupsen/logrus"
)

// NewAttestation creates a block attestation with minimum marshalable fields.
func NewAttestation() *ethpb.Attestation {
	return &ethpb.Attestation{
		AggregationBits: bitfield.Bitlist{0b1101},
		Data: &ethpb.AttestationData{
			BeaconBlockRoot: make([]byte, fieldparams.RootLength),
			Source: &ethpb.Checkpoint{
				Root: make([]byte, fieldparams.RootLength),
			},
			Target: &ethpb.Checkpoint{
				Root: make([]byte, fieldparams.RootLength),
			},
		},
		Signature: make([]byte, 96),
	}
}

// NewAttestationElectra creates a block attestation with minimum marshalable fields.
func NewAttestationElectra() *ethpb.AttestationElectra {
	cb := primitives.NewAttestationCommitteeBits()
	cb.SetBitAt(0, true)
	return &ethpb.AttestationElectra{
		AggregationBits: bitfield.Bitlist{0b1101},
		CommitteeBits:   cb,
		Data: &ethpb.AttestationData{
			BeaconBlockRoot: make([]byte, fieldparams.RootLength),
			Source: &ethpb.Checkpoint{
				Root: make([]byte, fieldparams.RootLength),
			},
			Target: &ethpb.Checkpoint{
				Root: make([]byte, fieldparams.RootLength),
			},
		},
		Signature: make([]byte, 96),
	}
}

// GenerateAttestations creates attestations that are entirely valid, for all
// the committees of the current state slot. This function expects attestations
// requested to be cleanly divisible by committees per slot. If there is 1 committee
// in the slot, and numToGen is set to 4, then it will return 4 attestations
// for the same data with their aggregation bits split uniformly.
//
// If you request 4 attestations, but there are 8 committees, you will get 4 fully aggregated attestations.
func GenerateAttestations(bState state.BeaconState, privs []bls.SecretKey, numToGen uint64, slot primitives.Slot, randomRoot bool) ([]ethpb.Att, error) { // nolint:gocognit
	var attestations []ethpb.Att
	generateHeadState := false
	bState = bState.Copy()
	if slot > bState.Slot() {
		// Going back a slot here so there's no inclusion delay issues.
		slot--
		generateHeadState = true
	}
	currentEpoch := slots.ToEpoch(slot)

	targetRoot := make([]byte, fieldparams.RootLength)
	var headRoot []byte
	var err error
	// Only calculate head state if its an attestation for the current slot or future slot.
	if generateHeadState || slot == bState.Slot() {
		var headState state.BeaconState
		switch bState.Version() {
		case version.Phase0:
			pbState, err := state_native.ProtobufBeaconStatePhase0(bState.ToProto())
			if err != nil {
				return nil, err
			}
			genState, err := state_native.InitializeFromProtoUnsafePhase0(pbState)
			if err != nil {
				return nil, err
			}
			headState = genState
		case version.Altair:
			pbState, err := state_native.ProtobufBeaconStateAltair(bState.ToProto())
			if err != nil {
				return nil, err
			}
			genState, err := state_native.InitializeFromProtoUnsafeAltair(pbState)
			if err != nil {
				return nil, err
			}
			headState = genState
		case version.Bellatrix:
			pbState, err := state_native.ProtobufBeaconStateBellatrix(bState.ToProto())
			if err != nil {
				return nil, err
			}
			genState, err := state_native.InitializeFromProtoUnsafeBellatrix(pbState)
			if err != nil {
				return nil, err
			}
			headState = genState
		case version.Capella:
			pbState, err := state_native.ProtobufBeaconStateCapella(bState.ToProto())
			if err != nil {
				return nil, err
			}
			genState, err := state_native.InitializeFromProtoUnsafeCapella(pbState)
			if err != nil {
				return nil, err
			}
			headState = genState
		case version.Deneb:
			pbState, err := state_native.ProtobufBeaconStateDeneb(bState.ToProto())
			if err != nil {
				return nil, err
			}
			genState, err := state_native.InitializeFromProtoUnsafeDeneb(pbState)
			if err != nil {
				return nil, err
			}
			headState = genState
		case version.Electra:
			pbState, err := state_native.ProtobufBeaconStateElectra(bState.ToProto())
			if err != nil {
				return nil, err
			}
			genState, err := state_native.InitializeFromProtoUnsafeElectra(pbState)
			if err != nil {
				return nil, err
			}
			headState = genState
		case version.Fulu:
			pbState, err := state_native.ProtobufBeaconStateFulu(bState.ToProto())
			if err != nil {
				return nil, err
			}
			genState, err := state_native.InitializeFromProtoUnsafeFulu(pbState)
			if err != nil {
				return nil, err
			}
			headState = genState
		default:
			return nil, fmt.Errorf("state version %s isn't supported", version.String(bState.Version()))
		}

		headState, err = transition.ProcessSlots(context.Background(), headState, slot+1)
		if err != nil {
			return nil, err
		}
		headRoot, err = helpers.BlockRootAtSlot(headState, slot)
		if err != nil {
			return nil, err
		}
		targetRoot, err = helpers.BlockRoot(headState, currentEpoch)
		if err != nil {
			return nil, err
		}
	} else {
		headRoot, err = helpers.BlockRootAtSlot(bState, slot)
		if err != nil {
			return nil, err
		}
	}
	if randomRoot {
		randGen := rand.NewDeterministicGenerator()
		b := make([]byte, fieldparams.RootLength)
		_, err := randGen.Read(b)
		if err != nil {
			return nil, err
		}
		headRoot = b
	}

	activeValidatorCount, err := helpers.ActiveValidatorCount(context.Background(), bState, currentEpoch)
	if err != nil {
		return nil, err
	}
	committeesPerSlot := helpers.SlotCommitteeCount(activeValidatorCount)

	if numToGen < committeesPerSlot {
		log.Printf(
			"Warning: %d attestations requested is less than %d committees in current slot, not all validators will be attesting.",
			numToGen,
			committeesPerSlot,
		)
	} else if numToGen > committeesPerSlot {
		log.Printf(
			"Warning: %d attestations requested are more than %d committees in current slot, attestations will not be perfectly efficient.",
			numToGen,
			committeesPerSlot,
		)
	}

	attsPerCommittee := math.Max(float64(numToGen/committeesPerSlot), 1)
	if math.Trunc(attsPerCommittee) != attsPerCommittee {
		return nil, fmt.Errorf(
			"requested attestations %d must be easily divisible by committees in slot %d, calculated %f",
			numToGen,
			committeesPerSlot,
			attsPerCommittee,
		)
	}

	domain, err := signing.Domain(bState.Fork(), currentEpoch, params.BeaconConfig().DomainBeaconAttester, bState.GenesisValidatorsRoot())
	if err != nil {
		return nil, err
	}
	for c := primitives.CommitteeIndex(0); uint64(c) < committeesPerSlot && uint64(c) < numToGen; c++ {
		committee, err := helpers.BeaconCommitteeFromState(context.Background(), bState, slot, c)
		if err != nil {
			return nil, err
		}

		ci := c
		if bState.Version() >= version.Electra {
			// committee index must be 0 post-Electra
			ci = 0
		}
		attData := &ethpb.AttestationData{
			Slot:            slot,
			CommitteeIndex:  ci,
			BeaconBlockRoot: headRoot,
			Source:          bState.CurrentJustifiedCheckpoint(),
			Target: &ethpb.Checkpoint{
				Epoch: currentEpoch,
				Root:  targetRoot,
			},
		}

		dataRoot, err := signing.ComputeSigningRoot(attData, domain)
		if err != nil {
			return nil, err
		}

		committeeSize := uint64(len(committee))
		bitsPerAtt := committeeSize / uint64(attsPerCommittee)
		for i := uint64(0); i < committeeSize; i += bitsPerAtt {
			aggregationBits := bitfield.NewBitlist(committeeSize)
			var sigs []bls.Signature
			for b := i; b < i+bitsPerAtt; b++ {
				aggregationBits.SetBitAt(b, true)
				sigs = append(sigs, privs[committee[b]].Sign(dataRoot[:]))
			}

			// bls.AggregateSignatures will return nil if sigs is 0.
			if len(sigs) == 0 {
				continue
			}

			var att ethpb.Att
			if bState.Version() >= version.Electra {
				cb := primitives.NewAttestationCommitteeBits()
				cb.SetBitAt(uint64(c), true)
				att = &ethpb.AttestationElectra{
					Data:            attData,
					CommitteeBits:   cb,
					AggregationBits: aggregationBits,
					Signature:       bls.AggregateSignatures(sigs).Marshal(),
				}
			} else {
				att = &ethpb.Attestation{
					Data:            attData,
					AggregationBits: aggregationBits,
					Signature:       bls.AggregateSignatures(sigs).Marshal(),
				}
			}
			attestations = append(attestations, att)
		}
	}
	return attestations, nil
}

// HydrateAttestation hydrates an attestation object with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateAttestation(a *ethpb.Attestation) *ethpb.Attestation {
	if a.Signature == nil {
		a.Signature = make([]byte, 96)
	}
	if a.AggregationBits == nil {
		a.AggregationBits = make([]byte, 1)
	}
	if a.Data == nil {
		a.Data = &ethpb.AttestationData{}
	}
	a.Data = HydrateAttestationData(a.Data)
	return a
}

// HydrateAttestationElectra hydrates an attestation object with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateAttestationElectra(a *ethpb.AttestationElectra) *ethpb.AttestationElectra {
	if a.Signature == nil {
		a.Signature = make([]byte, 96)
	}
	if a.AggregationBits == nil {
		a.AggregationBits = make([]byte, 1)
	}
	if a.CommitteeBits == nil {
		a.CommitteeBits = primitives.NewAttestationCommitteeBits()
	}
	if a.Data == nil {
		a.Data = &ethpb.AttestationData{}
	}
	a.Data = HydrateAttestationData(a.Data)
	return a
}

func HydrateSingleAttestation(a *ethpb.SingleAttestation) *ethpb.SingleAttestation {
	if a.Signature == nil {
		a.Signature = make([]byte, 96)
	}
	if a.Data == nil {
		a.Data = &ethpb.AttestationData{}
	}
	a.Data = HydrateAttestationData(a.Data)
	return a
}

// HydrateV1Attestation hydrates a v1 attestation object with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV1Attestation(a *attv1.Attestation) *attv1.Attestation {
	if a.Signature == nil {
		a.Signature = make([]byte, 96)
	}
	if a.AggregationBits == nil {
		a.AggregationBits = make([]byte, 1)
	}
	if a.Data == nil {
		a.Data = &attv1.AttestationData{}
	}
	a.Data = HydrateV1AttestationData(a.Data)
	return a
}

// HydrateAttestationData hydrates an attestation data object with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateAttestationData(d *ethpb.AttestationData) *ethpb.AttestationData {
	if d.BeaconBlockRoot == nil {
		d.BeaconBlockRoot = make([]byte, fieldparams.RootLength)
	}
	if d.Target == nil {
		d.Target = &ethpb.Checkpoint{}
	}
	if d.Target.Root == nil {
		d.Target.Root = make([]byte, fieldparams.RootLength)
	}
	if d.Source == nil {
		d.Source = &ethpb.Checkpoint{}
	}
	if d.Source.Root == nil {
		d.Source.Root = make([]byte, fieldparams.RootLength)
	}
	return d
}

// HydrateV1AttestationData hydrates a v1 attestation data object with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateV1AttestationData(d *attv1.AttestationData) *attv1.AttestationData {
	if d.BeaconBlockRoot == nil {
		d.BeaconBlockRoot = make([]byte, fieldparams.RootLength)
	}
	if d.Target == nil {
		d.Target = &attv1.Checkpoint{}
	}
	if d.Target.Root == nil {
		d.Target.Root = make([]byte, fieldparams.RootLength)
	}
	if d.Source == nil {
		d.Source = &attv1.Checkpoint{}
	}
	if d.Source.Root == nil {
		d.Source.Root = make([]byte, fieldparams.RootLength)
	}
	return d
}

// HydrateIndexedAttestation hydrates an indexed attestation with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateIndexedAttestation(a *ethpb.IndexedAttestation) *ethpb.IndexedAttestation {
	if a.Signature == nil {
		a.Signature = make([]byte, 96)
	}
	if a.Data == nil {
		a.Data = &ethpb.AttestationData{}
	}
	a.Data = HydrateAttestationData(a.Data)
	return a
}

// HydrateIndexedAttestationElectra hydrates an indexed attestation with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateIndexedAttestationElectra(a *ethpb.IndexedAttestationElectra) *ethpb.IndexedAttestationElectra {
	if a.Signature == nil {
		a.Signature = make([]byte, 96)
	}
	if a.Data == nil {
		a.Data = &ethpb.AttestationData{}
	}
	a.Data = HydrateAttestationData(a.Data)
	return a
}
