package sync

import (
	"context"
	"errors"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/decoupled"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	prysmTime "github.com/OffchainLabs/prysm/v7/time"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"
)

var errDecoupledSeen = errors.New("decoupled message already seen")

// Spec: on_ac_message. IGNORE while syncing, outside the slot window or past two distinct votes by the validator, REJECT on the wire checks. Head availability is not a wire check.
func (s *Service) validateAvailableAttestation(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}
	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		return pubsub.ValidationReject, err
	}
	att, ok := m.(*eth.AvailableAttestationMessage)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}
	if att.Data == nil {
		return pubsub.ValidationReject, errNilMessage
	}
	if att.Data.Slot+1 < s.cfg.clock.CurrentSlot() {
		return pubsub.ValidationIgnore, nil
	}
	start, err := slots.StartTime(s.cfg.clock.GenesisTime(), att.Data.Slot)
	if err != nil || start.After(prysmTime.Now().Add(params.BeaconConfig().MaximumGossipClockDisparityDuration())) {
		return pubsub.ValidationIgnore, err
	}
	if s.cfg.decoupledVotePool.Seen(att.ValidatorIndex, att.Data.Slot, bytesutil.ToBytes32(att.Data.BeaconBlockRoot)) {
		return pubsub.ValidationIgnore, errDecoupledSeen
	}
	st, err := s.cfg.chain.HeadStateReadOnly(ctx)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	if err := decoupled.VerifyAvailableAttestation(st, att); err != nil {
		return pubsub.ValidationReject, err
	}
	msg.ValidatorData = att
	return pubsub.ValidationAccept, nil
}

// Spec: on_attestation. IGNORE while syncing or on a repeat, REJECT on the wire checks. No age window, a delayed row for a current height is still useful.
func (s *Service) validateFinalityAttestation(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}
	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		return pubsub.ValidationReject, err
	}
	att, ok := m.(*eth.FinalityAttestationMessage)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}
	if att.Data == nil {
		return pubsub.ValidationReject, errNilMessage
	}
	if s.cfg.decoupledFGPool.Has(att.ValidatorIndex, att.Data) {
		return pubsub.ValidationIgnore, errDecoupledSeen
	}
	st, err := s.cfg.chain.HeadStateReadOnly(ctx)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	if err := decoupled.VerifyFinalityAttestation(st, att); err != nil {
		return pubsub.ValidationReject, err
	}
	msg.ValidatorData = att
	return pubsub.ValidationAccept, nil
}

func (s *Service) availableAttestationSubscriber(ctx context.Context, msg proto.Message) error {
	att, ok := msg.(*eth.AvailableAttestationMessage)
	if !ok {
		return errWrongMessage
	}
	return s.cfg.chain.ReceiveAvailableAttestation(ctx, att)
}

// Spec: fg_on_attestation, the inclusion pool only, finality moves through blocks.
func (s *Service) finalityAttestationSubscriber(_ context.Context, msg proto.Message) error {
	att, ok := msg.(*eth.FinalityAttestationMessage)
	if !ok {
		return errWrongMessage
	}
	return s.cfg.decoupledFGPool.InsertSingle(att.ValidatorIndex, att.Data, att.Signature)
}
