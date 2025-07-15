// Package attestations defines an attestation pool
// service implementation which is used to manage the lifecycle
// of aggregated, unaggregated, and fork-choice attestations.
package attestations

import (
	"context"
	"errors"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	lruwrpr "github.com/OffchainLabs/prysm/v6/cache/lru"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/config/params"
	lru "github.com/hashicorp/golang-lru"
)

var forkChoiceProcessedAttsSize = 1 << 16

// Service of attestation pool operations.
type Service struct {
	cfg                     *Config
	ctx                     context.Context
	cancel                  context.CancelFunc
	err                     error
	forkChoiceProcessedAtts *lru.Cache
	genesisTime             time.Time
}

// Config options for the service.
type Config struct {
	Cache               *cache.AttestationCache
	Pool                Pool
	pruneInterval       time.Duration
	InitialSyncComplete chan struct{}
}

// NewService instantiates a new attestation service instance that will
// be registered into a running beacon node.
func NewService(ctx context.Context, cfg *Config) (*Service, error) {
	cache := lruwrpr.New(forkChoiceProcessedAttsSize)

	if cfg.pruneInterval == 0 {
		// Prune expired attestations from the pool every slot interval.
		cfg.pruneInterval = time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second
	}

	ctx, cancel := context.WithCancel(ctx)
	return &Service{
		cfg:                     cfg,
		ctx:                     ctx,
		cancel:                  cancel,
		forkChoiceProcessedAtts: cache,
	}, nil
}

// Start an attestation pool service's main event loop.
func (s *Service) Start() {
	if err := s.waitForSync(s.cfg.InitialSyncComplete); err != nil {
		log.WithError(err).Error("failed to wait for initial sync")
		return
	}
	go s.prepareForkChoiceAtts()

	if features.Get().EnableExperimentalAttestationPool {
		go s.pruneExpiredExperimental()
	} else {
		go s.pruneExpired()
	}
}

// waitForSync waits until the beacon node is synced to the latest head.
func (s *Service) waitForSync(syncChan chan struct{}) error {
	select {
	case <-syncChan:
		return nil
	case <-s.ctx.Done():
		return errors.New("context closed, exiting goroutine")
	}
}

// Stop the beacon block attestation pool service's main event loop
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

// SetGenesisTime sets genesis time for operation service to use.
func (s *Service) SetGenesisTime(t time.Time) {
	s.genesisTime = t.Truncate(time.Second) // Genesis time has a precision of 1 second.
}
