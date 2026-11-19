package blocks

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/validators"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// Spec: process_proposer_slashing
func ProcessProposerSlashingsPQ(
	ctx context.Context,
	st state.BeaconState,
	slashings []*ethpb.ProposerSlashingPQ,
	exitInfo *validators.ExitInfo,
) (state.BeaconState, error) {
	if exitInfo == nil && len(slashings) > 0 {
		return nil, errors.New("exit info required to process proposer slashings")
	}
	for _, slashing := range slashings {
		if err := VerifyProposerSlashingPQ(st, slashing); err != nil {
			return nil, errors.Wrap(err, "could not verify proposer slashing")
		}
		var err error
		st, err = validators.SlashValidator(ctx, st, slashing.Header_1.Header.ProposerIndex, exitInfo)
		if err != nil {
			return nil, errors.Wrapf(err, "could not slash proposer index %d", slashing.Header_1.Header.ProposerIndex)
		}
	}
	return st, nil
}

func VerifyProposerSlashingPQ(st state.ReadOnlyBeaconState, slashing *ethpb.ProposerSlashingPQ) error {
	if slashing == nil || slashing.Header_1 == nil || slashing.Header_1.Header == nil || slashing.Header_2 == nil || slashing.Header_2.Header == nil {
		return errors.New("nil header cannot be verified")
	}
	h1, h2 := slashing.Header_1.Header, slashing.Header_2.Header
	if h1.Slot != h2.Slot {
		return fmt.Errorf("mismatched header slots, received %d == %d", h1.Slot, h2.Slot)
	}
	if h1.ProposerIndex != h2.ProposerIndex {
		return fmt.Errorf("mismatched indices, received %d == %d", h1.ProposerIndex, h2.ProposerIndex)
	}
	if proto.Equal(h1, h2) {
		return errors.New("expected slashing headers to differ")
	}
	proposer, err := st.ValidatorAtIndexReadOnly(h1.ProposerIndex)
	if err != nil {
		return err
	}
	if !helpers.IsSlashableValidatorUsingTrie(proposer, time.CurrentEpoch(st)) {
		return fmt.Errorf("validator with key %#x is not slashable", proposer.PublicKey())
	}
	for _, header := range []*ethpb.SignedBeaconBlockHeaderPQ{slashing.Header_1, slashing.Header_2} {
		if err := pq.VerifyBlockHeaderSignature(st, header); err != nil {
			return errors.Wrap(ErrCouldNotVerifyBlockHeader, err.Error())
		}
	}
	return nil
}
