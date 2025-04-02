package enginev1_test

import (
	"fmt"
	"testing"

	fuzz "github.com/google/gofuzz"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestCopyExecutionPayload_Fuzz(t *testing.T) {
	fuzzCopies(t, &enginev1.ExecutionPayloadDeneb{})
	fuzzCopies(t, &enginev1.ExecutionPayloadCapella{})
	fuzzCopies(t, &enginev1.ExecutionPayload{})
}

func TestCopyExecutionPayloadHeader_Fuzz(t *testing.T) {
	fuzzCopies(t, &enginev1.ExecutionPayloadHeaderDeneb{})
	fuzzCopies(t, &enginev1.ExecutionPayloadHeaderCapella{})
	fuzzCopies(t, &enginev1.ExecutionPayloadHeader{})
}

func fuzzCopies[T any, C enginev1.Copier[T]](t *testing.T, obj C) {
	fuzzer := fuzz.NewWithSeed(0)
	amount := 1000
	t.Run(fmt.Sprintf("%T", obj), func(t *testing.T) {
		for i := 0; i < amount; i++ {
			fuzzer.Fuzz(obj) // Populate thing with random values
			got := obj.Copy()
			require.DeepEqual(t, obj, got)
			// check shallow copy working
			fuzzer.Fuzz(got)
			require.DeepNotEqual(t, obj, got)
			// TODO: think of deeper not equal fuzzing
		}
	})
}
