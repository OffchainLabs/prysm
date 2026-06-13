package node

import (
	"context"
	"os"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
)

// waitForPendingProposals blocks while any validator controlled by a connected
// validator client still has a block-proposal duty in the current or next epoch.
// It returns when no such duty remains or when the operator sends a second
// shutdown signal on sigc.
func (b *BeaconNode) waitForPendingProposals(sigc <-chan os.Signal) {
	var chainService *blockchain.Service
	if err := b.services.FetchService(&chainService); err != nil {
		log.WithError(err).Warning("Could not fetch blockchain service, proceeding with shutdown")
		return
	}

	genesis := chainService.GenesisTime()
	if genesis.IsZero() {
		// The chain has not started, there can be no proposal duties to wait for.
		return
	}

	// Re-evaluate the pending duties on every slot boundary.
	ticker := slots.NewSlotTicker(genesis, params.BeaconConfig().SecondsPerSlot)
	defer ticker.Done()

	b.waitForPendingProposalsLoop(sigc, ticker.C(), genesis, chainService.HeadState, chainService.CurrentSlot)
}

func (b *BeaconNode) waitForPendingProposalsLoop(
	sigc <-chan os.Signal,
	tick <-chan primitives.Slot,
	genesis time.Time,
	headStateFn func(context.Context) (state.BeaconState, error),
	currentSlotFn func() primitives.Slot,
) {
	for {
		tracked := b.trackedValidatorsCache.Indices()
		// No validator client connected, nothing to wait for.
		if len(tracked) == 0 {
			return
		}

		st, err := headStateFn(b.ctx)
		if err != nil {
			log.WithError(err).Warning("Could not get head state while checking pending proposals, proceeding with shutdown")
			return
		}

		slot, valIdx, ok := nextTrackedProposalSlot(b.ctx, st, currentSlotFn(), tracked)
		if !ok {
			return
		}

		fields := logrus.Fields{"validatorIndex": valIdx, "slot": slot}
		if slotTime, err := slots.StartTime(genesis, slot); err == nil {
			fields["eta"] = time.Until(slotTime).Round(time.Second)
		}
		log.WithFields(fields).Warning("Postponing shutdown: a connected validator client has an upcoming block proposal. Send the shutdown signal again (SIGINT/SIGTERM, e.g. Ctrl-C on Linux) to force the node to stop immediately")

		select {
		case <-sigc:
			log.Info("Got second shutdown signal, forcing shutdown")
			return
		case <-tick:
			// Re-evaluate on the next loop iteration.
		}
	}
}

// nextTrackedProposalSlot returns the earliest upcoming slot (>= the current
// slot) at which a validator in the tracked set is assigned to propose, looking
// at the current and next epoch. The boolean is false when no such duty exists
// or the assignments cannot be computed.
func nextTrackedProposalSlot(ctx context.Context, st state.BeaconState, cur primitives.Slot, tracked map[primitives.ValidatorIndex]bool) (primitives.Slot, primitives.ValidatorIndex, bool) {
	curEpoch := slots.ToEpoch(cur)

	var (
		best    primitives.Slot
		bestIdx primitives.ValidatorIndex
		found   bool
	)

	for epoch := curEpoch; epoch <= curEpoch+params.BeaconConfig().MinSeedLookahead; epoch++ {
		assignments, err := helpers.ProposerAssignments(ctx, st, epoch)
		if err != nil {
			log.WithError(err).WithField("epoch", epoch).Warning("Could not compute proposer assignments while checking pending proposals")
			continue
		}

		for idx := range tracked {
			for _, slot := range assignments[idx] {
				if slot < cur {
					continue
				}
				if !found || slot < best {
					best, bestIdx, found = slot, idx, true
				}
			}
		}
	}

	return best, bestIdx, found
}
