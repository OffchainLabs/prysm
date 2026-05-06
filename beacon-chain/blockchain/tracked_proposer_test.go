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
	prefCache.Add(0, addr.Bytes(), 42_000_000)

	val, ok := service.trackedProposer(st, 0)
	require.Equal(t, true, ok)
	require.Equal(t, true, val.Active)
	require.Equal(t, primitives.ExecutionAddress(addr), val.FeeRecipient)
	require.Equal(t, uint64(42_000_000), val.GasLimit)
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
	prefCache.Add(0, prefAddr.Bytes(), 50_000_000)

	val, ok := service.trackedProposer(st, 0)
	require.Equal(t, true, ok)
	// Proposer preference overrides tracked validator.
	require.Equal(t, primitives.ExecutionAddress(prefAddr), val.FeeRecipient)
	require.Equal(t, uint64(50_000_000), val.GasLimit)
}

func TestTrackedProposer_PostGloas_BypassesTrackedValidatorsCache(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	service, _ := minimalTestService(t, WithPayloadIDCache(cache.NewPayloadIDCache()))
	st, _ := util.DeterministicGenesisStateBellatrix(t, 1)

	stalePreGloasFeeRecipient := common.HexToAddress("0xdead000000000000000000000000000000000000")
	service.cfg.TrackedValidatorsCache.Set(cache.TrackedValidator{
		Active:       true,
		Index:        0,
		FeeRecipient: primitives.ExecutionAddress(stalePreGloasFeeRecipient),
	})

	val, ok := service.trackedProposer(st, 0)
	require.Equal(t, true, ok)
	require.Equal(t, true, val.Active)
	require.Equal(t, primitives.ExecutionAddress{}, val.FeeRecipient)
}
