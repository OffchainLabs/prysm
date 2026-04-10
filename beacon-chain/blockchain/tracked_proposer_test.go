package blockchain

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common"
)

func TestTrackedProposer_NotTracked(t *testing.T) {
	service, _ := minimalTestService(t, WithPayloadIDCache(cache.NewPayloadIDCache()))
	st, _ := util.DeterministicGenesisStateBellatrix(t, 1)
	_, ok := service.trackedProposer(st, 0)
	require.Equal(t, false, ok)
}

func TestTrackedProposer_Tracked(t *testing.T) {
	service, _ := minimalTestService(t, WithPayloadIDCache(cache.NewPayloadIDCache()))
	st, _ := util.DeterministicGenesisStateBellatrix(t, 1)
	addr := common.HexToAddress("0x1234")
	service.cfg.TrackedValidatorsCache.Set(cache.TrackedValidator{Active: true, FeeRecipient: primitives.ExecutionAddress(addr), Index: 0})
	val, ok := service.trackedProposer(st, 0)
	require.Equal(t, true, ok)
	require.Equal(t, primitives.ExecutionAddress(addr), val.FeeRecipient)
}

func TestTrackedProposer_PrepareAllPayloads_Default(t *testing.T) {
	resetCfg := features.InitWithReset(&features.Flags{PrepareAllPayloads: true})
	defer resetCfg()

	service, _ := minimalTestService(t, WithPayloadIDCache(cache.NewPayloadIDCache()))
	st, _ := util.DeterministicGenesisStateBellatrix(t, 1)
	val, ok := service.trackedProposer(st, 0)
	require.Equal(t, true, ok)
	require.Equal(t, true, val.Active)
	require.Equal(t, params.BeaconConfig().EthBurnAddressHex, common.BytesToAddress(val.FeeRecipient[:]).String())
}

func TestTrackedProposer_PrepareAllPayloads_WithProposerPreference(t *testing.T) {
	resetCfg := features.InitWithReset(&features.Flags{PrepareAllPayloads: true})
	defer resetCfg()

	prefCache := cache.NewProposerPreferencesCache()
	service, _ := minimalTestService(t,
		WithPayloadIDCache(cache.NewPayloadIDCache()),
		WithProposerPreferencesCache(prefCache),
	)
	st, _ := util.DeterministicGenesisStateBellatrix(t, 1)

	addr := common.HexToAddress("0xabcd")
	prefCache.Add(0, 0, addr.Bytes(), 42_000_000)

	val, ok := service.trackedProposer(st, 0)
	require.Equal(t, true, ok)
	require.Equal(t, true, val.Active)
	require.Equal(t, primitives.ExecutionAddress(addr), val.FeeRecipient)
	require.Equal(t, uint64(42_000_000), val.GasLimit)
}

// TestTrackedProposer_ReorgSwitchesProposerPreference proves that after a
// reorg changes the proposer lookahead, trackedProposer resolves the correct
// (new) proposer's preference for the same slot.
func TestTrackedProposer_ReorgSwitchesProposerPreference(t *testing.T) {
	resetCfg := features.InitWithReset(&features.Flags{PrepareAllPayloads: true})
	defer resetCfg()

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	prefCache := cache.NewProposerPreferencesCache()
	service, _ := minimalTestService(t,
		WithPayloadIDCache(cache.NewPayloadIDCache()),
		WithProposerPreferencesCache(prefCache),
	)

	spe := params.BeaconConfig().SlotsPerEpoch
	st, _ := util.DeterministicGenesisStateGloas(t, 64)
	targetSlot := primitives.Slot(5) // slot 5, current epoch

	// Build a proposer lookahead where validator 10 is the proposer at slot 5.
	lookahead := make([]primitives.ValidatorIndex, 2*spe)
	lookahead[targetSlot%spe] = 10
	require.NoError(t, st.SetProposerLookahead(lookahead))

	// Cache preferences for both validators at the same slot — this is
	// possible because the cache is keyed by (slot, validatorIndex).
	v1Addr := common.HexToAddress("0xaaaa")
	v2Addr := common.HexToAddress("0xbbbb")
	prefCache.Add(targetSlot, 10, v1Addr.Bytes(), 30_000_000)
	prefCache.Add(targetSlot, 20, v2Addr.Bytes(), 40_000_000)

	// Before reorg: proposer at slot 5 is validator 10.
	val, ok := service.trackedProposer(st, targetSlot)
	require.Equal(t, true, ok)
	require.Equal(t, primitives.ExecutionAddress(v1Addr), val.FeeRecipient)
	require.Equal(t, uint64(30_000_000), val.GasLimit)

	// Simulate reorg: proposer lookahead changes, now validator 20 is the
	// proposer at slot 5. This happens when a reorg changes the RANDAO mix
	// at an epoch boundary, producing a different proposer shuffling.
	lookahead2 := make([]primitives.ValidatorIndex, 2*spe)
	lookahead2[targetSlot%spe] = 20
	require.NoError(t, st.SetProposerLookahead(lookahead2))

	// After reorg: same slot 5, different proposer → different preference.
	val, ok = service.trackedProposer(st, targetSlot)
	require.Equal(t, true, ok)
	require.Equal(t, primitives.ExecutionAddress(v2Addr), val.FeeRecipient)
	require.Equal(t, uint64(40_000_000), val.GasLimit)
}

func TestTrackedProposer_TrackedWithProposerPreferenceOverride(t *testing.T) {
	prefCache := cache.NewProposerPreferencesCache()
	service, _ := minimalTestService(t,
		WithPayloadIDCache(cache.NewPayloadIDCache()),
		WithProposerPreferencesCache(prefCache),
	)
	st, _ := util.DeterministicGenesisStateBellatrix(t, 1)

	trackedAddr := common.HexToAddress("0x1111")
	prefAddr := common.HexToAddress("0x2222")
	service.cfg.TrackedValidatorsCache.Set(cache.TrackedValidator{Active: true, FeeRecipient: primitives.ExecutionAddress(trackedAddr), Index: 0})
	prefCache.Add(0, 0, prefAddr.Bytes(), 50_000_000)

	val, ok := service.trackedProposer(st, 0)
	require.Equal(t, true, ok)
	// Proposer preference overrides tracked validator.
	require.Equal(t, primitives.ExecutionAddress(prefAddr), val.FeeRecipient)
	require.Equal(t, uint64(50_000_000), val.GasLimit)
}
