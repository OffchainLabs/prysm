package sync

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/transition/interop"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/io/file"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
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

	// TODO: do we only do this for super nodes?
	go s.reconstructAndBroadcastBlobs(ctx, signed)

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

// reconstructAndBroadcastBlobs processes and broadcasts blob sidecars for a given beacon block.
func (s *Service) reconstructAndBroadcastBlobs(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock) {
	if block.Version() >= version.Fulu {
		s.reconstructAndBroadcastBlobsInDataColumn(ctx, block)
		return
	}

	if block.Version() >= version.Deneb {
		s.reconstructAndBroadcastFullBlobs(ctx, block)
		return
	}
}

// reconstructAndBroadcastBlobsInDataColumn reconstructs and broadcasts blobs in data column format for a given beacon block, it also saves data column sidecars into the blob storage.
func (s *Service) reconstructAndBroadcastBlobsInDataColumn(ctx context.Context, roSignedBlock interfaces.ReadOnlySignedBeaconBlock) {
	block := roSignedBlock.Block()

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

	if s.cfg.dataColumnStorage == nil {
		log.Warning("Data column storage is not enabled, skip saving data column, but continue to reconstruct and broadcast data column")
	}

	// When this function is called, it's from the time when the block is received, so in almost all situations we need to get the data column from EL instead of the blob storage.
	sidecars, err := s.cfg.executionReconstructor.ReconstructDataColumnSidecars(ctx, roSignedBlock, blockRoot)
	if err != nil {
		log.WithError(err).Debug("Cannot reconstruct data column sidecars after receiving the block")
		return
	}

	nodeID := s.cfg.p2p.NodeID()
	s.cfg.custodyInfo.Mut.RLock()
	defer s.cfg.custodyInfo.Mut.RUnlock()
	groupCount := s.cfg.custodyInfo.ActualGroupCount()
	info, _, err := peerdas.Info(nodeID, groupCount)
	if err != nil {
		log.WithError(err).Error("Failed to get peer info")
		return
	}

	// Broadcast data column and then save to db (if needs to be in custody)
	for _, sidecar := range sidecars {
		if !info.CustodyColumns[sidecar.Index] {
			continue
		}

		// first broadcast the data column
		if err := s.cfg.p2p.BroadcastDataColumn(ctx, blockRoot, sidecar.Index, sidecar.DataColumnSidecar); err != nil {
			log.WithFields(dataColumnFields(sidecar.RODataColumn)).WithError(err).Error("Failed to broadcast data column")
		}

		if err := s.receiveDataColumn(ctx, sidecar); err != nil {
			log.WithFields(dataColumnFields(sidecar.RODataColumn)).WithError(err).Error("Failed to receive data column")
		}
	}
}

// reconstructAndBroadcastFullBlobs reconstructs the blob sidecars from the EL using the block's KZG commitments,
// broadcasts the reconstructed blobs over P2P, and saves them into the blob storage.
func (s *Service) reconstructAndBroadcastFullBlobs(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock) {
	startTime, err := slots.ToTime(uint64(s.cfg.chain.GenesisTime().Unix()), block.Block().Slot())
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
