package cache

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

type testBidFunc func(t *testing.T, slot primitives.Slot, parentHash, parentRoot [32]byte, value uint64) interfaces.ROSignedExecutionPayloadBid

func TestHighestExecutionPayloadBidCache(t *testing.T) {
	hash, root := [32]byte{0x01}, [32]byte{0x02}
	forks := []struct {
		name   string
		newBid testBidFunc
	}{
		{name: "gloas", newBid: testWrappedBid},
		{name: "heze", newBid: testWrappedBidHeze},
	}
	scenarios := []struct {
		name string
		run  func(t *testing.T, c *HighestExecutionPayloadBidCache, newBid testBidFunc)
	}{
		{
			name: "insert when absent",
			run: func(t *testing.T, c *HighestExecutionPayloadBidCache, newBid testBidFunc) {
				bid := newBid(t, 10, hash, root, 100)
				require.Equal(t, true, c.SetIfHigher(bid))
				got, ok := c.Get(10, hash, root)
				require.Equal(t, true, ok)
				require.DeepEqual(t, bid, got)
			},
		},
		{
			name: "nil bid is rejected",
			run: func(t *testing.T, c *HighestExecutionPayloadBidCache, _ testBidFunc) {
				require.Equal(t, false, c.SetIfHigher(nil))
				_, ok := c.Get(10, hash, root)
				require.Equal(t, false, ok)
			},
		},
		{
			name: "equal value does not replace",
			run: func(t *testing.T, c *HighestExecutionPayloadBidCache, newBid testBidFunc) {
				first := newBid(t, 10, hash, root, 100)
				require.Equal(t, true, c.SetIfHigher(first))
				require.Equal(t, false, c.SetIfHigher(newBid(t, 10, hash, root, 100)))
				got, ok := c.Get(10, hash, root)
				require.Equal(t, true, ok)
				require.DeepEqual(t, first, got)
			},
		},
		{
			name: "lower value does not replace",
			run: func(t *testing.T, c *HighestExecutionPayloadBidCache, newBid testBidFunc) {
				first := newBid(t, 10, hash, root, 100)
				require.Equal(t, true, c.SetIfHigher(first))
				require.Equal(t, false, c.SetIfHigher(newBid(t, 10, hash, root, 99)))
				got, ok := c.Get(10, hash, root)
				require.Equal(t, true, ok)
				require.DeepEqual(t, first, got)
			},
		},
		{
			name: "higher value replaces",
			run: func(t *testing.T, c *HighestExecutionPayloadBidCache, newBid testBidFunc) {
				require.Equal(t, true, c.SetIfHigher(newBid(t, 10, hash, root, 100)))
				high := newBid(t, 10, hash, root, 101)
				require.Equal(t, true, c.SetIfHigher(high))
				got, ok := c.Get(10, hash, root)
				require.Equal(t, true, ok)
				require.DeepEqual(t, high, got)
			},
		},
		{
			name: "distinct tuples kept apart",
			run: func(t *testing.T, c *HighestExecutionPayloadBidCache, newBid testBidFunc) {
				bids := []struct {
					slot       primitives.Slot
					hash, root [32]byte
					bid        interfaces.ROSignedExecutionPayloadBid
				}{
					{slot: 10, hash: hash, root: root},
					{slot: 10, hash: [32]byte{0x03}, root: root},
					{slot: 10, hash: hash, root: [32]byte{0x04}},
					{slot: 11, hash: hash, root: root},
				}
				for i := range bids {
					// Lower values prove that no tuple replaces another.
					bids[i].bid = newBid(t, bids[i].slot, bids[i].hash, bids[i].root, uint64(100-i))
					require.Equal(t, true, c.SetIfHigher(bids[i].bid))
				}
				for _, b := range bids {
					got, ok := c.Get(b.slot, b.hash, b.root)
					require.Equal(t, true, ok)
					require.DeepEqual(t, b.bid, got)
				}
			},
		},
		{
			name: "highest value",
			run: func(t *testing.T, c *HighestExecutionPayloadBidCache, newBid testBidFunc) {
				_, ok := c.HighestValue(10, hash, root)
				require.Equal(t, false, ok)
				require.Equal(t, true, c.SetIfHigher(newBid(t, 10, hash, root, 100)))
				require.Equal(t, true, c.SetIfHigher(newBid(t, 10, hash, root, 200)))
				v, ok := c.HighestValue(10, hash, root)
				require.Equal(t, true, ok)
				require.Equal(t, primitives.Gwei(200), v)
			},
		},
		{
			name: "prune before",
			run: func(t *testing.T, c *HighestExecutionPayloadBidCache, newBid testBidFunc) {
				current := newBid(t, 10, [32]byte{0x03}, [32]byte{0x04}, 101)
				require.Equal(t, true, c.SetIfHigher(newBid(t, 9, hash, root, 100)))
				require.Equal(t, true, c.SetIfHigher(current))

				c.PruneBefore(10)

				_, ok := c.Get(9, hash, root)
				require.Equal(t, false, ok)
				got, ok := c.Get(10, [32]byte{0x03}, [32]byte{0x04})
				require.Equal(t, true, ok)
				require.DeepEqual(t, current, got)
			},
		},
	}
	for _, fork := range forks {
		for _, sc := range scenarios {
			t.Run(fork.name+"/"+sc.name, func(t *testing.T) {
				sc.run(t, NewHighestExecutionPayloadBidCache(), fork.newBid)
			})
		}
	}
}

func testWrappedBid(t *testing.T, slot primitives.Slot, parentHash, parentRoot [32]byte, value uint64) interfaces.ROSignedExecutionPayloadBid {
	bid, err := blocks.WrappedROSignedExecutionPayloadBid(testSignedExecutionPayloadBid(slot, parentHash, parentRoot, value))
	require.NoError(t, err)
	return bid
}

func testWrappedBidHeze(t *testing.T, slot primitives.Slot, parentHash, parentRoot [32]byte, value uint64) interfaces.ROSignedExecutionPayloadBid {
	bid, err := blocks.WrappedROSignedExecutionPayloadBidHeze(testSignedExecutionPayloadBidHeze(slot, parentHash, parentRoot, value))
	require.NoError(t, err)
	return bid
}

func testSignedExecutionPayloadBid(
	slot primitives.Slot,
	parentHash [32]byte,
	parentRoot [32]byte,
	value uint64,
) *ethpb.SignedExecutionPayloadBid {
	return &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			Slot:                  slot,
			ParentBlockHash:       bytes.Clone(parentHash[:]),
			ParentBlockRoot:       bytes.Clone(parentRoot[:]),
			BlockHash:             bytes.Repeat([]byte{0x03}, 32),
			PrevRandao:            bytes.Repeat([]byte{0x04}, 32),
			FeeRecipient:          bytes.Repeat([]byte{0x05}, 20),
			GasLimit:              30_000_000,
			BuilderIndex:          1,
			Value:                 primitives.Gwei(value),
			ExecutionPayment:      10,
			ExecutionRequestsRoot: make([]byte, 32),
		},
		Signature: bytes.Repeat([]byte{0x06}, 96),
	}
}

func testSignedExecutionPayloadBidHeze(
	slot primitives.Slot,
	parentHash [32]byte,
	parentRoot [32]byte,
	value uint64,
) *ethpb.SignedExecutionPayloadBidHeze {
	gloas := testSignedExecutionPayloadBid(slot, parentHash, parentRoot, value).Message
	return &ethpb.SignedExecutionPayloadBidHeze{
		Message: &ethpb.ExecutionPayloadBidHeze{
			Slot:                  gloas.Slot,
			ParentBlockHash:       gloas.ParentBlockHash,
			ParentBlockRoot:       gloas.ParentBlockRoot,
			BlockHash:             gloas.BlockHash,
			PrevRandao:            gloas.PrevRandao,
			FeeRecipient:          gloas.FeeRecipient,
			GasLimit:              gloas.GasLimit,
			BuilderIndex:          gloas.BuilderIndex,
			Value:                 gloas.Value,
			ExecutionPayment:      gloas.ExecutionPayment,
			ExecutionRequestsRoot: gloas.ExecutionRequestsRoot,
			InclusionListBits:     bitfield.NewBitvector16(),
		},
		Signature: bytes.Repeat([]byte{0x06}, 96),
	}
}
