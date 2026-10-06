package equality_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz/equality"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
)

func TestDeepEqualBasicTypes(t *testing.T) {
	assert.Equal(t, true, equality.DeepEqual(true, true))
	assert.Equal(t, false, equality.DeepEqual(true, false))

	assert.Equal(t, true, equality.DeepEqual(byte(222), byte(222)))
	assert.Equal(t, false, equality.DeepEqual(byte(222), byte(111)))

	assert.Equal(t, true, equality.DeepEqual(uint64(1234567890), uint64(1234567890)))
	assert.Equal(t, false, equality.DeepEqual(uint64(1234567890), uint64(987653210)))
	assert.Equal(t, true, equality.DeepEqual(primitives.BuilderIndex(1), primitives.BuilderIndex(1)))
	assert.Equal(t, false, equality.DeepEqual(primitives.BuilderIndex(1), primitives.BuilderIndex(2)))
	assert.Equal(t, false, equality.DeepEqual(primitives.BuilderIndex(1), uint64(1)))

	assert.Equal(t, true, equality.DeepEqual("hello", "hello"))
	assert.Equal(t, false, equality.DeepEqual("hello", "world"))

	assert.Equal(t, true, equality.DeepEqual([3]byte{1, 2, 3}, [3]byte{1, 2, 3}))
	assert.Equal(t, false, equality.DeepEqual([3]byte{1, 2, 3}, [3]byte{1, 2, 4}))

	var nilSlice1, nilSlice2 []byte
	assert.Equal(t, true, equality.DeepEqual(nilSlice1, nilSlice2))
	assert.Equal(t, true, equality.DeepEqual(nilSlice1, []byte{}))
	assert.Equal(t, true, equality.DeepEqual([]byte{1, 2, 3}, []byte{1, 2, 3}))
	assert.Equal(t, false, equality.DeepEqual([]byte{1, 2, 3}, []byte{1, 2, 4}))
}

func TestDeepEqualStructs(t *testing.T) {
	type Store struct {
		V1 uint64
		V2 []byte
	}
	store1 := Store{uint64(1234), nil}
	store2 := Store{uint64(1234), []byte{}}
	store3 := Store{uint64(4321), []byte{}}
	assert.Equal(t, true, equality.DeepEqual(store1, store2))
	assert.Equal(t, false, equality.DeepEqual(store1, store3))
}

func TestDeepEqualStructs_Unexported(t *testing.T) {
	type Store struct {
		V1           uint64
		V2           []byte
		dontIgnoreMe string
	}
	store1 := Store{uint64(1234), nil, "hi there"}
	store2 := Store{uint64(1234), []byte{}, "hi there"}
	store3 := Store{uint64(4321), []byte{}, "wow"}
	store4 := Store{uint64(4321), []byte{}, "bow wow"}
	assert.Equal(t, true, equality.DeepEqual(store1, store2))
	assert.Equal(t, false, equality.DeepEqual(store1, store3))
	assert.Equal(t, false, equality.DeepEqual(store3, store4))
}

func TestDeepEqualProto(t *testing.T) {
	t.Run("nested messages", func(t *testing.T) {
		type state struct{ Fork *ethpb.Fork }
		a := &state{Fork: &ethpb.Fork{Epoch: 1}}
		b := &state{Fork: &ethpb.Fork{Epoch: 1}}
		assert.Equal(t, true, equality.DeepEqual(a, b))
		b.Fork.Epoch = 2
		assert.Equal(t, false, equality.DeepEqual(a, b))
		assert.Equal(t, false, equality.DeepEqual([]*ethpb.Fork{a.Fork}, []*ethpb.Fork{b.Fork}))
		b.Fork = nil
		assert.Equal(t, false, equality.DeepEqual(a, b))
	})

	var fork1, fork2 *ethpb.Fork
	assert.Equal(t, true, equality.DeepEqual(fork1, fork2))

	fork1 = &ethpb.Fork{
		PreviousVersion: []byte{123},
		CurrentVersion:  []byte{124},
		Epoch:           1234567890,
	}
	fork2 = &ethpb.Fork{
		PreviousVersion: []byte{123},
		CurrentVersion:  []byte{125},
		Epoch:           1234567890,
	}
	assert.Equal(t, true, equality.DeepEqual(fork1, fork1))
	assert.Equal(t, false, equality.DeepEqual(fork1, fork2))

	checkpoint1 := &ethpb.Checkpoint{
		Epoch: 1234567890,
		Root:  []byte{},
	}
	checkpoint2 := &ethpb.Checkpoint{
		Epoch: 1234567890,
		Root:  nil,
	}
	assert.Equal(t, true, equality.DeepEqual(checkpoint1, checkpoint2))
}

// A value type that gets proto.Message through an embedded pointer (like blocks.ROBlob) must be
// compared on exported fields only, without panicking on its unexported ones.
type embedsProto struct {
	*ethpb.Fork
	root [32]byte
}

func TestDeepEqualProto_EmbeddedInValue(t *testing.T) {
	a := embedsProto{Fork: &ethpb.Fork{Epoch: 1}, root: [32]byte{1}}
	b := embedsProto{Fork: &ethpb.Fork{Epoch: 1}, root: [32]byte{2}}
	c := embedsProto{Fork: &ethpb.Fork{Epoch: 2}, root: [32]byte{1}}
	assert.Equal(t, true, equality.DeepEqual(a, b))
	assert.Equal(t, false, equality.DeepEqual(a, c))
	assert.Equal(t, true, equality.DeepEqual([]embedsProto{a}, []embedsProto{b}))
}
