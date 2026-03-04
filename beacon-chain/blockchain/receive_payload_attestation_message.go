package blockchain

import (
	"context"
	"slices"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/pkg/errors"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// PayloadAttestationReceiver interface defines the methods of chain service for receiving
// validated payload attestation messages.
type PayloadAttestationReceiver interface {
	ReceivePayloadAttestationMessage(context.Context, *ethpb.PayloadAttestationMessage) error
}

// ReceivePayloadAttestationMessage accepts a payload attestation message and updates the
// forkchoice PTC vote bitvectors for the referenced beacon block.
func (s *Service) ReceivePayloadAttestationMessage(ctx context.Context, a *ethpb.PayloadAttestationMessage) error {
	if a == nil || a.Data == nil {
		return errors.New("nil payload attestation message")
	}
	root := bytesutil.ToBytes32(a.Data.BeaconBlockRoot)

	st, err := s.HeadStateReadOnly(ctx)
	if err != nil {
		return err
	}
	ptc, err := gloas.PayloadCommittee(ctx, st, a.Data.Slot)
	if err != nil {
		return err
	}
	idx := slices.Index(ptc, a.ValidatorIndex)
	if idx == -1 {
		return errors.New("validator not in PTC")
	}
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	s.cfg.ForkChoiceStore.SetPTCVote(root, uint64(idx), a.Data.PayloadPresent, a.Data.BlobDataAvailable)
	return nil
}
