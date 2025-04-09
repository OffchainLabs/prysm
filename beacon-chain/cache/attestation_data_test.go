package cache_test

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/cache"
	forkchoicetypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/forkchoice/types"
	"github.com/stretchr/testify/require"
)

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
