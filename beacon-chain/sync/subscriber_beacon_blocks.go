package sync

import (
	"context"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/io/file"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"google.golang.org/protobuf/proto"
)

const slotDeadline = time.Duration(5) * time.Second

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

	go s.reconstructAndBroadcastBlobs(ctx, signed)

	if err := s.cfg.chain.ReceiveBlock(ctx, signed, root, nil); err != nil {
		if blockchain.IsInvalidBlock(err) {
			r := blockchain.InvalidBlockRoot(err)
			if r != [32]byte{} {
				s.setBadBlock(ctx, r) // Setting head block as bad.
			} else {
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
	go s.processPendingPayloads(root)
	return nil
}

// reconstructAndBroadcastBlobs processes and broadcasts blob sidecars for a given beacon block.
// This function reconstructs the blob sidecars from the EL using the block's KZG commitments,
// broadcasts the reconstructed blobs over P2P, and saves them into the blob storage.
func (s *Service) reconstructAndBroadcastBlobs(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock) {
	if block.Version() < version.Deneb {
		return
	}

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
	indices, err := s.cfg.blobStorage.Indices(blockRoot, block.Block().Slot())
	if err != nil {
		log.WithError(err).Error("Failed to retrieve indices for block")
		return
	}
	for _, index := range indices {
		if index {
			blobExistedInDBTotal.Inc()
		}
	}

	// Reconstruct blob sidecars from the EL
	blobSidecars, err := s.cfg.executionReconstructor.ReconstructBlobSidecars(ctx, block, blockRoot, indices)
	if err != nil {
		log.WithError(err).Error("Failed to reconstruct blob sidecars")
		return
	}
	if len(blobSidecars) == 0 {
		return
	}

	// Refresh indices as new blobs may have been added to the db
	indices, err = s.cfg.blobStorage.Indices(blockRoot, block.Block().Slot())
	if err != nil {
		log.WithError(err).Error("Failed to retrieve indices for block")
		return
	}

	// Broadcast blob sidecars first than save them to the db
	for _, sidecar := range blobSidecars {
		if sidecar.Index >= uint64(len(indices)) || indices[sidecar.Index] {
			continue
		}
		if err := s.cfg.p2p.BroadcastBlob(ctx, sidecar.Index, sidecar.BlobSidecar); err != nil {
			log.WithFields(blobFields(sidecar.ROBlob)).WithError(err).Error("Failed to broadcast blob sidecar")
		}
	}

	for _, sidecar := range blobSidecars {
		if sidecar.Index >= uint64(len(indices)) || indices[sidecar.Index] {
			blobExistedInDBTotal.Inc()
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

func (s *Service) processPendingPayloads(root [32]byte) {
	ctx, cancel := context.WithTimeout(context.Background(), slotDeadline)
	defer cancel()
	payload := s.pendingExecutionPayloads.Get(root)
	if payload == nil {
		return
	}
	if s.cfg.chain.PayloadBeingSynced(root) || s.cfg.chain.HashInForkchoice([32]byte(payload.Message.Payload.BlockHash)) {
		s.pendingExecutionPayloads.Remove(root)
		return
	}
	e, err := blocks.WrappedROSignedExecutionPayloadEnvelope(payload)
	if err != nil {
		log.WithError(err).Error("failed to create read only signed payload execution envelope")
		return
	}
	v := s.newExecutionPayloadEnvelopeVerifier(e, verification.GossipExecutionPayloadEnvelopeRequirements)
	if err := v.VerifyBlockRootSeen(func([32]byte) bool { return true }); err != nil {
		// This can't happen
		return
	}
	if _, err := s.validateAfterBlockRootSeen(ctx, payload, v); err != nil {
		log.WithError(err).Error("failed to validate pending payload")
		return
	}
	if err := s.cfg.chain.ReceiveExecutionPayloadEnvelope(ctx, e, nil); err != nil {
		log.WithError(err).Error("failed to receive pending payload")
		return
	}
	if err := s.cfg.p2p.Broadcast(ctx, e.Proto()); err != nil {
		log.WithError(err).Error("failed to broadcast pending payload")
	}
	s.pendingExecutionPayloads.Remove(root)
	return
}
