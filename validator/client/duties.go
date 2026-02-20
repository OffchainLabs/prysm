package client

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
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

// UpdateDuties checks the slot number to determine if the validator's
// list of upcoming assignments needs to be updated. For example, at the
// beginning of a new epoch.
func (v *validator) UpdateDuties(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "validator.UpdateDuties")
	defer span.End()

	validatingKeys, err := v.km.FetchValidatingPublicKeys(ctx)
	if err != nil {
		return err
	}

	// Filter out the slashable public keys from the duties request.
	filteredKeys := make([][fieldparams.BLSPubkeyLength]byte, 0, len(validatingKeys))
	v.blacklistedPubkeysLock.RLock()
	for _, pubKey := range validatingKeys {
		if ok := v.blacklistedPubkeys[pubKey]; !ok {
			filteredKeys = append(filteredKeys, pubKey)
		} else {
			log.WithField(
				"pubkey", fmt.Sprintf("%#x", bytesutil.Trunc(pubKey[:])),
			).Warn("Not including slashable public key from slashing protection import " +
				"in request to update validator duties")
		}
	}
	v.blacklistedPubkeysLock.RUnlock()
	epoch := slots.ToEpoch(slots.CurrentSlot(v.genesisTime) + 1)

	if epoch >= params.BeaconConfig().GloasForkEpoch {
		if err := v.updateDutiesSplit(ctx, epoch, filteredKeys); err != nil {
			return err
		}
	} else {
		if err := v.updateDutiesLegacy(ctx, epoch, filteredKeys); err != nil {
			return err
		}
	}

	v.dutiesLock.RLock()
	initialized := v.duties != nil && v.duties.IsInitialized()
	v.dutiesLock.RUnlock()
	if !initialized {
		return nil
	}

	ss, err := slots.EpochStart(epoch)
	if err != nil {
		return err
	}
	v.dutiesLock.Lock()
	v.logDuties(ss)
	v.dutiesLock.Unlock()

	return v.onDutiesUpdated(ctx)
}

// clearDuties resets the duty store under lock, used on fetch errors.
func (v *validator) clearDuties() {
	v.dutiesLock.Lock()
	defer v.dutiesLock.Unlock()
	if v.duties == nil {
		v.duties = &dutyStore{}
	}
	v.duties.Reset()
}

// updateDutiesLegacy uses the combined Duties() endpoint for backward compat.
func (v *validator) updateDutiesLegacy(ctx context.Context, epoch primitives.Epoch, filteredKeys [][fieldparams.BLSPubkeyLength]byte) error {
	req := &ethpb.DutiesRequest{
		Epoch:      epoch,
		PublicKeys: bytesutil.FromBytes48Array(filteredKeys),
	}

	resp, err := v.validatorClient.Duties(ctx, req)
	if err != nil || resp == nil {
		v.clearDuties()
		log.WithError(err).Error("Error getting validator duties")
		return err
	}

	v.dutiesLock.Lock()
	if v.duties == nil {
		v.duties = &dutyStore{}
	}
	v.duties.SetLegacy(resp, v.pubkeyToStatus)
	v.dutiesLock.Unlock()
	return nil
}

// onDutiesUpdated checks for all-exited validators and starts subnet subscriptions.
func (v *validator) onDutiesUpdated(ctx context.Context) error {
	v.dutiesLock.RLock()
	exited, total := v.duties.AllCurrentExitedCount()
	v.dutiesLock.RUnlock()
	if exited != 0 && exited == total {
		return ErrValidatorsAllExited
	}

	// Non-blocking call for beacon node to start subscriptions for aggregators.
	md, exists := metadata.FromOutgoingContext(ctx)
	ctx = context.Background()
	if exists {
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	go func() {
		if err := v.subscribeToSubnets(ctx); err != nil {
			log.WithError(err).Error("Failed to subscribe to subnets")
		}
	}()

	return nil
}

// cachedAttesterDuties checks if the attester cache is still valid by probing the dependent root.
// Returns cached current/next responses and ok=true if the cache hit, ok=false to signal refetch.
func (v *validator) cachedAttesterDuties(
	ctx context.Context, epoch primitives.Epoch, indices []primitives.ValidatorIndex,
) (current, next *ethpb.AttesterDutiesResponse, ok bool) {
	v.dutiesLock.RLock()
	var cached *attesterDutiesCacheEntry
	if v.duties != nil {
		cached = v.duties.AttesterDutiesCache()
	}
	v.dutiesLock.RUnlock()

	if cached == nil || cached.epoch != epoch {
		return nil, nil, false
	}
	probe, err := v.validatorClient.AttesterDuties(ctx, epoch, indices[:1])
	if err != nil {
		return nil, nil, false
	}
	if !bytes.Equal(probe.DependentRoot, cached.current.DependentRoot) {
		return nil, nil, false
	}
	return cached.current, cached.next, true
}

// cachedSyncDuties returns cached sync committee duties if still in the same period.
func (v *validator) cachedSyncDuties(epoch primitives.Epoch) (*syncDutiesCacheEntry, bool) {
	currentPeriod := uint64(epoch) / uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod)

	v.dutiesLock.RLock()
	var entry *syncDutiesCacheEntry
	if v.duties != nil {
		entry = v.duties.SyncDutiesCache()
	}
	v.dutiesLock.RUnlock()

	if entry != nil && entry.period == currentPeriod {
		return entry, true
	}
	return nil, false
}

// updateDutiesSplit fetches duties from the split V3 endpoints with per-duty caching.
func (v *validator) updateDutiesSplit(ctx context.Context, epoch primitives.Epoch, filteredKeys [][fieldparams.BLSPubkeyLength]byte) error {
	// Resolve pubkeys → indices via pubkeyToStatus (populated in WaitForActivation).
	indices := make([]primitives.ValidatorIndex, 0, len(filteredKeys))
	indexToPubkey := make(map[primitives.ValidatorIndex][fieldparams.BLSPubkeyLength]byte, len(filteredKeys))
	for _, pk := range filteredKeys {
		if st, ok := v.pubkeyToStatus[pk]; ok && st.status != nil && st.status.Status != ethpb.ValidatorStatus_UNKNOWN_STATUS {
			indices = append(indices, st.index)
			indexToPubkey[st.index] = pk
		}
	}
	if len(indices) == 0 {
		return nil
	}

	propResp, err := v.validatorClient.ProposerDuties(ctx, epoch)
	if err != nil {
		v.clearDuties()
		log.WithError(err).Error("Error getting proposer duties")
		return err
	}

	// Check attester and sync caches.
	attCurrent, attNext, attSkipped := v.cachedAttesterDuties(ctx, epoch, indices)
	cachedSync, syncSkipped := v.cachedSyncDuties(epoch)
	var syncCurrent, syncNext *ethpb.SyncCommitteeDutiesResponse
	if syncSkipped {
		syncCurrent, syncNext = cachedSync.current, cachedSync.next
	}

	// Fetch missing duties in parallel.
	var (
		fetchErr error
		errOnce  sync.Once
	)
	setErr := func(err error) { errOnce.Do(func() { fetchErr = err }) }

	var wg sync.WaitGroup

	if !attSkipped {
		wg.Go(func() {
			resp, err := v.validatorClient.AttesterDuties(ctx, epoch, indices)
			if err != nil {
				setErr(err)
				return
			}
			attCurrent = resp
		})
		wg.Go(func() {
			resp, err := v.validatorClient.AttesterDuties(ctx, epoch+1, indices)
			if err != nil {
				setErr(err)
				return
			}
			attNext = resp
		})
	}

	if !syncSkipped && epoch >= params.BeaconConfig().AltairForkEpoch {
		wg.Go(func() {
			resp, err := v.validatorClient.SyncCommitteeDuties(ctx, epoch, indices)
			if err != nil {
				setErr(err)
				return
			}
			syncCurrent = resp
		})
		wg.Go(func() {
			resp, err := v.validatorClient.SyncCommitteeDuties(ctx, epoch+1, indices)
			if err != nil {
				log.WithError(err).Debug("Could not get next epoch sync committee duties")
				return
			}
			syncNext = resp
		})
	}

	wg.Wait()

	if fetchErr != nil {
		v.clearDuties()
		log.WithError(fetchErr).Error("Error getting validator duties (split)")
		return fetchErr
	}
	if attCurrent == nil {
		v.clearDuties()
		return errors.New("incomplete duty response from beacon node")
	}

	// Build cache entries and store.
	currentPeriod := uint64(epoch) / uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod)
	attCache := &attesterDutiesCacheEntry{current: attCurrent, next: attNext, epoch: epoch}
	propCache := &proposerDutiesCacheEntry{current: propResp, epoch: epoch}
	var syncCache *syncDutiesCacheEntry
	if syncSkipped {
		syncCache = cachedSync
	} else if syncCurrent != nil {
		syncCache = &syncDutiesCacheEntry{current: syncCurrent, next: syncNext, epoch: epoch, period: currentPeriod}
	}

	v.dutiesLock.Lock()
	if v.duties == nil {
		v.duties = &dutyStore{}
	}
	v.duties.SetSplit(attCache, propCache, syncCache, indexToPubkey, v.pubkeyToStatus)
	v.dutiesLock.Unlock()

	return nil
}

func (v *validator) logDuties(slot primitives.Slot) {
	epochStartSlot, err := slots.EpochStart(slots.ToEpoch(slot))
	if err != nil {
		log.WithError(err).Error("Could not calculate epoch start. Ignoring logging duties.")
		return
	}
	attesterKeys := make([][]string, params.BeaconConfig().SlotsPerEpoch)
	for i := range attesterKeys {
		attesterKeys[i] = make([]string, 0)
	}
	proposerKeys := make([]string, params.BeaconConfig().SlotsPerEpoch)
	var totalProposingKeys, totalAttestingKeys uint64

	for _, duty := range v.duties.CurrentEpochDuties() {
		if v.emitAccountMetrics {
			v.emitCurrentDutyMetrics(duty)
		}
		if duty.Status != ethpb.ValidatorStatus_ACTIVE && duty.Status != ethpb.ValidatorStatus_EXITING {
			continue
		}
		truncatedPubkey := fmt.Sprintf("%#x", bytesutil.Trunc(duty.Pubkey[:]))
		attesterSlotInEpoch := duty.Slot - epochStartSlot
		if attesterSlotInEpoch >= params.BeaconConfig().SlotsPerEpoch {
			log.WithField("duty", duty).Warn("Invalid attester slot")
		} else {
			attesterKeys[attesterSlotInEpoch] = append(attesterKeys[attesterSlotInEpoch], truncatedPubkey)
			totalAttestingKeys++
		}
		for _, proposerSlot := range duty.ProposerSlots {
			proposerSlotInEpoch := proposerSlot - epochStartSlot
			if proposerSlotInEpoch >= params.BeaconConfig().SlotsPerEpoch {
				log.WithField("duty", duty).Warn("Invalid proposer slot")
			} else {
				proposerKeys[proposerSlotInEpoch] = truncatedPubkey
				totalProposingKeys++
			}
		}
	}
	if v.emitAccountMetrics {
		for _, duty := range v.duties.NextEpochDuties() {
			if duty.Status == ethpb.ValidatorStatus_ACTIVE || duty.Status == ethpb.ValidatorStatus_EXITING {
				v.emitNextDutyMetrics(duty)
			}
		}
	}

	log.WithFields(logrus.Fields{
		"proposerCount": totalProposingKeys,
		"attesterCount": totalAttestingKeys,
	}).Infof("Schedule for epoch %d", slots.ToEpoch(slot))
	v.logSlotSchedule(epochStartSlot, attesterKeys, proposerKeys)
}

func (v *validator) emitCurrentDutyMetrics(duty *attesterDutyView) {
	pubkey := fmt.Sprintf("%#x", duty.Pubkey)
	ValidatorStatusesGaugeVec.WithLabelValues(pubkey, fmt.Sprintf("%#x", duty.ValidatorIndex)).Set(float64(duty.Status))
	if duty.Status != ethpb.ValidatorStatus_ACTIVE && duty.Status != ethpb.ValidatorStatus_EXITING {
		return
	}
	ValidatorNextAttestationSlotGaugeVec.WithLabelValues(pubkey).Set(float64(duty.Slot))
	if duty.IsSyncCommittee {
		ValidatorInSyncCommitteeGaugeVec.WithLabelValues(pubkey).Set(float64(1))
	} else {
		ValidatorInSyncCommitteeGaugeVec.WithLabelValues(pubkey).Set(float64(0))
	}
	for _, proposerSlot := range duty.ProposerSlots {
		ValidatorNextProposalSlotGaugeVec.WithLabelValues(pubkey).Set(float64(proposerSlot))
	}
}

func (v *validator) emitNextDutyMetrics(duty *attesterDutyView) {
	pubkey := fmt.Sprintf("%#x", duty.Pubkey)
	if duty.IsSyncCommittee {
		ValidatorInNextSyncCommitteeGaugeVec.WithLabelValues(pubkey).Set(float64(1))
	} else {
		ValidatorInNextSyncCommitteeGaugeVec.WithLabelValues(pubkey).Set(float64(0))
	}
}

func (v *validator) logSlotSchedule(epochStartSlot primitives.Slot, attesterKeys [][]string, proposerKeys []string) {
	for i := primitives.Slot(0); i < params.BeaconConfig().SlotsPerEpoch; i++ {
		isProposer := proposerKeys[i] != ""
		isAttester := len(attesterKeys[i]) > 0
		if !isProposer && !isAttester {
			continue
		}
		startTime, err := slots.StartTime(v.genesisTime, epochStartSlot+i)
		if err != nil {
			log.WithError(err).WithField("slot", epochStartSlot+i).Error("Slot overflows, unable to log duties!")
			return
		}
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
		durationTillDuty := (time.Until(startTime) + time.Second).Truncate(time.Second)
		if durationTillDuty > 0 {
			slotLog = slotLog.WithField("timeUntilDuty", durationTillDuty)
		}
		slotLog.Infof("Duties schedule")
	}
}

// dependentRootChangeReason checks whether the stored dependent roots differ
// from the head event roots. Returns "previous", "current", or "" (no change).
func (v *validator) dependentRootChangeReason(prevRoot, currRoot []byte) string {
	v.dutiesLock.RLock()
	defer v.dutiesLock.RUnlock()
	if v.duties == nil || !v.duties.IsInitialized() {
		return "previous"
	}
	storedPrev, storedCurr := v.duties.DependentRoots()
	if !bytes.Equal(prevRoot, storedPrev) {
		return "previous"
	}
	if bytes.Equal(currRoot, params.BeaconConfig().ZeroHash[:]) {
		return ""
	}
	if !bytes.Equal(currRoot, storedCurr) {
		return "current"
	}
	return ""
}

func (v *validator) checkDependentRoots(ctx context.Context, head *structs.HeadEvent) error {
	if head == nil {
		return errors.New("received empty head event")
	}
	prevDependentRoot, err := bytesutil.DecodeHexWithLength(head.PreviousDutyDependentRoot, fieldparams.RootLength)
	if err != nil {
		return errors.Wrap(err, "failed to decode previous duty dependent root")
	}
	if bytes.Equal(prevDependentRoot, params.BeaconConfig().ZeroHash[:]) {
		return nil
	}
	currDependentRoot, err := bytesutil.DecodeHexWithLength(head.CurrentDutyDependentRoot, fieldparams.RootLength)
	if err != nil {
		return errors.Wrap(err, "failed to decode current duty dependent root")
	}
	reason := v.dependentRootChangeReason(prevDependentRoot, currDependentRoot)
	if reason == "" {
		return nil
	}
	epoch := slots.ToEpoch(slots.CurrentSlot(v.genesisTime) + 1)
	ss, err := slots.EpochStart(epoch + 1)
	if err != nil {
		return errors.Wrap(err, "failed to get epoch start")
	}
	dutiesCtx, cancel := context.WithDeadline(ctx, v.SlotDeadline(ss-1))
	defer cancel()
	if err := v.UpdateDuties(dutiesCtx); err != nil {
		return errors.Wrap(err, "failed to update duties")
	}
	log.Infof("Updated duties due to %s dependent root change", reason)
	return nil
}
