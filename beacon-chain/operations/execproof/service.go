package execproof

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// Service manages the execution proof pool operations.
type Service struct {
	cfg        *Config
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	slotTicker slots.Ticker
}

// Config options for the service.
type Config struct {
	Pool             PoolManager
	pruneInterval    time.Duration
	ClockWaiter      startup.ClockWaiter
	FinalizedFetcher FinalizedCheckptFetcher
}

// FinalizedCheckptFetcher defines the interface for fetching the finalized checkpoint.
type FinalizedCheckptFetcher interface {
	FinalizedCheckpt() *ethpb.Checkpoint
}

// NewService instantiates a new execution proof service instance that will
// be registered into a running beacon node.
func NewService(ctx context.Context, cfg *Config) (*Service, error) {
	if cfg.pruneInterval == 0 {
		// Prune finalized execution proofs from the pool every slot interval.
		cfg.pruneInterval = time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second
	}

	ctx, cancel := context.WithCancel(ctx)
	return &Service{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// Start the execution proof pool service's main event loop.
func (s *Service) Start() {
	clock, err := s.cfg.ClockWaiter.WaitForClock(s.ctx)
	if err != nil {
		log.WithError(err).Error("failed to wait for clock")
		return
	}
	log.Info("Execution proof service starting after clock is set")
	s.slotTicker = slots.NewSlotTicker(
		slots.UnsafeStartTime(clock.GenesisTime(), 0),
		params.BeaconConfig().SecondsPerSlot,
	)

	go s.pruneFinalizedProofs()
}

// Stop the execution proof pool service's main event loop
// and associated goroutines.
func (s *Service) Stop() error {
	defer s.cancel()
	return nil
}

// Status returns the current service err if there's any.
func (s *Service) Status() error {
	if s.err != nil {
		return s.err
	}
	return nil
}
