package client

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	prysmTime "github.com/OffchainLabs/prysm/v7/time"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

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

func (v *validator) slotComponentSpanName(component primitives.BP) string {
	cfg := params.BeaconConfig()
	switch component {
	case cfg.AttestationDueBPS:
		return "validator.waitAttestationWindow"
	case cfg.AggregrateDueBPS:
		return "validator.waitAggregateWindow"
	case cfg.SyncMessageDueBPS:
		return "validator.waitSyncMessageWindow"
	case cfg.ContributionDueBPS:
		return "validator.waitContributionWindow"
	case cfg.ProposerReorgCutoffBPS:
		return "validator.waitProposerReorgWindow"
	default:
		return "validator.waitSlotComponent"
	}
}
