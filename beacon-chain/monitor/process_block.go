package monitor

import (
	"context"
	"fmt"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/blocks"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
)

// AggregateReportingPeriod defines the number of epochs between aggregate reports.
const AggregateReportingPeriod = 5

// processBlock handles the cases when
// - A block was proposed by one of our tracked validators
// - An attestation by one of our tracked validators was included
// - An Exit by one of our validators was included
// - A Slashing by one of our tracked validators was included
// - A Sync Committee Contribution by one of our tracked validators was included
func (s *Service) processBlock(ctx context.Context, b interfaces.ReadOnlySignedBeaconBlock) {
	if b == nil || b.Block() == nil {
		return
	}
	blk := b.Block()

	s.processSlashings(blk)
	s.processExitsFromBlock(blk)

	root, err := blk.HashTreeRoot()
	if err != nil {
		log.WithError(err).Error("Could not compute block's hash tree root")
		return
	}
	st := s.config.StateGen.StateByRootIfCachedNoCopy(root)
	if st == nil {
		log.WithField("beaconBlockRoot", fmt.Sprintf("%#x", bytesutil.Trunc(root[:]))).Debug(
			"Skipping block collection due to state not found in cache")
		return
	}

	currEpoch := slots.ToEpoch(blk.Slot())
	s.RLock()
	lastSyncedEpoch := s.lastSyncedEpoch
	s.RUnlock()

	if currEpoch != lastSyncedEpoch &&
		slots.SyncCommitteePeriod(currEpoch) == slots.SyncCommitteePeriod(lastSyncedEpoch) {
		s.updateSyncCommitteeTrackedVals(st)
	}

	s.processSyncAggregate(st, blk)
	s.processProposedBlock(st, root, blk)
	s.processAttestations(ctx, st, blk)

	if blk.Slot()%(AggregateReportingPeriod*params.BeaconConfig().SlotsPerEpoch) == 0 {
		s.logAggregatedPerformance()
	}
}

// processProposedBlock logs when the beacon node observes a beacon block from a tracked validator.
func (s *Service) processProposedBlock(state state.BeaconState, root [32]byte, blk interfaces.ReadOnlyBeaconBlock) {
	s.Lock()
	defer s.Unlock()
	if s.trackedIndex(blk.ProposerIndex()) {
		// update metrics
		proposedSlotsCounter.WithLabelValues(fmt.Sprintf("%d", blk.ProposerIndex())).Inc()

		// update the performance map
		balance, err := state.BalanceAtIndex(blk.ProposerIndex())
		if err != nil {
			log.WithError(err).Error("Could not get balance")
			return
		}

		latestPerf := s.latestPerformance[blk.ProposerIndex()]
		balanceChg := int64(balance - latestPerf.balance)
		latestPerf.balanceChange = balanceChg
		latestPerf.balance = balance
		s.latestPerformance[blk.ProposerIndex()] = latestPerf

		aggPerf := s.aggregatedPerformance[blk.ProposerIndex()]
		aggPerf.totalProposedCount++
		s.aggregatedPerformance[blk.ProposerIndex()] = aggPerf

		parentRoot := blk.ParentRoot()
		log.WithFields(logrus.Fields{
			"proposerIndex": blk.ProposerIndex(),
			"slot":          blk.Slot(),
			"version":       blk.Version(),
			"parentRoot":    fmt.Sprintf("%#x", bytesutil.Trunc(parentRoot[:])),
			"blockRoot":     fmt.Sprintf("%#x", bytesutil.Trunc(root[:])),
			"newBalance":    balance,
			"balanceChange": balanceChg,
		}).Info("Proposed beacon block was included")
	}
}

// processSlashings logs the event when tracked validators was slashed
func (s *Service) processSlashings(blk interfaces.ReadOnlyBeaconBlock) {
	s.RLock()
	defer s.RUnlock()
	for _, slashing := range blk.Body().ProposerSlashings() {
		idx := slashing.Header_1.Header.ProposerIndex
		if s.trackedIndex(idx) {
			log.WithFields(logrus.Fields{
				"proposerIndex": idx,
				"slot":          blk.Slot(),
				"slashingSlot":  slashing.Header_1.Header.Slot,
				"bodyRoot1":     fmt.Sprintf("%#x", bytesutil.Trunc(slashing.Header_1.Header.BodyRoot)),
				"bodyRoot2":     fmt.Sprintf("%#x", bytesutil.Trunc(slashing.Header_2.Header.BodyRoot)),
			}).Info("Proposer slashing was included")
		}
	}

	for _, slashing := range blk.Body().AttesterSlashings() {
		for _, idx := range blocks.SlashableAttesterIndices(slashing) {
			if s.trackedIndex(primitives.ValidatorIndex(idx)) {
				data1 := slashing.FirstAttestation().GetData()
				data2 := slashing.SecondAttestation().GetData()

				log.WithFields(logrus.Fields{
					"attesterIndex":      idx,
					"blockInclusionSlot": blk.Slot(),
					"attestationSlot1":   data1.Slot,
					"beaconBlockRoot1":   fmt.Sprintf("%#x", bytesutil.Trunc(data1.BeaconBlockRoot)),
					"sourceEpoch1":       data1.Source.Epoch,
					"targetEpoch1":       data1.Target.Epoch,
					"attestationSlot2":   data2.Slot,
					"beaconBlockRoot2":   fmt.Sprintf("%#x", bytesutil.Trunc(data2.BeaconBlockRoot)),
					"sourceEpoch2":       data2.Source.Epoch,
					"targetEpoch2":       data2.Target.Epoch,
				}).Info("Attester slashing was included")
			}
		}
	}
}

// logAggregatedPerformance logs the collected performance statistics since the start of the service.
func (s *Service) logAggregatedPerformance() {
	s.RLock()
	defer s.RUnlock()

	for idx, p := range s.aggregatedPerformance {
		if p.totalAttestedCount == 0 || p.totalRequestedCount == 0 || p.startBalance == 0 {
			break
		}
		l, ok := s.latestPerformance[idx]
		if !ok {
			break
		}
		percentAtt := float64(p.totalAttestedCount) / float64(p.totalRequestedCount)
		percentBal := float64(l.balance-p.startBalance) / float64(p.startBalance)
		percentDistance := float64(p.totalDistance) / float64(p.totalAttestedCount)
		percentCorrectSource := float64(p.totalCorrectSource) / float64(p.totalAttestedCount)
		percentCorrectHead := float64(p.totalCorrectHead) / float64(p.totalAttestedCount)
		percentCorrectTarget := float64(p.totalCorrectTarget) / float64(p.totalAttestedCount)

		log.WithFields(logrus.Fields{
			"validatorIndex":           idx,
			"startEpoch":               p.startEpoch,
			"startBalance":             p.startBalance,
			"totalRequested":           p.totalRequestedCount,
			"attestationInclusion":     fmt.Sprintf("%.2f%%", percentAtt*100),
			"balanceChangePct":         fmt.Sprintf("%.2f%%", percentBal*100),
			"correctlyVotedSourcePct":  fmt.Sprintf("%.2f%%", percentCorrectSource*100),
			"correctlyVotedTargetPct":  fmt.Sprintf("%.2f%%", percentCorrectTarget*100),
			"correctlyVotedHeadPct":    fmt.Sprintf("%.2f%%", percentCorrectHead*100),
			"averageInclusionDistance": fmt.Sprintf("%.1f", percentDistance),
			"totalProposedBlocks":      p.totalProposedCount,
			"totalAggregations":        p.totalAggregations,
			"totalSyncContributions":   p.totalSyncCommitteeContributions,
		}).Info("Aggregated performance since launch")
	}
}
