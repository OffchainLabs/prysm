package eth_test

import (
	"fmt"
	"testing"

	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	fuzz "github.com/google/gofuzz"
)

func fuzzCopies[T any, C eth.Copier[T]](t *testing.T, obj C) {
	fuzzer := fuzz.NewWithSeed(0)
	amount := 1000
	t.Run(fmt.Sprintf("%T", obj), func(t *testing.T) {
		for i := 0; i < amount; i++ {
			fuzzer.Fuzz(obj) // Populate thing with random values

			got := obj.Copy()
			require.DeepEqual(t, obj, got)
			// TODO: add deep fuzzing and checks for deep not equals
			// we should test that modifying the copy doesn't modify the original object
		}
	})
}
