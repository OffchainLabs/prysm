package sync

import (
	"context"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition/interop"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/io/file"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
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

	go s.processSidecarsFromExecutionFromBlock(ctx, roBlock)

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
func (s *Service) processSidecarsFromExecutionFromBlock(ctx context.Context, roBlock blocks.ROBlock) {
	if roBlock.Version() >= version.Fulu {
		if err := s.processDataColumnSidecarsFromExecution(ctx, peerdas.PopulateFromBlock(roBlock)); err != nil {
			log.WithError(err).Error("Failed to process data column sidecars from execution")
			return
		}

		return
	}

	if roBlock.Version() >= version.Deneb {
		s.processBlobSidecarsFromExecution(ctx, roBlock)
		return
	}
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
	if _, err, _ := s.columnSidecarsExecSingleFlight.Do(key, func() (interface{}, error) {
		const delay = 250 * time.Millisecond
		secondsPerHalfSlot := time.Duration(params.BeaconConfig().SecondsPerSlot/2) * time.Second

		numberOfColumns := params.BeaconConfig().NumberOfColumns

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

		ctx, cancel := context.WithTimeout(ctx, secondsPerHalfSlot)
		defer cancel()

		for iteration := uint64(0); ; /*no stop condition*/ iteration++ {
			// Exit early if all sidecars to sample have been seen.
			if s.haveAllSidecarsBeenSeen(source.Slot(), source.ProposerIndex(), columnIndicesToSample) {
				return nil, nil
			}

			// Try to reconstruct data column constructedSidecars from the execution client.
			constructedSidecars, err := s.cfg.executionReconstructor.ConstructDataColumnSidecars(ctx, source)
			if err != nil {
				return nil, errors.Wrap(err, "reconstruct data column sidecars")
			}

			// No sidecars are retrieved from the EL, retry later
			sidecarCount := uint64(len(constructedSidecars))
			if sidecarCount == 0 {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}

				time.Sleep(delay)
				continue
			}

			// Boundary check.
			if sidecarCount != numberOfColumns {
				return nil, errors.Errorf("reconstruct data column sidecars returned %d sidecars, expected %d - should never happen", sidecarCount, numberOfColumns)
			}

			unseenIndices, err := s.broadcastAndReceiveUnseenDataColumnSidecars(ctx, source.Slot(), source.ProposerIndex(), columnIndicesToSample, constructedSidecars)
			if err != nil {
				return nil, errors.Wrap(err, "broadcast and receive unseen data column sidecars")
			}

			if len(unseenIndices) > 0 {
				log.WithFields(logrus.Fields{
					"root":          fmt.Sprintf("%#x", source.Root()),
					"slot":          source.Slot(),
					"proposerIndex": source.ProposerIndex(),
					"iteration":     iteration,
					"type":          source.Type(),
					"count":         len(unseenIndices),
					"indices":       helpers.SortedPrettySliceFromMap(unseenIndices),
				}).Debug("Constructed data column sidecars from the execution client")
			}

			return nil, nil
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

	// Broadcast all the data column sidecars we reconstructed but did not see via gossip (non blocking).
	if err := s.cfg.p2p.BroadcastDataColumnSidecars(ctx, unseenSidecars); err != nil {
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
