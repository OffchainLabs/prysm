package sync

import (
	"context"
	"time"

	lightClient "github.com/OffchainLabs/prysm/v6/beacon-chain/core/light-client"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
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

	// [IGNORE] The optimistic_update is received after the block at signature_slot was given enough time
	// to propagate through the network -- i.e. validate that one-third of optimistic_update.signature_slot
	// has transpired (SECONDS_PER_SLOT / INTERVALS_PER_SLOT seconds after the start of the slot,
	// with a MAXIMUM_GOSSIP_CLOCK_DISPARITY allowance)
	earliestValidTime := slots.StartTime(uint64(s.cfg.clock.GenesisTime().Unix()), newUpdate.SignatureSlot()).
		Add(time.Second * time.Duration(params.BeaconConfig().SecondsPerSlot/params.BeaconConfig().IntervalsPerSlot)).
		Add(-params.BeaconConfig().MaximumGossipClockDisparityDuration())
	if s.cfg.clock.Now().Before(earliestValidTime) {
		log.Debug("Newly received light client optimistic update ignored. not enough time passed for block to propagate")
		return pubsub.ValidationIgnore, nil
	}

	lastStoredUpdate := s.lcStore.LastOptimisticUpdate()
	if lastStoredUpdate != nil {
		lastUpdateSlot := lastStoredUpdate.AttestedHeader().Beacon().Slot
		newUpdateSlot := newUpdate.AttestedHeader().Beacon().Slot

		// [IGNORE] The attested_header.beacon.slot is greater than that of all previously forwarded optimistic_updates
		if newUpdateSlot <= lastUpdateSlot {
			log.Debug("Newly received light client optimistic update ignored. new update is older than stored update")
			return pubsub.ValidationIgnore, nil
		}
	}

	msg.ValidatorData = newUpdate.Proto()
	log.Debug("New gossiped light client optimistic update validated. Attested slot %d, Signature slot %d", newUpdate.AttestedHeader().Beacon().Slot, newUpdate.SignatureSlot())
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

	// [IGNORE] The optimistic_update is received after the block at signature_slot was given enough time
	// to propagate through the network -- i.e. validate that one-third of optimistic_update.signature_slot
	// has transpired (SECONDS_PER_SLOT / INTERVALS_PER_SLOT seconds after the start of the slot,
	// with a MAXIMUM_GOSSIP_CLOCK_DISPARITY allowance)
	earliestValidTime := slots.StartTime(uint64(s.cfg.clock.GenesisTime().Unix()), newUpdate.SignatureSlot()).
		Add(time.Second * time.Duration(params.BeaconConfig().SecondsPerSlot/params.BeaconConfig().IntervalsPerSlot)).
		Add(-params.BeaconConfig().MaximumGossipClockDisparityDuration())
	if s.cfg.clock.Now().Before(earliestValidTime) {
		log.Debug("Newly received light client finality update ignored. not enough time passed for block to propagate")
		return pubsub.ValidationIgnore, nil
	}

	lastStoredUpdate := s.lcStore.LastFinalityUpdate()
	if lastStoredUpdate != nil {
		lastUpdateSlot := lastStoredUpdate.FinalizedHeader().Beacon().Slot
		newUpdateSlot := newUpdate.FinalizedHeader().Beacon().Slot
		lastUpdateHasSupermajority := lightClient.UpdateHasSupermajority(lastStoredUpdate.SyncAggregate())
		newUpdateHasSupermajority := lightClient.UpdateHasSupermajority(newUpdate.SyncAggregate())

		// [IGNORE] The finalized_header.beacon.slot is greater than that of all previously forwarded finality_updates,
		// or it matches the highest previously forwarded slot and also has a sync_aggregate indicating supermajority (> 2/3)
		// sync committee participation while the previously forwarded finality_update for that slot did not indicate supermajority
		if newUpdateSlot < lastUpdateSlot {
			log.Debug("Newly received light client finality update ignored. new update is older than stored update")
			return pubsub.ValidationIgnore, nil
		}
		if newUpdateSlot == lastUpdateSlot && (lastUpdateHasSupermajority || !newUpdateHasSupermajority) {
			log.Debug("Newly received light client finality update ignored. no supermajority advantage. Attested slot %d, Signature slot %d", newUpdate.AttestedHeader().Beacon().Slot, newUpdate.SignatureSlot())
			return pubsub.ValidationIgnore, nil
		}
	}

	msg.ValidatorData = newUpdate.Proto()
	log.Debug("New gossiped light client finality update validated. Attested slot %d, Signature slot %d", newUpdate.AttestedHeader().Beacon().Slot, newUpdate.SignatureSlot())
	return pubsub.ValidationAccept, nil
}
