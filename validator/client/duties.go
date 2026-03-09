package client

import (
	"bytes"
	"context"
	"fmt"
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
	req := &ethpb.DutiesRequest{
		Epoch:      epoch,
		PublicKeys: bytesutil.FromBytes48Array(filteredKeys),
	}

	resp, err := v.validatorClient.Duties(ctx, req)
	if err != nil || resp == nil {
		v.dutiesLock.Lock()
		v.duties = nil
		v.dutiesLock.Unlock()
		log.WithError(err).Error("Error getting validator duties")
		return err
	}

	ss, err := slots.EpochStart(epoch)
	if err != nil {
		return err
	}
	v.dutiesLock.Lock()
	v.duties = resp
	v.logDuties(ss, v.duties.CurrentEpochDuties, v.duties.NextEpochDuties)
	v.dutiesLock.Unlock()

	allExitedCounter := 0
	for i := range resp.CurrentEpochDuties {
		if resp.CurrentEpochDuties[i].Status == ethpb.ValidatorStatus_EXITED {
			allExitedCounter++
		}
	}
	if allExitedCounter != 0 && allExitedCounter == len(resp.CurrentEpochDuties) {
		return ErrValidatorsAllExited
	}

	// Non-blocking call for beacon node to start subscriptions for aggregators.
	md, exists := metadata.FromOutgoingContext(ctx)
	ctx = context.Background()
	if exists {
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	go func() {
		if err := v.subscribeToSubnets(ctx, resp); err != nil {
			log.WithError(err).Error("Failed to subscribe to subnets")
		}
	}()

	return nil
}

func (v *validator) logDuties(slot primitives.Slot, currentEpochDuties []*ethpb.ValidatorDuty, nextEpochDuties []*ethpb.ValidatorDuty) {
	attesterKeys := make([][]string, params.BeaconConfig().SlotsPerEpoch)
	ptcKeys := make([][]string, params.BeaconConfig().SlotsPerEpoch)
	for i := range attesterKeys {
		attesterKeys[i] = make([]string, 0)
		ptcKeys[i] = make([]string, 0)
	}
	proposerKeys := make([]string, params.BeaconConfig().SlotsPerEpoch)
	epochStartSlot, err := slots.EpochStart(slots.ToEpoch(slot))
	if err != nil {
		log.WithError(err).Error("Could not calculate epoch start. Ignoring logging duties.")
		return
	}
	var totalProposingKeys, totalAttestingKeys, totalPTCKeys uint64
	for _, duty := range currentEpochDuties {
		pubkey := fmt.Sprintf("%#x", duty.PublicKey)
		if v.emitAccountMetrics {
			ValidatorStatusesGaugeVec.WithLabelValues(pubkey, fmt.Sprintf("%#x", duty.ValidatorIndex)).Set(float64(duty.Status))
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
				ValidatorNextAttestationSlotGaugeVec.WithLabelValues(pubkey).Set(float64(duty.AttesterSlot))
			}
		}
		if v.emitAccountMetrics && duty.IsSyncCommittee {
			ValidatorInSyncCommitteeGaugeVec.WithLabelValues(pubkey).Set(float64(1))
		} else if v.emitAccountMetrics && !duty.IsSyncCommittee {
			ValidatorInSyncCommitteeGaugeVec.WithLabelValues(pubkey).Set(float64(0))
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
				ValidatorNextProposalSlotGaugeVec.WithLabelValues(pubkey).Set(float64(proposerSlot))
			}
		}
	}
	for _, duty := range nextEpochDuties {
		pubkey := fmt.Sprintf("%#x", duty.PublicKey)
		if duty.Status != ethpb.ValidatorStatus_ACTIVE && duty.Status != ethpb.ValidatorStatus_EXITING {
			continue
		}
		if v.emitAccountMetrics && duty.IsSyncCommittee {
			ValidatorInNextSyncCommitteeGaugeVec.WithLabelValues(pubkey).Set(float64(1))
		} else if v.emitAccountMetrics && !duty.IsSyncCommittee {
			ValidatorInNextSyncCommitteeGaugeVec.WithLabelValues(pubkey).Set(float64(0))
		}
	}

	log.WithFields(logrus.Fields{
		"proposerCount": totalProposingKeys,
		"attesterCount": totalAttestingKeys,
		"ptcCount":      totalPTCKeys,
	}).Infof("Schedule for epoch %d", slots.ToEpoch(slot))
	for i := primitives.Slot(0); i < params.BeaconConfig().SlotsPerEpoch; i++ {
		startTime, err := slots.StartTime(v.genesisTime, epochStartSlot+i)
		if err != nil {
			log.WithError(err).WithField("slot", slot).Error("Slot overflows, unable to log duties!")
			return
		}
		durationTillDuty := (time.Until(startTime) + time.Second).Truncate(time.Second)

		slotLog := log.WithFields(logrus.Fields{})
		isProposer := proposerKeys[i] != ""
		if isProposer {
			slotLog = slotLog.WithField("proposerPubkey", proposerKeys[i])
		}
		isAttester := len(attesterKeys[i]) > 0
		if isAttester {
			slotLog = slotLog.WithFields(logrus.Fields{
				"slot":            epochStartSlot + i,
				"slotInEpoch":     (epochStartSlot + i) % params.BeaconConfig().SlotsPerEpoch,
				"attesterCount":   len(attesterKeys[i]),
				"attesterPubkeys": attesterKeys[i],
			})
		}
		isPTCMember := len(ptcKeys[i]) > 0
		if isPTCMember {
			slotLog = slotLog.WithFields(logrus.Fields{
				"ptcCount":   len(ptcKeys[i]),
				"ptcPubkeys": ptcKeys[i],
			})
		}
		if durationTillDuty > 0 {
			slotLog = slotLog.WithField("timeUntilDuty", durationTillDuty)
		}
		if isProposer || isAttester || isPTCMember {
			slotLog.Infof("Duties schedule")
		}
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
	deadline := v.SlotDeadline(ss - 1)
	dutiesCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	v.dutiesLock.RLock()
	needsPrevDependentRootUpdate := v.duties == nil || !bytes.Equal(prevDependentRoot, v.duties.PrevDependentRoot)
	v.dutiesLock.RUnlock()
	if needsPrevDependentRootUpdate {
		if err := v.UpdateDuties(dutiesCtx); err != nil {
			return errors.Wrap(err, "failed to update duties")
		}
		log.Info("Updated duties due to previous dependent root change")
		return nil
	}
	currDepedentRoot, err := bytesutil.DecodeHexWithLength(head.CurrentDutyDependentRoot, fieldparams.RootLength)
	if err != nil {
		return errors.Wrap(err, "failed to decode current duty dependent root")
	}
	if bytes.Equal(currDepedentRoot, params.BeaconConfig().ZeroHash[:]) {
		return nil
	}
	v.dutiesLock.RLock()
	needsCurrDependentRootUpdate := v.duties == nil || !bytes.Equal(currDepedentRoot, v.duties.CurrDependentRoot)
	v.dutiesLock.RUnlock()
	if !needsCurrDependentRootUpdate {
		return nil
	}
	if err := v.UpdateDuties(dutiesCtx); err != nil {
		return errors.Wrap(err, "failed to update duties")
	}
	log.Info("Updated duties due to current dependent root change")
	return nil
}
