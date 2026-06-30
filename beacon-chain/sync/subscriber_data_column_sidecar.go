package sync

import (
	"context"
	"fmt"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
)

// dataColumnSubscriber is the subscriber function for data column sidecars.
func (s *Service) dataColumnSubscriber(ctx context.Context, msg proto.Message) error {
	var wg errgroup.Group

	var sidecar blocks.VerifiedRODataColumn
	switch dc := msg.(type) {
	case *ethpb.DataColumnSidecar:
		ro, err := blocks.NewRODataColumn(dc)
		if err != nil {
			return err
		}
		sidecar = blocks.NewVerifiedRODataColumn(ro)
	case *ethpb.DataColumnSidecarGloas:
		ro, err := blocks.NewRODataColumnGloas(dc)
		if err != nil {
			return err
		}
		sidecar = blocks.NewVerifiedRODataColumn(ro)
	default:
		return fmt.Errorf("unexpected data column type: %T", msg)
	}

	if !sidecar.IsGloas() {
		// Track useful full columns received via gossip (not previously seen)
		proposerIndex, err := sidecar.ProposerIndex()
		if err != nil {
			return errors.Wrap(err, "proposer index")
		}
		if !s.hasSeenDataColumnIndex(sidecar.Slot(), proposerIndex, sidecar.Index()) && !sidecar.IsGloas() {
			usefulFullColumnsReceivedTotal.WithLabelValues(strconv.FormatUint(sidecar.Index(), 10)).Inc()
			// re-publish the full column on the partial column extension as we don't send full columns to peers
			// who have explicitly requested for partial columns. This method is idempotent so this is fine.
			if broadcaster := s.cfg.p2p.PartialColumnBroadcaster(); broadcaster != nil {
				digest, err := s.currentForkDigest()
				if err != nil {
					log.Error("Failed to get current fork digest")
				} else {
					err := broadcaster.Publish(ctx, func(yield func(string, blocks.PartialDataColumn) bool) {
						subnet := peerdas.ComputeSubnetForDataColumnSidecar(sidecar.Index())
						topic := fmt.Sprintf(p2p.DataColumnSubnetTopicFormat, digest, subnet) + s.cfg.p2p.Encoding().ProtocolSuffix()
						partialColumn, err := blocks.NewPartialDataColumnFromVerifiedRODataColumn(sidecar)
						if err != nil {
							log.WithError(err).Error("Failed to create partial data column from verified RO data column")
							return
						}
						yield(topic, partialColumn)
					})
					if err != nil {
						log.WithError(err).Error("Failed to publish partial column on getting data column sidecar")
					}
				}
			}
		}
	}

	if sidecar.IsGloas() {
		s.republishGloasColumnAsPartial(ctx, sidecar)
	}

	if err := s.receiveDataColumnSidecar(ctx, sidecar); err != nil {
		return wrapDataColumnError(sidecar, "receive data column sidecar", err)
	}

	// CL reconstruction (from >=50% seen columns) runs for both Fulu and Gloas.
	wg.Go(func() error {
		if err := s.processDataColumnSidecarsFromReconstruction(ctx, sidecar); err != nil {
			return wrapDataColumnError(sidecar, "process data column sidecars from reconstruction", err)
		}

		return nil
	})

	if !sidecar.IsGloas() {
		wg.Go(func() error {
			if err := s.processDataColumnSidecarsFromExecution(ctx, peerdas.PopulateFromSidecar(sidecar)); err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}

				return wrapDataColumnError(sidecar, "process data column sidecars from execution", err)
			}

			return nil
		})
	}

	if err := wg.Wait(); err != nil {
		return err
	}

	return nil
}

// republishGloasColumnAsPartial republishes a verified Gloas full column received via gossip as
// a partial column so partial-column peers (who don't receive full columns) can fill in cells.
// The column was re-wrapped from the raw proto and carries no commitments, so they are seeded
// from the block's bid (the block is in the DB — Gloas validation required it). De-dup is not
// needed here: the Gloas validator delivers only the first valid receipt per (root, index), and
// the broadcaster is idempotent per group id.
func (s *Service) republishGloasColumnAsPartial(ctx context.Context, sidecar blocks.VerifiedRODataColumn) {
	broadcaster := s.cfg.p2p.PartialColumnBroadcaster()
	if broadcaster == nil {
		return
	}

	commitments, err := s.bidCommitmentsForRoot(ctx, sidecar.BlockRoot())
	if err != nil {
		log.WithError(err).Error("Failed to get bid commitments for gloas partial column republish")
		return
	}
	sidecar.SetBidCommitments(commitments)

	digest, err := s.currentForkDigest()
	if err != nil {
		log.WithError(err).Error("Failed to get current fork digest")
		return
	}

	if err := broadcaster.Publish(ctx, func(yield func(string, blocks.PartialDataColumn) bool) {
		subnet := peerdas.ComputeSubnetForDataColumnSidecar(sidecar.Index())
		topic := fmt.Sprintf(p2p.DataColumnSubnetTopicFormat, digest, subnet) + s.cfg.p2p.Encoding().ProtocolSuffix()
		partialColumn, err := blocks.NewPartialDataColumnFromVerifiedRODataColumn(sidecar)
		if err != nil {
			log.WithError(err).Error("Failed to create gloas partial data column from verified RO data column")
			return
		}
		yield(topic, partialColumn)
	}); err != nil {
		log.WithError(err).Error("Failed to publish gloas partial column on getting data column sidecar")
	}
}

// bidCommitmentsForRoot returns the bid KZG commitments for the block with the given root,
// which must be present in the database.
func (s *Service) bidCommitmentsForRoot(ctx context.Context, root [32]byte) ([][]byte, error) {
	block, err := s.cfg.beaconDB.Block(ctx, root)
	if err != nil {
		return nil, errors.Wrap(err, "get block")
	}
	if block == nil || block.IsNil() {
		return nil, errors.New("nil block")
	}
	commitments, err := block.Block().Body().BlobKzgCommitments()
	if err != nil {
		return nil, errors.Wrap(err, "blob kzg commitments")
	}
	return commitments, nil
}

func (s *Service) verifiedRODataColumnSubscriber(ctx context.Context, sidecar blocks.VerifiedRODataColumn) error {
	if err := s.receiveDataColumnSidecar(ctx, sidecar); err != nil {
		return errors.Wrap(err, "receive data column sidecar")
	}

	var wg errgroup.Group
	wg.Go(func() error {
		// Broadcast our complete column for peers that don't use partial messages
		if err := s.cfg.p2p.BroadcastDataColumnSidecars(ctx, []blocks.VerifiedRODataColumn{sidecar}, nil); err != nil {
			return errors.Wrap(err, "broadcast data column sidecars")
		}

		return nil
	})

	if err := s.processDataColumnSidecarsFromReconstruction(ctx, sidecar); err != nil {
		return errors.Wrap(err, "process data column sidecars from reconstruction")
	}

	return wg.Wait()
}

// receiveDataColumnSidecar receives a single data column sidecar: marks it as seen and saves it to the chain.
// Do not loop over this function to receive multiple sidecars, use receiveDataColumnSidecars instead.
func (s *Service) receiveDataColumnSidecar(ctx context.Context, sidecar blocks.VerifiedRODataColumn) error {
	return s.receiveDataColumnSidecars(ctx, []blocks.VerifiedRODataColumn{sidecar})
}

// receiveDataColumnSidecars receives multiple data column sidecars: marks them as seen and saves them to the chain.
func (s *Service) receiveDataColumnSidecars(ctx context.Context, sidecars []blocks.VerifiedRODataColumn) error {
	for _, sidecar := range sidecars {
		if sidecar.IsGloas() {
			s.setSeenDataColumnRootIndex(sidecar.BlockRoot(), sidecar.Index(), sidecar.Slot())
		} else {
			proposerIndex, err := sidecar.ProposerIndex()
			if err != nil {
				return err
			}
			s.setSeenDataColumnIndex(sidecar.Slot(), proposerIndex, sidecar.Index())
		}
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

func wrapDataColumnError(sidecar blocks.VerifiedRODataColumn, message string, err error) error {
	return fmt.Errorf("%s - slot %d, root %s: %w", message, sidecar.Slot(), fmt.Sprintf("%#x", sidecar.BlockRoot()), err)
}
