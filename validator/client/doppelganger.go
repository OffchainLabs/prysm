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
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	dbCommon "github.com/OffchainLabs/prysm/v7/validator/db/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/emptypb"
)

// doppelGangerWaitEpochs is how long a reloaded key sits out before clearing: it must cover
// the 2-epoch band in CheckDoppelGanger (beacon-chain/rpc/prysm/v1alpha1/validator/status.go).
const doppelGangerWaitEpochs = 2

type doppelGangerPendingKey struct {
	addedEpoch primitives.Epoch
	blocked    bool // duplicate detected: excluded permanently, never re-checked
}

// doppelGangerTracker quarantines keys added after startup until a scoped
// doppelganger check clears them. All methods are concurrency-safe.
type doppelGangerTracker struct {
	startupDone  bool        // startup check completed once; later beginStartup calls are runner restarts
	inFlight     atomic.Bool // single-flight guard for the background check
	mu           sync.RWMutex
	pendingCount atomic.Int64     // mirrors len(pending) for lock-free empty checks
	lastWarnMark primitives.Epoch // last warned failure epoch + 1, rate-limiting to one per epoch
	lastPollMark primitives.Epoch // last successful poll epoch + 1; zero means never polled
	bootSet      map[pubkey]bool  // wallet snapshot at boot; non-nil only until the startup check completes
	checked      map[pubkey]bool
	pending      map[pubkey]*doppelGangerPendingKey
}

// beginStartup snapshots the wallet at boot: these keys belong to the one-shot
// startup check; anything arriving later is a reload. Retries keep the first snapshot.
func (d *doppelGangerTracker) beginStartup(keys []pubkey, epoch primitives.Epoch) {
	if !d.trySnapshotBoot(keys) {
		// Runner restart: keys imported while it was down missed their reload
		// event, so quarantine them rather than re-snapshotting the wallet.
		d.trackReload(keys, epoch)
	}
}

// trySnapshotBoot claims the boot snapshot; false once startup has completed.
func (d *doppelGangerTracker) trySnapshotBoot(keys []pubkey) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.startupDone {
		return false
	}
	if d.bootSet == nil {
		d.bootSet = make(map[pubkey]bool, len(keys))
		for _, pk := range keys {
			d.bootSet[pk] = true
		}
	}
	return true
}

// completeStartup closes the boot phase after a successful vetting; a failed
// attempt leaves it open, so initialize retries vet the identical boot set.
func (d *doppelGangerTracker) completeStartup() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.bootSet = nil
	d.startupDone = true
}

// partitionStartup classifies keys for the startup check: checkable boot keys
// and lateAdds that appeared mid-initialization. Pending keys are omitted, and
// with no active snapshot every key is checkable (reruns re-vet checked keys).
func (d *doppelGangerTracker) partitionStartup(keys []pubkey) (checkable, lateAdds []pubkey) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	checkable = make([]pubkey, 0, len(keys))
	for _, pk := range keys {
		switch {
		case d.pending[pk] != nil:
		case d.bootSet == nil || d.bootSet[pk]:
			checkable = append(checkable, pk)
		default:
			lateAdds = append(lateAdds, pk)
		}
	}
	return checkable, lateAdds
}

// pendingAddedEpoch returns the quarantine start epoch for a pending key.
func (d *doppelGangerTracker) pendingAddedEpoch(pk pubkey) (primitives.Epoch, bool) {
	if d.pendingCount.Load() == 0 {
		return 0, false
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	p, ok := d.pending[pk]
	if !ok {
		return 0, false
	}
	return p.addedEpoch, true
}

// trackReload quarantines never-checked keys as of epoch and forgets removed
// keys so a later re-add is checked again. Logs outside the tracker lock.
func (d *doppelGangerTracker) trackReload(currentKeys []pubkey, epoch primitives.Epoch) {
	addedKeys := d.rebuildFromReload(currentKeys, epoch)
	for _, pk := range addedKeys {
		log.WithField("pubkey", fmt.Sprintf("%#x", bytesutil.Trunc(pk[:]))).Debug("Key held out of duties pending doppelganger check")
	}
	if len(addedKeys) > 0 {
		log.WithFields(logrus.Fields{
			"keyCount":      len(addedKeys),
			"eligibleEpoch": epoch + doppelGangerWaitEpochs + 1,
		}).Info("Reloaded keys held out of duties pending doppelganger check")
	}
}

// rebuildFromReload rebuilds both maps in one pass: keys absent from currentKeys
// drop out, surviving pending entries keep their state, unseen keys quarantine.
func (d *doppelGangerTracker) rebuildFromReload(currentKeys []pubkey, epoch primitives.Epoch) (addedKeys []pubkey) {
	d.mu.Lock()
	defer d.mu.Unlock()
	newChecked := make(map[pubkey]bool, len(d.checked))
	newPending := make(map[pubkey]*doppelGangerPendingKey, len(d.pending))
	for _, pk := range currentKeys {
		if d.checked[pk] {
			newChecked[pk] = true
			continue
		}
		if p, ok := d.pending[pk]; ok {
			newPending[pk] = p
			continue
		}
		if _, ok := newPending[pk]; ok { // duplicate input key
			continue
		}
		if d.bootSet[pk] { // boot keys are the startup check's job
			continue
		}
		newPending[pk] = &doppelGangerPendingKey{addedEpoch: epoch}
		addedKeys = append(addedKeys, pk)
	}
	d.checked = newChecked
	d.pending = newPending
	d.pendingCount.Store(int64(len(newPending)))
	return addedKeys
}

// vetStartup settles a startup check in one pass: evaluated keys become checked
// (a blocked verdict is never overwritten), the rest are quarantined unless
// already tracked. Returns how many keys were newly quarantined.
func (d *doppelGangerTracker) vetStartup(keys []pubkey, evaluated map[pubkey]bool, epoch primitives.Epoch) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.checked == nil {
		d.checked = make(map[pubkey]bool, len(keys))
	}
	if d.pending == nil {
		d.pending = make(map[pubkey]*doppelGangerPendingKey)
	}
	held := 0
	for _, pk := range keys {
		if evaluated[pk] {
			// A key flagged as a duplicate stays excluded even if a later check
			// reports it clean; removing the key is the only way to retry it.
			if p, ok := d.pending[pk]; ok && p.blocked {
				continue
			}
			d.checked[pk] = true
			delete(d.pending, pk)
			continue
		}
		if d.checked[pk] {
			continue
		}
		if _, ok := d.pending[pk]; ok {
			continue
		}
		d.pending[pk] = &doppelGangerPendingKey{addedEpoch: epoch}
		held++
	}
	d.pendingCount.Store(int64(len(d.pending)))
	return held
}

// snapshotBootKeysForDoppelGanger records the wallet at boot: the startup check
// owns exactly these keys, and anything appearing later is treated as a reload.
func (v *validator) snapshotBootKeysForDoppelGanger(ctx context.Context) error {
	if !features.Get().EnableDoppelGanger {
		return nil
	}
	keys, err := v.km.FetchValidatingPublicKeys(ctx)
	if err != nil {
		return errors.Wrap(err, "could not fetch validating keys for doppelganger startup snapshot")
	}
	v.doppelGanger.beginStartup(keys, slots.EpochsSinceGenesis(v.genesisTime))
	return nil
}

// CheckDoppelGangerAtStartup is the one-shot startup gate: it vets every key the
// per-epoch polls do not own, and refuses to start on a live duplicate.
func (v *validator) CheckDoppelGangerAtStartup(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "validator.CheckDoppelganger")
	defer span.End()

	if !features.Get().EnableDoppelGanger {
		return nil
	}
	if err := v.runStartupCheck(ctx); err != nil {
		return err
	}
	v.doppelGanger.completeStartup()
	return nil
}

// runStartupCheck quarantines keys that arrived mid-initialization, then vets
// the boot-snapshot keys against the beacon node.
func (v *validator) runStartupCheck(ctx context.Context) error {
	wallet, err := v.km.FetchValidatingPublicKeys(ctx)
	if err != nil {
		return errors.Wrap(err, "could not fetch validating keys for doppelganger check")
	}
	pubkeys, lateAdds := v.doppelGanger.partitionStartup(wallet)
	if held := v.doppelGanger.vetStartup(lateAdds, nil, slots.EpochsSinceGenesis(v.genesisTime)); held > 0 {
		log.WithField("keyCount", held).Info("Keys imported during startup are held out of duties pending doppelganger check")
	}
	log.WithField("keyCount", len(pubkeys)).Debug("Running doppelganger check")
	if len(pubkeys) == 0 {
		return nil
	}
	resp, err := v.checkDoppelGangerForKeys(ctx, pubkeys)
	if err != nil {
		return err
	}
	if err := buildDuplicateError(resp.Responses); err != nil {
		return err
	}
	v.vetStartupKeys(pubkeys, resp.Responses)
	return nil
}

// checkDoppelGangerForKeys queries the beacon node's doppelganger check for the
// given keys, using each key's latest local attestation record as its watermark.
func (v *validator) checkDoppelGangerForKeys(ctx context.Context, pubkeys []pubkey) (*ethpb.DoppelGangerResponse, error) {
	req := &ethpb.DoppelGangerRequest{ValidatorRequests: []*ethpb.DoppelGangerRequest_ValidatorRequest{}}
	for _, pkey := range pubkeys {
		attRec, err := v.db.AttestationHistoryForPubKey(ctx, pkey)
		if err != nil {
			return nil, errors.Wrapf(err, "could not get attestation history for pubkey %#x", bytesutil.Trunc(pkey[:]))
		}
		if len(attRec) == 0 {
			// Reloaded keys are checked from their import epoch, so attestations
			// made before the import are not flagged as duplicates; boot keys check from 0.
			epoch, _ := v.doppelGanger.pendingAddedEpoch(pkey)
			req.ValidatorRequests = append(req.ValidatorRequests,
				&ethpb.DoppelGangerRequest_ValidatorRequest{
					PublicKey:  pkey[:],
					Epoch:      epoch,
					SignedRoot: make([]byte, fieldparams.RootLength),
				})
			continue
		}
		r := retrieveLatestRecord(attRec)
		if pkey != r.PubKey {
			return nil, errors.Errorf("attestation record mismatched public key %#x", bytesutil.Trunc(pkey[:]))
		}
		watermark := r.Target
		// Pending keys are checked from their import epoch: an older record epoch would
		// flag pre-import attestations, a newer one defers evaluation past the quarantine.
		if added, ok := v.doppelGanger.pendingAddedEpoch(pkey); ok {
			watermark = added
		}
		req.ValidatorRequests = append(req.ValidatorRequests,
			&ethpb.DoppelGangerRequest_ValidatorRequest{
				PublicKey:  r.PubKey[:],
				Epoch:      watermark,
				SignedRoot: r.SigningRoot,
			})
	}
	resp, err := v.validatorClient.CheckDoppelGanger(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "doppelganger check request to beacon node failed")
	}
	if resp == nil {
		return nil, errors.New("nil doppelganger response from beacon node")
	}
	return resp, nil
}

func buildDuplicateError(response []*ethpb.DoppelGangerResponse_ValidatorResponse) error {
	duplicates := make([][]byte, 0)
	for _, valRes := range response {
		if valRes.DuplicateExists {
			copiedKey := bytesutil.ToBytes48(valRes.PublicKey)
			duplicates = append(duplicates, copiedKey[:])
		}
	}
	if len(duplicates) == 0 {
		return nil
	}
	return errors.Errorf("Duplicate instances exists in the network for validator keys: %#x", duplicates)
}

// Ensures that the latest attestation history is retrieved.
func retrieveLatestRecord(recs []*dbCommon.AttestationRecord) *dbCommon.AttestationRecord {
	if len(recs) == 0 {
		return nil
	}
	lastSource := recs[len(recs)-1].Source
	chosenRec := recs[len(recs)-1]
	for i := len(recs) - 1; i >= 0; i-- {
		// Exit if we are now on a different source
		// as it is assumed that all source records are
		// byte sorted.
		if recs[i].Source != lastSource {
			break
		}
		// If we have a smaller target, we do
		// change our chosen record.
		if chosenRec.Target < recs[i].Target {
			chosenRec = recs[i]
		}
	}
	return chosenRec
}

// vetStartupKeys marks keys the startup doppelganger check explicitly evaluated
// as checked; the rest are held pending for the per-epoch polls to vet later.
func (v *validator) vetStartupKeys(pubkeys []pubkey, responses []*ethpb.DoppelGangerResponse_ValidatorResponse) {
	evaluated := make(map[pubkey]bool, len(responses))
	for _, r := range responses {
		evaluated[bytesutil.ToBytes48(r.PublicKey)] = true
	}
	held := v.doppelGanger.vetStartup(pubkeys, evaluated, slots.EpochsSinceGenesis(v.genesisTime))
	switch {
	case held == 0:
	case held == len(pubkeys):
		// Nothing could be evaluated: the validator will sit idle, so say it loudly.
		log.WithField("keyCount", held).Warn(
			"Beacon node could not evaluate any validating keys; they are held out of duties until it can")
	default:
		log.WithField("keyCount", held).Debug(
			"Keys not yet in the beacon state are held out of duties until they can be checked")
	}
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
func (d *doppelGangerTracker) pollDue(epoch primitives.Epoch) []pubkey {
	if d.pendingCount.Load() == 0 {
		return nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if epoch < d.lastPollMark {
		return nil
	}
	var due []pubkey
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
	d.lastPollMark = max(d.lastPollMark, epoch+1)
}

// shouldWarnFailure reports whether a check failure at epoch was not yet warned.
func (d *doppelGangerTracker) shouldWarnFailure(epoch primitives.Epoch) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if epoch < d.lastWarnMark {
		return false
	}
	d.lastWarnMark = epoch + 1
	return true
}

// clearElapsed clears and returns the given clean keys strictly past their
// quarantine at epoch; the rest stay pending. epoch must not exceed the BN head.
func (d *doppelGangerTracker) clearElapsed(keys []pubkey, epoch primitives.Epoch) []pubkey {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.checked == nil {
		d.checked = make(map[pubkey]bool, len(keys))
	}
	var cleared []pubkey
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
// Logs outside the tracker lock.
func (d *doppelGangerTracker) block(keys []pubkey) {
	d.mu.Lock()
	var flagged []pubkey
	for _, pk := range keys {
		p, ok := d.pending[pk]
		if !ok {
			continue
		}
		p.blocked = true
		flagged = append(flagged, pk)
	}
	d.mu.Unlock()
	for _, pk := range flagged {
		log.WithField("pubkey", fmt.Sprintf("%#x", bytesutil.Trunc(pk[:]))).Error(
			"Doppelganger detected for reloaded key; key remains excluded from duties")
	}
}

// trackReloadedKeysForDoppelGanger quarantines never-checked keys from a reload.
func (v *validator) trackReloadedKeysForDoppelGanger(currentKeys []pubkey) {
	if !features.Get().EnableDoppelGanger {
		return
	}
	v.doppelGanger.trackReload(currentKeys, slots.EpochsSinceGenesis(v.genesisTime))
}

// isDoppelGangerPending reports whether a key must be excluded from duties.
func (v *validator) isDoppelGangerPending(pk pubkey) bool {
	return v.doppelGanger.isPending(pk)
}

// doppelGangerPollSlot is the slot-in-epoch from which quarantined keys are
// polled: late enough that most of the epoch's activity is visible to the node.
func doppelGangerPollSlot() primitives.Slot {
	return params.BeaconConfig().SlotsPerEpoch * 3 / 4
}

// CheckDoppelGangerMidEpoch is the per-epoch check owning all quarantined keys:
// duplicates are blocked on sight, clean keys clear only after the quarantine.
func (v *validator) CheckDoppelGangerMidEpoch(ctx context.Context, slot primitives.Slot) {
	if !features.Get().EnableDoppelGanger {
		return
	}
	if slots.SinceEpochStarts(slot) < doppelGangerPollSlot() {
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
func (v *validator) checkReloadedKeys(ctx context.Context, due []pubkey, epoch primitives.Epoch) {
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
func splitByDuplicate(responses []*ethpb.DoppelGangerResponse_ValidatorResponse) (clean, dups []pubkey) {
	for _, r := range responses {
		if r.DuplicateExists {
			dups = append(dups, bytesutil.ToBytes48(r.PublicKey))
		} else {
			clean = append(clean, bytesutil.ToBytes48(r.PublicKey))
		}
	}
	return clean, dups
}
