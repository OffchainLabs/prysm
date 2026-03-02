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

// isActiveOrExiting returns true if the pubkey belongs to an ACTIVE or EXITING validator.
// Used as the include filter when populating the duty store.
func (v *validator) isActiveOrExiting(pk []byte) bool {
	if v.pubkeyToStatus == nil {
		return false
	}
	st, ok := v.pubkeyToStatus[bytesutil.ToBytes48(pk)]
	if !ok || st.status == nil {
		return false
	}
	return st.status.Status == ethpb.ValidatorStatus_ACTIVE || st.status.Status == ethpb.ValidatorStatus_EXITING
}

// UpdateDuties checks the slot number to determine if the validator's
// list of upcoming assignments needs to be updated. For example, at the
// beginning of a new epoch.
// filterBlacklistedKeys returns validating keys with slashable keys removed.
func (v *validator) filterBlacklistedKeys(ctx context.Context) ([][fieldparams.BLSPubkeyLength]byte, error) {
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

func (v *validator) UpdateDuties(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "validator.UpdateDuties")
	defer span.End()

	filteredKeys, err := v.filterBlacklistedKeys(ctx)
	if err != nil {
		return err
	}
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

func filterSlice[T any](s []*T, include func(*T) bool) []*T {
	n := 0
	for _, v := range s {
		if v != nil && include(v) {
			s[n] = v
			n++
		}
	}
	return s[:n]
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

	resp.CurrentEpochDuties = filterSlice(resp.CurrentEpochDuties, func(d *ethpb.ValidatorDuty) bool {
		return v.isActiveOrExiting(d.PublicKey)
	})
	resp.NextEpochDuties = filterSlice(resp.NextEpochDuties, func(d *ethpb.ValidatorDuty) bool {
		return v.isActiveOrExiting(d.PublicKey)
	})

	v.dutiesLock.Lock()
	if v.duties == nil {
		v.duties = &dutyStore{}
	}
	v.duties.SetLegacy(resp)
	v.dutiesLock.Unlock()
	return nil
}

// onDutiesUpdated checks for all-exited validators and starts subnet subscriptions.
func (v *validator) onDutiesUpdated(ctx context.Context) error {
	v.dutiesLock.RLock()
	ds := v.duties
	v.dutiesLock.RUnlock()

	allExited := len(v.pubkeyToStatus) > 0
	for _, s := range v.pubkeyToStatus {
		if s.status != nil && s.status.Status != ethpb.ValidatorStatus_EXITED {
			allExited = false
			break
		}
	}
	if allExited {
		return ErrValidatorsAllExited
	}

	// Non-blocking call for beacon node to start subscriptions for aggregators.
	md, exists := metadata.FromOutgoingContext(ctx)
	ctx = context.Background()
	if exists {
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	go func() {
		if v.distributed {
			if err := v.aggregatedSelectionProofs(ctx, ds); err != nil {
				log.WithError(err).Error("Failed to get aggregated selection proofs")
				return
			}
		}
		if err := v.subscribeToSubnets(ctx); err != nil {
			log.WithError(err).Error("Failed to subscribe to subnets")
		}
	}()

	return nil
}

// fetchAttesterDuties fetches attester duties for current and next epoch in parallel.
func (v *validator) fetchAttesterDuties(
	ctx context.Context, epoch primitives.Epoch, indices []primitives.ValidatorIndex,
) (current, next *ethpb.AttesterDutiesResponse, err error) {
	var (
		currErr error
		nextErr error
		wg      sync.WaitGroup
	)
	wg.Go(func() {
		current, currErr = v.validatorClient.AttesterDuties(ctx, epoch, indices)
	})
	wg.Go(func() {
		next, nextErr = v.validatorClient.AttesterDuties(ctx, epoch+1, indices)
	})
	wg.Wait()

	if currErr != nil {
		return nil, nil, currErr
	}
	if nextErr != nil {
		return nil, nil, nextErr
	}
	return current, next, nil
}

// fetchSyncDuties fetches sync committee duties, using the dutyStore cache when still in the same period.
// Returns (0, nil, nil, nil) on cache hit. Returns (0, nil, nil, nil) for pre-Altair epochs.
func (v *validator) fetchSyncDuties(
	ctx context.Context, epoch primitives.Epoch, indices []primitives.ValidatorIndex,
) (period uint64, current, next *ethpb.SyncCommitteeDutiesResponse, err error) {
	if epoch < params.BeaconConfig().AltairForkEpoch {
		return 0, nil, nil, nil
	}

	currentPeriod := uint64(epoch) / uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod)

	// Check cache via dutyStore.
	v.dutiesLock.RLock()
	cacheValid := v.duties != nil && v.duties.SyncCacheValid(currentPeriod)
	v.dutiesLock.RUnlock()

	if cacheValid {
		return 0, nil, nil, nil
	}

	// Cache miss — fetch current and next in parallel.
	var (
		currErr error
		nextErr error
		wg      sync.WaitGroup
	)
	wg.Go(func() {
		current, currErr = v.validatorClient.SyncCommitteeDuties(ctx, epoch, indices)
	})
	wg.Go(func() {
		next, nextErr = v.validatorClient.SyncCommitteeDuties(ctx, epoch+1, indices)
		if nextErr != nil {
			log.WithError(nextErr).Debug("Could not get next epoch sync committee duties")
			nextErr = nil // non-fatal
		}
	})
	wg.Wait()

	if currErr != nil {
		return 0, nil, nil, currErr
	}
	return currentPeriod, current, next, nil
}

// fetchProposerDuties fetches proposer duties.
// Post-Fulu, also fetches next-epoch duties (deterministic via proposer_lookahead).
func (v *validator) fetchProposerDuties(
	ctx context.Context, epoch primitives.Epoch,
) (current, next *ethpb.ProposerDutiesResponse, err error) {
	var (
		currErr error
		nextErr error
		wg      sync.WaitGroup
	)
	wg.Go(func() {
		current, currErr = v.validatorClient.ProposerDuties(ctx, epoch)
	})
	if epoch >= params.BeaconConfig().FuluForkEpoch {
		wg.Go(func() {
			next, nextErr = v.validatorClient.ProposerDuties(ctx, epoch+1)
		})
	}
	wg.Wait()

	if currErr != nil {
		return nil, nil, currErr
	}
	if nextErr != nil {
		log.WithError(nextErr).Debug("Could not get next epoch proposer duties")
	}
	return current, next, nil
}

// updateDutiesSplit fetches duties from the split V3 endpoints.
func (v *validator) updateDutiesSplit(ctx context.Context, epoch primitives.Epoch, filteredKeys [][fieldparams.BLSPubkeyLength]byte) error {
	// Resolve pubkeys → indices via pubkeyToStatus (populated in WaitForActivation).
	indices := make([]primitives.ValidatorIndex, 0, len(filteredKeys))
	for _, pk := range filteredKeys {
		if st, ok := v.pubkeyToStatus[pk]; ok && st.status != nil && st.status.Status != ethpb.ValidatorStatus_UNKNOWN_STATUS {
			indices = append(indices, st.index)
		}
	}
	if len(indices) == 0 {
		return nil
	}

	// Fetch all three duty types in parallel.
	var (
		propCurr, propNext *ethpb.ProposerDutiesResponse
		attCurr, attNext   *ethpb.AttesterDutiesResponse
		syncCurr, syncNext *ethpb.SyncCommitteeDutiesResponse
		syncPeriod         uint64
		propErr            error
		attErr             error
		syncErr            error
		wg                 sync.WaitGroup
	)
	wg.Go(func() { propCurr, propNext, propErr = v.fetchProposerDuties(ctx, epoch) })
	wg.Go(func() { attCurr, attNext, attErr = v.fetchAttesterDuties(ctx, epoch, indices) })
	wg.Go(func() { syncPeriod, syncCurr, syncNext, syncErr = v.fetchSyncDuties(ctx, epoch, indices) })
	wg.Wait()

	// Proposer or attester failure is fatal.
	if propErr != nil {
		v.clearDuties()
		log.WithError(propErr).Error("Error getting proposer duties")
		return propErr
	}
	if attErr != nil {
		v.clearDuties()
		log.WithError(attErr).Error("Error getting attester duties")
		return attErr
	}

	// Filter attester duties to active/exiting validators.
	includeAtt := func(d *ethpb.AttesterDuty) bool { return v.isActiveOrExiting(d.Pubkey) }
	if attCurr != nil {
		attCurr.Duties = filterSlice(attCurr.Duties, includeAtt)
	}
	if attNext != nil {
		attNext.Duties = filterSlice(attNext.Duties, includeAtt)
	}

	// Build proposer slots (merge current + next epoch).
	propSlots := proposerSlotsMap(propCurr)
	if propNext != nil {
		for _, d := range propNext.Duties {
			propSlots[d.ValidatorIndex] = append(propSlots[d.ValidatorIndex], d.Slot)
		}
	}

	var propDepRoot []byte
	if propCurr != nil {
		propDepRoot = propCurr.DependentRoot
	}

	v.dutiesLock.Lock()
	// Sync failure is non-fatal — reuse cached sync data.
	if syncErr != nil {
		log.WithError(syncErr).Warn("Error getting sync committee duties, reusing cached data")
		if v.duties != nil {
			syncCurr = v.duties.syncCurrentResp
			syncNext = v.duties.syncNextResp
			syncPeriod = v.duties.syncPeriod
		}
	} else if syncCurr == nil {
		// Sync cache hit — reuse existing.
		if v.duties != nil {
			syncCurr = v.duties.syncCurrentResp
			syncNext = v.duties.syncNextResp
			syncPeriod = v.duties.syncPeriod
		}
	}

	v.duties = &dutyStore{
		currentDuties:         attesterMap(attCurr),
		nextDuties:            attesterMap(attNext),
		attesterDependentRoot: attCurr.DependentRoot,
		proposerDependentRoot: propDepRoot,
		proposerSlots:         propSlots,
		syncCurrentMap:        syncMap(syncCurr),
		syncNextMap:           syncMap(syncNext),
		syncPeriod:            syncPeriod,
		syncCurrentResp:       syncCurr,
		syncNextResp:          syncNext,
		initialized:           true,
	}
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
		truncatedPubkey := fmt.Sprintf("%#x", bytesutil.Trunc(duty.Pubkey))
		attesterSlotInEpoch := duty.Slot - epochStartSlot
		if attesterSlotInEpoch >= params.BeaconConfig().SlotsPerEpoch {
			log.WithField("duty", duty).Warn("Invalid attester slot")
		} else {
			attesterKeys[attesterSlotInEpoch] = append(attesterKeys[attesterSlotInEpoch], truncatedPubkey)
			totalAttestingKeys++
		}
		for _, proposerSlot := range v.duties.ProposerSlots(duty.ValidatorIndex) {
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
			v.emitNextDutyMetrics(duty)
		}
	}

	log.WithFields(logrus.Fields{
		"proposerCount": totalProposingKeys,
		"attesterCount": totalAttestingKeys,
	}).Infof("Schedule for epoch %d", slots.ToEpoch(slot))
	v.logSlotSchedule(epochStartSlot, attesterKeys, proposerKeys)
}

func (v *validator) emitCurrentDutyMetrics(duty *ethpb.AttesterDuty) {
	pubkey := fmt.Sprintf("%#x", duty.Pubkey)
	ValidatorNextAttestationSlotGaugeVec.WithLabelValues(pubkey).Set(float64(duty.Slot))
	if v.duties.IsSyncCommittee(duty.ValidatorIndex) {
		ValidatorInSyncCommitteeGaugeVec.WithLabelValues(pubkey).Set(float64(1))
	} else {
		ValidatorInSyncCommitteeGaugeVec.WithLabelValues(pubkey).Set(float64(0))
	}
	for _, proposerSlot := range v.duties.ProposerSlots(duty.ValidatorIndex) {
		ValidatorNextProposalSlotGaugeVec.WithLabelValues(pubkey).Set(float64(proposerSlot))
	}
}

func (v *validator) emitNextDutyMetrics(duty *ethpb.AttesterDuty) {
	pubkey := fmt.Sprintf("%#x", duty.Pubkey)
	if v.duties.IsNextSyncCommittee(duty.ValidatorIndex) {
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
	storedAttester, storedProposer := v.duties.DependentRoots()
	if !bytes.Equal(prevRoot, storedAttester) {
		return "previous"
	}
	if bytes.Equal(currRoot, params.BeaconConfig().ZeroHash[:]) {
		return ""
	}
	if !bytes.Equal(currRoot, storedProposer) {
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
