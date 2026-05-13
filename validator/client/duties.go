package client

import (
	"bytes"
	"context"
	"fmt"
	"slices"
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

// filteredKeysAndIndices returns the subset of keys with duties to fetch for
// the given epoch (see isActiveForDuties), and the corresponding sorted
// validator indices. Sorted indices let callers compare against a previously
// stored set to detect drift.
func (v *validator) filteredKeysAndIndices(keys [][fieldparams.BLSPubkeyLength]byte, epoch primitives.Epoch) ([][fieldparams.BLSPubkeyLength]byte, []primitives.ValidatorIndex) {
	outKeys := make([][fieldparams.BLSPubkeyLength]byte, 0, len(keys))
	indices := make([]primitives.ValidatorIndex, 0, len(keys))
	for _, pk := range keys {
		st, ok := v.pubkeyToStatus[pk]
		if !ok || !isActiveForDuties(st.status, epoch) {
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

	if !v.duties.IsInitialized() {
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
	v.duties.Write(data)

	if allCurrentDutiesExited(resp.CurrentEpochDuties) {
		return ErrValidatorsAllExited
	}
	return nil
}

// depRootsDiverged reports whether the freshly fetched next-epoch attester and
// proposer dependent roots disagree.
func depRootsDiverged(epoch primitives.Epoch, res dutiesFetchResult) bool {
	if epoch < params.BeaconConfig().FuluForkEpoch {
		// Pre-fulu, attester dep root for epoch+1 is get_block_root_at_slot(
		// compute_start_slot_at_epoch(epoch) - 1)
		// proposer dep root is get_block_root_at_slot(compute_start_slot_at_epoch(epoch+1) - 1)
		// they differ by design
		return false
	}
	if res.propNext == nil || res.attNext == nil {
		return false
	}
	return !bytes.Equal(res.propNext.DependentRoot, res.attNext.DependentRoot)
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

// dutiesFetchResult holds the successful results from fetching or
// promoting current-epoch duties plus raw next-epoch API responses.
type dutiesFetchResult struct {
	currentDuties []*ethpb.ValidatorDuty
	prevDepRoot   []byte
	currDepRoot   []byte
	attNext       *ethpb.AttesterDutiesResponse
	propNext      *ethpb.ProposerDutiesResponse
	syncNext      *ethpb.SyncCommitteeDutiesResponse
	ptcNext       *ethpb.PTCDutiesResponse
	missingNext   missingNextDuties
}

// missingNextDuties is a bitmask of next-epoch duty types that were expected
// but missing after a fetch (soft failures). Tracked so the next promotion
// can fall back to a full fresh fetch instead of propagating incomplete data.
type missingNextDuties uint8

const (
	missingNextProposer missingNextDuties = 1 << iota
	missingNextSync
	missingNextPtc
)

// updateDutiesSplit fetches duties from the split V3 endpoints and
// populates the duty store. When the epoch has advanced by exactly one
// and duties are already initialized, it promotes the cached next-epoch
// duties to current and only fetches the new next-epoch. indices must be
// sorted (see filteredKeysAndIndices).
func (v *validator) updateDutiesSplit(ctx context.Context, epoch primitives.Epoch, indices []primitives.ValidatorIndex) error {
	if len(indices) == 0 {
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
		res, err = v.promoteDuties(ctx, epoch, indices)
		if err != nil {
			return errors.Wrap(err, "promote duties")
		}
	} else {
		res, err = v.fetchAllDuties(ctx, epoch, indices)
		if err != nil {
			return errors.Wrap(err, "fetch all duties")
		}
	}

	if depRootsDiverged(epoch, res) {
		if canPromote {
			log.Warn("Proposer and attester dependent roots diverged on promotion, refetching all duties")
			res, err = v.fetchAllDuties(ctx, epoch, indices)
			if err != nil {
				return errors.Wrap(err, "refetch all duties after promotion divergence")
			}
		} else {
			log.Warn("Proposer and attester dependent roots diverged on fresh fetch")
		}
	}

	nextDuties := v.buildNextDuties(res)

	var data dutyStoreData
	data.setFromContainer(&ethpb.ValidatorDutiesContainer{
		PrevDependentRoot:  res.prevDepRoot,
		CurrDependentRoot:  res.currDepRoot,
		CurrentEpochDuties: res.currentDuties,
		NextEpochDuties:    nextDuties,
	})
	data.epoch = epoch
	data.missingNext = res.missingNext
	data.indices = indices
	v.duties.Write(data)

	if allCurrentDutiesExited(res.currentDuties) {
		return ErrValidatorsAllExited
	}
	return nil
}

// promoteDuties promotes cached next-epoch duties to current and fetches the
// new next-epoch duties. Cached duties already carry PtcSlots from the prior
// fetch, so no current-epoch refetch is needed.
func (v *validator) promoteDuties(ctx context.Context, epoch primitives.Epoch, indices []primitives.ValidatorIndex) (dutiesFetchResult, error) {
	snap := v.duties.Snapshot()
	currentDuties := make([]*ethpb.ValidatorDuty, 0, len(snap.nextDuties))
	for _, d := range snap.nextDuties {
		if d == nil {
			continue
		}
		currentDuties = append(currentDuties, d)
	}
	res := dutiesFetchResult{
		currentDuties: currentDuties,
		// On promotion, last cycle's currDependentRoot (which covered next-epoch
		// duties) becomes this cycle's prevDepRoot (covering current-epoch
		// duties).
		prevDepRoot: snap.currDependentRoot,
	}

	var (
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
		if epoch.Add(1) < params.BeaconConfig().AltairForkEpoch {
			return
		}
		res.syncNext, syncErr = v.validatorClient.SyncCommitteeDuties(ctx, epoch.Add(1), indices)
	})
	wg.Go(func() {
		if epoch.Add(1) < params.BeaconConfig().GloasForkEpoch {
			return
		}
		res.ptcNext, ptcErr = v.validatorClient.PTCDuties(ctx, epoch.Add(1), indices)
	})
	wg.Wait()

	if attErr != nil {
		return res, attErr
	}
	if propErr != nil {
		log.WithError(propErr).Debug("Could not get next epoch proposer duties")
	}
	if syncErr != nil {
		log.WithError(syncErr).Debug("Could not get next epoch sync committee duties")
	}
	if ptcErr != nil {
		log.WithError(ptcErr).Debug("Could not get next epoch PTC duties")
	}

	res.missingNext = missingNextMask(epoch.Add(1), res.propNext, res.syncNext, res.ptcNext)

	// currDepRoot comes from the newly fetched next-epoch attester root,
	// which matches the head event's CurrentDutyDependentRoot.
	if res.attNext != nil {
		res.currDepRoot = res.attNext.DependentRoot
	}
	return res, nil
}

// missingNextMask reports which next-epoch duty types are missing post-fetch.
// Only types that were expected at nextEpoch (per fork gating) are flagged.
func missingNextMask(nextEpoch primitives.Epoch, prop *ethpb.ProposerDutiesResponse, sync *ethpb.SyncCommitteeDutiesResponse, ptc *ethpb.PTCDutiesResponse) missingNextDuties {
	var m missingNextDuties
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
		ptcCurr, res.ptcNext, ptcErr = v.fetchPtcDuties(ctx, epoch, indices)
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

	res.missingNext = missingNextMask(epoch.Add(1), res.propNext, res.syncNext, res.ptcNext)

	if attCurr != nil {
		res.prevDepRoot = attCurr.DependentRoot
	}
	// Use the next-epoch attester dependent root as currDepRoot.
	// The head event's CurrentDutyDependentRoot = DependentRoot(epoch),
	// and attester duties for epoch+1 have DependentRoot(epoch), so they match.
	if res.attNext != nil {
		res.currDepRoot = res.attNext.DependentRoot
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
	ptcSlots := make(map[primitives.ValidatorIndex][]primitives.Slot)
	if res.ptcNext != nil {
		for _, d := range res.ptcNext.Duties {
			ptcSlots[d.ValidatorIndex] = append(ptcSlots[d.ValidatorIndex], d.Slot)
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
				PtcSlots:                ptcSlots[d.ValidatorIndex],
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
// Post-fulu, also fetches next-epoch duties (deterministic via proposer_lookahead).
// Pre-fulu, next-epoch proposer duties are not deterministic and not fetched.
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
	if epoch >= params.BeaconConfig().FuluForkEpoch {
		wg.Go(func() {
			next, nextErr = v.validatorClient.ProposerDuties(ctx, epoch.Add(1))
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
	})
	wg.Wait()

	if currErr != nil {
		return nil, nil, currErr
	}
	if nextErr != nil {
		log.WithError(nextErr).Debug("Could not get next epoch sync committee duties")
	}
	return current, next, nil
}

// fetchPtcDuties fetches PTC duties for the current and next epoch in parallel.
func (v *validator) fetchPtcDuties(
	ctx context.Context, epoch primitives.Epoch, indices []primitives.ValidatorIndex,
) (current, next *ethpb.PTCDutiesResponse, err error) {
	if epoch < params.BeaconConfig().GloasForkEpoch {
		return nil, nil, nil
	}
	var (
		currErr, nextErr error
		wg               sync.WaitGroup
	)
	wg.Go(func() {
		current, currErr = v.validatorClient.PTCDuties(ctx, epoch, indices)
	})
	wg.Go(func() {
		next, nextErr = v.validatorClient.PTCDuties(ctx, epoch.Add(1), indices)
	})
	wg.Wait()
	if currErr != nil {
		return nil, nil, currErr
	}
	if nextErr != nil {
		log.WithError(nextErr).Debug("Could not get next epoch PTC duties")
	}
	return current, next, nil
}

// onDutiesUpdated kicks off subnet subscriptions for the current duty set.
func (v *validator) onDutiesUpdated(ctx context.Context) error {
	md, exists := metadata.FromOutgoingContext(ctx)
	ctx = context.Background()
	if exists {
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	container := v.duties.ToContainer()
	go func() {
		if err := v.subscribeToSubnets(ctx, container); err != nil {
			log.WithError(err).Error("Failed to subscribe to subnets")
		}
	}()

	return nil
}

func (v *validator) logDuties(slot primitives.Slot) {
	snap := v.duties.Snapshot()
	if !snap.initialized {
		return
	}

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

	for _, duty := range snap.currentDuties {
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
	for _, duty := range snap.nextDuties {
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

	storedPrev := v.duties.PrevDependentRoot()
	needsPrevUpdate := storedPrev == nil || !bytes.Equal(prevDependentRoot, storedPrev)

	if needsPrevUpdate {
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
	storedCurr := v.duties.CurrDependentRoot()
	needsCurrUpdate := storedCurr == nil || !bytes.Equal(currDependentRoot, storedCurr)
	if !needsCurrUpdate {
		return nil
	}
	if err := v.UpdateDuties(dutiesCtx); err != nil {
		return errors.Wrap(err, "failed to update duties")
	}
	log.Info("Updated duties due to current dependent root change")
	return nil
}
