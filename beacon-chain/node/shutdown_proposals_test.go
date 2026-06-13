package node

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/runtime"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/pkg/errors"
)

// deterministicGenesisWithEarliestProposer returns a genesis beacon state along
// with the validator index and slot of the earliest proposer assignment in the
// genesis epoch.
func deterministicGenesisWithEarliestProposer(t *testing.T) (state.BeaconState, primitives.ValidatorIndex, primitives.Slot) {
	helpers.ClearCache()

	depChainStart := params.MinimalSpecConfig().MinGenesisActiveValidatorCount
	deposits, _, err := util.DeterministicDepositsAndKeys(depChainStart)
	require.NoError(t, err)
	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)
	bs, err := transition.GenesisBeaconState(t.Context(), deposits, 0, eth1Data)
	require.NoError(t, err)

	assignments, err := helpers.ProposerAssignments(t.Context(), bs, 0)
	require.NoError(t, err)
	require.Equal(t, true, len(assignments) > 0)

	var earliestIdx primitives.ValidatorIndex
	earliestSlot := primitives.Slot(1 << 62)
	for idx, slts := range assignments {
		for _, s := range slts {
			if s < earliestSlot {
				earliestSlot, earliestIdx = s, idx
			}
		}
	}
	return bs, earliestIdx, earliestSlot
}

func TestNextTrackedProposalSlot(t *testing.T) {
	bs, earliestIdx, earliestSlot := deterministicGenesisWithEarliestProposer(t)

	t.Run("tracked validator with upcoming duty is found", func(t *testing.T) {
		tracked := map[primitives.ValidatorIndex]bool{earliestIdx: true}
		slot, idx, ok := nextTrackedProposalSlot(t.Context(), bs, 0, tracked)
		require.Equal(t, true, ok)
		require.Equal(t, earliestSlot, slot)
		require.Equal(t, earliestIdx, idx)
	})

	t.Run("non-proposing validator index returns no duty", func(t *testing.T) {
		// An index beyond the validator set is never assigned to propose.
		tracked := map[primitives.ValidatorIndex]bool{primitives.ValidatorIndex(params.BeaconConfig().MinGenesisActiveValidatorCount + 1000): true}
		_, _, ok := nextTrackedProposalSlot(t.Context(), bs, 0, tracked)
		require.Equal(t, false, ok)
	})

	t.Run("empty tracked set returns no duty", func(t *testing.T) {
		_, _, ok := nextTrackedProposalSlot(t.Context(), bs, 0, map[primitives.ValidatorIndex]bool{})
		require.Equal(t, false, ok)
	})

	t.Run("returned duty is never before the current slot", func(t *testing.T) {
		// Advancing the current slot past the earliest duty must never return it.
		tracked := map[primitives.ValidatorIndex]bool{earliestIdx: true}
		cur := earliestSlot + 1
		slot, _, ok := nextTrackedProposalSlot(t.Context(), bs, cur, tracked)
		if ok {
			require.Equal(t, true, slot >= cur)
		}
	})
}

func TestWaitForPendingProposalsLoop(t *testing.T) {
	bs, earliestIdx, _ := deterministicGenesisWithEarliestProposer(t)

	// genesis is only used for ETA logging here; any non-zero time works.
	genesis := time.Unix(0, 0).Add(time.Hour)
	headStateOK := func(context.Context) (state.BeaconState, error) { return bs, nil }
	currentSlotZero := func() primitives.Slot { return 0 }

	newNode := func(tracked ...primitives.ValidatorIndex) *BeaconNode {
		c := cache.NewTrackedValidatorsCache()
		for _, idx := range tracked {
			c.Set(cache.TrackedValidator{Active: true, Index: idx})
		}
		return &BeaconNode{ctx: context.Background(), trackedValidatorsCache: c}
	}

	run := func(n *BeaconNode, sigc <-chan os.Signal, tick <-chan primitives.Slot, headStateFn func(context.Context) (state.BeaconState, error)) <-chan struct{} {
		done := make(chan struct{})
		go func() {
			n.waitForPendingProposalsLoop(sigc, tick, genesis, headStateFn, currentSlotZero)
			close(done)
		}()
		return done
	}
	requireReturns := func(t *testing.T, done <-chan struct{}) {
		t.Helper()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("waitForPendingProposalsLoop did not return")
		}
	}
	requireBlocks := func(t *testing.T, done <-chan struct{}) {
		t.Helper()
		select {
		case <-done:
			t.Fatal("waitForPendingProposalsLoop returned but should block on a pending duty")
		case <-time.After(100 * time.Millisecond):
		}
	}

	t.Run("returns when no validators are tracked", func(t *testing.T) {
		requireReturns(t, run(newNode(), nil, nil, headStateOK))
	})

	t.Run("returns when the head state cannot be read", func(t *testing.T) {
		headErr := func(context.Context) (state.BeaconState, error) { return nil, errors.New("boom") }
		requireReturns(t, run(newNode(earliestIdx), nil, nil, headErr))
	})

	t.Run("returns when the tracked validator has no upcoming duty", func(t *testing.T) {
		// An index beyond the validator set is never assigned to propose.
		notAProposer := primitives.ValidatorIndex(params.BeaconConfig().MinGenesisActiveValidatorCount + 1000)
		requireReturns(t, run(newNode(notAProposer), nil, nil, headStateOK))
	})

	t.Run("blocks while a duty is pending, then a second signal forces stop", func(t *testing.T) {
		sigc := make(chan os.Signal, 1)
		done := run(newNode(earliestIdx), sigc, make(chan primitives.Slot), headStateOK)
		requireBlocks(t, done)
		sigc <- syscall.SIGINT
		requireReturns(t, done)
	})

	t.Run("re-evaluates on each tick and returns once duties clear", func(t *testing.T) {
		n := newNode(earliestIdx)
		tick := make(chan primitives.Slot)
		done := run(n, make(chan os.Signal), tick, headStateOK)
		requireBlocks(t, done)
		// The validator client disconnects: its tracked entries are gone, so the
		// next slot tick must let the shutdown proceed.
		n.trackedValidatorsCache.Prune()
		tick <- 1
		requireReturns(t, done)
	})
}

func TestWaitForPendingProposals_EarlyReturns(t *testing.T) {
	requireReturns := func(t *testing.T, n *BeaconNode) {
		t.Helper()
		done := make(chan struct{})
		go func() {
			n.waitForPendingProposals(make(chan os.Signal))
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("waitForPendingProposals did not return")
		}
	}

	t.Run("no blockchain service registered", func(t *testing.T) {
		n := &BeaconNode{ctx: context.Background(), services: runtime.NewServiceRegistry()}
		requireReturns(t, n)
	})

	t.Run("chain not started (zero genesis time)", func(t *testing.T) {
		registry := runtime.NewServiceRegistry()
		require.NoError(t, registry.RegisterService(&blockchain.Service{}))
		n := &BeaconNode{ctx: context.Background(), services: registry}
		requireReturns(t, n)
	})
}
