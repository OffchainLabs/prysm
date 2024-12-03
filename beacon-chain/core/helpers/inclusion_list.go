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

// GetInclusionListCommittee retrieves the validator indices assigned to the inclusion list committee
// for a given slot. Returns an error if the state or slot does not meet the required constraints.
func GetInclusionListCommittee(ctx context.Context, state state.ReadOnlyBeaconState, slot primitives.Slot) ([]primitives.ValidatorIndex, error) {
	if slots.ToEpoch(slot) < params.BeaconConfig().Eip7805ForkEpoch {
		return nil, errIncorrectState
	}
	epoch := slots.ToEpoch(slot)
	seed, err := Seed(state, epoch, params.BeaconConfig().DomainInclusionListCommittee)
	if err != nil {
		return nil, errors.Wrap(err, "could not get seed")
	}
	indices, err := ActiveValidatorIndices(ctx, state, epoch)
	if err != nil {
		return nil, err
	}
	start := uint64(slot%params.BeaconConfig().SlotsPerEpoch) * params.BeaconConfig().InclusionListCommitteeSize
	end := start + params.BeaconConfig().InclusionListCommitteeSize

	shuffledIndices := make([]primitives.ValidatorIndex, len(indices))
	copy(shuffledIndices, indices)
	shuffledList, err := UnshuffleList(shuffledIndices, seed)
	if err != nil {
		return nil, err
	}
	return shuffledList[start:end], nil
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

	currentEpoch := slots.ToEpoch(st.Slot())
	domain, err := signing.Domain(st.Fork(), currentEpoch, params.BeaconConfig().DomainInclusionListCommittee, st.GenesisValidatorsRoot())
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
