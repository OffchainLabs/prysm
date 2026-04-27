package blockchain

import (
	"bytes"
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// PayloadAttestationReceiver interface defines the methods of chain service for receiving
// validated payload attestation messages.
type PayloadAttestationReceiver interface {
	ReceivePayloadAttestationMessage(context.Context, *ethpb.PayloadAttestationMessage) error
	PtcLookupState(ctx context.Context, blockRoot [32]byte, blockSlot primitives.Slot) (state.ReadOnlyBeaconState, error)
}

// ReceivePayloadAttestationMessage accepts a payload attestation message and updates the
// forkchoice PTC vote bitvectors for the referenced beacon block.
func (s *Service) ReceivePayloadAttestationMessage(ctx context.Context, a *ethpb.PayloadAttestationMessage) error {
	if a == nil || a.Data == nil {
		return errors.New("nil payload attestation message")
	}
	root := bytesutil.ToBytes32(a.Data.BeaconBlockRoot)

	st, err := s.PtcLookupState(ctx, root, a.Data.Slot)
	if err != nil {
		return err
	}
	if st == nil {
		return errors.New("unable to find state for payload attestation")
	}
	idx, err := gloas.PayloadCommitteeIndex(ctx, st, a.Data.Slot, a.ValidatorIndex)
	if err != nil {
		return err
	}
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	s.cfg.ForkChoiceStore.SetPTCVote(root, idx, a.Data.PayloadPresent, a.Data.BlobDataAvailable)
	return nil
}

func (s *Service) PtcLookupState(ctx context.Context, blockRoot [32]byte, blockSlot primitives.Slot) (state.ReadOnlyBeaconState, error) {
	blockEpoch := slots.ToEpoch(blockSlot)
	headEpoch := slots.ToEpoch(s.HeadSlot())
	headRoot, err := s.HeadRoot(ctx)
	if err != nil {
		return nil, err
	}
	blockDependent, err := s.DependentRootForEpoch(blockRoot, blockEpoch)
	if err != nil {
		return nil, err
	}

	if blockEpoch == headEpoch {
		if bytes.Equal(blockRoot[:], headRoot) {
			return s.HeadStateReadOnly(ctx)
		}
		headDependent, err := s.DependentRootForEpoch(bytesutil.ToBytes32(headRoot), blockEpoch)
		if err != nil {
			return nil, err
		}
		if bytes.Equal(headDependent[:], blockDependent[:]) {
			return s.HeadStateReadOnly(ctx)
		}
	}
	if bytes.Equal(blockDependent[:], headRoot) {
		headState, err := s.HeadState(ctx)
		if err != nil {
			return nil, err
		}

		return transition.ProcessSlotsUsingNextSlotCache(ctx, headState, headRoot, blockSlot)
	}
	return nil, nil
}
