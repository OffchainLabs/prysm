package cache

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestHighestExecutionPayloadBidCache_GetSetIfHigher(t *testing.T) {
	c := NewHighestExecutionPayloadBidCache()
	bid := testSignedExecutionPayloadBid(10, [32]byte{0x01}, [32]byte{0x02}, 100)

	inserted := c.SetIfHigher(bid)
	require.Equal(t, true, inserted)

	got, ok := c.Get(10, [32]byte{0x01}, [32]byte{0x02})
	require.Equal(t, true, ok)
	require.DeepEqual(t, bid, got)
}

func TestHighestExecutionPayloadBidCache_SetIfHigher_ReplacesOnlyOnHigherValue(t *testing.T) {
	c := NewHighestExecutionPayloadBidCache()
	low := testSignedExecutionPayloadBid(10, [32]byte{0x01}, [32]byte{0x02}, 100)
	same := testSignedExecutionPayloadBid(10, [32]byte{0x01}, [32]byte{0x02}, 100)
	high := testSignedExecutionPayloadBid(10, [32]byte{0x01}, [32]byte{0x02}, 101)

	require.Equal(t, true, c.SetIfHigher(low))
	require.Equal(t, false, c.SetIfHigher(same))

	got, ok := c.Get(10, [32]byte{0x01}, [32]byte{0x02})
	require.Equal(t, true, ok)
	require.DeepEqual(t, low, got)

	require.Equal(t, true, c.SetIfHigher(high))
	got, ok = c.Get(10, [32]byte{0x01}, [32]byte{0x02})
	require.Equal(t, true, ok)
	require.DeepEqual(t, high, got)
}

func TestHighestExecutionPayloadBidCache_SetIfHigher_KeepsDistinctTuples(t *testing.T) {
	c := NewHighestExecutionPayloadBidCache()
	first := testSignedExecutionPayloadBid(10, [32]byte{0x01}, [32]byte{0x02}, 100)
	second := testSignedExecutionPayloadBid(10, [32]byte{0x03}, [32]byte{0x02}, 50)
	third := testSignedExecutionPayloadBid(10, [32]byte{0x01}, [32]byte{0x04}, 75)

	require.Equal(t, true, c.SetIfHigher(first))
	require.Equal(t, true, c.SetIfHigher(second))
	require.Equal(t, true, c.SetIfHigher(third))

	got, ok := c.Get(10, [32]byte{0x01}, [32]byte{0x02})
	require.Equal(t, true, ok)
	require.DeepEqual(t, first, got)

	got, ok = c.Get(10, [32]byte{0x03}, [32]byte{0x02})
	require.Equal(t, true, ok)
	require.DeepEqual(t, second, got)

	got, ok = c.Get(10, [32]byte{0x01}, [32]byte{0x04})
	require.Equal(t, true, ok)
	require.DeepEqual(t, third, got)
}

func TestHighestExecutionPayloadBidCache_PruneBefore(t *testing.T) {
	c := NewHighestExecutionPayloadBidCache()
	oldBid := testSignedExecutionPayloadBid(9, [32]byte{0x01}, [32]byte{0x02}, 100)
	currentBid := testSignedExecutionPayloadBid(10, [32]byte{0x03}, [32]byte{0x04}, 101)

	require.Equal(t, true, c.SetIfHigher(oldBid))
	require.Equal(t, true, c.SetIfHigher(currentBid))

	c.PruneBefore(10)

	_, ok := c.Get(9, [32]byte{0x01}, [32]byte{0x02})
	require.Equal(t, false, ok)

	got, ok := c.Get(10, [32]byte{0x03}, [32]byte{0x04})
	require.Equal(t, true, ok)
	require.DeepEqual(t, currentBid, got)
}

func testSignedExecutionPayloadBid(
	slot primitives.Slot,
	parentHash [32]byte,
	parentRoot [32]byte,
	value uint64,
) *ethpb.SignedExecutionPayloadBid {
	return &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			Slot:             slot,
			ParentBlockHash:  bytes.Clone(parentHash[:]),
			ParentBlockRoot:  bytes.Clone(parentRoot[:]),
			BlockHash:        bytes.Repeat([]byte{0x03}, 32),
			PrevRandao:       bytes.Repeat([]byte{0x04}, 32),
			FeeRecipient:     bytes.Repeat([]byte{0x05}, 20),
			GasLimit:         30_000_000,
			BuilderIndex:     1,
			Value:            primitives.Gwei(value),
			ExecutionPayment: 10,
		},
		Signature: bytes.Repeat([]byte{0x06}, 96),
	}
}
