package stateutil_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
)

func BenchmarkUint64ListRootWithRegistryLimit(b *testing.B) {
	balances := make([]uint64, 100000)
	for i := range balances {
		balances[i] = uint64(i)
	}
	b.Run("100k balances", func(b *testing.B) {
		for b.Loop() {
			_, err := stateutil.Uint64ListRootWithRegistryLimit(balances)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
