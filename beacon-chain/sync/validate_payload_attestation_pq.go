package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	payloadattestation "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attestation"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
)

// The shared checks and fork choice read only the index and data, so a BLS shaped copy carries them.
func payloadAttestationStandIn(msg *eth.PayloadAttestationMessagePQ) *eth.PayloadAttestationMessage {
	return &eth.PayloadAttestationMessage{
		ValidatorIndex: msg.ValidatorIndex,
		Data:           msg.Data,
		Signature:      make([]byte, fieldparams.BLSSignatureLength),
	}
}

// A message that arrives before its block is ignored, the pending queue verifies BLS only.
func (s *Service) validatePayloadAttestationPQ(ctx context.Context, v verification.PayloadAttestationMsgVerifier, pa payloadattestation.ROMessage, msg *eth.PayloadAttestationMessagePQ) (pubsub.ValidationResult, error) {
	if err := v.VerifyBlockRootSeen(s.cfg.chain.InForkchoice); err != nil {
		return pubsub.ValidationIgnore, err
	}
	blockSlot, err := s.cfg.chain.RecentBlockSlot(pa.BeaconBlockRoot())
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	if err := v.VerifyBlockSlotMatches(blockSlot); err != nil {
		return pubsub.ValidationIgnore, err
	}
	if err := v.VerifyBlockRootValid(s.hasBadBlock); err != nil {
		return pubsub.ValidationReject, err
	}
	st, err := s.cfg.chain.PtcLookupState(ctx, pa.BeaconBlockRoot(), pa.Slot())
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	if st == nil {
		return pubsub.ValidationIgnore, errors.New("unable to find state for payload attestation")
	}
	if err := v.VerifyValidatorInPTC(ctx, st); err != nil {
		if errors.Is(err, state.ErrNoPayloadCommitteeAvailable) {
			return pubsub.ValidationIgnore, err
		}
		return pubsub.ValidationReject, err
	}
	if err := pq.VerifyPayloadAttestationMessage(st, msg); err != nil {
		return pubsub.ValidationReject, err
	}
	return pubsub.ValidationAccept, nil
}
