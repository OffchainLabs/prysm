package sweepthreshold

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/pkg/errors"
)

func request(threshold uint64) *enginev1.SetSweepThresholdRequest {
	return &enginev1.SetSweepThresholdRequest{
		SourceAddress:   bytes.Repeat([]byte{0x11}, 20),
		ValidatorPubkey: bytes.Repeat([]byte{0x22}, 48),
		Threshold:       threshold,
	}
}

func TestMockPool_InsertAndDrain(t *testing.T) {
	pool := NewMockPool()
	require.NoError(t, pool.Insert(request(64_000_000_000), request(128_000_000_000)))
	require.Equal(t, 2, pool.Pending())

	drained := pool.Drain()
	require.Equal(t, 2, len(drained))
	require.Equal(t, uint64(64_000_000_000), drained[0].Threshold)
	require.Equal(t, uint64(128_000_000_000), drained[1].Threshold)
}

// TestMockPool_DrainIsOneShot checks that a re-proposal does not duplicate the injection.
func TestMockPool_DrainIsOneShot(t *testing.T) {
	pool := NewMockPool()
	require.NoError(t, pool.Insert(request(64_000_000_000)))

	require.Equal(t, 1, len(pool.Drain()))
	require.Equal(t, 0, len(pool.Drain()))
	require.Equal(t, 0, pool.Pending())
}

func TestMockPool_InsertRejectsOverLimit(t *testing.T) {
	limit := params.BeaconConfig().MaxSetSweepThresholdRequestsPerPayload
	pool := NewMockPool()

	requests := make([]*enginev1.SetSweepThresholdRequest, 0, limit)
	for range limit {
		requests = append(requests, request(64_000_000_000))
	}
	require.NoError(t, pool.Insert(requests...))

	err := pool.Insert(request(64_000_000_000))
	require.Equal(t, true, errors.Is(err, ErrPoolFull))
	// The rejected batch does not partially land.
	require.Equal(t, int(limit), pool.Pending())
}

// TestMockPool_CopyDoesNotDrainOrAlias checks that the read path leaves the queue intact and
// hands out copies rather than the queued pointers.
func TestMockPool_CopyDoesNotDrainOrAlias(t *testing.T) {
	pool := NewMockPool()
	require.NoError(t, pool.Insert(request(64_000_000_000)))

	got := pool.Copy()
	require.Equal(t, 1, len(got))
	require.Equal(t, 1, pool.Pending())

	got[0].Threshold = 1
	require.Equal(t, uint64(64_000_000_000), pool.Copy()[0].Threshold)
}

func TestMockPool_NilAndEmptyAreNoOps(t *testing.T) {
	var nilPool *MockPool
	require.Equal(t, 0, nilPool.Pending())
	require.Equal(t, 0, len(nilPool.Drain()))
	require.Equal(t, 0, len(nilPool.Copy()))
	require.NotNil(t, nilPool.Insert(request(1)))

	pool := NewMockPool()
	require.Equal(t, 0, pool.Pending())
	require.Equal(t, 0, len(pool.Drain()))
	// Inserting nothing is not an error.
	require.NoError(t, pool.Insert())
}
