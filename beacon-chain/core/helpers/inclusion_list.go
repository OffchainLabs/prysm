package helpers

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// InclusionListCommittee returns the inclusion list committee for the slot.
func InclusionListCommittee(ctx context.Context, st state.ReadOnlyBeaconState, slot primitives.Slot) ([]primitives.ValidatorIndex, error) {
	epoch := slots.ToEpoch(slot)
	if epoch < params.BeaconConfig().HezeForkEpoch {
		return nil, errors.New("inclusion list slot is before the Heze fork")
	}

	activeCount, err := ActiveValidatorCount(ctx, st, epoch)
	if err != nil {
		return nil, fmt.Errorf("get active validator count: %w", err)
	}

	var indices []primitives.ValidatorIndex
	for i := range SlotCommitteeCount(activeCount) {
		committee, err := BeaconCommitteeFromState(ctx, st, slot, primitives.CommitteeIndex(i))
		if err != nil {
			return nil, fmt.Errorf("get beacon committee %d: %w", i, err)
		}
		indices = append(indices, committee...)

		// Only the first InclusionListCommitteeSize indices are used.
		if len(indices) >= fieldparams.InclusionListCommitteeSize {
			break
		}
	}
	if len(indices) == 0 {
		return nil, errors.New("no beacon committee members for inclusion list committee")
	}

	committee := make([]primitives.ValidatorIndex, fieldparams.InclusionListCommitteeSize)
	for i := range committee {
		committee[i] = indices[i%len(indices)]
	}
	return committee, nil
}

// InclusionListCommitteeAssignment returns the slot in the epoch where the validator is in the inclusion list committee.
func InclusionListCommitteeAssignment(ctx context.Context, st state.ReadOnlyBeaconState, epoch primitives.Epoch, validatorIndex primitives.ValidatorIndex) (primitives.Slot, bool, error) {
	if epoch > slots.ToEpoch(st.Slot())+1 {
		return 0, false, errors.New("inclusion list assignment epoch is after the next epoch")
	}

	start, err := slots.EpochStart(epoch)
	if err != nil {
		return 0, false, fmt.Errorf("get epoch start: %w", err)
	}

	spe := params.BeaconConfig().SlotsPerEpoch
	for slot := start; slot < start+spe; slot++ {
		committee, err := InclusionListCommittee(ctx, st, slot)
		if err != nil {
			return 0, false, fmt.Errorf("get inclusion list committee: %w", err)
		}
		if slices.Contains(committee, validatorIndex) {
			return slot, true, nil
		}
	}
	return 0, false, nil
}

// InclusionListSigningDomain returns the inclusion list domain from the fork version scheduled at the epoch, not from the state fork.
func InclusionListSigningDomain(epoch primitives.Epoch, genesisValidatorsRoot []byte) ([]byte, error) {
	fork, err := params.Fork(epoch)
	if err != nil {
		return nil, fmt.Errorf("get fork: %w", err)
	}
	return signing.ComputeDomain(params.BeaconConfig().DomainInclusionListCommittee, fork.CurrentVersion, genesisValidatorsRoot)
}

// VerifyInclusionListSignature verifies the signature of the inclusion list.
func VerifyInclusionListSignature(ctx context.Context, st state.ReadOnlyBeaconState, signed *ethpb.SignedInclusionList) error {
	if signed == nil || signed.Message == nil {
		return errors.New("nil inclusion list")
	}
	if len(signed.Signature) != fieldparams.BLSSignatureLength {
		return fmt.Errorf("inclusion list signature length is %d, want %d", len(signed.Signature), fieldparams.BLSSignatureLength)
	}

	val, err := st.ValidatorAtIndexReadOnly(signed.Message.ValidatorIndex)
	if err != nil {
		return fmt.Errorf("get validator: %w", err)
	}

	pubkey := val.PublicKey()
	pub, err := bls.PublicKeyFromBytes(pubkey[:])
	if err != nil {
		return fmt.Errorf("convert public key: %w", err)
	}
	sig, err := bls.SignatureFromBytes(signed.Signature)
	if err != nil {
		return fmt.Errorf("convert signature: %w", err)
	}

	domain, err := InclusionListSigningDomain(slots.ToEpoch(signed.Message.Slot), st.GenesisValidatorsRoot())
	if err != nil {
		return fmt.Errorf("get inclusion list signing domain: %w", err)
	}
	root, err := signing.ComputeSigningRoot(signed.Message, domain)
	if err != nil {
		return fmt.Errorf("compute signing root: %w", err)
	}
	if !sig.Verify(pub, root[:]) {
		return signing.ErrSigFailedToVerify
	}
	return nil
}
