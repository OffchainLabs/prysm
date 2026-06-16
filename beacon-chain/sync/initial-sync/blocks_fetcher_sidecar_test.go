package initialsync

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// makeEnvelopeForRoot creates an envelope whose BeaconBlockRoot is set to the given block root,
// i.e. the payload revealed for that block.
func makeEnvelopeForRoot(t *testing.T, slot primitives.Slot, beaconBlockRoot [32]byte) interfaces.ROSignedExecutionPayloadEnvelope {
	env := &ethpb.SignedExecutionPayloadEnvelope{
		Signature: make([]byte, fieldparams.BLSSignatureLength),
		Message: &ethpb.ExecutionPayloadEnvelope{
			BeaconBlockRoot:       beaconBlockRoot[:],
			ParentBeaconBlockRoot: make([]byte, fieldparams.RootLength),
			ExecutionRequests:     &enginev1.ExecutionRequests{},
			Payload: &enginev1.ExecutionPayloadGloas{
				ParentHash:    make([]byte, fieldparams.RootLength),
				FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:     make([]byte, fieldparams.RootLength),
				ReceiptsRoot:  make([]byte, fieldparams.RootLength),
				LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:    make([]byte, fieldparams.RootLength),
				BaseFeePerGas: make([]byte, fieldparams.RootLength),
				BlockHash:     make([]byte, fieldparams.RootLength),
				SlotNumber:    slot,
			},
		},
	}
	wrapped, err := blocks.WrappedROSignedExecutionPayloadEnvelope(env)
	require.NoError(t, err)
	return wrapped
}

func rootSet(bs []blocks.ROBlock) map[[32]byte]bool {
	m := make(map[[32]byte]bool, len(bs))
	for _, b := range bs {
		m[b.Root()] = true
	}
	return m
}

func noResolveBlock([32]byte) (blocks.ROBlock, bool) { return blocks.ROBlock{}, false }

func TestColumnFetchBlocks(t *testing.T) {
	currentEpoch := primitives.Epoch(0)

	// A linear chain of Gloas blocks (slots within the DA period).
	b0 := makeGloasBlock(t, 1, [32]byte{}, [32]byte{0x10})
	b1 := makeGloasBlock(t, 2, b0.Root(), [32]byte{0x11})
	b2 := makeGloasBlock(t, 3, b1.Root(), [32]byte{0x12})

	t.Run("gloas: only blocks with a revealed payload are selected", func(t *testing.T) {
		// Envelopes exist for b0 and b2 only; b1's payload is absent.
		envs := []interfaces.ROSignedExecutionPayloadEnvelope{
			makeEnvelopeForRoot(t, 1, b0.Root()),
			makeEnvelopeForRoot(t, 3, b2.Root()),
		}
		bwb := []blocks.BlockWithROSidecars{{Block: b0}, {Block: b1}, {Block: b2}}
		got, err := columnFetchBlocks(bwb, envs, currentEpoch, noResolveBlock)
		require.NoError(t, err)
		roots := rootSet(got)
		require.Equal(t, 2, len(got))
		require.Equal(t, true, roots[b0.Root()])
		require.Equal(t, false, roots[b1.Root()]) // payload-absent slot must NOT be requested
		require.Equal(t, true, roots[b2.Root()])
	})

	t.Run("gloas: no envelopes means no columns requested", func(t *testing.T) {
		bwb := []blocks.BlockWithROSidecars{{Block: b0}, {Block: b1}}
		got, err := columnFetchBlocks(bwb, nil, currentEpoch, noResolveBlock)
		require.NoError(t, err)
		require.Equal(t, 0, len(got))
	})

	t.Run("gloas: out-of-batch parent payload resolved via resolveBlock", func(t *testing.T) {
		parent := makeGloasBlock(t, 0, [32]byte{}, [32]byte{0x09})
		// Envelope is for the parent (the payload the first block builds on), which is not in bwb.
		envs := []interfaces.ROSignedExecutionPayloadEnvelope{
			makeEnvelopeForRoot(t, 0, parent.Root()),
		}
		bwb := []blocks.BlockWithROSidecars{{Block: b0}, {Block: b1}}
		resolve := func(root [32]byte) (blocks.ROBlock, bool) {
			if root == parent.Root() {
				return parent, true
			}
			return blocks.ROBlock{}, false
		}
		got, err := columnFetchBlocks(bwb, envs, currentEpoch, resolve)
		require.NoError(t, err)
		roots := rootSet(got)
		require.Equal(t, 1, len(got))
		require.Equal(t, true, roots[parent.Root()])
	})

	t.Run("gloas: unresolvable out-of-batch root is skipped without error", func(t *testing.T) {
		envs := []interfaces.ROSignedExecutionPayloadEnvelope{
			makeEnvelopeForRoot(t, 0, [32]byte{0xde, 0xad}),
		}
		bwb := []blocks.BlockWithROSidecars{{Block: b0}}
		got, err := columnFetchBlocks(bwb, envs, currentEpoch, noResolveBlock)
		require.NoError(t, err)
		require.Equal(t, 0, len(got))
	})

	t.Run("pre-gloas Fulu blocks are always selected", func(t *testing.T) {
		fulu := util.NewBeaconBlockFulu()
		fulu.Block.Slot = 1
		signed, err := blocks.NewSignedBeaconBlock(fulu)
		require.NoError(t, err)
		roFulu, err := blocks.NewROBlock(signed)
		require.NoError(t, err)
		bwb := []blocks.BlockWithROSidecars{{Block: roFulu}}
		// No envelopes: a pre-Gloas block is still considered full and requested by root.
		got, err := columnFetchBlocks(bwb, nil, currentEpoch, noResolveBlock)
		require.NoError(t, err)
		require.Equal(t, 1, len(got))
		require.Equal(t, true, rootSet(got)[roFulu.Root()])
	})
}
