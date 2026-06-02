package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	payloadattestation "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attestation"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	prysmTime "github.com/OffchainLabs/prysm/v7/time"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

var (
	errAlreadySeenPayloadAttestation = errors.New("payload attestation already seen for validator index")
)

func (s *Service) validatePayloadAttestation(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}
	receivedTime := prysmTime.Now()
	ctx, span := trace.StartSpan(ctx, "sync.validatePayloadAttestation")
	defer span.End()

	if msg.Topic == nil {
		return pubsub.ValidationReject, p2p.ErrInvalidTopic
	}
	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		return pubsub.ValidationReject, err
	}
	att, ok := m.(*eth.PayloadAttestationMessage)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}
	pa, err := payloadattestation.NewReadOnly(att)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	v := s.newPayloadAttestationVerifier(pa, verification.GossipPayloadAttestationMessageRequirements)

	// [IGNORE] The message's slot is for the current slot (with a MAXIMUM_GOSSIP_CLOCK_DISPARITY allowance),
	// i.e. data.slot == current_slot.
	if err := v.VerifyCurrentSlot(); err != nil {
		return pubsub.ValidationIgnore, err
	}

	// [IGNORE] The payload_attestation_message is the first valid message received from the validator with
	// index payload_attestation_message.validator_index.
	if s.payloadAttestationCache.Seen(pa.Slot(), pa.ValidatorIndex()) {
		return pubsub.ValidationIgnore, errAlreadySeenPayloadAttestation
	}

	// [IGNORE] The message's block data.beacon_block_root has been seen (via gossip or non-gossip sources)
	// (a client MAY queue attestation for processing once the block is retrieved. Note a client might want to request payload after).
	if err := v.VerifyBlockRootSeen(s.cfg.chain.InForkchoice); err != nil {
		// TODO: queue attestation
		return pubsub.ValidationIgnore, err
	}

	// [REJECT] The message's block data.beacon_block_root passes validation.
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

	// [REJECT] The message's validator index is within the payload committee in get_ptc(state, data.slot).
	// The state is the head state corresponding to processing the block up to the current slot.
	if err := v.VerifyValidatorInPTC(ctx, st); err != nil {
		return pubsub.ValidationReject, err
	}

	// [REJECT] payload_attestation_message.signature is valid with respect to the validator's public key.
	if err := v.VerifySignature(st); err != nil {
		return pubsub.ValidationReject, err
	}

	msg.ValidatorData = att
	startTime, err := slots.StartTime(s.cfg.clock.GenesisTime(), pa.Slot())
	if err == nil {
		syncPayloadAttestationArrivalDelaySeconds.Observe(receivedTime.Sub(startTime).Seconds())
	} else {
		log.WithError(err).WithField("slot", pa.Slot()).Debug("Could not compute payload attestation slot start time")
	}

	return pubsub.ValidationAccept, nil
}
