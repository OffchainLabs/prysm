package blockchain

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	consensus_types "github.com/OffchainLabs/prysm/v7/consensus-types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/io/logs"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	prysmTime "github.com/OffchainLabs/prysm/v7/time"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// logs state transition related data every slot.
func logStateTransitionData(b interfaces.ReadOnlyBeaconBlock) error {
	log := log.WithField("slot", b.Slot())
	if len(b.Body().Attestations()) > 0 {
		log = log.WithField("attestations", len(b.Body().Attestations()))
	}
	if len(b.Body().AttesterSlashings()) > 0 {
		log = log.WithField("attesterSlashings", len(b.Body().AttesterSlashings()))
	}
	if len(b.Body().ProposerSlashings()) > 0 {
		log = log.WithField("proposerSlashings", len(b.Body().ProposerSlashings()))
	}
	if len(b.Body().VoluntaryExits()) > 0 {
		log = log.WithField("voluntaryExits", len(b.Body().VoluntaryExits()))
	}
	if b.Version() >= version.Altair {
		agg, err := b.Body().SyncAggregate()
		if err != nil {
			return err
		}
		log = log.WithField("syncBitsCount", agg.SyncCommitteeBits.Count())
	}
	if b.Version() >= version.Bellatrix && b.Version() < version.Gloas {
		p, err := b.Body().Execution()
		if err != nil {
			return err
		}
		log = log.WithField("payloadHash", fmt.Sprintf("%#x", bytesutil.Trunc(p.BlockHash())))
		txs, err := p.Transactions()
		switch {
		case errors.Is(err, consensus_types.ErrUnsupportedField):
		case err != nil:
			return err
		default:
			log = log.WithField("txCount", len(txs))
			txsPerSlotCount.Set(float64(len(txs)))
		}
	}
	if b.Version() >= version.Deneb && b.Version() < version.Gloas {
		kzgs, err := b.Body().BlobKzgCommitments()
		if err != nil {
			log.WithError(err).Error("Failed to get blob KZG commitments")
		} else if len(kzgs) > 0 {
			log = log.WithField("kzgCommitmentCount", len(kzgs))
		}
	}
	if b.Version() >= version.Electra && b.Version() < version.Gloas {
		eReqs, err := b.Body().ExecutionRequests()
		if err != nil {
			log.WithError(err).Error("Failed to get execution requests")
		} else {
			if len(eReqs.Deposits) > 0 {
				log = log.WithField("depositRequestCount", len(eReqs.Deposits))
			}
			if len(eReqs.Consolidations) > 0 {
				log = log.WithField("consolidationRequestCount", len(eReqs.Consolidations))
			}
			if len(eReqs.Withdrawals) > 0 {
				log = log.WithField("withdrawalRequestCount", len(eReqs.Withdrawals))
			}
		}
	}
	if b.Version() >= version.Gloas {
		signedBid, err := b.Body().SignedExecutionPayloadBid()
		if err != nil {
			log.WithError(err).Error("Failed to get signed execution payload bid")
		} else {
			bid := signedBid.Message
			log = log.WithFields(logrus.Fields{
				"blobKzgCommitmentCount": len(bid.BlobKzgCommitments),
				"payloadHash":            fmt.Sprintf("%#x", bytesutil.Trunc(bid.BlockHash)),
				"parentHash":             fmt.Sprintf("%#x", bytesutil.Trunc(bid.ParentBlockHash)),
				"builderIndex":           bid.BuilderIndex,
			})
		}
	}
	log.Info("Finished applying state transition")
	return nil
}

func logBlockSyncStatus(block interfaces.ReadOnlyBeaconBlock, blockRoot [32]byte, justified, finalized *ethpb.Checkpoint, receivedTime time.Time, genesis time.Time, daWaitedTime time.Duration) error {
	startTime, err := slots.StartTime(genesis, block.Slot())
	if err != nil {
		return errors.Wrap(err, "failed to get slot start time")
	}
	parentRoot := block.ParentRoot()
	blkRoot := fmt.Sprintf("0x%s...", hex.EncodeToString(blockRoot[:])[:8])
	finalizedRoot := fmt.Sprintf("0x%s...", hex.EncodeToString(finalized.Root)[:8])
	sinceSlotStartTime := prysmTime.Now().Sub(startTime)

	lessFields := logrus.Fields{
		"slot":               block.Slot(),
		"block":              blkRoot,
		"finalizedEpoch":     finalized.Epoch,
		"finalizedRoot":      finalizedRoot,
		"epoch":              slots.ToEpoch(block.Slot()),
		"sinceSlotStartTime": sinceSlotStartTime,
	}
	moreFields := logrus.Fields{
		"slot":                      block.Slot(),
		"slotInEpoch":               block.Slot() % params.BeaconConfig().SlotsPerEpoch,
		"block":                     blkRoot,
		"epoch":                     slots.ToEpoch(block.Slot()),
		"justifiedEpoch":            justified.Epoch,
		"justifiedRoot":             fmt.Sprintf("0x%s...", hex.EncodeToString(justified.Root)[:8]),
		"finalizedEpoch":            finalized.Epoch,
		"finalizedRoot":             finalizedRoot,
		"parentRoot":                fmt.Sprintf("0x%s...", hex.EncodeToString(parentRoot[:])[:8]),
		"version":                   version.String(block.Version()),
		"sinceSlotStartTime":        sinceSlotStartTime,
		"chainServiceProcessedTime": prysmTime.Now().Sub(receivedTime) - daWaitedTime,
	}
	if block.Version() < version.Gloas {
		moreFields["dataAvailabilityWaitedTime"] = daWaitedTime
	}

	level := logs.PackageVerbosity("beacon-chain/blockchain")
	if level >= logrus.DebugLevel {
		log.WithFields(moreFields).Info("Synced new block")
		return nil
	}

	log.WithFields(lessFields).WithField(logs.LogTargetField, logs.LogTargetUser).Info("Synced new block")
	log.WithFields(moreFields).WithField(logs.LogTargetField, logs.LogTargetEphemeral).Info("Synced new block")
	return nil
}

// logs payload related data every slot.
func logPayload(block interfaces.ReadOnlyBeaconBlock) error {
	if block.Version() < version.Bellatrix || block.Version() >= version.Gloas {
		return nil
	}
	payload, err := block.Body().Execution()
	if err != nil {
		return err
	}
	if payload.GasLimit() == 0 {
		return errors.New("gas limit should not be 0")
	}
	gasUtilized := float64(payload.GasUsed()) / float64(payload.GasLimit())
	fields := logrus.Fields{
		"blockHash":   fmt.Sprintf("%#x", bytesutil.Trunc(payload.BlockHash())),
		"parentHash":  fmt.Sprintf("%#x", bytesutil.Trunc(payload.ParentHash())),
		"blockNumber": payload.BlockNumber(),
		"gasUtilized": fmt.Sprintf("%.2f", gasUtilized),
	}
	if block.Version() >= version.Capella {
		withdrawals, err := payload.Withdrawals()
		if err != nil {
			return errors.Wrap(err, "could not get withdrawals")
		}
		fields["withdrawals"] = len(withdrawals)
		changes, err := block.Body().BLSToExecutionChanges()
		if err != nil {
			return errors.Wrap(err, "could not get BLSToExecutionChanges")
		}
		if len(changes) > 0 {
			fields["blsToExecutionChanges"] = len(changes)
		}
	}
	log.WithFields(fields).Debug("Synced new payload")
	return nil
}
