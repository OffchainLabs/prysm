package cache_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/stretchr/testify/require"
)

func TestAttestationConsensusData_IsFreshFor(t *testing.T) {
	headRoot := [32]byte{1}
	a := &cache.AttestationConsensusData{Slot: 1, HeadRoot: headRoot[:]}

	var missing *cache.AttestationConsensusData
	require.False(t, missing.IsFreshFor(1, headRoot, false), "nil data is never fresh")
	require.True(t, a.IsFreshFor(1, headRoot, false))
	require.False(t, a.IsFreshFor(2, headRoot, false), "another slot")
	require.False(t, a.IsFreshFor(1, [32]byte{2}, false), "another head root")
	require.False(t, a.IsFreshFor(1, headRoot, true), "another head payload status")
}

func TestAttestationCache_RoundTrip(t *testing.T) {
	c := cache.NewAttestationDataCache()

	a := c.Get()
	require.Nil(t, a)

	insert := &cache.AttestationConsensusData{
		Slot:     1,
		HeadRoot: []byte{1},
		Target: forkchoicetypes.Checkpoint{
			Epoch: 2,
			Root:  [32]byte{3},
		},
		Source: forkchoicetypes.Checkpoint{
			Epoch: 4,
			Root:  [32]byte{5},
		},
	}
	err := c.Put(insert)
	require.NoError(t, err)

	a = c.Get()
	require.Equal(t, insert, a)

	insert = &cache.AttestationConsensusData{
		Slot:     6,
		HeadRoot: []byte{7},
		Target: forkchoicetypes.Checkpoint{
			Epoch: 8,
			Root:  [32]byte{9},
		},
		Source: forkchoicetypes.Checkpoint{
			Epoch: 10,
			Root:  [32]byte{11},
		},
	}

	err = c.Put(insert)
	require.NoError(t, err)

	a = c.Get()
	require.Equal(t, insert, a)
}
