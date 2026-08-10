package backfill

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
)

// easCustody is the narrow subset of p2p.CustodyManager that backfill uses to advertise
// backfilled history via the earliest available slot in Status v2 messages.
type easCustody interface {
	EarliestAvailableSlot(ctx context.Context) (primitives.Slot, error)
	CustodyGroupCount(ctx context.Context) (uint64, error)
	UpdateEarliestAvailableSlot(earliestAvailableSlot primitives.Slot) error
}

// configureEASUpdates determines whether backfill may progressively lower the earliest available
// slot as batches are imported, and snapshots the custody group count that the decision is based on.
func (s *Service) configureEASUpdates(ctx context.Context) {
	if !params.FuluEnabled() || s.custody == nil {
		return
	}
	cgc, err := s.custody.CustodyGroupCount(ctx)
	if err != nil {
		log.WithError(err).Error("Could not read custody group count, backfill will not update the earliest available slot")
		return
	}
	eas, err := s.custody.EarliestAvailableSlot(ctx)
	if err != nil {
		log.WithError(err).Error("Could not read the earliest available slot, backfill will not update it")
		return
	}
	origin := primitives.Slot(s.store.status().OriginSlot)
	// At checkpoint sync startup the earliest available slot is initialized to the start of the
	// checkpoint epoch, at or shortly above the origin block slot. A value above that indicates it
	// was raised, eg. by a custody group count increase beyond the sampling requirement, because
	// older history no longer has complete column coverage for the advertised custody scope.
	// Backfill only establishes coverage for slots at or below the origin, so in that case the
	// higher value must be preserved and progressive updates are disabled for this run.
	if slots.ToEpoch(eas) > slots.ToEpoch(origin)+1 {
		log.WithFields(logrus.Fields{
			"earliestAvailableSlot": eas,
			"originSlot":            origin,
		}).Warn("Earliest available slot is above the backfill origin, backfill will not lower it")
		return
	}
	s.easAllowed = true
	s.easCustodyGroupCount = cgc
}

// updateEarliestAvailableSlot lowers the persisted and advertised earliest available slot to
// lowSlot, the lowest slot of a backfill batch that has already been durably imported. lowSlot
// must come from the importer-returned BackfillStatus rather than the batch boundary, because
// the lowest slots of a batch may be skipped.
// The database and p2p updates are attempted independently and failures are only logged: the
// batch is already committed, so a failed update must not make it look unimported or stop the
// import loop. A subsequent batch retries naturally by publishing its own, lower slot.
func (s *Service) updateEarliestAvailableSlot(ctx context.Context, lowSlot primitives.Slot) {
	if !params.FuluEnabled() || !s.easAllowed || s.custody == nil {
		return
	}
	cgc, err := s.custody.CustodyGroupCount(ctx)
	if err != nil {
		log.WithError(err).Error("Could not read custody group count, skipping earliest available slot update")
		return
	}
	// A custody group count increase invalidates column coverage for history imported with the
	// smaller count and may have raised the earliest available slot to the head slot. Keep the
	// higher value; re-establishing column coverage for a larger custody scope is out of scope
	// for backfill (see issue #15982). This check is best-effort: an increase landing between it
	// and the updates below can still race, as with the equivalent pruner update.
	if cgc != s.easCustodyGroupCount {
		if !s.easCgcWarned {
			s.easCgcWarned = true
			log.WithFields(logrus.Fields{
				"startCustodyGroupCount":   s.easCustodyGroupCount,
				"currentCustodyGroupCount": cgc,
			}).Warn("Custody group count changed during backfill, the earliest available slot will not be lowered")
		}
		return
	}
	current, err := s.custody.EarliestAvailableSlot(ctx)
	if err != nil {
		log.WithError(err).Error("Could not read the earliest available slot, skipping update")
		return
	}
	// Only ever move the earliest available slot toward older slots.
	if lowSlot >= current {
		return
	}
	if err := s.store.updateEarliestAvailableSlot(ctx, lowSlot); err != nil {
		log.WithError(err).WithField("earliestAvailableSlot", lowSlot).Error("Could not persist the earliest available slot after backfill import")
	}
	if err := s.custody.UpdateEarliestAvailableSlot(lowSlot); err != nil {
		log.WithError(err).WithField("earliestAvailableSlot", lowSlot).Error("Could not advertise the earliest available slot after backfill import")
	}
}
