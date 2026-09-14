package client

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	prysmTime "github.com/OffchainLabs/prysm/v7/time"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// sinceSlotStartTime returns the elapsed time between the start of the provided slot and now.
func (v *validator) sinceSlotStartTime(slot primitives.Slot) (time.Duration, error) {
	sinceSlotStartTime, err := slots.SinceSlotStart(slot, v.genesisTime, prysmTime.Now())
	if err != nil {
		return 0, fmt.Errorf("since slot start: %w", err)
	}

	return sinceSlotStartTime.Round(time.Millisecond), nil
}

// slotComponentDeadline returns the absolute time corresponding to the provided slot component.
func (v *validator) slotComponentDeadline(slot primitives.Slot, component params.SlotComponent) (time.Time, error) {
	startTime, err := slots.StartTime(v.genesisTime, slot)
	if err != nil {
		return time.Time{}, err
	}
	delay := params.BeaconConfig().SlotComponentDurationAt(component, slot)
	return startTime.Add(delay), nil
}

func (v *validator) waitUntilSlotComponent(ctx context.Context, slot primitives.Slot, component params.SlotComponent) {
	ctx, span := trace.StartSpan(ctx, v.slotComponentSpanName(component))
	defer span.End()

	finalTime, err := v.slotComponentDeadline(slot, component)
	if err != nil {
		log.WithError(err).WithField("slot", slot).Error("Slot overflows, unable to wait for slot component deadline")
		return
	}
	v.waitUntil(ctx, span, finalTime)
}

// waitUntilSlotFraction waits until the given basis-point fraction of the slot, for Prysm-internal timers that are not protocol deadlines.
func (v *validator) waitUntilSlotFraction(ctx context.Context, slot primitives.Slot, bp primitives.BP) {
	ctx, span := trace.StartSpan(ctx, "validator.waitSlotFraction")
	defer span.End()

	startTime, err := slots.StartTime(v.genesisTime, slot)
	if err != nil {
		log.WithError(err).WithField("slot", slot).Error("Slot overflows, unable to wait for slot fraction")
		return
	}
	v.waitUntil(ctx, span, startTime.Add(params.BeaconConfig().SlotFractionAt(bp, slot)))
}

func (v *validator) waitUntil(ctx context.Context, span oteltrace.Span, deadline time.Time) {
	wait := prysmTime.Until(deadline)
	if wait <= 0 {
		return
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		tracing.AnnotateError(span, ctx.Err())
	case <-t.C:
	}
}

// waitForPayloadAvailableOrDeadline blocks until the execution_payload_available
// event for slot is received or the payload attestation deadline is reached,
// whichever comes first.
func (v *validator) waitForPayloadAvailableOrDeadline(ctx context.Context, slot primitives.Slot) {
	ctx, span := trace.StartSpan(ctx, "validator.waitForPayloadAvailableOrDeadline")
	defer span.End()

	deadline, err := v.slotComponentDeadline(slot, params.PayloadAttestationDue)
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

func (v *validator) slotComponentSpanName(component params.SlotComponent) string {
	switch component {
	case params.AttestationDue:
		return "validator.waitAttestationWindow"
	case params.AggregateDue:
		return "validator.waitAggregateWindow"
	case params.SyncMessageDue:
		return "validator.waitSyncMessageWindow"
	case params.ContributionDue:
		return "validator.waitContributionWindow"
	case params.ProposerReorgCutoff:
		return "validator.waitProposerReorgWindow"
	case params.PayloadAttestationDue:
		return "validator.waitPayloadAttestationWindow"
	default:
		return "validator.waitSlotComponent"
	}
}
