package decoupled

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/validators"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// Spec: E1 or E2 between two attestations of one validator, PDF §4. d1 and d2 can be the same attestation.
func IsSlashableAttestationData(d1, d2 *ethpb.AttestationDataDecoupled) bool {
	return conflictsWithFinality(d1, d2) || conflictsWithFinality(d2, d1) || isDoubleTargetVote(d1, d2)
}

// Spec: E1, PDF §4. A timeout at the finality pair's height conflicts whatever entry it names.
func conflictsWithFinality(heightPair, finalityPair *ethpb.AttestationDataDecoupled) bool {
	if isEmptyCheckpoint(finalityPair.FinalityTarget) || IsEmptyHeightPair(heightPair) || heightPair.Height != finalityPair.FinalityHeight {
		return false
	}
	return heightPair.Timeout || !bytes.Equal(heightPair.Target.GetRoot(), finalityPair.FinalityTarget.GetRoot())
}

// Spec: E2, PDF §4. A timeout is never an E2 occurrence.
func isDoubleTargetVote(d1, d2 *ethpb.AttestationDataDecoupled) bool {
	if IsEmptyHeightPair(d1) || d1.Timeout || d2.Timeout || d1.Height != d2.Height {
		return false
	}
	return !bytes.Equal(d1.Target.GetRoot(), d2.Target.GetRoot())
}

// Spec: the domain in is_valid_indexed_attestation, by the attestation's own slot epoch. The fork-schedule branch only matters once a fork follows Decoupled.
func attestationDomain(st state.ReadOnlyBeaconState, slot primitives.Slot) ([]byte, error) {
	return signing.Domain(st.Fork(), slots.ToEpoch(slot), params.BeaconConfig().DomainBeaconAttester, st.GenesisValidatorsRoot())
}

func validIndices(st state.ReadOnlyBeaconState, indices []uint64) error {
	if len(indices) == 0 {
		return errors.New("attesting indices is empty")
	}
	n := uint64(st.NumValidators())
	for i, idx := range indices {
		if idx >= n {
			return fmt.Errorf("validator index %d out of range %d", idx, n)
		}
		if i > 0 && indices[i-1] >= idx {
			return errors.New("attesting indices are not sorted and unique")
		}
	}
	return nil
}

// Spec: is_valid_indexed_attestation
func IsValidIndexedAttestation(ctx context.Context, st state.ReadOnlyBeaconState, att *ethpb.IndexedAttestationDecoupled) error {
	if att == nil || att.Data == nil {
		return errors.New("nil indexed attestation")
	}
	if err := validIndices(st, att.AttestingIndices); err != nil {
		return err
	}
	domain, err := attestationDomain(st, att.Data.Slot)
	if err != nil {
		return err
	}
	root, err := signing.ComputeSigningRoot(att.Data, domain)
	if err != nil {
		return err
	}
	pubkeys := make([]bls.PublicKey, len(att.AttestingIndices))
	for i, idx := range att.AttestingIndices {
		pk := st.PubkeyAtIndex(primitives.ValidatorIndex(idx))
		pubkeys[i], err = bls.PublicKeyFromBytes(pk[:])
		if err != nil {
			return err
		}
	}
	sig, err := bls.SignatureFromBytes(att.Signature)
	if err != nil {
		return err
	}
	if !sig.FastAggregateVerify(pubkeys, root) {
		return errors.New("indexed attestation signature did not verify")
	}
	return nil
}

// Spec: the assertions of process_attester_slashing
func VerifyAttesterSlashing(ctx context.Context, st state.ReadOnlyBeaconState, slashing *ethpb.AttesterSlashingDecoupled) error {
	a1, a2 := slashing.Attestation_1, slashing.Attestation_2
	if a1 == nil || a2 == nil || a1.Data == nil || a2.Data == nil {
		return errors.New("nil attestation in slashing")
	}
	if !isAttestationFromActiveDecoupledFork(st, a1.Data) || !isAttestationFromActiveDecoupledFork(st, a2.Data) {
		return errors.New("attester slashing reaches before the Decoupled fork")
	}
	if !IsSlashableAttestationData(a1.Data, a2.Data) {
		return errors.New("attestations are not slashable")
	}
	if err := IsValidIndexedAttestation(ctx, st, a1); err != nil {
		return errors.Wrap(err, "attestation 1")
	}
	if err := IsValidIndexedAttestation(ctx, st, a2); err != nil {
		return errors.Wrap(err, "attestation 2")
	}
	return nil
}

func intersectSorted(a, b []uint64) []uint64 {
	set := make(map[uint64]struct{}, len(a))
	for _, x := range a {
		set[x] = struct{}{}
	}
	var out []uint64
	for _, y := range b {
		if _, ok := set[y]; ok {
			out = append(out, y)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Spec: process_attester_slashing
func ProcessAttesterSlashings(ctx context.Context, st state.BeaconState, slashings []*ethpb.AttesterSlashingDecoupled, exitInfo *validators.ExitInfo) (state.BeaconState, error) {
	for _, s := range slashings {
		if err := VerifyAttesterSlashing(ctx, st, s); err != nil {
			return nil, err
		}
		currentEpoch := time.CurrentEpoch(st)
		slashedAny := false
		for _, idx := range intersectSorted(s.Attestation_1.AttestingIndices, s.Attestation_2.AttestingIndices) {
			val, err := st.ValidatorAtIndexReadOnly(primitives.ValidatorIndex(idx))
			if err != nil {
				return nil, err
			}
			if !helpers.IsSlashableValidatorUsingTrie(val, currentEpoch) {
				continue
			}
			st, err = validators.SlashValidator(ctx, st, primitives.ValidatorIndex(idx), exitInfo)
			if err != nil {
				return nil, err
			}
			slashedAny = true
		}
		if !slashedAny {
			return nil, errors.New("attester slashing did not slash any validator")
		}
	}
	return st, nil
}

// Spec: process_proposer_slashing. Verification is fork-agnostic, only the payment step differs from Gloas.
func ProcessProposerSlashings(ctx context.Context, st state.BeaconState, slashings []*ethpb.ProposerSlashing, exitInfo *validators.ExitInfo) (state.BeaconState, error) {
	for _, s := range slashings {
		if err := blocks.VerifyProposerSlashing(st, s); err != nil {
			return nil, errors.Wrap(err, "could not verify proposer slashing")
		}
		if err := CancelBuilderPendingPayment(st, s.Header_1.Header); err != nil {
			return nil, err
		}
		var err error
		st, err = validators.SlashValidator(ctx, st, s.Header_1.Header.ProposerIndex, exitInfo)
		if err != nil {
			return nil, errors.Wrapf(err, "could not slash proposer index %d", s.Header_1.Header.ProposerIndex)
		}
	}
	return st, nil
}
