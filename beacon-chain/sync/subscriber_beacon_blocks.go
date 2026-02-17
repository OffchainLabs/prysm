package sync

import (
	"context"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition/interop"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/io/file"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

func (s *Service) beaconBlockSubscriber(ctx context.Context, msg proto.Message) error {
	signed, err := blocks.NewSignedBeaconBlock(msg)
	if err != nil {
		return err
	}
	if err := blocks.BeaconBlockIsNil(signed); err != nil {
		return err
	}

	s.setSeenBlockIndexSlot(signed.Block().Slot(), signed.Block().ProposerIndex())

	block := signed.Block()

	root, err := block.HashTreeRoot()
	if err != nil {
		return err
	}

	roBlock, err := blocks.NewROBlockWithRoot(signed, root)
	if err != nil {
		return errors.Wrap(err, "new ro block with root")
	}

	go func() {
		if err := s.processSidecarsFromExecutionFromBlock(ctx, roBlock); err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"root": fmt.Sprintf("%#x", root),
				"slot": block.Slot(),
			}).Error("Failed to process sidecars from execution from block")
		}
	}()

	if err := s.cfg.chain.ReceiveBlock(ctx, signed, root, nil); err != nil {
		if blockchain.IsInvalidBlock(err) {
			r := blockchain.InvalidBlockRoot(err)
			if r != [32]byte{} {
				s.setBadBlock(ctx, r) // Setting head block as bad.
			} else {
				// TODO(13721): Remove this once we can deprecate the flag.
				interop.WriteBlockToDisk(signed, true /*failed*/)

				saveInvalidBlockToTemp(signed)
				s.setBadBlock(ctx, root)
			}
		}
		// Set the returned invalid ancestors as bad.
		for _, root := range blockchain.InvalidAncestorRoots(err) {
			s.setBadBlock(ctx, root)
		}
		return err
	}

	if err := s.processPendingAttsForBlock(ctx, root); err != nil {
		return errors.Wrap(err, "process pending atts for block")
	}

	return nil
}

// processSidecarsFromExecutionFromBlock retrieves (if available) sidecars data from the execution client,
// builds corresponding sidecars, save them to the storage, and broadcasts them over P2P if necessary.
func (s *Service) processSidecarsFromExecutionFromBlock(ctx context.Context, roBlock blocks.ROBlock) error {
	if roBlock.Version() >= version.Fulu {
		if err := s.processDataColumnSidecarsFromExecution(ctx, peerdas.PopulateFromBlock(roBlock)); err != nil {
			// Do not log if the context was cancelled on purpose.
			// (Still log other context errors such as deadlines exceeded).
			if errors.Is(err, context.Canceled) {
				return nil
			}

			return errors.Wrap(err, "process data column sidecars from execution")
		}

		return nil
	}

	if roBlock.Version() >= version.Deneb {
		s.processBlobSidecarsFromExecution(ctx, roBlock)
		return nil
	}

	return nil
}

// processBlobSidecarsFromExecution retrieves (if available) blob sidecars data from the execution client,
// builds corresponding sidecars, save them to the storage, and broadcasts them over P2P if necessary.
func (s *Service) processBlobSidecarsFromExecution(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock) {
	startTime, err := slots.StartTime(s.cfg.clock.GenesisTime(), block.Block().Slot())
	if err != nil {
		log.WithError(err).Error("Failed to convert slot to time")
	}

	blockRoot, err := block.Block().HashTreeRoot()
	if err != nil {
		log.WithError(err).Error("Failed to calculate block root")
		return
	}

	if s.cfg.blobStorage == nil {
		return
	}
	summary := s.cfg.blobStorage.Summary(blockRoot)
	cmts, err := block.Block().Body().BlobKzgCommitments()
	if err != nil {
		log.WithError(err).Error("Failed to read commitments from block")
		return
	}
	for i := range cmts {
		if summary.HasIndex(uint64(i)) {
			blobExistedInDBTotal.Inc()
		}
	}

	// Reconstruct blob sidecars from the EL
	blobSidecars, err := s.cfg.executionReconstructor.ReconstructBlobSidecars(ctx, block, blockRoot, summary.HasIndex)
	if err != nil {
		log.WithError(err).Error("Failed to reconstruct blob sidecars")
		return
	}
	if len(blobSidecars) == 0 {
		return
	}

	// Refresh indices as new blobs may have been added to the db
	summary = s.cfg.blobStorage.Summary(blockRoot)

	// Broadcast blob sidecars first than save them to the db
	for _, sidecar := range blobSidecars {
		// Don't broadcast the blob if it has appeared on disk.
		if summary.HasIndex(sidecar.Index) {
			continue
		}
		if err := s.cfg.p2p.BroadcastBlob(ctx, sidecar.Index, sidecar.BlobSidecar); err != nil {
			log.WithFields(blobFields(sidecar.ROBlob)).WithError(err).Error("Failed to broadcast blob sidecar")
		}
	}

	for _, sidecar := range blobSidecars {
		if summary.HasIndex(sidecar.Index) {
			continue
		}
		if err := s.subscribeBlob(ctx, sidecar); err != nil {
			log.WithFields(blobFields(sidecar.ROBlob)).WithError(err).Error("Failed to receive blob")
			continue
		}

		blobRecoveredFromELTotal.Inc()
		fields := blobFields(sidecar.ROBlob)
		fields["sinceSlotStartTime"] = s.cfg.clock.Now().Sub(startTime)
		log.WithFields(fields).Debug("Processed blob sidecar from EL")
	}
}

// processDataColumnSidecarsFromExecution retrieves (if available) data column sidecars data from the execution client,
// builds corresponding sidecars, save them to the storage, and broadcasts them over P2P if necessary.
func (s *Service) processDataColumnSidecarsFromExecution(ctx context.Context, source peerdas.ConstructionPopulator) error {
	key := fmt.Sprintf("%#x", source.Root())
	if _, err, _ := s.columnSidecarsExecSingleFlight.Do(key, func() (any, error) {
		const delay = 250 * time.Millisecond

		commitments, err := source.Commitments()
		if err != nil {
			return nil, errors.Wrap(err, "blob kzg commitments")
		}

		// Exit early if there are no commitments.
		if len(commitments) == 0 {
			return nil, nil
		}

		// Retrieve the indices of sidecars we should sample.
		columnIndicesToSample, err := s.columnIndicesToSample()
		if err != nil {
			return nil, errors.Wrap(err, "column indices to sample")
		}

		digest, err := s.currentForkDigest()
		if err != nil {
			return nil, err
		}
		log := log.WithFields(logrus.Fields{
			"root":          fmt.Sprintf("%#x", source.Root()),
			"slot":          source.Slot(),
			"proposerIndex": source.ProposerIndex(),
			"type":          source.Type(),
		})

		var constructedSidecarCount uint64
		for iteration := uint64(0); ; /*no stop condition*/ iteration++ {
			log = log.WithField("iteration", iteration)

			// Exit early if all sidecars to sample have been seen.
			if s.haveAllSidecarsBeenSeen(source.Slot(), source.ProposerIndex(), columnIndicesToSample) {
				if iteration > 0 && constructedSidecarCount == 0 {
					log.Debug("No data column sidecars constructed from the execution client")
				}

				return nil, nil
			}

			// Return if the context is done.
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			if iteration == 0 {
				dataColumnsRecoveredFromELAttempts.Inc()
			}

			// Try to reconstruct data column constructedSidecars from the execution client.
			constructedSidecars, partialColumns, err := s.cfg.executionReconstructor.ConstructDataColumnSidecars(ctx, source)
			if err != nil {
				return nil, errors.Wrap(err, "reconstruct data column sidecars")
			}

			partialBroadcaster := s.cfg.p2p.PartialColumnBroadcaster()
			if partialBroadcaster != nil {
				log.WithField("len(partialColumns)", len(partialColumns)).Debug("Publishing partial columns")
				for i := range uint64(len(partialColumns)) {
					if !columnIndicesToSample[i] {
						continue
					}
					subnet := peerdas.ComputeSubnetForDataColumnSidecar(i)
					topic := fmt.Sprintf(p2p.DataColumnSubnetTopicFormat, digest, subnet) + s.cfg.p2p.Encoding().ProtocolSuffix()
					// Publish the partial column. This is idempotent if we republish the same data twice.
					// Note, the "partial column" may indeed be complete. We still
					// should publish to help our peers.
					err = partialBroadcaster.Publish(topic, partialColumns[i], true)
					if err != nil {
						log.WithError(err).Warn("Failed to publish partial column")
					}
				}
			}

			// No sidecars are retrieved from the EL, retry later
			constructedCount := uint64(len(constructedSidecars))

			// Boundary check.
			if constructedSidecarCount > 0 && constructedSidecarCount != fieldparams.NumberOfColumns {
				return nil, errors.Errorf("reconstruct data column sidecars returned %d sidecars, expected %d - should never happen", constructedSidecarCount, fieldparams.NumberOfColumns)
			}

			unseenIndices, err := s.broadcastAndReceiveUnseenDataColumnSidecars(ctx, source.Slot(), source.ProposerIndex(), columnIndicesToSample, constructedSidecars)
			if err != nil {
				return nil, errors.Wrap(err, "broadcast and receive unseen data column sidecars")
			}

			if constructedCount > 0 {
				dataColumnsRecoveredFromELTotal.Inc()

				log.WithFields(logrus.Fields{
					"root":          fmt.Sprintf("%#x", source.Root()),
					"slot":          source.Slot(),
					"proposerIndex": source.ProposerIndex(),
					"iteration":     iteration,
					"type":          source.Type(),
					"count":         len(unseenIndices),
					"indices":       helpers.SortedPrettySliceFromMap(unseenIndices),
				}).Debug("Constructed data column sidecars from the execution client")

				return nil, nil
			}

			// Wait before retrying.
			time.Sleep(delay)
		}
	}); err != nil {
		return err
	}

	return nil
}

// broadcastAndReceiveUnseenDataColumnSidecars broadcasts and receives unseen data column sidecars.
func (s *Service) broadcastAndReceiveUnseenDataColumnSidecars(
	ctx context.Context,
	slot primitives.Slot,
	proposerIndex primitives.ValidatorIndex,
	neededIndices map[uint64]bool,
	sidecars []blocks.VerifiedRODataColumn,
) (map[uint64]bool, error) {
	// Compute sidecars we need to broadcast and receive.
	unseenSidecars := make([]blocks.VerifiedRODataColumn, 0, len(sidecars))
	unseenIndices := make(map[uint64]bool, len(sidecars))
	for _, sidecar := range sidecars {
		// Skip data column sidecars we don't need.
		if !neededIndices[sidecar.Index] {
			continue
		}

		// Skip already seen data column sidecars.
		if s.hasSeenDataColumnIndex(slot, proposerIndex, sidecar.Index) {
			continue
		}

		unseenSidecars = append(unseenSidecars, sidecar)
		unseenIndices[sidecar.Index] = true
	}

	// Exit early if there are no nothing to broadcast or receive.
	if len(unseenSidecars) == 0 {
		return nil, nil
	}

	// Broadcast all the data column sidecars we reconstructed but did not see via gossip (non blocking).
	if err := s.cfg.p2p.BroadcastDataColumnSidecars(ctx, unseenSidecars, nil); err != nil {
		return nil, errors.Wrap(err, "broadcast data column sidecars")
	}

	// Receive data column sidecars.
	if err := s.receiveDataColumnSidecars(ctx, unseenSidecars); err != nil {
		return nil, errors.Wrap(err, "receive data column sidecars")
	}

	return unseenIndices, nil
}

// haveAllSidecarsBeenSeen checks if all sidecars for the given slot, proposer index, and data column indices have been seen.
func (s *Service) haveAllSidecarsBeenSeen(slot primitives.Slot, proposerIndex primitives.ValidatorIndex, indices map[uint64]bool) bool {
	for index := range indices {
		if !s.hasSeenDataColumnIndex(slot, proposerIndex, index) {
			return false
		}
	}
	return true
}

// columnIndicesToSample returns the data column indices we should sample for the node.
func (s *Service) columnIndicesToSample() (map[uint64]bool, error) {
	// Retrieve our node ID.
	nodeID := s.cfg.p2p.NodeID()

	// Get the custody group sampling size for the node.
	custodyGroupCount, err := s.cfg.p2p.CustodyGroupCount(s.ctx)
	if err != nil {
		return nil, errors.Wrap(err, "custody group count")
	}

	// Compute the sampling size.
	// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/das-core.md#custody-sampling
	samplesPerSlot := params.BeaconConfig().SamplesPerSlot
	samplingSize := max(samplesPerSlot, custodyGroupCount)

	// Get the peer info for the node.
	peerInfo, _, err := peerdas.Info(nodeID, samplingSize)
	if err != nil {
		return nil, errors.Wrap(err, "peer info")
	}

	return peerInfo.CustodyColumns, nil
}

// WriteInvalidBlockToDisk as a block ssz. Writes to temp directory.
func saveInvalidBlockToTemp(block interfaces.ReadOnlySignedBeaconBlock) {
	if !features.Get().SaveInvalidBlock {
		return
	}
	filename := fmt.Sprintf("beacon_block_%d.ssz", block.Block().Slot())
	fp := path.Join(os.TempDir(), filename)
	log.Warnf("Writing invalid block to disk at %s", fp)
	enc, err := block.MarshalSSZ()
	if err != nil {
		log.WithError(err).Error("Failed to ssz encode block")
		return
	}
	if err := file.WriteFile(fp, enc); err != nil {
		log.WithError(err).Error("Failed to write to disk")
	}
}
