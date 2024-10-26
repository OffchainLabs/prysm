package monitor

import (
	"context"
	"fmt"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/altair"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/attestation"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
)

// canUpdateAttestedValidator returns true if the validator is tracked and if the
// given slot is different than the last attested slot from this validator.
// It assumes that a read lock is held on the monitor service.
func (s *Service) canUpdateAttestedValidator(idx primitives.ValidatorIndex, slot primitives.Slot) bool {
	if !s.trackedIndex(idx) {
		return false
	}

	if lp, ok := s.latestPerformance[idx]; ok {
		return lp.attestedSlot != slot
	}
	return false
}

// attestingIndices returns the indices of validators that participated in the given aggregated attestation.
func attestingIndices(ctx context.Context, state state.BeaconState, att ethpb.Att) ([]uint64, error) {
	committee, err := helpers.BeaconCommitteeFromState(ctx, state, att.GetData().Slot, att.GetData().CommitteeIndex)
	if err != nil {
		return nil, err
	}
	return attestation.AttestingIndices(att, committee)
}

// logMessageTimelyFlagsForIndex returns the log message with performance info for the attestation (head, source, target)
func logMessageTimelyFlagsForIndex(idx primitives.ValidatorIndex, data *ethpb.AttestationData) logrus.Fields {
	return logrus.Fields{
		"validatorIndex": idx,
		"slot":           data.Slot,
		"source":         fmt.Sprintf("%#x", bytesutil.Trunc(data.Source.Root)),
		"target":         fmt.Sprintf("%#x", bytesutil.Trunc(data.Target.Root)),
		"head":           fmt.Sprintf("%#x", bytesutil.Trunc(data.BeaconBlockRoot)),
	}
}

// processAttestations logs the event for the tracked validators' attestations inclusion in block
func (s *Service) processAttestations(ctx context.Context, state state.BeaconState, blk interfaces.ReadOnlyBeaconBlock) {
	if blk == nil || blk.Body() == nil {
		return
	}
	for _, att := range blk.Body().Attestations() {
		s.processIncludedAttestation(ctx, state, att)
	}
}

// processIncludedAttestation logs in the event for the tracked validators' and their latest attestation gets processed.
func (s *Service) processIncludedAttestation(ctx context.Context, state state.BeaconState, att ethpb.Att) {
	attestingIndices, err := attestingIndices(ctx, state, att)
	if err != nil {
		log.WithError(err).Error("Could not get attesting indices")
		return
	}
	s.Lock()
	defer s.Unlock()
	for _, idx := range attestingIndices {
		if s.canUpdateAttestedValidator(primitives.ValidatorIndex(idx), att.GetData().Slot) {
			logFields := logMessageTimelyFlagsForIndex(primitives.ValidatorIndex(idx), att.GetData())
			balance, err := state.BalanceAtIndex(primitives.ValidatorIndex(idx))
			if err != nil {
				log.WithError(err).Error("Could not get balance")
				return
			}

			aggregatedPerf := s.aggregatedPerformance[primitives.ValidatorIndex(idx)]
			aggregatedPerf.totalAttestedCount++
			aggregatedPerf.totalRequestedCount++

			latestPerf := s.latestPerformance[primitives.ValidatorIndex(idx)]
			balanceChg := int64(balance - latestPerf.balance)
			latestPerf.balanceChange = balanceChg
			latestPerf.balance = balance
			latestPerf.attestedSlot = att.GetData().Slot
			latestPerf.inclusionSlot = state.Slot()
			inclusionSlotGauge.WithLabelValues(fmt.Sprintf("%d", idx)).Set(float64(latestPerf.inclusionSlot))
			aggregatedPerf.totalDistance += uint64(latestPerf.inclusionSlot - latestPerf.attestedSlot)

			if state.Version() == version.Altair {
				targetIdx := params.BeaconConfig().TimelyTargetFlagIndex
				sourceIdx := params.BeaconConfig().TimelySourceFlagIndex
				headIdx := params.BeaconConfig().TimelyHeadFlagIndex

				var participation []byte
				if slots.ToEpoch(latestPerf.inclusionSlot) ==
					slots.ToEpoch(latestPerf.attestedSlot) {
					participation, err = state.CurrentEpochParticipation()
					if err != nil {
						log.WithError(err).Error("Could not get current epoch participation")
						return
					}
				} else {
					participation, err = state.PreviousEpochParticipation()
					if err != nil {
						log.WithError(err).Error("Could not get previous epoch participation")
						return
					}
				}
				flags := participation[idx]
				hasFlag, err := altair.HasValidatorFlag(flags, sourceIdx)
				if err != nil {
					log.WithError(err).Error("Could not get timely Source flag")
					return
				}
				latestPerf.timelySource = hasFlag
				hasFlag, err = altair.HasValidatorFlag(flags, headIdx)
				if err != nil {
					log.WithError(err).Error("Could not get timely Head flag")
					return
				}
				latestPerf.timelyHead = hasFlag
				hasFlag, err = altair.HasValidatorFlag(flags, targetIdx)
				if err != nil {
					log.WithError(err).Error("Could not get timely Target flag")
					return
				}
				latestPerf.timelyTarget = hasFlag

				if latestPerf.timelySource {
					timelySourceCounter.WithLabelValues(fmt.Sprintf("%d", idx)).Inc()
					aggregatedPerf.totalCorrectSource++
				}
				if latestPerf.timelyHead {
					timelyHeadCounter.WithLabelValues(fmt.Sprintf("%d", idx)).Inc()
					aggregatedPerf.totalCorrectHead++
				}
				if latestPerf.timelyTarget {
					timelyTargetCounter.WithLabelValues(fmt.Sprintf("%d", idx)).Inc()
					aggregatedPerf.totalCorrectTarget++
				}
			}
			logFields["correctHead"] = latestPerf.timelyHead
			logFields["correctSource"] = latestPerf.timelySource
			logFields["correctTarget"] = latestPerf.timelyTarget
			logFields["inclusionSlot"] = latestPerf.inclusionSlot
			logFields["newBalance"] = balance
			logFields["balanceChange"] = balanceChg

			s.latestPerformance[primitives.ValidatorIndex(idx)] = latestPerf
			s.aggregatedPerformance[primitives.ValidatorIndex(idx)] = aggregatedPerf
			log.WithFields(logFields).Info("Attestation included")
		}
	}
}

// processUnaggregatedAttestation logs when the beacon node observes an unaggregated attestation from tracked validator.
func (s *Service) processUnaggregatedAttestation(ctx context.Context, att ethpb.Att) {
	s.RLock()
	defer s.RUnlock()
	root := bytesutil.ToBytes32(att.GetData().BeaconBlockRoot)
	st := s.config.StateGen.StateByRootIfCachedNoCopy(root)
	if st == nil {
		log.WithField("beaconBlockRoot", fmt.Sprintf("%#x", bytesutil.Trunc(root[:]))).Debug(
			"Skipping unaggregated attestation due to state not found in cache")
		return
	}
	attestingIndices, err := attestingIndices(ctx, st, att)
	if err != nil {
		log.WithError(err).Error("Could not get attesting indices")
		return
	}
	for _, idx := range attestingIndices {
		if s.canUpdateAttestedValidator(primitives.ValidatorIndex(idx), att.GetData().Slot) {
			logFields := logMessageTimelyFlagsForIndex(primitives.ValidatorIndex(idx), att.GetData())
			log.WithFields(logFields).Info("Processed unaggregated attestation")
		}
	}
}

// processUnaggregatedAttestation logs when the beacon node observes an aggregated attestation from tracked validator.
func (s *Service) processAggregatedAttestation(ctx context.Context, att ethpb.AggregateAttAndProof) {
	s.Lock()
	defer s.Unlock()
	if s.trackedIndex(att.GetAggregatorIndex()) {
		log.WithFields(logrus.Fields{
			"aggregatorIndex": att.GetAggregatorIndex(),
			"slot":            att.AggregateVal().GetData().Slot,
			"beaconBlockRoot": fmt.Sprintf("%#x", bytesutil.Trunc(
				att.AggregateVal().GetData().BeaconBlockRoot)),
			"sourceRoot": fmt.Sprintf("%#x", bytesutil.Trunc(
				att.AggregateVal().GetData().Source.Root)),
			"targetRoot": fmt.Sprintf("%#x", bytesutil.Trunc(
				att.AggregateVal().GetData().Target.Root)),
		}).Info("Processed attestation aggregation")
		aggregatedPerf := s.aggregatedPerformance[att.GetAggregatorIndex()]
		aggregatedPerf.totalAggregations++
		s.aggregatedPerformance[att.GetAggregatorIndex()] = aggregatedPerf
		aggregationCounter.WithLabelValues(fmt.Sprintf("%d", att.GetAggregatorIndex())).Inc()
	}

	var root [32]byte
	copy(root[:], att.AggregateVal().GetData().BeaconBlockRoot)
	st := s.config.StateGen.StateByRootIfCachedNoCopy(root)
	if st == nil {
		log.WithField("beaconBlockRoot", fmt.Sprintf("%#x", bytesutil.Trunc(root[:]))).Debug(
			"Skipping aggregated attestation due to state not found in cache")
		return
	}
	attestingIndices, err := attestingIndices(ctx, st, att.AggregateVal())
	if err != nil {
		log.WithError(err).Error("Could not get attesting indices")
		return
	}
	for _, idx := range attestingIndices {
		if s.canUpdateAttestedValidator(primitives.ValidatorIndex(idx), att.AggregateVal().GetData().Slot) {
			logFields := logMessageTimelyFlagsForIndex(primitives.ValidatorIndex(idx), att.AggregateVal().GetData())
			log.WithFields(logFields).Info("Processed aggregated attestation")
		}
	}
}
