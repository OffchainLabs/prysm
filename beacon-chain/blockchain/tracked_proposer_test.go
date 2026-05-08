package blockchain

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common"
)

// trackedProposer now anchors preferences on dependent_root derived from the
// passed state (state.block_roots lookup). At slot 0 the lookup underflows so
// proposerPreference falls back to the no-cache path; cached-preference
// behavior is exercised end-to-end by the gossip and bid validation tests
// under beacon-chain/sync.

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
	service.cfg.ProposerPreferencesCache.Set(cache.ProposerPreference{FeeRecipient: addr.Bytes(), ValidatorIndex: 0})
	val, ok := service.trackedProposer(st, 0)
	require.Equal(t, true, ok)
	require.DeepEqual(t, addr.Bytes(), val.FeeRecipient)
}

func TestTrackedProposer_PrepareAllPayloads_Default(t *testing.T) {
	resetCfg := features.InitWithReset(&features.Flags{PrepareAllPayloads: true})
	defer resetCfg()

	service, _ := minimalTestService(t, WithPayloadIDCache(cache.NewPayloadIDCache()))
	st, _ := util.DeterministicGenesisStateBellatrix(t, 1)
	val, ok := service.trackedProposer(st, 0)
	require.Equal(t, true, ok)
	require.Equal(t, params.BeaconConfig().EthBurnAddressHex, common.BytesToAddress(val.FeeRecipient).String())
}
