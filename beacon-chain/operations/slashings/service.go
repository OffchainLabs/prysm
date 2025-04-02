package slashings

import (
	"context"
	"time"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// WithElectraTimer includes functional options for the blockchain service related to CLI flags.
func WithElectraTimer(cw startup.ClockWaiter, currentSlotFn func() primitives.Slot) Option {
	return func(p *PoolService) error {
		p.runElectraTimer = true
		p.cw = cw
		p.currentSlotFn = currentSlotFn
		return nil
	}
}

// NewPoolService returns a service that manages the Pool.
func NewPoolService(ctx context.Context, pool PoolManager, opts ...Option) *PoolService {
	ctx, cancel := context.WithCancel(ctx)
	p := &PoolService{
		ctx:         ctx,
		cancel:      cancel,
		poolManager: pool,
	}

	for _, opt := range opts {
		if err := opt(p); err != nil {
			return nil
		}
	}

	return p
}

// Start the slashing pool service.
func (p *PoolService) Start() {
	go p.run()
}

func (p *PoolService) run() {
	if !p.runElectraTimer {
		return
	}

	electraSlot, err := slots.EpochStart(params.BeaconConfig().ElectraForkEpoch)
	if err != nil {
		return
	}

	// If run() is executed after the transition to Electra has already happened,
	// there is nothing to convert because the slashing pool is empty at startup.
	if p.currentSlotFn() >= electraSlot {
		return
	}

	p.waitForChainInitialization()

	electraTime, err := slots.ToTime(uint64(p.clock.GenesisTime().Unix()), electraSlot)
	if err != nil {
		return
	}

	t := time.NewTimer(electraTime.Sub(p.clock.Now()))
	defer t.Stop()

	select {
	case <-t.C:
		log.Info("Converting Phase0 slashings to Electra slashings")
		p.poolManager.ConvertToElectra()
		return
	case <-p.ctx.Done():
		log.Warn("Context cancelled, ConvertToElectra timer will not execute")
		return
	}
}

func (p *PoolService) waitForChainInitialization() {
	clock, err := p.cw.WaitForClock(p.ctx)
	if err != nil {
		log.WithError(err).Error("Could not receive chain start notification")
	}
	p.clock = clock
	log.WithField("genesisTime", clock.GenesisTime()).Info(
		"Slashing pool service received chain initialization event",
	)
}

// Stop the slashing pool service.
func (p *PoolService) Stop() error {
	p.cancel()
	return nil
}

// Status of the slashing pool service.
func (p *PoolService) Status() error {
	return nil
}
