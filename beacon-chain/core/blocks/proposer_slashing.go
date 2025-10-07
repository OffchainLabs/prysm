package blocks

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/validators"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// ErrCouldNotVerifyBlockHeader is returned when a block header's signature cannot be verified.
var ErrCouldNotVerifyBlockHeader = errors.New("could not verify beacon block header")

// ProcessProposerSlashings is one of the operations performed
// on each processed beacon block to slash proposers based on
// slashing conditions if any slashable events occurred.
//
// Spec pseudocode definition:
//
//	def process_proposer_slashing(state: BeaconState, proposer_slashing: ProposerSlashing) -> None:
//	 header_1 = proposer_slashing.signed_header_1.message
//	 header_2 = proposer_slashing.signed_header_2.message
//
//	 # Verify header slots match
//	 assert header_1.slot == header_2.slot
//	 # Verify header proposer indices match
//	 assert header_1.proposer_index == header_2.proposer_index
//	 # Verify the headers are different
//	 assert header_1 != header_2
//	 # Verify the proposer is slashable
//	 proposer = state.validators[header_1.proposer_index]
//	 assert is_slashable_validator(proposer, get_current_epoch(state))
//	 # Verify signatures
//	 for signed_header in (proposer_slashing.signed_header_1, proposer_slashing.signed_header_2):
//	     domain = get_domain(state, DOMAIN_BEACON_PROPOSER, compute_epoch_at_slot(signed_header.message.slot))
//	     signing_root = compute_signing_root(signed_header.message, domain)
//	     assert bls.Verify(proposer.pubkey, signing_root, signed_header.signature)
//
//	 slash_validator(state, header_1.proposer_index)
func ProcessProposerSlashings(
	ctx context.Context,
	beaconState state.BeaconState,
	slashings []*ethpb.ProposerSlashing,
	exitInfo *validators.ExitInfo,
) (state.BeaconState, error) {
	if exitInfo == nil && len(slashings) > 0 {
		return nil, errors.New("exit info required to process proposer slashings")
	}
	var err error
	for _, slashing := range slashings {
		beaconState, err = ProcessProposerSlashing(ctx, beaconState, slashing, exitInfo)
		if err != nil {
			return nil, err
		}
	}
	return beaconState, nil
}

// ProcessProposerSlashingsNoVerify processes proposer slashings without verifying them.
// This is useful in scenarios such as block reward calculation, where we can assume the data
// in the block is valid.
func ProcessProposerSlashingsNoVerify(
	ctx context.Context,
	beaconState state.BeaconState,
	slashings []*ethpb.ProposerSlashing,
	exitInfo *validators.ExitInfo,
) (state.BeaconState, error) {
	if exitInfo == nil && len(slashings) > 0 {
		return nil, errors.New("exit info required to process proposer slashings")
	}
	var err error
	for _, slashing := range slashings {
		beaconState, err = ProcessProposerSlashingNoVerify(ctx, beaconState, slashing, exitInfo)
		if err != nil {
			return nil, err
		}
	}
	return beaconState, nil
}

// ProcessProposerSlashing processes individual proposer slashing.
func ProcessProposerSlashing(
	ctx context.Context,
	beaconState state.BeaconState,
	slashing *ethpb.ProposerSlashing,
	exitInfo *validators.ExitInfo,
) (state.BeaconState, error) {
	if slashing == nil {
		return nil, errors.New("nil proposer slashings in block body")
	}
	if err := VerifyProposerSlashing(beaconState, slashing); err != nil {
		return nil, errors.Wrap(err, "could not verify proposer slashing")
	}
	return processProposerSlashing(ctx, beaconState, slashing, exitInfo)
}

// ProcessProposerSlashingNoVerify processes individual proposer slashing without verifying it.
// This is useful in scenarios such as block reward calculation, where we can assume the data
// in the block is valid.
func ProcessProposerSlashingNoVerify(
	ctx context.Context,
	beaconState state.BeaconState,
	slashing *ethpb.ProposerSlashing,
	exitInfo *validators.ExitInfo,
) (state.BeaconState, error) {
	if slashing == nil {
		return nil, errors.New("nil proposer slashings in block body")
	}
	return processProposerSlashing(ctx, beaconState, slashing, exitInfo)
}

func processProposerSlashing(
	ctx context.Context,
	beaconState state.BeaconState,
	slashing *ethpb.ProposerSlashing,
	exitInfo *validators.ExitInfo,
) (state.BeaconState, error) {
	if exitInfo == nil {
		return nil, errors.New("exit info is required to process proposer slashing")
	}
	var err error
	beaconState, err = validators.SlashValidator(ctx, beaconState, slashing.Header_1.Header.ProposerIndex, exitInfo)
	if err != nil {
		return nil, errors.Wrapf(err, "could not slash proposer index %d", slashing.Header_1.Header.ProposerIndex)
	}
	return beaconState, nil
}

// VerifyProposerSlashing verifies that the data provided from slashing is valid.
func VerifyProposerSlashing(
	beaconState state.ReadOnlyBeaconState,
	slashing *ethpb.ProposerSlashing,
) error {
	if slashing.Header_1 == nil || slashing.Header_1.Header == nil || slashing.Header_2 == nil || slashing.Header_2.Header == nil {
		return errors.New("nil header cannot be verified")
	}
	hSlot := slashing.Header_1.Header.Slot
	if hSlot != slashing.Header_2.Header.Slot {
		return fmt.Errorf("mismatched header slots, received %d == %d", slashing.Header_1.Header.Slot, slashing.Header_2.Header.Slot)
	}
	pIdx := slashing.Header_1.Header.ProposerIndex
	if pIdx != slashing.Header_2.Header.ProposerIndex {
		return fmt.Errorf("mismatched indices, received %d == %d", slashing.Header_1.Header.ProposerIndex, slashing.Header_2.Header.ProposerIndex)
	}
	if proto.Equal(slashing.Header_1.Header, slashing.Header_2.Header) {
		return errors.New("expected slashing headers to differ")
	}
	proposer, err := beaconState.ValidatorAtIndexReadOnly(slashing.Header_1.Header.ProposerIndex)
	if err != nil {
		return err
	}
	if !helpers.IsSlashableValidatorUsingTrie(proposer, time.CurrentEpoch(beaconState)) {
		return fmt.Errorf("validator with key %#x is not slashable", proposer.PublicKey())
	}
	headers := []*ethpb.SignedBeaconBlockHeader{slashing.Header_1, slashing.Header_2}
	for _, header := range headers {
		if err := signing.ComputeDomainVerifySigningRoot(beaconState, pIdx, slots.ToEpoch(hSlot),
			header.Header, params.BeaconConfig().DomainBeaconProposer, header.Signature); err != nil {
			return errors.Wrap(ErrCouldNotVerifyBlockHeader, err.Error())
		}
	}
	return nil
}
