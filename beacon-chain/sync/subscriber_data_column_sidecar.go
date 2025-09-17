package sync

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
)

// dataColumnSubscriber is the subscriber function for data column sidecars.
func (s *Service) dataColumnSubscriber(ctx context.Context, msg proto.Message) error {
	var wg errgroup.Group

	sidecar, ok := msg.(blocks.VerifiedRODataColumn)
	if !ok {
		return fmt.Errorf("message was not type blocks.VerifiedRODataColumn, type=%T", msg)
	}

	if err := s.receiveDataColumnSidecar(ctx, sidecar); err != nil {
		return errors.Wrap(err, "receive data column sidecar")
	}

	wg.Go(func() error {
		if err := s.processDataColumnSidecarsFromReconstruction(ctx, sidecar); err != nil {
			return errors.Wrap(err, "process data column sidecars from reconstruction")
		}

		return nil
	})

	wg.Go(func() error {
		if err := s.processDataColumnSidecarsFromExecution(ctx, peerdas.PopulateFromSidecar(sidecar)); err != nil {
			return errors.Wrap(err, "process data column sidecars from execution")
		}

		return nil
	})

	if err := wg.Wait(); err != nil {
		return err
	}

	return nil
}

// receiveDataColumnSidecar receives a single data column sidecar: marks it as seen and saves it to the chain.
// Do not loop over this function to receive multiple sidecars, use receiveDataColumnSidecars instead.
func (s *Service) receiveDataColumnSidecar(ctx context.Context, sidecar blocks.VerifiedRODataColumn) error {
	return s.receiveDataColumnSidecars(ctx, []blocks.VerifiedRODataColumn{sidecar})
}

// receiveDataColumnSidecars receives multiple data column sidecars: marks them as seen and saves them to the chain.
func (s *Service) receiveDataColumnSidecars(ctx context.Context, sidecars []blocks.VerifiedRODataColumn) error {
	for _, sidecar := range sidecars {
		slot := sidecar.SignedBlockHeader.Header.Slot
		proposerIndex := sidecar.SignedBlockHeader.Header.ProposerIndex
		columnIndex := sidecar.Index

		s.setSeenDataColumnIndex(slot, proposerIndex, columnIndex)
	}

	if err := s.cfg.chain.ReceiveDataColumns(sidecars); err != nil {
		return errors.Wrap(err, "receive data column")
	}

	for _, sidecar := range sidecars {
		s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
			Type: opfeed.DataColumnSidecarReceived,
			Data: &opfeed.DataColumnSidecarReceivedData{
				DataColumn: &sidecar,
			},
		})
	}

	return nil
}

// allDataColumnSubnets returns the data column subnets for which we need to find peers
// but don't need to subscribe to. This is used to ensure we have peers available in all subnets
// when we are serving validators. When a validator proposes a block, they need to publish data
// column sidecars on all subnets. This method returns a nil map when there is no validators custody
// requirement.
func (s *Service) allDataColumnSubnets(_ primitives.Slot) map[uint64]bool {
	validatorsCustodyRequirement, err := s.validatorsCustodyRequirement()
	if err != nil {
		log.WithError(err).Error("Could not retrieve validators custody requirement")
		return nil
	}

	// If no validators are tracked, return early
	if validatorsCustodyRequirement == 0 {
		return nil
	}

	// When we have validators with custody requirements, we need peers in all subnets
	// because validators need to be able to publish data columns to all subnets when proposing
	dataColumnSidecarSubnetCount := params.BeaconConfig().DataColumnSidecarSubnetCount
	allSubnets := make(map[uint64]bool, dataColumnSidecarSubnetCount)
	for i := range dataColumnSidecarSubnetCount {
		allSubnets[i] = true
	}

	return allSubnets
}
