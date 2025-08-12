package sync

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition/interop"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/io/file"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
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

	go s.processSidecarsFromExecution(ctx, signed)

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

// processSidecarsFromExecution retrieves (if available) sidecars data from the execution client,
// builds corresponding sidecars, save them to the storage, and broadcasts them over P2P if necessary.
func (s *Service) processSidecarsFromExecution(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock) {
	if block.Version() >= version.Fulu {
		s.processDataColumnSidecarsFromExecution(ctx, block)
		return
	}

	if block.Version() >= version.Deneb {
		s.processBlobSidecarsFromExecution(ctx, block)
		return
	}
}

// processDataColumnSidecarsFromExecution retrieves (if available) data column sidecars data from the execution client,
// builds corresponding sidecars, save them to the storage, and broadcasts them over P2P if necessary.
func (s *Service) processDataColumnSidecarsFromExecution(ctx context.Context, roSignedBlock interfaces.ReadOnlySignedBeaconBlock) {
	block := roSignedBlock.Block()

	log := log.WithFields(logrus.Fields{
		"slot":          block.Slot(),
		"proposerIndex": block.ProposerIndex(),
	})

	kzgCommitments, err := block.Body().BlobKzgCommitments()
	if err != nil {
		log.WithError(err).Error("Failed to read commitments from block")
		return
	}

	if len(kzgCommitments) == 0 {
		// No blobs to reconstruct.
		return
	}

	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		log.WithError(err).Error("Failed to calculate block root")
		return
	}

	log = log.WithField("blockRoot", fmt.Sprintf("%#x", blockRoot))

	if s.cfg.dataColumnStorage == nil {
		log.Warning("Data column storage is not enabled, skip saving data column, but continue to reconstruct and broadcast data column")
	}

	// When this function is called, it's from the time when the block is received, so in almost all situations we need to get the data column from EL instead of the blob storage.
	sidecars, err := s.cfg.executionReconstructor.ReconstructDataColumnSidecars(ctx, roSignedBlock, blockRoot)
	if err != nil {
		log.WithError(err).Debug("Cannot reconstruct data column sidecars after receiving the block")
		return
	}

	// Return early if no blobs are retrieved from the EL.
	if len(sidecars) == 0 {
		return
	}

	nodeID := s.cfg.p2p.NodeID()
	custodyGroupCount, err := s.cfg.p2p.CustodyGroupCount()
	if err != nil {
		log.WithError(err).Error("Failed to get custody group count")
		return
	}

	info, _, err := peerdas.Info(nodeID, custodyGroupCount)
	if err != nil {
		log.WithError(err).Error("Failed to get peer info")
		return
	}

	blockSlot := block.Slot()
	proposerIndex := block.ProposerIndex()

	// Broadcast and save data column sidecars to custody but not yet received.
	sidecarCount := uint64(len(sidecars))
	for columnIndex := range info.CustodyColumns {
		log := log.WithField("columnIndex", columnIndex)
		if columnIndex >= sidecarCount {
			log.Error("Column custody index out of range - should never happen")
			continue
		}

		if s.hasSeenDataColumnIndex(blockSlot, proposerIndex, columnIndex) {
			continue
		}

		sidecar := sidecars[columnIndex]

		if err := s.cfg.p2p.BroadcastDataColumn(blockRoot, sidecar.Index, sidecar.DataColumnSidecar); err != nil {
			log.WithError(err).Error("Failed to broadcast data column")
		}

		if err := s.receiveDataColumnSidecar(ctx, sidecar); err != nil {
			log.WithError(err).Error("Failed to receive data column")
		}
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
