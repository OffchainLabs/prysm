package sync

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

func (s *Service) dataColumnSubscriber(ctx context.Context, msg proto.Message) error {
	sidecar, ok := msg.(blocks.VerifiedRODataColumn)
	if !ok {
		return fmt.Errorf("message was not type blocks.VerifiedRODataColumn, type=%T", msg)
	}

	if err := s.receiveDataColumnSidecar(ctx, sidecar); err != nil {
		return errors.Wrap(err, "receive data column")
	}

	slot := sidecar.Slot()
	proposerIndex := sidecar.ProposerIndex()
	root := sidecar.BlockRoot()

	if err := s.reconstructSaveBroadcastDataColumnSidecars(ctx, slot, proposerIndex, root); err != nil {
		return errors.Wrap(err, "reconstruct data columns")
	}

	// Trigger getBlobsV2 when receiving data column sidecar
	if err := s.triggerGetBlobsV2ForDataColumnSidecar(ctx, root); err != nil {
		// Log error but don't fail - this is a best-effort retry trigger
		log.WithError(err).Debug("Failed to trigger getBlobsV2 for data column sidecar")
	}

	return nil
}

func (s *Service) receiveDataColumnSidecar(ctx context.Context, sidecar blocks.VerifiedRODataColumn) error {
	slot := sidecar.SignedBlockHeader.Header.Slot
	proposerIndex := sidecar.SignedBlockHeader.Header.ProposerIndex
	columnIndex := sidecar.Index

	s.setSeenDataColumnIndex(slot, proposerIndex, columnIndex)

	if err := s.cfg.chain.ReceiveDataColumn(sidecar); err != nil {
		return errors.Wrap(err, "receive data column")
	}

	s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.DataColumnSidecarReceived,
		Data: &opfeed.DataColumnSidecarReceivedData{
			DataColumn: &sidecar,
		},
	})

	return nil
}

// triggerGetBlobsV2ForDataColumnSidecar triggers getBlobsV2 retry when receiving a data column sidecar.
// This function attempts to fetch the block and trigger the execution service's retry mechanism.
func (s *Service) triggerGetBlobsV2ForDataColumnSidecar(ctx context.Context, blockRoot [32]byte) error {
	// Check if service is properly configured
	if s.cfg == nil || s.cfg.chain == nil || s.cfg.beaconDB == nil {
		log.Debug("Service not properly configured for getBlobsV2 retry trigger")
		return nil
	}

	// Try to get the block from the database or cache
	if !s.cfg.chain.HasBlock(ctx, blockRoot) {
		// Block not available yet, this is expected in some cases
		log.WithField("blockRoot", fmt.Sprintf("%#x", blockRoot)).Debug("Block not available for getBlobsV2 retry trigger")
		return nil
	}

	// Get the specific block by root from database
	signedBlock, err := s.cfg.beaconDB.Block(ctx, blockRoot)
	if err != nil {
		log.WithError(err).Debug("Could not fetch block from database for getBlobsV2 retry trigger")
		return nil
	}
	if signedBlock == nil || signedBlock.IsNil() {
		log.Debug("Block not found in database for getBlobsV2 retry trigger")
		return nil
	}

	// Check if this block has blob commitments that would need getBlobsV2
	blockBody := signedBlock.Block().Body()
	commitments, err := blockBody.BlobKzgCommitments()
	if err != nil || len(commitments) == 0 {
		// No commitments, no need to trigger retry
		return nil
	}

	// Trigger the retry by calling the execution service's reconstruct method
	go func() {
		log.WithField("blockRoot", fmt.Sprintf("%#x", blockRoot)).Debug("Triggering getBlobsV2 retry for data column sidecar")
		
		// This will trigger retry logic if needed
		if s.cfg.executionReconstructor != nil {
			_, err := s.cfg.executionReconstructor.ReconstructDataColumnSidecars(ctx, signedBlock, blockRoot)
			if err != nil {
				log.WithError(err).Debug("getBlobsV2 retry triggered by data column sidecar failed")
			}
		}
	}()

	return nil
}
