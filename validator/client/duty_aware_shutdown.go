package client

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
)

const (
	// dutyAwareShutdownRestartBudget is the time a restarted validator client needs to be ready
	// to perform its duties. The validator client stops only if at least this much time
	// is left before the start of the next slot.
	dutyAwareShutdownRestartBudget = 3 * time.Second

	// dutyAwareShutdownMaxWait is the maximum time to wait for a shutdown window.
	// It is lower than the 10 seconds Docker waits by default before killing a stopping container.
	dutyAwareShutdownMaxWait = 8 * time.Second

	msgNoDutyInProgress    = "No duty in progress, shutting down immediately"
	msgNoShutdownWindow    = "No shutdown window found without missing a rewarded duty, shutting down anyway"
	msgShutdownInterrupted = "Duty-aware shutdown interrupted, shutting down immediately"
)

// dutyAwareShutdownTracker records, for each slot, when the duties earning rewards
// (attestation, sync committee message and block proposal) are done.
// It is used to find the moment in the slot where the validator client
// can be stopped and restarted without missing any rewarded duty.
type dutyAwareShutdownTracker struct {
	mu sync.Mutex

	active          bool
	genesis         time.Time
	hasRewardedDuty func(primitives.Slot) bool
	done            map[primitives.Slot]chan struct{}
	restartBudget   time.Duration
	maxWait         time.Duration
}

func newDutyAwareShutdownTracker() *dutyAwareShutdownTracker {
	return &dutyAwareShutdownTracker{
		done:          make(map[primitives.Slot]chan struct{}),
		restartBudget: dutyAwareShutdownRestartBudget,
		maxWait:       dutyAwareShutdownMaxWait,
	}
}

// start records that the runner performs duties. `hasRewardedDuty` reports whether any
// validator may have a rewarded duty at a slot, and must return true when unknown.
func (t *dutyAwareShutdownTracker) start(genesis time.Time, hasRewardedDuty func(primitives.Slot) bool) {
	if t == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.active = true
	t.genesis = genesis
	t.hasRewardedDuty = hasRewardedDuty
}

// stop records that the runner no longer performs duties, and wakes up any waiter.
func (t *dutyAwareShutdownTracker) stop() {
	if t == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.active = false
	for slot, ch := range t.done {
		closeIfOpen(ch)
		delete(t.done, slot)
	}
}

// markDone records that all rewarded duties of the slot are done.
func (t *dutyAwareShutdownTracker) markDone(slot primitives.Slot) {
	if t == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	closeIfOpen(t.doneChanLocked(slot))

	// Prune old slots.
	for s := range t.done {
		if s+1 < slot {
			delete(t.done, s)
		}
	}
}

// doneChan returns the channel of the slot.
func (t *dutyAwareShutdownTracker) doneChan(slot primitives.Slot) <-chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.doneChanLocked(slot)
}

// doneChanLocked returns the channel of the slot.
func (t *dutyAwareShutdownTracker) doneChanLocked(slot primitives.Slot) chan struct{} {
	ch, ok := t.done[slot]
	if !ok {
		ch = make(chan struct{})
		t.done[slot] = ch
	}

	return ch
}

func (t *dutyAwareShutdownTracker) state() (bool, time.Time, func(primitives.Slot) bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.active, t.genesis, t.hasRewardedDuty
}

// wait blocks until the validator client can be stopped without missing any rewarded duty:
// all rewarded duties of the current slot are done, and either enough time is left before
// the start of the next slot to restart, or the next slot has no rewarded duty.
// It returns immediately if no duty is being performed, and after `maxWait` if no such
// window was found.
func (t *dutyAwareShutdownTracker) wait(ctx context.Context) {
	if t == nil {
		return
	}

	giveUp := time.After(t.maxWait)

	// The slot for which the wait for rewarded duties was already logged, to log it only once.
	var (
		waitLogged     bool
		waitLoggedSlot primitives.Slot
	)

	for {
		active, genesis, hasRewardedDuty := t.state()
		if !active {
			log.Debug(msgNoDutyInProgress)
			return
		}

		slot := slots.CurrentSlot(genesis)
		nextSlotStart := slots.UnsafeStartTime(genesis, slot+1)

		// Wait for the rewarded duties of the current slot to be done.
		slotHasRewardedDuty := hasRewardedDuty(slot)
		if slotHasRewardedDuty {
			done := t.doneChan(slot)
			if !waitLogged || waitLoggedSlot != slot {
				select {
				case <-done:
				default:
					log.WithField("slot", slot).Info("Waiting for the rewarded duties of the slot to be done before shutting down. Interrupt again to shut down immediately")
				}
			}

			select {
			case <-done:
			case <-giveUp:
				log.WithField("maxWait", t.maxWait).Warning(msgNoShutdownWindow)
				return
			case <-ctx.Done():
				log.Info(msgShutdownInterrupted)
				return
			}

			if active, _, _ := t.state(); !active {
				log.Debug(msgNoDutyInProgress)
				return
			}
		}

		// A restart started now must be ready before the start of the next slot,
		// unless the next slot has no rewarded duty.
		now := time.Now()
		if !now.Before(nextSlotStart) {
			// The duties ended after the end of the slot: check the new slot.
			continue
		}

		timeLeft := nextSlotStart.Sub(now).Round(time.Millisecond)
		fields := logrus.Fields{
			"slot":                   slot,
			"timeLeftBeforeNextSlot": timeLeft,
		}

		if timeLeft >= t.restartBudget {
			if slotHasRewardedDuty {
				log.WithFields(fields).Debug("Rewarded duties of the slot done, shutting down")
				return
			}

			log.WithFields(fields).Debug("No rewarded duty in the slot, shutting down")
			return
		}

		if !hasRewardedDuty(slot + 1) {
			log.WithFields(fields).Debug("No rewarded duty in the next slot, shutting down")
			return
		}

		log.WithFields(fields).Info("Too late in the slot to restart before the next one, waiting for the rewarded duties of the next slot to be done before shutting down. Interrupt again to shut down immediately")
		waitLogged, waitLoggedSlot = true, slot+1
		if !t.sleep(ctx, giveUp, time.Until(nextSlotStart)) {
			return
		}
	}
}

// sleep waits for the duration and returns true, or returns false if the
// give up timer fires or the context is done.
func (t *dutyAwareShutdownTracker) sleep(ctx context.Context, giveUp <-chan time.Time, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-giveUp:
		log.WithField("maxWait", t.maxWait).Warning(msgNoShutdownWindow)
		return false
	case <-ctx.Done():
		log.Info(msgShutdownInterrupted)
		return false
	}
}

func closeIfOpen(ch chan struct{}) {
	select {
	case <-ch:
		// Nothing is ever sent on this channel, so a receive succeeds right away
		// only if the channel is closed (it then returns the zero value). Nothing to do.
	default:
		// A receive would block: the channel is still open.
		close(ch)
	}
}

// hasRewardedDutyAt returns true if any validator may have a rewarded duty (attestation,
// sync committee message or block proposal) at the slot. It returns true if unknown,
// i.e. if the duties are not initialized or are not for the epoch of the slot.
func (v *validator) hasRewardedDutyAt(slot primitives.Slot) bool {
	snap := v.duties.snapshot()
	if !snap.isInitialized() || slots.ToEpoch(slot) != snap.epoch() {
		return true
	}

	for pk, duty := range snap.currentDuties() {
		// Quarantined keys get no roles at all.
		if duty == nil || v.isDoppelGangerPending(pk) {
			continue
		}

		if duty.AttesterSlot == slot || slices.Contains(duty.ProposerSlots, slot) {
			return true
		}

		// At the last slot of the epoch, sync committee messages are produced by the
		// sync committee of the next epoch.
		inSyncCommittee := duty.IsSyncCommittee
		if slots.IsEpochEnd(slot) {
			inSyncCommittee = snap.isNextSyncCommittee(duty.ValidatorIndex)
		}

		if inSyncCommittee {
			return true
		}
	}

	return false
}
