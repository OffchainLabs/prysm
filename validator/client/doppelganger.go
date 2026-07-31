package client

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
)

// doppelGangerWaitEpochs is how long a reloaded key sits out before its check:
// attestations can take a full epoch to appear, so the add window must age out.
const doppelGangerWaitEpochs = 2

type doppelGangerPendingKey struct {
	addedEpoch primitives.Epoch
	blocked    bool // duplicate detected: excluded permanently, never re-checked
}

// doppelGangerTracker quarantines keys added after startup until a scoped
// doppelganger check clears them. All methods are concurrency-safe.
type doppelGangerTracker struct {
	mu            sync.RWMutex
	pending       map[pubkey]*doppelGangerPendingKey
	checked       map[pubkey]bool
	lastPollEpoch primitives.Epoch // last epoch a check succeeded; one poll per epoch
	pendingCount  atomic.Int64     // mirrors len(pending) for lock-free empty checks
	inFlight      atomic.Bool      // single-flight guard for the background check
}

// trackReload diffs a key reload against keys already cleared: new keys are
// quarantined from duties as of epoch, removed keys are forgotten so a later
// re-add is checked again.
func (d *doppelGangerTracker) trackReload(currentKeys [][fieldparams.BLSPubkeyLength]byte, epoch primitives.Epoch) {
	current := make(map[pubkey]bool, len(currentKeys))
	for _, pk := range currentKeys {
		current[pk] = true
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pending == nil {
		d.pending = make(map[pubkey]*doppelGangerPendingKey)
	}
	added := 0
	for _, pk := range currentKeys {
		if d.checked[pk] {
			continue
		}
		if _, ok := d.pending[pk]; ok {
			continue
		}
		// Genesis epoch: no prior liveness can exist, so quarantine adds delay
		// without protection (matches Lighthouse).
		if epoch == 0 {
			if d.checked == nil {
				d.checked = make(map[pubkey]bool)
			}
			d.checked[pk] = true
			continue
		}
		d.pending[pk] = &doppelGangerPendingKey{addedEpoch: epoch}
		added++
		log.WithField("pubkey", fmt.Sprintf("%#x", bytesutil.Trunc(pk[:]))).Debug("Key held out of duties pending doppelganger check")
	}
	if added > 0 {
		log.WithFields(logrus.Fields{
			"keyCount":      added,
			"eligibleEpoch": epoch + doppelGangerWaitEpochs + 1,
		}).Info("Reloaded keys held out of duties pending doppelganger check")
	}
	for pk := range d.pending {
		if !current[pk] {
			delete(d.pending, pk)
		}
	}
	for pk := range d.checked {
		if !current[pk] {
			delete(d.checked, pk)
		}
	}
	d.pendingCount.Store(int64(len(d.pending)))
}

// markChecked records keys that passed a doppelganger check.
func (d *doppelGangerTracker) markChecked(keys [][fieldparams.BLSPubkeyLength]byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.checked == nil {
		d.checked = make(map[pubkey]bool, len(keys))
	}
	for _, pk := range keys {
		d.checked[pk] = true
		delete(d.pending, pk)
	}
	d.pendingCount.Store(int64(len(d.pending)))
}

// isPending reports whether a key must be excluded from duties. Lock-free when
// nothing is quarantined, so duty updates pay nothing in steady state.
func (d *doppelGangerTracker) isPending(pk pubkey) bool {
	if d.pendingCount.Load() == 0 {
		return false
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, ok := d.pending[pk]
	return ok
}

// pollDue returns the pending, unblocked keys to check at epoch — at most one
// successful poll per epoch, so a live duplicate is caught before the
// quarantine ends (clearing still waits for clearElapsed).
func (d *doppelGangerTracker) pollDue(epoch primitives.Epoch) [][fieldparams.BLSPubkeyLength]byte {
	if d.pendingCount.Load() == 0 {
		return nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if epoch <= d.lastPollEpoch {
		return nil
	}
	var due [][fieldparams.BLSPubkeyLength]byte
	for pk, p := range d.pending {
		if !p.blocked {
			due = append(due, pk)
		}
	}
	return due
}

// markPolled records a successful check; failures skip it so the next slot retries.
func (d *doppelGangerTracker) markPolled(epoch primitives.Epoch) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if epoch > d.lastPollEpoch {
		d.lastPollEpoch = epoch
	}
}

// clearElapsed clears the given clean keys whose quarantine has elapsed at
// epoch and returns them; keys still inside the wait stay pending. Strictly
// beyond the wait: the clearing poll must postdate the BN's cannot-determine
// band, so an unevaluated default-clean can never clear a key.
func (d *doppelGangerTracker) clearElapsed(keys [][fieldparams.BLSPubkeyLength]byte, epoch primitives.Epoch) [][fieldparams.BLSPubkeyLength]byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.checked == nil {
		d.checked = make(map[pubkey]bool, len(keys))
	}
	var cleared [][fieldparams.BLSPubkeyLength]byte
	for _, pk := range keys {
		p, ok := d.pending[pk]
		if !ok || p.blocked || p.addedEpoch+doppelGangerWaitEpochs >= epoch {
			continue
		}
		d.checked[pk] = true
		delete(d.pending, pk)
		cleared = append(cleared, pk)
	}
	d.pendingCount.Store(int64(len(d.pending)))
	return cleared
}

// block permanently excludes keys with a detected duplicate.
func (d *doppelGangerTracker) block(keys [][fieldparams.BLSPubkeyLength]byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, pk := range keys {
		if p, ok := d.pending[pk]; ok {
			p.blocked = true
		}
		log.WithField("pubkey", fmt.Sprintf("%#x", bytesutil.Trunc(pk[:]))).Error(
			"Doppelganger detected for reloaded key; key remains excluded from duties")
	}
}

// trackReloadedKeysForDoppelGanger quarantines never-checked keys from a reload.
func (v *validator) trackReloadedKeysForDoppelGanger(currentKeys [][fieldparams.BLSPubkeyLength]byte) {
	if !features.Get().EnableDoppelGanger {
		return
	}
	v.doppelGanger.trackReload(currentKeys, slots.ToEpoch(slots.CurrentSlot(v.genesisTime)))
}

// markDoppelGangerChecked records keys that passed a doppelganger check.
func (v *validator) markDoppelGangerChecked(keys [][fieldparams.BLSPubkeyLength]byte) {
	v.doppelGanger.markChecked(keys)
}

// isDoppelGangerPending reports whether a key must be excluded from duties.
func (v *validator) isDoppelGangerPending(pk pubkey) bool {
	return v.doppelGanger.isPending(pk)
}

// MaybeCheckDoppelGanger polls quarantined keys once per epoch in the
// background: a live duplicate is blocked as soon as it is seen, while clean
// keys are cleared only after their quarantine elapses.
func (v *validator) MaybeCheckDoppelGanger(ctx context.Context, slot primitives.Slot) {
	if !features.Get().EnableDoppelGanger {
		return
	}
	// Poll late in the epoch (Lighthouse's 3/4 offset): the beacon node has seen
	// this epoch's activity, and its head epoch cannot trail the poll epoch.
	if slots.SinceEpochStarts(slot) < params.BeaconConfig().SlotsPerEpoch*3/4 {
		return
	}
	epoch := slots.ToEpoch(slot)
	due := v.doppelGanger.pollDue(epoch)
	if len(due) == 0 || !v.doppelGanger.inFlight.CompareAndSwap(false, true) {
		return
	}
	checkCtx, cancel := context.WithDeadline(ctx, v.SlotDeadline(slot))
	go func() {
		defer func() {
			cancel()
			v.doppelGanger.inFlight.Store(false)
		}()
		v.checkReloadedKeys(checkCtx, due, epoch)
	}()
}

// checkReloadedKeys runs one scoped doppelganger check: keys with a live
// duplicate are blocked permanently; clean keys whose quarantine elapsed are
// cleared to rejoin duties, the rest stay quarantined for the next poll.
func (v *validator) checkReloadedKeys(ctx context.Context, due [][fieldparams.BLSPubkeyLength]byte, epoch primitives.Epoch) {
	resp, err := v.checkDoppelGangerForKeys(ctx, due)
	if err != nil {
		log.WithError(err).Debug("Could not run doppelganger check for reloaded keys; will retry")
		return
	}
	duplicates := duplicateKeysFromResponse(resp.Responses)
	if len(duplicates) > 0 {
		v.doppelGanger.block(duplicates)
	}
	// Only keys the beacon node explicitly reported clean may clear; a key absent
	// from the response stays quarantined for the next poll (fail-closed).
	if cleared := v.doppelGanger.clearElapsed(cleanKeysFromResponse(resp.Responses), epoch); len(cleared) > 0 {
		log.WithField("keyCount", len(cleared)).Info(
			"Reloaded keys passed doppelganger check and will receive duties at the next update")
	}
	v.doppelGanger.markPolled(epoch)
}

// cleanKeysFromResponse extracts the keys the beacon node explicitly reported
// as having no live duplicate.
func cleanKeysFromResponse(responses []*ethpb.DoppelGangerResponse_ValidatorResponse) [][fieldparams.BLSPubkeyLength]byte {
	var clean [][fieldparams.BLSPubkeyLength]byte
	for _, r := range responses {
		if !r.DuplicateExists {
			clean = append(clean, bytesutil.ToBytes48(r.PublicKey))
		}
	}
	return clean
}

// duplicateKeysFromResponse extracts the keys the beacon node flagged as having
// a live duplicate.
func duplicateKeysFromResponse(responses []*ethpb.DoppelGangerResponse_ValidatorResponse) [][fieldparams.BLSPubkeyLength]byte {
	var dups [][fieldparams.BLSPubkeyLength]byte
	for _, r := range responses {
		if r.DuplicateExists {
			dups = append(dups, bytesutil.ToBytes48(r.PublicKey))
		}
	}
	return dups
}
