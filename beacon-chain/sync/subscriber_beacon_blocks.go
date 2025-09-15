package sync

import (
	"context"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition/interop"
	"github.com/OffchainLabs/prysm/v6/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/io/file"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
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
	return err
}

// processSidecarsFromExecutionFromBlock retrieves (if available) sidecars data from the execution client,
// builds corresponding sidecars, save them to the storage, and broadcasts them over P2P if necessary.
func (s *Service) processSidecarsFromExecutionFromBlock(ctx context.Context, roBlock blocks.ROBlock) {
	if roBlock.Version() >= version.Fulu {
		key := fmt.Sprintf("%#x", roBlock.Root())
		if _, err, _ := s.columnSidecarsExecSingleFlight.Do(key, func() (interface{}, error) {
			if err := s.processDataColumnSidecarsFromExecution(ctx, peerdas.PopulateFromBlock(roBlock)); err != nil {
				return nil, err
			}

			return nil, nil
		}); err != nil {
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
	const delay = 250 * time.Millisecond
	secondsPerHalfSlot := time.Duration(params.BeaconConfig().SecondsPerSlot/2) * time.Second

	numberOfColumns := params.BeaconConfig().NumberOfColumns

	commitments, err := source.Commitments()
	if err != nil {
		return errors.Wrap(err, "blob kzg commitments")
	}

	ctx, cancel := context.WithTimeout(ctx, secondsPerHalfSlot)
	defer cancel()

	for {
		// Check if some data column sidecars to custody are missing.
		missingIndices, err := s.missingDataColumnSidecars(source.Root(), commitments)
		if err != nil {
			return errors.Wrap(err, "missing data column sidecars")
		}

		// Return early if all needed data column sidecars are already available in storage.
		if len(missingIndices) == 0 {
			return nil
		}

		// Try to reconstruct data column sidecars from the execution client.
		sidecars, err := s.cfg.executionReconstructor.ConstructDataColumnSidecars(ctx, source)
		if err != nil {
			return errors.Wrap(err, "reconstruct data column sidecars")
		}

		// No sidecars are retrieved from the EL, retry later
		sidecarCount := uint64(len(sidecars))
		if sidecarCount == 0 {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			time.Sleep(delay)

			continue
		}

		// Boundary check.
		if sidecarCount != numberOfColumns {
			return errors.Errorf("reconstruct data column sidecars returned %d sidecars, expected %d - should never happen", sidecarCount, numberOfColumns)
		}

		// Broadcast and save data column sidecars to custody but not yet received.
		for index := range missingIndices {
			if index >= sidecarCount {
				return errors.Errorf("data column index %d >= sidecar count %d - should never happen", index, sidecarCount)
			}

			// This sidecar has been received in the meantime, skip it.
			if s.hasSeenDataColumnIndex(source.Slot(), source.ProposerIndex(), index) {
				continue
			}

			sidecar := sidecars[index]

			if err := s.cfg.p2p.BroadcastDataColumnSidecar(sidecar.Index, sidecar); err != nil {
				return errors.Wrap(err, "broadcast data column sidecar")
			}

			if err := s.receiveDataColumnSidecar(ctx, sidecar); err != nil {
				return errors.Wrap(err, "receive data column sidecar")
			}
		}

		return nil
	}
}

// missingDataColumnSidecars returns the data column indices we should custody and that are missing in our storage
// for the given read only beacon block.
func (s *Service) missingDataColumnSidecars(root [fieldparams.RootLength]byte, commitments [][]byte) (map[uint64]bool, error) {
	// Return early if there is not commitments.
	if len(commitments) == 0 {
		return nil, nil
	}

	// Retrieve our node ID.
	nodeID := s.cfg.p2p.NodeID()

	// Get the custody group sampling size for the node.
	custodyGroupCount, err := s.cfg.p2p.CustodyGroupCount()
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

	// Get the indices of the data column sidecars we have in the store.
	storedIndices := s.cfg.dataColumnStorage.Summary(root).Stored()

	// List indices we should custody and that are missing in our storage.
	missingIndices := make(map[uint64]bool, samplingSize)
	for index := range peerInfo.CustodyColumns {
		if !storedIndices[index] {
			missingIndices[index] = true
		}
	}

	return missingIndices, nil
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
