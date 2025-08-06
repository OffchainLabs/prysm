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
		return errors.Wrap(err, "receive data column sidecar")
	}

	slot := sidecar.Slot()
	proposerIndex := sidecar.ProposerIndex()
	root := sidecar.BlockRoot()

	if err := s.reconstructSaveBroadcastDataColumnSidecars(ctx, slot, proposerIndex, root); err != nil {
		return errors.Wrap(err, "reconstruct/save/broadcast data column sidecars")
	}

	// Trigger getBlobsV2 when receiving data column sidecar
	if err := s.triggerGetBlobsV2ForDataColumnSidecar(ctx, root); err != nil {
		return errors.Wrap(err, "failed to trigger getBlobsV2 for data column sidecar")
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
	if err != nil {
		return err
	}
	if len(commitments) == 0 {
		return nil
	}

	// Check if data is already available
	available, err := s.cfg.chain.IsDataAvailable(ctx, blockRoot, signedBlock)
	if err != nil {
		log.WithError(err).Debug("Error checking data availability during getBlobsV2 trigger")
	}
	if available {
		log.WithField("blockRoot", fmt.Sprintf("%#x", blockRoot)).Debug("Data already available, skipping getBlobsV2 retry")
		return nil
	}

	// Trigger the retry by calling the execution service's reconstruct method
	// ReconstructDataColumnSidecars handles concurrent calls internally
	log.WithField("blockRoot", fmt.Sprintf("%#x", blockRoot)).Debug("Triggering getBlobsV2 retry for data column sidecar")

	if s.cfg.executionReconstructor == nil {
		return nil
	}

	_, err = s.cfg.executionReconstructor.ReconstructDataColumnSidecars(ctx, signedBlock, blockRoot)
	if err != nil {
		return errors.Wrap(err, "getBlobsV2 retry triggered by data column sidecar failed")
	}

	return nil
}
