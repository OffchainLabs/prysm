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

// UpdateDuties checks the slot number to determine if the validator's
// list of upcoming assignments needs to be updated. For example, at the
// beginning of a new epoch.
func (v *validator) UpdateDuties(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "validator.UpdateDuties")
	defer span.End()

	filteredKeys, err := v.filterBlacklistedKeys(ctx)
	if err != nil {
		return err
	}

	epoch := slots.ToEpoch(slots.CurrentSlot(v.genesisTime) + 1)

	if epoch >= params.BeaconConfig().GloasForkEpoch {
		err = v.updateDutiesSplit(ctx, epoch, filteredKeys)
	} else {
		err = v.updateDutiesCombined(ctx, epoch, filteredKeys)
	}
	if err != nil {
		return err
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

// clearDuties resets the duty store under lock.
func (v *validator) clearDuties() {
	v.dutiesLock.Lock()
	defer v.dutiesLock.Unlock()
	if v.duties == nil {
		v.duties = &dutyStore{}
	}
	v.duties.Reset()
}

// updateDutiesCombined uses the combined Duties() endpoint (pre-GLOAS).
func (v *validator) updateDutiesCombined(ctx context.Context, epoch primitives.Epoch, filteredKeys [][fieldparams.BLSPubkeyLength]byte) error {
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
	v.duties.SetFromCombinedDutiesResponse(resp)
	v.dutiesLock.Unlock()

	allExitedCounter := 0
	for _, d := range resp.CurrentEpochDuties {
		if d.Status == ethpb.ValidatorStatus_EXITED {
			allExitedCounter++
		}
	}
	if allExitedCounter != 0 && allExitedCounter == len(resp.CurrentEpochDuties) {
		return ErrValidatorsAllExited
	}
	return nil
}

// dutiesFetchResult holds the successful results from fetching or
// promoting current-epoch duties plus raw next-epoch API responses.
type dutiesFetchResult struct {
	currentDuties []*ethpb.ValidatorDuty
	prevDepRoot   []byte
	currDepRoot   []byte
	attNext       *ethpb.AttesterDutiesResponse
	propNext      *ethpb.ProposerDutiesResponse
	syncNext      *ethpb.SyncCommitteeDutiesResponse
}

// updateDutiesSplit fetches duties from the split V3 endpoints and
// populates the duty store. When the epoch has advanced by exactly one
// and duties are already initialized, it promotes the cached next-epoch
// duties to current and only fetches the new next-epoch.
func (v *validator) updateDutiesSplit(ctx context.Context, epoch primitives.Epoch, filteredKeys [][fieldparams.BLSPubkeyLength]byte) error {
	indices := make([]primitives.ValidatorIndex, 0, len(filteredKeys))
	for _, pk := range filteredKeys {
		if st, ok := v.pubkeyToStatus[pk]; ok && st.status != nil && st.status.Status != ethpb.ValidatorStatus_UNKNOWN_STATUS {
			indices = append(indices, st.index)
		}
	}
	if len(indices) == 0 {
		v.clearDuties()
		return nil
	}

	v.dutiesLock.RLock()
	epochAdvanced := v.duties != nil && v.duties.IsInitialized() && v.duties.epoch+1 == epoch
	v.dutiesLock.RUnlock()

	var (
		res dutiesFetchResult
		err error
	)
	if epochAdvanced {
		res, err = v.promoteDuties(ctx, epoch, indices)
	} else {
		res, err = v.fetchAllDuties(ctx, epoch, indices)
	}
	if err != nil {
		v.clearDuties()
		return err
	}

	nextDuties := v.buildNextDuties(res)

	container := &ethpb.ValidatorDutiesContainer{
		PrevDependentRoot:  res.prevDepRoot,
		CurrDependentRoot:  res.currDepRoot,
		CurrentEpochDuties: res.currentDuties,
		NextEpochDuties:    nextDuties,
	}
	v.dutiesLock.Lock()
	v.duties.SetFromCombinedDutiesResponse(container)
	v.duties.epoch = epoch
	if res.attNext != nil {
		v.duties.nextAttDepRoot = res.attNext.DependentRoot
	}
	if res.propNext != nil {
		v.duties.nextPropDepRoot = res.propNext.DependentRoot
	}
	v.dutiesLock.Unlock()

	if epochAdvanced {
		log.WithField("epoch", epoch).Debug("Advanced duties from previous next-epoch cache")
	}
	return nil
}

// promoteDuties promotes the cached next-epoch duties to current and
// fetches only the new next-epoch duties plus current-epoch PTC.
func (v *validator) promoteDuties(ctx context.Context, epoch primitives.Epoch, indices []primitives.ValidatorIndex) (dutiesFetchResult, error) {
	v.dutiesLock.RLock()
	oldNext := v.duties.NextEpochDuties()
	currentDuties := make([]*ethpb.ValidatorDuty, 0, len(oldNext))
	for _, d := range oldNext {
		currentDuties = append(currentDuties, d)
	}
	res := dutiesFetchResult{
		currentDuties: currentDuties,
		prevDepRoot:   v.duties.nextAttDepRoot,
		currDepRoot:   v.duties.nextPropDepRoot,
	}
	v.dutiesLock.RUnlock()

	var (
		ptcCurr         *ethpb.PTCDutiesResponse
		attErr, propErr error
		syncErr, ptcErr error
		wg              sync.WaitGroup
	)
	wg.Go(func() {
		res.attNext, attErr = v.validatorClient.AttesterDuties(ctx, epoch.Add(1), indices)
	})
	wg.Go(func() {
		res.propNext, propErr = v.validatorClient.ProposerDuties(ctx, epoch.Add(1))
	})
	wg.Go(func() {
		res.syncNext, syncErr = v.validatorClient.SyncCommitteeDuties(ctx, epoch.Add(1), indices)
	})
	wg.Go(func() {
		ptcCurr, ptcErr = v.fetchPtcDuties(ctx, epoch, indices)
	})
	wg.Wait()

	if attErr != nil {
		return res, attErr
	}
	if propErr != nil {
		return res, propErr
	}
	if syncErr != nil {
		log.WithError(syncErr).Warn("Error getting sync committee duties")
	}
	if ptcErr != nil {
		log.WithError(ptcErr).Warn("Error getting PTC duties")
	}

	if ptcCurr != nil {
		ptcSlots := make(map[primitives.ValidatorIndex][]primitives.Slot)
		for _, d := range ptcCurr.Duties {
			ptcSlots[d.ValidatorIndex] = append(ptcSlots[d.ValidatorIndex], d.Slot)
		}
		for _, d := range res.currentDuties {
			d.PtcSlots = ptcSlots[d.ValidatorIndex]
		}
	}
	return res, nil
}

// fetchAllDuties fetches both current and next epoch duties from all endpoints.
func (v *validator) fetchAllDuties(ctx context.Context, epoch primitives.Epoch, indices []primitives.ValidatorIndex) (dutiesFetchResult, error) {
	var (
		res             dutiesFetchResult
		attCurr         *ethpb.AttesterDutiesResponse
		propCurr        *ethpb.ProposerDutiesResponse
		syncCurr        *ethpb.SyncCommitteeDutiesResponse
		ptcCurr         *ethpb.PTCDutiesResponse
		attErr, propErr error
		syncErr, ptcErr error
		wg              sync.WaitGroup
	)
	wg.Go(func() {
		attCurr, res.attNext, attErr = v.fetchAttesterDuties(ctx, epoch, indices)
	})
	wg.Go(func() {
		propCurr, res.propNext, propErr = v.fetchProposerDuties(ctx, epoch)
	})
	wg.Go(func() {
		syncCurr, res.syncNext, syncErr = v.fetchSyncDuties(ctx, epoch, indices)
	})
	wg.Go(func() {
		ptcCurr, ptcErr = v.fetchPtcDuties(ctx, epoch, indices)
	})
	wg.Wait()

	if attErr != nil {
		return res, attErr
	}
	if propErr != nil {
		return res, propErr
	}
	if syncErr != nil {
		log.WithError(syncErr).Warn("Error getting sync committee duties")
	}
	if ptcErr != nil {
		log.WithError(ptcErr).Warn("Error getting PTC duties")
	}

	if attCurr != nil {
		res.prevDepRoot = attCurr.DependentRoot
	}
	if propCurr != nil {
		res.currDepRoot = propCurr.DependentRoot
	}

	proposerSlots := make(map[primitives.ValidatorIndex][]primitives.Slot)
	if propCurr != nil {
		for _, d := range propCurr.Duties {
			proposerSlots[d.ValidatorIndex] = append(proposerSlots[d.ValidatorIndex], d.Slot)
		}
	}
	ptcSlots := make(map[primitives.ValidatorIndex][]primitives.Slot)
	if ptcCurr != nil {
		for _, d := range ptcCurr.Duties {
			ptcSlots[d.ValidatorIndex] = append(ptcSlots[d.ValidatorIndex], d.Slot)
		}
	}
	syncSet := make(map[primitives.ValidatorIndex]bool)
	if syncCurr != nil {
		for _, d := range syncCurr.Duties {
			syncSet[d.ValidatorIndex] = true
		}
	}
	if attCurr != nil {
		for _, d := range attCurr.Duties {
			res.currentDuties = append(res.currentDuties, &ethpb.ValidatorDuty{
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
	}
	return res, nil
}

// buildNextDuties constructs next-epoch ValidatorDuty entries from
// the raw API responses in the fetch result.
func (v *validator) buildNextDuties(res dutiesFetchResult) []*ethpb.ValidatorDuty {
	proposerSlots := make(map[primitives.ValidatorIndex][]primitives.Slot)
	if res.propNext != nil {
		for _, d := range res.propNext.Duties {
			proposerSlots[d.ValidatorIndex] = append(proposerSlots[d.ValidatorIndex], d.Slot)
		}
	}
	syncSet := make(map[primitives.ValidatorIndex]bool)
	if res.syncNext != nil {
		for _, d := range res.syncNext.Duties {
			syncSet[d.ValidatorIndex] = true
		}
	}
	var duties []*ethpb.ValidatorDuty
	if res.attNext != nil {
		for _, d := range res.attNext.Duties {
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
				Status:                  v.statusForPubkey(d.Pubkey),
			})
		}
	}
	return duties
}

// statusForPubkey returns the cached validator status for a pubkey.
func (v *validator) statusForPubkey(pk []byte) ethpb.ValidatorStatus {
	if v.pubkeyToStatus == nil {
		return ethpb.ValidatorStatus_UNKNOWN_STATUS
	}
	st, ok := v.pubkeyToStatus[bytesutil.ToBytes48(pk)]
	if !ok || st.status == nil {
		return ethpb.ValidatorStatus_UNKNOWN_STATUS
	}
	return st.status.Status
}

// fetchAttesterDuties fetches attester duties for current and next epoch in parallel.
func (v *validator) fetchAttesterDuties(
	ctx context.Context, epoch primitives.Epoch, indices []primitives.ValidatorIndex,
) (current, next *ethpb.AttesterDutiesResponse, err error) {
	var (
		currErr, nextErr error
		wg               sync.WaitGroup
	)
	wg.Go(func() {
		current, currErr = v.validatorClient.AttesterDuties(ctx, epoch, indices)
	})
	wg.Go(func() {
		next, nextErr = v.validatorClient.AttesterDuties(ctx, epoch.Add(1), indices)
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

// fetchProposerDuties fetches proposer duties for the current epoch.
// Post-Fulu, also fetches next-epoch duties (deterministic via proposer_lookahead).
func (v *validator) fetchProposerDuties(
	ctx context.Context, epoch primitives.Epoch,
) (current, next *ethpb.ProposerDutiesResponse, err error) {
	var (
		currErr, nextErr error
		wg               sync.WaitGroup
	)
	wg.Go(func() {
		current, currErr = v.validatorClient.ProposerDuties(ctx, epoch)
	})
	wg.Go(func() {
		next, nextErr = v.validatorClient.ProposerDuties(ctx, epoch.Add(1))
	})
	wg.Wait()

	if currErr != nil {
		return nil, nil, currErr
	}
	if nextErr != nil {
		log.WithError(nextErr).Debug("Could not get next epoch proposer duties")
	}
	return current, next, nil
}

// fetchSyncDuties fetches sync committee duties for current and next epoch.
func (v *validator) fetchSyncDuties(
	ctx context.Context, epoch primitives.Epoch, indices []primitives.ValidatorIndex,
) (current, next *ethpb.SyncCommitteeDutiesResponse, err error) {
	if epoch < params.BeaconConfig().AltairForkEpoch {
		return nil, nil, nil
	}

	var (
		currErr, nextErr error
		wg               sync.WaitGroup
	)
	wg.Go(func() {
		current, currErr = v.validatorClient.SyncCommitteeDuties(ctx, epoch, indices)
	})
	wg.Go(func() {
		next, nextErr = v.validatorClient.SyncCommitteeDuties(ctx, epoch.Add(1), indices)
		if nextErr != nil {
			log.WithError(nextErr).Debug("Could not get next epoch sync committee duties")
			nextErr = nil
		}
	})
	wg.Wait()

	if currErr != nil {
		return nil, nil, currErr
	}
	return current, next, nil
}

// fetchPtcDuties fetches PTC duties for the current epoch.
// PTC assignments are not stable for the next epoch, so only fetch the current one.
func (v *validator) fetchPtcDuties(
	ctx context.Context, epoch primitives.Epoch, indices []primitives.ValidatorIndex,
) (*ethpb.PTCDutiesResponse, error) {
	if epoch < params.BeaconConfig().GloasForkEpoch {
		return nil, nil
	}
	return v.validatorClient.PTCDuties(ctx, epoch, indices)
}

// onDutiesUpdated checks for all-exited validators and starts subnet subscriptions.
func (v *validator) onDutiesUpdated(ctx context.Context) error {
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

	md, exists := metadata.FromOutgoingContext(ctx)
	ctx = context.Background()
	if exists {
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	go func() {
		if err := v.subscribeToSubnets(ctx, v.duties.ToContainer()); err != nil {
			log.WithError(err).Error("Failed to subscribe to subnets")
		}
	}()

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
	ptcKeys := make([][]string, params.BeaconConfig().SlotsPerEpoch)
	for i := range attesterKeys {
		attesterKeys[i] = make([]string, 0)
		ptcKeys[i] = make([]string, 0)
	}
	var totalProposingKeys, totalAttestingKeys, totalPTCKeys uint64

	for _, duty := range v.duties.CurrentEpochDuties() {
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
	for _, duty := range v.duties.NextEpochDuties() {
		pk := fmt.Sprintf("%#x", duty.PublicKey)
		if duty.Status != ethpb.ValidatorStatus_ACTIVE && duty.Status != ethpb.ValidatorStatus_EXITING {
			continue
		}
		if v.emitAccountMetrics && duty.IsSyncCommittee {
			ValidatorInNextSyncCommitteeGaugeVec.WithLabelValues(pk).Set(float64(1))
		} else if v.emitAccountMetrics && !duty.IsSyncCommittee {
			ValidatorInNextSyncCommitteeGaugeVec.WithLabelValues(pk).Set(float64(0))
		}
	}

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
	epoch := slots.ToEpoch(slots.CurrentSlot(v.genesisTime) + 1)
	ss, err := slots.EpochStart(epoch + 1)
	if err != nil {
		return errors.Wrap(err, "failed to get epoch start")
	}
	dutiesCtx, cancel := context.WithDeadline(ctx, v.SlotDeadline(ss-1))
	defer cancel()

	v.dutiesLock.RLock()
	storedPrev, _ := v.duties.DependentRoots()
	needsPrevUpdate := storedPrev == nil || !bytes.Equal(prevDependentRoot, storedPrev)
	v.dutiesLock.RUnlock()

	if needsPrevUpdate {
		v.clearDuties()
		if err := v.UpdateDuties(dutiesCtx); err != nil {
			return errors.Wrap(err, "failed to update duties")
		}
		log.Info("Updated duties due to previous dependent root change")
		return nil
	}

	currDependentRoot, err := bytesutil.DecodeHexWithLength(head.CurrentDutyDependentRoot, fieldparams.RootLength)
	if err != nil {
		return errors.Wrap(err, "failed to decode current duty dependent root")
	}
	if bytes.Equal(currDependentRoot, params.BeaconConfig().ZeroHash[:]) {
		return nil
	}
	v.dutiesLock.RLock()
	_, storedCurr := v.duties.DependentRoots()
	v.dutiesLock.RUnlock()
	needsCurrUpdate := storedCurr == nil || !bytes.Equal(currDependentRoot, storedCurr)
	if !needsCurrUpdate {
		return nil
	}
	v.clearDuties()
	if err := v.UpdateDuties(dutiesCtx); err != nil {
		return errors.Wrap(err, "failed to update duties")
	}
	log.Info("Updated duties due to current dependent root change")
	return nil
}
