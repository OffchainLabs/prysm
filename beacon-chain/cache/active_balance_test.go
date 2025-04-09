//go:build !fuzz

package cache_test

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/cache"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestBalanceCache_AddGetBalance(t *testing.T) {
	blockRoots := make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot)
	for i := 0; i < len(blockRoots); i++ {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, uint64(i))
		blockRoots[i] = b
	}
	raw := &ethpb.BeaconState{
		BlockRoots: blockRoots,
	}
	st, err := state_native.InitializeFromProtoPhase0(raw)
	require.NoError(t, err)

	cc := cache.NewEffectiveBalanceCache()
	_, err = cc.Get(st)
	require.ErrorContains(t, cache.ErrNotFound.Error(), err)

	b := uint64(100)
	require.NoError(t, cc.AddTotalEffectiveBalance(st, b))
	cachedB, err := cc.Get(st)
	require.NoError(t, err)
	require.Equal(t, b, cachedB)

	require.NoError(t, st.SetSlot(1000))
	_, err = cc.Get(st)
	require.ErrorContains(t, cache.ErrNotFound.Error(), err)

	b = uint64(200)
	require.NoError(t, cc.AddTotalEffectiveBalance(st, b))
	cachedB, err = cc.Get(st)
	require.NoError(t, err)
	require.Equal(t, b, cachedB)

	require.NoError(t, st.SetSlot(1000+params.BeaconConfig().SlotsPerHistoricalRoot))
	_, err = cc.Get(st)
	require.ErrorContains(t, cache.ErrNotFound.Error(), err)

	b = uint64(300)
	require.NoError(t, cc.AddTotalEffectiveBalance(st, b))
	cachedB, err = cc.Get(st)
	require.NoError(t, err)
	require.Equal(t, b, cachedB)
}

func TestBalanceCache_BalanceKey(t *testing.T) {
	blockRoots := make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot)
	for i := 0; i < len(blockRoots); i++ {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, uint64(i))
		blockRoots[i] = b
	}
	raw := &ethpb.BeaconState{
		BlockRoots: blockRoots,
	}
	st, err := state_native.InitializeFromProtoPhase0(raw)
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(primitives.Slot(math.MaxUint64)))

	_, err = cache.BalanceCacheKey(st)
	require.NoError(t, err)
}
