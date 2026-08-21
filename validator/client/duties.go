package client

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/metadata"
)

// nonBlacklistedKeys returns the keymanager's validating keys with
// slashing-protection-blacklisted keys removed.
func (v *validator) nonBlacklistedKeys(ctx context.Context) ([][fieldparams.BLSPubkeyLength]byte, error) {
	validatingKeys, err := v.km.FetchValidatingPublicKeys(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([][fieldparams.BLSPubkeyLength]byte, 0, len(validatingKeys))
	v.blacklistedPubkeysLock.RLock()
	defer v.blacklistedPubkeysLock.RUnlock()
	for _, pubKey := range validatingKeys {
		if v.blacklistedPubkeys[pubKey] {
			log.WithField(
				"pubkey", fmt.Sprintf("%#x", bytesutil.Trunc(pubKey[:])),
			).Warn("Not including slashable public key from slashing protection import " +
				"in request to update validator duties")
			continue
		}
		filtered = append(filtered, pubKey)
	}
	return filtered, nil
}

// isActiveForDuties reports whether a validator status entry indicates the
// validator currently has duties to perform — i.e. it is in (or about to be
// in) the beacon-state active set. Shared by filteredKeysAndIndices and
// filterAndCacheActiveKeys so both call sites agree on the same predicate.
func isActiveForDuties(s *ethpb.ValidatorStatusResponse, currEpoch primitives.Epoch) bool {
	if s == nil {
		return false
	}
	switch s.Status {
	case ethpb.ValidatorStatus_ACTIVE, ethpb.ValidatorStatus_EXITING:
		return true
	case ethpb.ValidatorStatus_PENDING:
		// Cache may be stale: include validators whose activation epoch has
		// already arrived but whose status hasn't been refreshed yet.
		return currEpoch >= s.ActivationEpoch
	}
	return false
}

// filteredKeysAndIndices returns the keys eligible for duties at the given epoch —
// active (see isActiveForDuties) and not held pending a doppelganger check — and the
// corresponding sorted indices, which let callers detect drift against a stored set.
func (v *validator) filteredKeysAndIndices(keys [][fieldparams.BLSPubkeyLength]byte, epoch primitives.Epoch) ([][fieldparams.BLSPubkeyLength]byte, []primitives.ValidatorIndex) {
	outKeys := make([][fieldparams.BLSPubkeyLength]byte, 0, len(keys))
	indices := make([]primitives.ValidatorIndex, 0, len(keys))
	statuses := v.statusCache()
	for _, pk := range keys {
		st, ok := statuses[pk]
		if !ok || !isActiveForDuties(st.status, epoch) {
			continue
		}
		// Reloaded keys stay out of duties until their doppelganger check clears.
		if v.isDoppelGangerPending(pk) {
			continue
		}
		outKeys = append(outKeys, pk)
		indices = append(indices, st.index)
	}
	slices.Sort(indices)
	return outKeys, indices
}

// UpdateDuties checks the slot number to determine if the validator's
// list of upcoming assignments needs to be updated. For example, at the
// beginning of a new epoch.
func (v *validator) UpdateDuties(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "validator.UpdateDuties")
	defer span.End()

	keys, err := v.nonBlacklistedKeys(ctx)
	if err != nil {
		return errors.Wrap(err, "could not filter blacklisted keys")
	}

	epoch := slots.ToEpoch(slots.CurrentSlot(v.genesisTime) + 1)

	filteredKeys, filteredIndices := v.filteredKeysAndIndices(keys, epoch)
	if epoch >= params.BeaconConfig().GloasForkEpoch {
		err = v.updateDutiesSplit(ctx, epoch, filteredIndices)
	} else {
		err = v.updateDutiesCombined(ctx, epoch, filteredKeys)
	}
	if err != nil {
		return errors.Wrap(err, "could not fetch duties")
	}

	if !v.duties.isInitialized() {
		return nil
	}
	snap := v.duties.snapshot()
	if snap.currentDutyCount() == 0 && snap.nextDutyCount() == 0 {
		// A known-empty schedule: skip duty logging and subnet subscriptions.
		return nil
	}

	ss, err := slots.EpochStart(epoch)
	if err != nil {
		return errors.Wrap(err, "could not compute epoch start slot")
	}
	v.logDuties(ss)

	return v.onDutiesUpdated(ctx)
}

// updateDutiesCombined uses the combined Duties() endpoint (pre-GLOAS).
func (v *validator) updateDutiesCombined(ctx context.Context, epoch primitives.Epoch, filteredKeys [][fieldparams.BLSPubkeyLength]byte) error {
	if len(filteredKeys) == 0 {
		// No eligible keys (none active, or all quarantined): duties for this
		// epoch are known to be none, which also drops any stale entries.
		v.duties.writeEmpty(epoch)
		return nil
	}
	req := &ethpb.DutiesRequest{
		Epoch:      epoch,
		PublicKeys: bytesutil.FromBytes48Array(filteredKeys),
	}

	resp, err := v.validatorClient.Duties(ctx, req)
	if err != nil {
		return errors.Wrap(err, "could not get validator duties")
	}
	if resp == nil {
		return errors.New("nil duties response from beacon node")
	}

	var data dutyStoreData
	data.setFromContainer(resp)
	data.missingNext = missingNextPtc
	v.duties.write(data)

	if allCurrentDutiesExited(resp.CurrentEpochDuties) {
		return ErrValidatorsAllExited
	}
	return nil
}

// dropIfDivergent treats a fetched duty response as missing (nil) when its
// dependent root contradicts the next-epoch attester root, so it gets flagged
// missing and re-fetched by the per-slot retry. Callers run >= gloas, where
// all duty dependent roots coincide (pre-fulu proposer roots differ by design).
func dropIfDivergent[T interface{ GetDependentRoot() []byte }](resp T, attRoot []byte, dutyType string) T {
	root := resp.GetDependentRoot()
	if len(root) == 0 || bytes.Equal(root, attRoot) {
		return resp
	}
	log.WithFields(logrus.Fields{
		"dutyType":              dutyType,
		"dependentRoot":         fmt.Sprintf("%#x", bytesutil.Trunc(root)),
		"attesterDependentRoot": fmt.Sprintf("%#x", bytesutil.Trunc(attRoot)),
	}).Warn("Duties have divergent dependent root, treating them as missing")
	var zero T
	return zero
}

// allCurrentDutiesExited reports whether there is at least one duty and all are EXITED.
func allCurrentDutiesExited(duties []*ethpb.ValidatorDuty) bool {
	if len(duties) == 0 {
		return false
	}
	for _, d := range duties {
		if d.Status != ethpb.ValidatorStatus_EXITED {
			return false
		}
	}
	return true
}

// dutiesFetchResult holds the current-epoch duties from a fetch or promotion.
// Next-epoch duties are always deferred, so this carries no next-epoch state.
type dutiesFetchResult struct {
	currentDuties []*ethpb.ValidatorDuty
	prevDepRoot   []byte
	missingNext   missingNextDuties
}

// missingNextDuties is a bitmask of next-epoch duty types not yet in the store,
// whether soft-failed during a fetch or deferred at promotion; the mid-epoch
// retry fills them, and the next promotion needs the mask clear.
type missingNextDuties uint8

const (
	missingNextProposer missingNextDuties = 1 << iota
	missingNextSync
	missingNextPtc
	missingNextAttester
)

// missingNextAll marks every next-epoch type missing. promoteDuties is only
// reached post-Gloas, so all four apply and no fork gating is needed.
const missingNextAll = missingNextProposer | missingNextSync | missingNextPtc | missingNextAttester

func (m missingNextDuties) String() string {
	if m == 0 {
		return "none"
	}
	var parts []string
	if m&missingNextProposer != 0 {
		parts = append(parts, "proposer")
	}
	if m&missingNextSync != 0 {
		parts = append(parts, "sync")
	}
	if m&missingNextPtc != 0 {
		parts = append(parts, "ptc")
	}
	if m&missingNextAttester != 0 {
		parts = append(parts, "attester")
	}
	return strings.Join(parts, "|")
}

// updateDutiesSplit fetches duties from the split V3 endpoints and
// populates the duty store. When the epoch has advanced by exactly one
// and duties are already initialized, it promotes the cached next-epoch
// duties to current and defers the new next-epoch fetch to the mid-epoch
// retry. indices must be sorted (see filteredKeysAndIndices).
func (v *validator) updateDutiesSplit(ctx context.Context, epoch primitives.Epoch, indices []primitives.ValidatorIndex) error {
	if len(indices) == 0 {
		// No eligible keys (none active, or all quarantined): duties for this
		// epoch are known to be none, which also drops any stale entries.
		v.duties.writeEmpty(epoch)
		return nil
	}

	canPromote := v.duties.canPromote(epoch, indices)

	var (
		res dutiesFetchResult
		err error
	)
	// On fetch failure, leave existing duties intact so the validator can
	// continue serving the current epoch from cache while we retry next tick.
	if canPromote {
		log.WithField("epoch", epoch).Debug("Promoting cached next-epoch duties to current")
		res = v.promoteDuties()
	} else {
		res, err = v.fetchAllDuties(ctx, epoch, indices)
		if err != nil {
			return errors.Wrap(err, "fetch all duties")
		}
	}

	// Next epoch is deferred: written empty here, populated later by
	// ensureNextEpochDuties, which also sets currDependentRoot.
	var data dutyStoreData
	data.setFromContainer(&ethpb.ValidatorDutiesContainer{
		PrevDependentRoot:  res.prevDepRoot,
		CurrentEpochDuties: res.currentDuties,
	})
	data.epoch = epoch
	data.missingNext = res.missingNext
	data.indices = indices
	v.duties.write(data)

	if allCurrentDutiesExited(res.currentDuties) {
		return ErrValidatorsAllExited
	}
	return nil
}

// promoteDuties promotes cached next-epoch duties to current, refreshing status.
// Next epoch is left for the mid-epoch retry to fetch, keeping slot 0 RPC-free.
func (v *validator) promoteDuties() dutiesFetchResult {
	snap := v.duties.snapshot()
	currentDuties := make([]*ethpb.ValidatorDuty, 0, snap.nextDutyCount())
	for _, d := range snap.nextDuties() {
		if d == nil {
			continue
		}
		// nextDuties yields read-only aliases into the live store; clone before
		// refreshing status so cached state isn't mutated in place.
		promoted := cloneValidatorDuty(d)
		promoted.Status = v.statusForPubkey(promoted.PublicKey)
		currentDuties = append(currentDuties, promoted)
	}
	return dutiesFetchResult{
		currentDuties: currentDuties,
		prevDepRoot:   snap.currDependentRoot(),
		missingNext:   missingNextAll,
	}
}

// missingNextMask flags next-epoch duty types missing post-fetch (fork-gated).
func missingNextMask(nextEpoch primitives.Epoch, att *ethpb.AttesterDutiesResponse, prop *ethpb.ProposerDutiesResponse, sync *ethpb.SyncCommitteeDutiesResponse, ptc *ethpb.PTCDutiesResponse) missingNextDuties {
	var m missingNextDuties
	if att == nil {
		m |= missingNextAttester
	}
	if prop == nil && nextEpoch >= params.BeaconConfig().FuluForkEpoch {
		m |= missingNextProposer
	}
	if sync == nil && nextEpoch >= params.BeaconConfig().AltairForkEpoch {
		m |= missingNextSync
	}
	if ptc == nil && nextEpoch >= params.BeaconConfig().GloasForkEpoch {
		m |= missingNextPtc
	}
	return m
}

// dutyResponses holds the per-type duty responses for one epoch; a nil
// response means the type was not requested, failed, or was dropped.
type dutyResponses struct {
	att  *ethpb.AttesterDutiesResponse
	prop *ethpb.ProposerDutiesResponse
	sync *ethpb.SyncCommitteeDutiesResponse
	ptc  *ethpb.PTCDutiesResponse

	attErr, propErr, syncErr, ptcErr error
}

// fetchDutyResponses requests the duty types flagged in wanted, in parallel.
func (v *validator) fetchDutyResponses(ctx context.Context, epoch primitives.Epoch, indices []primitives.ValidatorIndex, wanted missingNextDuties) dutyResponses {
	var r dutyResponses
	var wg sync.WaitGroup
	if wanted&missingNextAttester != 0 {
		wg.Go(func() { r.att, r.attErr = v.validatorClient.AttesterDuties(ctx, epoch, indices) })
	}
	if wanted&missingNextProposer != 0 {
		wg.Go(func() { r.prop, r.propErr = v.validatorClient.ProposerDuties(ctx, epoch) })
	}
	if wanted&missingNextSync != 0 {
		wg.Go(func() { r.sync, r.syncErr = v.validatorClient.SyncCommitteeDuties(ctx, epoch, indices) })
	}
	if wanted&missingNextPtc != 0 {
		wg.Go(func() { r.ptc, r.ptcErr = v.validatorClient.PTCDuties(ctx, epoch, indices) })
	}
	wg.Wait()
	return r
}

// fetchAllDuties fetches the current-epoch duties from all endpoints. Next-epoch
// duties are deferred to ensureNextEpochDuties, keeping the boundary off them.
func (v *validator) fetchAllDuties(ctx context.Context, epoch primitives.Epoch, indices []primitives.ValidatorIndex) (dutiesFetchResult, error) {
	var res dutiesFetchResult
	r := v.fetchDutyResponses(ctx, epoch, indices, missingNextAll)

	if r.attErr != nil {
		return res, r.attErr
	}
	if r.propErr != nil {
		return res, r.propErr
	}
	if r.syncErr != nil {
		log.WithError(r.syncErr).Warn("Error getting sync committee duties")
	}
	if r.ptcErr != nil {
		log.WithError(r.ptcErr).Warn("Error getting PTC duties")
	}

	if r.att != nil {
		res.prevDepRoot = r.att.DependentRoot
	}
	res.currentDuties = v.assembleDuties(r.att, r.prop, r.sync, r.ptc)
	// Next epoch left for the mid-epoch fetch; currDepRoot stays nil until then.
	res.missingNext = missingNextAll
	return res, nil
}

// assembleDuties stitches together the four per-duty-type API responses for
// a single epoch into a slice of ValidatorDuty entries, one per attester
// assignment.
func (v *validator) assembleDuties(
	att *ethpb.AttesterDutiesResponse,
	prop *ethpb.ProposerDutiesResponse,
	sync *ethpb.SyncCommitteeDutiesResponse,
	ptc *ethpb.PTCDutiesResponse,
) []*ethpb.ValidatorDuty {
	proposerSlots := make(map[primitives.ValidatorIndex][]primitives.Slot)
	if prop != nil {
		for _, d := range prop.Duties {
			proposerSlots[d.ValidatorIndex] = append(proposerSlots[d.ValidatorIndex], d.Slot)
		}
	}
	ptcSlots := make(map[primitives.ValidatorIndex][]primitives.Slot)
	if ptc != nil {
		for _, d := range ptc.Duties {
			ptcSlots[d.ValidatorIndex] = append(ptcSlots[d.ValidatorIndex], d.Slot)
		}
	}
	syncSet := make(map[primitives.ValidatorIndex]bool)
	if sync != nil {
		for _, d := range sync.Duties {
			syncSet[d.ValidatorIndex] = true
		}
	}
	if att == nil {
		return nil
	}
	duties := make([]*ethpb.ValidatorDuty, 0, len(att.Duties))
	for _, d := range att.Duties {
		duties = append(duties, &ethpb.ValidatorDuty{
			PublicKey:               d.Pubkey,
			ValidatorIndex:          d.ValidatorIndex,
			CommitteeIndex:          d.CommitteeIndex,
			CommitteeLength:         d.CommitteeLength,
			CommitteesAtSlot:        d.CommitteesAtSlot,
			ValidatorCommitteeIndex: d.ValidatorCommitteeIndex,
			AttesterSlot:            d.Slot,
			ProposerSlots:           proposerSlots[d.ValidatorIndex],
			IsSyncCommittee:         syncSet[d.ValidatorIndex],
			PtcSlots:                ptcSlots[d.ValidatorIndex],
			Status:                  v.statusForPubkey(d.Pubkey),
		})
	}
	return duties
}

// statusForPubkey returns the cached validator status for a pubkey.
func (v *validator) statusForPubkey(pk []byte) ethpb.ValidatorStatus {
	st, ok := v.statusCache()[bytesutil.ToBytes48(pk)]
	if !ok || st.status == nil {
		return ethpb.ValidatorStatus_UNKNOWN_STATUS
	}
	return st.status.Status
}

// fetchNextEpochDuties fetches the wanted next-epoch duty types, dropping
// proposer/PTC responses that diverge from the attester dependent root attRoot.
func (v *validator) fetchNextEpochDuties(ctx context.Context, nextEpoch primitives.Epoch, indices []primitives.ValidatorIndex, wanted missingNextDuties, attRoot []byte) dutyResponses {
	r := v.fetchDutyResponses(ctx, nextEpoch, indices, wanted)
	for _, e := range []error{r.attErr, r.propErr, r.syncErr, r.ptcErr} {
		if e != nil {
			log.WithError(e).Debug("Could not get a next epoch duty")
		}
	}
	if r.att != nil {
		attRoot = r.att.DependentRoot
	}
	if len(attRoot) != 0 {
		r.prop = dropIfDivergent(r.prop, attRoot, "proposer")
		r.ptc = dropIfDivergent(r.ptc, attRoot, "ptc")
	}
	return r
}

// nextDutiesFetchSlot returns the slot-in-epoch from which next-epoch duties are
// fetched in the background, past the boundary-heavy, reorg-prone first slots.
func nextDutiesFetchSlot() primitives.Slot {
	return max(1, params.BeaconConfig().SlotsPerEpoch/4)
}

// nextDutiesFetchBPS delays each background fetch to just after the Gloas
// aggregate broadcast (5000 BPS) and clear of PTC duties (7500 BPS).
const nextDutiesFetchBPS = primitives.BP(6000)

// MaybeFetchNextDuties runs ensureNextEpochDuties in a goroutine at nextDutiesFetchBPS when
// next-epoch duties are still needed and no fetch is in flight, bounded by the slot deadline.
func (v *validator) MaybeFetchNextDuties(ctx context.Context, slot primitives.Slot) {
	if !v.duties.needsNextFetch() || !v.nextFetchInFlight.CompareAndSwap(false, true) {
		return
	}
	fetchCtx, cancel := context.WithDeadline(ctx, v.SlotDeadline(slot))
	go func() {
		defer func() {
			cancel()
			v.nextFetchInFlight.Store(false)
		}()
		v.waitUntilSlotComponent(fetchCtx, slot, nextDutiesFetchBPS)
		if err := v.ensureNextEpochDuties(fetchCtx); err != nil {
			log.WithError(err).Debug("Could not fetch next-epoch duties")
		}
	}()
}

// ensureNextEpochDuties fetches the next-epoch duty types not yet in the store and
// merges them in, so the next promotion has them. No-op when already present.
func (v *validator) ensureNextEpochDuties(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "validator.ensureNextEpochDuties")
	defer span.End()

	snap := v.duties.snapshot()
	missing := snap.missingNext()
	if !snap.isInitialized() || missing == 0 {
		return nil
	}
	// Only the split duties path records indices; the combined pre-Gloas path
	// leaves them empty, so this guard alone scopes the fetch to split duties.
	indices := snap.indices()
	if len(indices) == 0 {
		return nil
	}
	nextEpoch := snap.epoch().AddEpoch(1)

	var (
		next        []*ethpb.ValidatorDuty
		newMissing  missingNextDuties
		currDepRoot []byte
	)
	if missing&missingNextAttester != 0 {
		// Attester is the spine: without it there are no rows to overlay onto, so
		// rebuild the whole epoch. Fetched again each slot until it succeeds.
		r := v.fetchNextEpochDuties(ctx, nextEpoch, indices, missingNextAll, nil)
		// Empty duties are guaranteed wrong with active validators; retry next slot.
		if r.att == nil || len(r.att.Duties) == 0 {
			return nil
		}
		next = v.assembleDuties(r.att, r.prop, r.sync, r.ptc)
		newMissing = missingNextMask(nextEpoch, r.att, r.prop, r.sync, r.ptc)
		currDepRoot = r.att.DependentRoot
	} else {
		// Spine intact: re-fetch only the missing types and overlay them, leaving
		// the attester duties and dependent root untouched.
		r := v.fetchNextEpochDuties(ctx, nextEpoch, indices, missing, snap.currDependentRoot())
		existing := make([]*ethpb.ValidatorDuty, 0, snap.nextDutyCount())
		for _, d := range snap.nextDuties() {
			existing = append(existing, d)
		}
		next = overlayNextDuties(existing, r.prop, r.sync, r.ptc)
		newMissing = missing
		if r.prop != nil {
			newMissing &^= missingNextProposer
		}
		if r.sync != nil {
			newMissing &^= missingNextSync
		}
		if r.ptc != nil {
			newMissing &^= missingNextPtc
		}
	}
	if newMissing == missing { // no progress; avoid needless re-subscribe / proof re-sign
		return nil
	}
	// Drop if the store advanced under us (a boundary/head-event update mid-fetch).
	if !v.duties.replaceNextDuties(snap.revision, next, newMissing, currDepRoot) {
		return nil
	}
	log.WithFields(logrus.Fields{
		"epoch":        nextEpoch,
		"fetched":      missing &^ newMissing,
		"stillMissing": newMissing,
	}).Debug("Fetched next-epoch duties")
	// logDuties only sees the deferred next-epoch set while still empty, so
	// next-epoch metrics are emitted here, once the fetch lands.
	v.emitNextEpochMetrics(next)
	return v.onDutiesUpdated(ctx)
}

// emitNextEpochMetrics records next-epoch duty metrics for active validators.
func (v *validator) emitNextEpochMetrics(next []*ethpb.ValidatorDuty) {
	if !v.emitAccountMetrics {
		return
	}
	for _, duty := range next {
		if duty == nil || (duty.Status != ethpb.ValidatorStatus_ACTIVE && duty.Status != ethpb.ValidatorStatus_EXITING) {
			continue
		}
		pk := fmt.Sprintf("%#x", duty.PublicKey)
		if duty.IsSyncCommittee {
			ValidatorInNextSyncCommitteeGaugeVec.WithLabelValues(pk).Set(float64(1))
		} else {
			ValidatorInNextSyncCommitteeGaugeVec.WithLabelValues(pk).Set(float64(0))
		}
	}
}

// overlayNextDuties clones existing next-epoch duties, overlaying re-fetched
// proposer/sync/PTC responses; a nil response leaves that field untouched.
func overlayNextDuties(
	existing []*ethpb.ValidatorDuty,
	prop *ethpb.ProposerDutiesResponse,
	sync *ethpb.SyncCommitteeDutiesResponse,
	ptc *ethpb.PTCDutiesResponse,
) []*ethpb.ValidatorDuty {
	var proposerSlots map[primitives.ValidatorIndex][]primitives.Slot
	if prop != nil {
		proposerSlots = make(map[primitives.ValidatorIndex][]primitives.Slot)
		for _, d := range prop.Duties {
			proposerSlots[d.ValidatorIndex] = append(proposerSlots[d.ValidatorIndex], d.Slot)
		}
	}
	var ptcSlots map[primitives.ValidatorIndex][]primitives.Slot
	if ptc != nil {
		ptcSlots = make(map[primitives.ValidatorIndex][]primitives.Slot)
		for _, d := range ptc.Duties {
			ptcSlots[d.ValidatorIndex] = append(ptcSlots[d.ValidatorIndex], d.Slot)
		}
	}
	var syncSet map[primitives.ValidatorIndex]bool
	if sync != nil {
		syncSet = make(map[primitives.ValidatorIndex]bool)
		for _, d := range sync.Duties {
			syncSet[d.ValidatorIndex] = true
		}
	}
	out := make([]*ethpb.ValidatorDuty, 0, len(existing))
	for _, d := range existing {
		nd := cloneValidatorDuty(d)
		if prop != nil {
			nd.ProposerSlots = proposerSlots[d.ValidatorIndex]
		}
		if sync != nil {
			nd.IsSyncCommittee = syncSet[d.ValidatorIndex]
		}
		if ptc != nil {
			nd.PtcSlots = ptcSlots[d.ValidatorIndex]
		}
		out = append(out, nd)
	}
	return out
}

// onDutiesUpdated kicks off subnet subscriptions for the current duty set.
func (v *validator) onDutiesUpdated(ctx context.Context) error {
	md, exists := metadata.FromOutgoingContext(ctx)
	ctx = context.Background()
	if exists {
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	container := v.duties.toContainer()
	go func() {
		if err := v.subscribeToSubnets(ctx, container); err != nil {
			log.WithError(err).Error("Failed to subscribe to subnets")
		}
	}()

	return nil
}

func (v *validator) logDuties(slot primitives.Slot) {
	snap := v.duties.snapshot()
	if !snap.isInitialized() {
		return
	}

	epochStartSlot, err := slots.EpochStart(slots.ToEpoch(slot))
	if err != nil {
		log.WithError(err).Error("Could not calculate epoch start. Ignoring logging duties.")
		return
	}
	attesterKeys := make([][]string, params.BeaconConfig().SlotsPerEpoch)
	proposerKeys := make([]string, params.BeaconConfig().SlotsPerEpoch)
	ptcKeys := make([][]string, params.BeaconConfig().SlotsPerEpoch)
	var totalProposingKeys, totalAttestingKeys, totalPTCKeys uint64

	for _, duty := range snap.currentDuties() {
		pk := fmt.Sprintf("%#x", duty.PublicKey)
		if v.emitAccountMetrics {
			ValidatorStatusesGaugeVec.WithLabelValues(pk, fmt.Sprintf("%#x", duty.ValidatorIndex)).Set(float64(duty.Status))
		}
		if duty.Status != ethpb.ValidatorStatus_ACTIVE && duty.Status != ethpb.ValidatorStatus_EXITING {
			continue
		}

		truncatedPubkey := fmt.Sprintf("%#x", bytesutil.Trunc(duty.PublicKey))
		attesterSlotInEpoch := duty.AttesterSlot - epochStartSlot
		if attesterSlotInEpoch >= params.BeaconConfig().SlotsPerEpoch {
			log.WithField("duty", duty).Warn("Invalid attester slot")
		} else {
			attesterKeys[attesterSlotInEpoch] = append(attesterKeys[attesterSlotInEpoch], truncatedPubkey)
			totalAttestingKeys++
			if v.emitAccountMetrics {
				ValidatorNextAttestationSlotGaugeVec.WithLabelValues(pk).Set(float64(duty.AttesterSlot))
			}
		}
		if v.emitAccountMetrics && duty.IsSyncCommittee {
			ValidatorInSyncCommitteeGaugeVec.WithLabelValues(pk).Set(float64(1))
		} else if v.emitAccountMetrics && !duty.IsSyncCommittee {
			ValidatorInSyncCommitteeGaugeVec.WithLabelValues(pk).Set(float64(0))
		}
		for _, ptcSlot := range duty.PtcSlots {
			if ptcSlot < epochStartSlot || ptcSlot >= epochStartSlot+params.BeaconConfig().SlotsPerEpoch {
				log.WithFields(logrus.Fields{
					"duty": duty,
					"slot": ptcSlot,
				}).Warn("Invalid PTC slot")
				continue
			}
			ptcSlotInEpoch := ptcSlot - epochStartSlot
			ptcKeys[ptcSlotInEpoch] = append(ptcKeys[ptcSlotInEpoch], truncatedPubkey)
			totalPTCKeys++
		}

		for _, proposerSlot := range duty.ProposerSlots {
			proposerSlotInEpoch := proposerSlot - epochStartSlot
			if proposerSlotInEpoch >= params.BeaconConfig().SlotsPerEpoch {
				log.WithField("duty", duty).Warn("Invalid proposer slot")
			} else {
				proposerKeys[proposerSlotInEpoch] = truncatedPubkey
				totalProposingKeys++
			}
			if v.emitAccountMetrics {
				ValidatorNextProposalSlotGaugeVec.WithLabelValues(pk).Set(float64(proposerSlot))
			}
		}
	}
	nextDuties := make([]*ethpb.ValidatorDuty, 0, snap.nextDutyCount())
	for _, duty := range snap.nextDuties() {
		nextDuties = append(nextDuties, duty)
	}
	v.emitNextEpochMetrics(nextDuties)

	log.WithFields(logrus.Fields{
		"proposerCount": totalProposingKeys,
		"attesterCount": totalAttestingKeys,
		"ptcCount":      totalPTCKeys,
	}).Infof("Schedule for epoch %d", slots.ToEpoch(slot))

	for i := primitives.Slot(0); i < params.BeaconConfig().SlotsPerEpoch; i++ {
		isProposer := proposerKeys[i] != ""
		isAttester := len(attesterKeys[i]) > 0
		isPTCMember := len(ptcKeys[i]) > 0
		if !isProposer && !isAttester && !isPTCMember {
			continue
		}
		startTime, err := slots.StartTime(v.genesisTime, epochStartSlot+i)
		if err != nil {
			log.WithError(err).WithField("slot", epochStartSlot+i).Error("Slot overflows, unable to log duties!")
			return
		}
		durationTillDuty := (time.Until(startTime) + time.Second).Truncate(time.Second)
		slotLog := log.WithFields(logrus.Fields{})
		if isProposer {
			slotLog = slotLog.WithField("proposerPubkey", proposerKeys[i])
		}
		if isAttester {
			slotLog = slotLog.WithFields(logrus.Fields{
				"slot":            epochStartSlot + i,
				"slotInEpoch":     (epochStartSlot + i) % params.BeaconConfig().SlotsPerEpoch,
				"attesterCount":   len(attesterKeys[i]),
				"attesterPubkeys": attesterKeys[i],
			})
		}
		if isPTCMember {
			slotLog = slotLog.WithFields(logrus.Fields{
				"ptcCount":   len(ptcKeys[i]),
				"ptcPubkeys": ptcKeys[i],
			})
		}
		if durationTillDuty > 0 {
			slotLog = slotLog.WithField("timeUntilDuty", durationTillDuty)
		}
		slotLog.Infof("Duties schedule")
	}
}

func (v *validator) checkDependentRoots(ctx context.Context, prevRoot, currRoot string) error {
	if prevRoot == "" || currRoot == "" {
		return errors.New("dependent root missing from head event")
	}

	prevDependentRoot, err := bytesutil.DecodeHexWithLength(prevRoot, fieldparams.RootLength)
	if err != nil {
		return errors.Wrap(err, "failed to decode previous duty dependent root")
	}
	if bytes.Equal(prevDependentRoot, params.BeaconConfig().ZeroHash[:]) {
		return nil
	}
	epoch := slots.ToEpoch(slots.CurrentSlot(v.genesisTime) + 1)
	ss, err := slots.EpochStart(epoch + 1)
	if err != nil {
		return errors.Wrap(err, "failed to get epoch start")
	}
	dutiesCtx, cancel := context.WithDeadline(ctx, v.SlotDeadline(ss-1))
	defer cancel()

	storedPrev := v.duties.prevDependentRoot()
	needsPrevUpdate := storedPrev == nil || !bytes.Equal(prevDependentRoot, storedPrev)

	if needsPrevUpdate {
		if err := v.UpdateDuties(dutiesCtx); err != nil {
			return errors.Wrap(err, "failed to update duties")
		}
		log.Info("Updated duties due to previous dependent root change")
		v.submitProposerPreferences(ctx)
		return nil
	}

	currDependentRoot, err := bytesutil.DecodeHexWithLength(currRoot, fieldparams.RootLength)
	if err != nil {
		return errors.Wrap(err, "failed to decode current duty dependent root")
	}
	if bytes.Equal(currDependentRoot, params.BeaconConfig().ZeroHash[:]) {
		return nil
	}
	// Only act as a correction layer over an already-known next-epoch root. An
	// unknown (nil) root — e.g. next-epoch not yet fetched — is left to the epoch
	// boundary and per-slot ensureNextEpochDuties, rather than triggering a full
	// UpdateDuties on every head event.
	storedCurr := v.duties.currDependentRoot()
	needsCurrUpdate := storedCurr != nil && !bytes.Equal(currDependentRoot, storedCurr)
	if !needsCurrUpdate {
		return nil
	}
	if err := v.UpdateDuties(dutiesCtx); err != nil {
		return errors.Wrap(err, "failed to update duties")
	}
	log.Info("Updated duties due to current dependent root change")
	v.submitProposerPreferences(ctx)
	return nil
}
