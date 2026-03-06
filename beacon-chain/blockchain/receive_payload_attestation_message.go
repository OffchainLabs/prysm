package blockchain

import (
	"context"

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
	idx, err := gloas.PayloadCommitteeIndex(ctx, st, a.Data.Slot, a.ValidatorIndex)
	if err != nil {
		return err
	}
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	s.cfg.ForkChoiceStore.SetPTCVote(root, idx, a.Data.PayloadPresent, a.Data.BlobDataAvailable)
	return nil
}
