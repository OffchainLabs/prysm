package state_native_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func FuzzFuluStateHashTreeRoot(f *testing.F) {
	genesis, _ := util.DeterministicGenesisStateFulu(f, 64)
	// Up to slot 39, across one epoch boundary.
	fuzzStateHashTreeRoot(f, genesis, func() *ethpb.BeaconStateFulu { return &ethpb.BeaconStateFulu{} }, 40)
}

func FuzzGloasStateHashTreeRoot(f *testing.F) {
	genesis, _ := util.DeterministicGenesisStateGloas(f, 64)
	// Stay before slot 32: with the fuzz build tag the committee cache is off, and one Gloas epoch
	// transition takes about 18 s.
	fuzzStateHashTreeRoot(f, genesis, func() *ethpb.BeaconStateGloas { return &ethpb.BeaconStateGloas{} }, 32)
}

func fuzzStateHashTreeRoot[T native.Container](f *testing.F, genesis state.BeaconState, newContainer func() T, slotBound uint64) {
	genesisSSZ, err := genesis.MarshalSSZ()
	require.NoError(f, err)

	// No edits, up to the last slot the bound allows.
	f.Add([]byte{}, slotBound-1)
	// Flip a byte of the slot (offset 40), then advance.
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 40, 1}, uint64(5))

	f.Fuzz(func(t *testing.T, edits []byte, slots uint64) {
		ctx := t.Context()
		// Each 9-byte edit is a big-endian index into the state, modulo its length, and a XOR mask.
		stateSSZ := bytes.Clone(genesisSSZ)
		for i := 0; i+9 <= len(edits); i += 9 {
			idx := binary.BigEndian.Uint64(edits[i:i+8]) % uint64(len(stateSSZ))
			stateSSZ[idx] ^= edits[i+8]
		}
		container := newContainer()
		if err := any(container).(interface{ UnmarshalSSZ([]byte) error }).UnmarshalSSZ(stateSSZ); err != nil {
			return
		}
		copied, err := native.New(container)
		if err != nil {
			return
		}
		owned, err := native.NewUnsafe(container)
		require.NoError(t, err)
		ownedRoot, ownedErr := owned.HashTreeRoot(ctx)
		copiedRoot, copiedErr := copied.HashTreeRoot(ctx)
		require.Equal(t, ownedErr == nil, copiedErr == nil)
		if ownedErr == nil {
			require.Equal(t, copiedRoot, ownedRoot)
		}

		target := primitives.Slot(slots % slotBound)
		for owned.Slot() < target {
			owned, err = transition.ProcessSlots(ctx, owned, owned.Slot()+1)
			if err != nil {
				return
			}
			// A copy shares fields with the original and must not change its root.
			_ = owned.Copy()
		}
		cold, err := native.NewUnsafe(owned.ToContainerUnsafe().(T))
		require.NoError(t, err)

		ownedRoot, ownedErr = owned.HashTreeRoot(ctx)
		coldRoot, coldErr := cold.HashTreeRoot(ctx)
		require.Equal(t, ownedErr == nil, coldErr == nil)
		if ownedErr == nil {
			require.Equal(t, coldRoot, ownedRoot)
		}

		ownedSSZ, ownedErr := owned.MarshalSSZ()
		coldSSZ, coldErr := cold.MarshalSSZ()
		require.Equal(t, ownedErr == nil, coldErr == nil)
		if ownedErr == nil {
			require.Equal(t, true, bytes.Equal(coldSSZ, ownedSSZ), "cold and warm SSZ differ")
		}
	})
}
