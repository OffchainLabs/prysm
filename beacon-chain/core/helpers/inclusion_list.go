package helpers

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
)

var (
	errNilIl            = errors.New("nil inclusion list")
	errNilCommitteeRoot = errors.New("nil inclusion list committee root")
	errNilSignature     = errors.New("nil signature")
	errIncorrectState   = errors.New("incorrect state version")
	errNoCommittee      = errors.New("no beacon committee members for inclusion list committee")
)

// ValidateNilSignedInclusionList validates that a SignedInclusionList is not nil and contains a signature.
func ValidateNilSignedInclusionList(il *eth.SignedInclusionList) error {
	if il == nil {
		return errNilIl
	}
	if il.Signature == nil {
		return errNilSignature
	}
	return ValidateNilInclusionList(il.Message)
}

// ValidateNilInclusionList validates that an InclusionList is not nil and contains a committee root.
func ValidateNilInclusionList(il *eth.InclusionList) error {
	if il == nil {
		return errNilIl
	}
	if il.InclusionListCommitteeRoot == nil {
		return errNilCommitteeRoot
	}
	return nil
}

// GetInclusionListCommittee returns the inclusion list committee for the given slot,
// formed by concatenating the slot's beacon committees in order and cycling over
// them to fill INCLUSION_LIST_COMMITTEE_SIZE entries.
func GetInclusionListCommittee(ctx context.Context, state state.ReadOnlyBeaconState, slot primitives.Slot) ([]primitives.ValidatorIndex, error) {
	epoch := slots.ToEpoch(slot)
	if epoch < params.BeaconConfig().Eip7805ForkEpoch {
		return nil, errIncorrectState
	}

	activeCount, err := ActiveValidatorCount(ctx, state, epoch)
	if err != nil {
		return nil, err
	}
	committeesPerSlot := SlotCommitteeCount(activeCount)

	var indices []primitives.ValidatorIndex
	for i := uint64(0); i < committeesPerSlot; i++ {
		committee, err := BeaconCommitteeFromState(ctx, state, slot, primitives.CommitteeIndex(i))
		if err != nil {
			return nil, err
		}
		indices = append(indices, committee...)
	}
	if len(indices) == 0 {
		return nil, errNoCommittee
	}

	size := params.BeaconConfig().InclusionListCommitteeSize
	committee := make([]primitives.ValidatorIndex, size)
	for i := uint64(0); i < size; i++ {
		committee[i] = indices[i%uint64(len(indices))]
	}
	return committee, nil
}

// ValidateInclusionListSignature verifies the signature on a SignedInclusionList against the public key
// of the validator specified in the inclusion list.
func ValidateInclusionListSignature(ctx context.Context, st state.ReadOnlyBeaconState, il *eth.SignedInclusionList) error {
	if err := ValidateNilSignedInclusionList(il); err != nil {
		return err
	}

	val, err := st.ValidatorAtIndex(il.Message.ValidatorIndex)
	if err != nil {
		return err
	}
	pub, err := bls.PublicKeyFromBytes(val.PublicKey)
	if err != nil {
		return err
	}
	sig, err := bls.SignatureFromBytes(il.Signature)
	if err != nil {
		return err
	}

	epoch := slots.ToEpoch(il.Message.Slot)
	domain, err := signing.Domain(st.Fork(), epoch, params.BeaconConfig().DomainInclusionListCommittee, st.GenesisValidatorsRoot())
	if err != nil {
		return err
	}

	root, err := signing.ComputeSigningRoot(il.Message, domain)
	if err != nil {
		return err
	}

	if !sig.Verify(pub, root[:]) {
		return signing.ErrSigFailedToVerify
	}
	return nil
}
