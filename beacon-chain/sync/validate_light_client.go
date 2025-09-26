package sync

import (
	"context"
	"fmt"
	"time"

	lightclient "github.com/OffchainLabs/prysm/v6/beacon-chain/light-client"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

func (s *Service) validateLightClientOptimisticUpdate(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	// Validation runs on publish (not just subscriptions), so we should approve any message from
	// ourselves.
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}

	// Ignore updates while syncing
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	_, span := trace.StartSpan(ctx, "sync.validateLightClientOptimisticUpdate")
	defer span.End()

	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationReject, err
	}

	newUpdate, ok := m.(interfaces.LightClientOptimisticUpdate)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}

	attestedHeaderRoot, err := newUpdate.AttestedHeader().Beacon().HashTreeRoot()
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	// [IGNORE] The optimistic_update is received after the block at signature_slot was given enough time
	// to propagate through the network -- i.e. validate that one-third of optimistic_update.signature_slot
	// has transpired (SECONDS_PER_SLOT / INTERVALS_PER_SLOT seconds after the start of the slot,
	// with a MAXIMUM_GOSSIP_CLOCK_DISPARITY allowance)
	slotStart, err := slots.StartTime(s.cfg.clock.GenesisTime(), newUpdate.SignatureSlot())
	if err != nil {
		log.WithError(err).Debug("Peer sent a slot that would overflow slot start time")
		return pubsub.ValidationReject, nil
	}
	earliestValidTime := slotStart.
		Add(time.Second * time.Duration(params.BeaconConfig().SecondsPerSlot/params.BeaconConfig().IntervalsPerSlot)).
		Add(-params.BeaconConfig().MaximumGossipClockDisparityDuration())
	if s.cfg.clock.Now().Before(earliestValidTime) {
		log.Debug("Newly received light client optimistic update ignored. not enough time passed for block to propagate")
		return pubsub.ValidationIgnore, nil
	}

	if !lightclient.IsBetterOptimisticUpdate(newUpdate, s.lcStore.LastOptimisticUpdate()) {
		log.WithFields(logrus.Fields{
			"attestedSlot":       fmt.Sprintf("%d", newUpdate.AttestedHeader().Beacon().Slot),
			"signatureSlot":      fmt.Sprintf("%d", newUpdate.SignatureSlot()),
			"attestedHeaderRoot": fmt.Sprintf("%x", attestedHeaderRoot),
		}).Debug("Newly received light client optimistic update ignored. current update is better.")
		return pubsub.ValidationIgnore, nil
	}

	log.WithFields(logrus.Fields{
		"attestedSlot":       fmt.Sprintf("%d", newUpdate.AttestedHeader().Beacon().Slot),
		"signatureSlot":      fmt.Sprintf("%d", newUpdate.SignatureSlot()),
		"attestedHeaderRoot": fmt.Sprintf("%x", attestedHeaderRoot),
	}).Debug("New gossiped light client optimistic update validated.")

	msg.ValidatorData = newUpdate.Proto()
	return pubsub.ValidationAccept, nil
}

func (s *Service) validateLightClientFinalityUpdate(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	// Validation runs on publish (not just subscriptions), so we should approve any message from
	// ourselves.
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}

	// Ignore updates while syncing
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	_, span := trace.StartSpan(ctx, "sync.validateLightClientFinalityUpdate")
	defer span.End()

	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationReject, err
	}

	newUpdate, ok := m.(interfaces.LightClientFinalityUpdate)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}

	attestedHeaderRoot, err := newUpdate.AttestedHeader().Beacon().HashTreeRoot()
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	// [IGNORE] The optimistic_update is received after the block at signature_slot was given enough time
	// to propagate through the network -- i.e. validate that one-third of optimistic_update.signature_slot
	// has transpired (SECONDS_PER_SLOT / INTERVALS_PER_SLOT seconds after the start of the slot,
	// with a MAXIMUM_GOSSIP_CLOCK_DISPARITY allowance)
	slotStart, err := slots.StartTime(s.cfg.clock.GenesisTime(), newUpdate.SignatureSlot())
	if err != nil {
		log.WithError(err).Debug("Peer sent a slot that would overflow slot start time")
		return pubsub.ValidationReject, nil
	}
	earliestValidTime := slotStart.
		Add(time.Second * time.Duration(params.BeaconConfig().SecondsPerSlot/params.BeaconConfig().IntervalsPerSlot)).
		Add(-params.BeaconConfig().MaximumGossipClockDisparityDuration())
	if s.cfg.clock.Now().Before(earliestValidTime) {
		log.Debug("Newly received light client finality update ignored. not enough time passed for block to propagate")
		return pubsub.ValidationIgnore, nil
	}

	if !lightclient.IsFinalityUpdateValidForBroadcast(newUpdate, s.lcStore.LastFinalityUpdate()) {
		log.WithFields(logrus.Fields{
			"attestedSlot":       fmt.Sprintf("%d", newUpdate.AttestedHeader().Beacon().Slot),
			"signatureSlot":      fmt.Sprintf("%d", newUpdate.SignatureSlot()),
			"attestedHeaderRoot": fmt.Sprintf("%x", attestedHeaderRoot),
		}).Debug("Newly received light client finality update ignored. current update is better.")
		return pubsub.ValidationIgnore, nil
	}

	log.WithFields(logrus.Fields{
		"attestedSlot":       fmt.Sprintf("%d", newUpdate.AttestedHeader().Beacon().Slot),
		"signatureSlot":      fmt.Sprintf("%d", newUpdate.SignatureSlot()),
		"attestedHeaderRoot": fmt.Sprintf("%x", attestedHeaderRoot),
	}).Debug("New gossiped light client finality update validated.")

	msg.ValidatorData = newUpdate.Proto()
	return pubsub.ValidationAccept, nil
}
