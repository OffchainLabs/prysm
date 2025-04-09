package cache

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestRegistrationCache(t *testing.T) {
	pubkey, err := hexutil.Decode("0x93247f2209abcacf57b75a51dafae777f9dd38bc7053d1af526f220a7489a6d3a2753e5f3e8b1cfe39b56f43611df74a")
	require.NoError(t, err)
	validatorIndex := primitives.ValidatorIndex(1)
	cache := NewRegistrationCache()
	m := make(map[primitives.ValidatorIndex]*ethpb.ValidatorRegistrationV1)

	m[validatorIndex] = &ethpb.ValidatorRegistrationV1{
		FeeRecipient: []byte{},
		GasLimit:     100,
		Timestamp:    uint64(time.Now().Unix()),
		Pubkey:       pubkey,
	}
	cache.UpdateIndexToRegisteredMap(context.Background(), m)
	reg, err := cache.RegistrationByIndex(validatorIndex)
	require.NoError(t, err)
	require.Equal(t, string(reg.Pubkey), string(pubkey))
	t.Run("successfully updates", func(t *testing.T) {
		pubkey, err := hexutil.Decode("0x88247f2209abcacf57b75a51dafae777f9dd38bc7053d1af526f220a7489a6d3a2753e5f3e8b1cfe39b56f43611df74a")
		require.NoError(t, err)
		validatorIndex2 := primitives.ValidatorIndex(2)
		m[validatorIndex2] = &ethpb.ValidatorRegistrationV1{
			FeeRecipient: []byte{},
			GasLimit:     100,
			Timestamp:    uint64(time.Now().Unix()),
			Pubkey:       pubkey,
		}
		cache.UpdateIndexToRegisteredMap(context.Background(), m)
		reg, err := cache.RegistrationByIndex(validatorIndex2)
		require.NoError(t, err)
		require.Equal(t, string(reg.Pubkey), string(pubkey))
	})
}
