package client

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	prysmTime "github.com/OffchainLabs/prysm/v7/time"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// proposerTimingGameSafetyBudget is the time reserved at the end of the
// timing-game window for the builder getHeader round-trip (~1s) plus block
// propagation, so that a delayed proposal still lands before the attestation
// deadline and the slot is not missed.
const proposerTimingGameSafetyBudget = 1500 * time.Millisecond

// slotComponentDeadline returns the absolute time corresponding to the provided slot component.
func (v *validator) slotComponentDeadline(slot primitives.Slot, component primitives.BP) (time.Time, error) {
	startTime, err := slots.StartTime(v.genesisTime, slot)
	if err != nil {
		return time.Time{}, err
	}
	delay := params.BeaconConfig().SlotComponentDuration(component)
	return startTime.Add(delay), nil
}

func (v *validator) waitUntilSlotComponent(ctx context.Context, slot primitives.Slot, component primitives.BP) {
	ctx, span := trace.StartSpan(ctx, v.slotComponentSpanName(component))
	defer span.End()

	finalTime, err := v.slotComponentDeadline(slot, component)
	if err != nil {
		log.WithError(err).WithField("slot", slot).Error("Slot overflows, unable to wait for slot component deadline")
		return
	}
	wait := prysmTime.Until(finalTime)
	if wait <= 0 {
		return
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		tracing.AnnotateError(span, ctx.Err())
		return
	case <-t.C:
		return
	}
}

// waitForPayloadAvailableOrDeadline blocks until the execution_payload_available
// event for slot is received or the payload attestation deadline is reached,
// whichever comes first.
func (v *validator) waitForPayloadAvailableOrDeadline(ctx context.Context, slot primitives.Slot) {
	ctx, span := trace.StartSpan(ctx, "validator.waitForPayloadAvailableOrDeadline")
	defer span.End()

	deadline, err := v.slotComponentDeadline(slot, params.BeaconConfig().PayloadAttestationDueBPS)
	if err != nil {
		log.WithError(err).WithField("slot", slot).Error("Slot overflows, unable to wait for payload attestation deadline")
		return
	}
	available := v.payloadAvailability.waiter(slot)
	wait := prysmTime.Until(deadline)
	if wait <= 0 {
		return
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		tracing.AnnotateError(span, ctx.Err())
	case <-available:
	case <-t.C:
	}
}

// waitUntilSlotOffset blocks until the given absolute offset into the slot has
// elapsed (slot start + offset), or the context is cancelled. It is used by the
// proposer timing-game path to release the block proposal request later in the slot.
func (v *validator) waitUntilSlotOffset(ctx context.Context, slot primitives.Slot, offset time.Duration) {
	ctx, span := trace.StartSpan(ctx, "validator.waitProposerTimingGame")
	defer span.End()

	startTime, err := slots.StartTime(v.genesisTime, slot)
	if err != nil {
		log.WithError(err).WithField("slot", slot).Error("Slot overflows, unable to wait for proposer timing-game delay")
		return
	}
	wait := prysmTime.Until(startTime.Add(offset))
	if wait <= 0 {
		return
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		tracing.AnnotateError(span, ctx.Err())
		return
	case <-t.C:
		return
	}
}

// proposalReleaseDelay returns the proposer timing-game delay to apply before
// releasing the block proposal request for slot. The configured delay is clamped
// so that the request, the builder getHeader round-trip and block propagation
// still complete before the attestation deadline (never missing the slot because
// of the delay). It logs a warning when the value is clamped, or when it is set
// beyond the honest-reorg-safe threshold.
func (v *validator) proposalReleaseDelay(slot primitives.Slot) time.Duration {
	configured := features.Get().ProposerTimingGameDelay
	if configured <= 0 {
		return 0
	}
	cfg := params.BeaconConfig()
	// The attestation deadline is the point by which attesters vote on the head;
	// the block must be requested early enough to fetch the builder bid and
	// propagate before it. Use the fork-appropriate deadline.
	dueBPS := cfg.AttestationDueBPS
	if slots.ToEpoch(slot) >= cfg.GloasForkEpoch {
		dueBPS = cfg.AttestationDueBPSGloas
	}
	maxDelay := cfg.SlotComponentDuration(dueBPS) - proposerTimingGameSafetyBudget
	if maxDelay < 0 {
		maxDelay = 0
	}
	if configured > maxDelay {
		log.WithField("configuredDelay", configured).WithField("clampedDelay", maxDelay).
			Warn("Proposer timing-game delay exceeds the safe maximum before the attestation deadline; clamping to avoid a missed slot")
		return maxDelay
	}
	if reorgCutoff := cfg.SlotComponentDuration(cfg.ProposerReorgCutoffBPS); configured > reorgCutoff {
		log.WithField("configuredDelay", configured).WithField("reorgCutoff", reorgCutoff).
			Warn("Proposer timing-game delay is beyond the honest-reorg-safe threshold; the block may be orphaned")
	}
	return configured
}

func (v *validator) slotComponentSpanName(component primitives.BP) string {
	cfg := params.BeaconConfig()
	switch component {
	case cfg.AttestationDueBPS:
		return "validator.waitAttestationWindow"
	case cfg.AttestationDueBPSGloas:
		return "validator.waitAttestationWindow"
	case cfg.AggregateDueBPS:
		return "validator.waitAggregateWindow"
	case cfg.AggregateDueBPSGloas:
		return "validator.waitAggregateWindow"
	case cfg.SyncMessageDueBPS:
		return "validator.waitSyncMessageWindow"
	case cfg.SyncMessageDueBPSGloas:
		return "validator.waitSyncMessageWindow"
	case cfg.ContributionDueBPS:
		return "validator.waitContributionWindow"
	case cfg.ContributionDueBPSGloas:
		return "validator.waitContributionWindow"
	case cfg.ProposerReorgCutoffBPS:
		return "validator.waitProposerReorgWindow"
	case cfg.PayloadAttestationDueBPS:
		return "validator.waitPayloadAttestationWindow"
	default:
		return "validator.waitSlotComponent"
	}
}
