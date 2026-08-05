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
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/emptypb"
)

// doppelGangerWaitEpochs is how long a reloaded key sits out before clearing.
// Must cover the BN CheckDoppelGanger 2-epoch determination band (status.go).
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
	lastWarnEpoch primitives.Epoch // rate-limits the failure warning to one per epoch
	pendingCount  atomic.Int64     // mirrors len(pending) for lock-free empty checks
	inFlight      atomic.Bool      // single-flight guard for the background check
}

// trackReload quarantines never-checked keys as of epoch and forgets removed
// keys so a later re-add is checked again.
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

// pollDue returns the pending, unblocked keys to check at epoch, at most once
// per epoch; duplicates are blocked at any poll, clearing waits for clearElapsed.
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

// shouldWarnFailure reports whether a check failure at epoch was not yet warned.
func (d *doppelGangerTracker) shouldWarnFailure(epoch primitives.Epoch) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if epoch <= d.lastWarnEpoch {
		return false
	}
	d.lastWarnEpoch = epoch
	return true
}

// clearElapsed clears and returns the given clean keys strictly past their
// quarantine at epoch; the rest stay pending. epoch must not exceed the BN head.
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

// block permanently excludes still-tracked keys with a detected duplicate.
func (d *doppelGangerTracker) block(keys [][fieldparams.BLSPubkeyLength]byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, pk := range keys {
		p, ok := d.pending[pk]
		if !ok {
			continue
		}
		p.blocked = true
		log.WithField("pubkey", fmt.Sprintf("%#x", bytesutil.Trunc(pk[:]))).Error(
			"Doppelganger detected for reloaded key; key remains excluded from duties")
	}
}

// trackReloadedKeysForDoppelGanger quarantines never-checked keys from a reload.
func (v *validator) trackReloadedKeysForDoppelGanger(currentKeys [][fieldparams.BLSPubkeyLength]byte) {
	if !features.Get().EnableDoppelGanger {
		return
	}
	v.doppelGanger.trackReload(currentKeys, slots.EpochsSinceGenesis(v.genesisTime))
}

// markDoppelGangerChecked records keys that passed a doppelganger check.
func (v *validator) markDoppelGangerChecked(keys [][fieldparams.BLSPubkeyLength]byte) {
	v.doppelGanger.markChecked(keys)
}

// isDoppelGangerPending reports whether a key must be excluded from duties.
func (v *validator) isDoppelGangerPending(pk pubkey) bool {
	return v.doppelGanger.isPending(pk)
}

// MaybeCheckDoppelGanger polls quarantined keys in the background: duplicates
// are blocked as soon as seen, clean keys clear only after the quarantine.
func (v *validator) MaybeCheckDoppelGanger(ctx context.Context, slot primitives.Slot) {
	if !features.Get().EnableDoppelGanger {
		return
	}
	// Poll late in the epoch (Lighthouse's 3/4 offset) so the beacon node has
	// seen most of this epoch's activity.
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

// checkReloadedKeys runs one scoped check: duplicates are blocked permanently,
// elapsed clean keys are cleared to rejoin duties, the rest stay quarantined.
func (v *validator) checkReloadedKeys(ctx context.Context, due [][fieldparams.BLSPubkeyLength]byte, epoch primitives.Epoch) {
	resp, err := v.checkDoppelGangerForKeys(ctx, due)
	if err != nil {
		if v.doppelGanger.shouldWarnFailure(epoch) {
			log.WithError(err).Warn("Doppelganger check for reloaded keys failed; keys stay out of duties until it succeeds")
		} else {
			log.WithError(err).Debug("Could not run doppelganger check for reloaded keys; will retry")
		}
		return
	}
	// Empty response is definitive: none of the keys are known to the beacon
	// node yet. Count the poll and keep them quarantined (fail-closed).
	if len(resp.Responses) == 0 {
		log.Debug("Reloaded keys not known to beacon node yet; doppelganger quarantine continues")
		v.doppelGanger.markPolled(epoch)
		return
	}
	clean, duplicates := splitByDuplicate(resp.Responses)
	if len(duplicates) > 0 {
		v.doppelGanger.block(duplicates)
	}
	// Keys absent from the response stay quarantined for the next poll (fail-closed).
	if len(clean) > 0 {
		clearEpoch, err := v.clearingEpoch(ctx, epoch)
		if err != nil {
			// Leave the poll unconsumed so the next slot retries within this epoch.
			if v.doppelGanger.shouldWarnFailure(epoch) {
				log.WithError(err).Warn("Could not get chain head; doppelganger clearing deferred")
			} else {
				log.WithError(err).Debug("Could not get chain head; deferring doppelganger clearing")
			}
			return
		}
		if cleared := v.doppelGanger.clearElapsed(clean, clearEpoch); len(cleared) > 0 {
			log.WithField("keyCount", len(cleared)).Info(
				"Reloaded keys passed doppelganger check and will receive duties at the next update")
		}
	}
	v.doppelGanger.markPolled(epoch)
}

// clearingEpoch caps the wall-clock epoch at the beacon node's head epoch: a
// lagging head returns unevaluated defaults, which must not end a quarantine.
func (v *validator) clearingEpoch(ctx context.Context, epoch primitives.Epoch) (primitives.Epoch, error) {
	head, err := v.chainClient.ChainHead(ctx, &emptypb.Empty{})
	if err != nil {
		return 0, errors.Wrap(err, "chain head unavailable")
	}
	if head == nil {
		return 0, errors.New("nil chain head from beacon node")
	}
	return min(epoch, head.HeadEpoch), nil
}

// splitByDuplicate partitions responses into keys the beacon node reported
// clean and keys with a live duplicate.
func splitByDuplicate(responses []*ethpb.DoppelGangerResponse_ValidatorResponse) (clean, dups [][fieldparams.BLSPubkeyLength]byte) {
	for _, r := range responses {
		if r.DuplicateExists {
			dups = append(dups, bytesutil.ToBytes48(r.PublicKey))
		} else {
			clean = append(clean, bytesutil.ToBytes48(r.PublicKey))
		}
	}
	return clean, dups
}
