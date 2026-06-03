package client

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestBatchAttestationCoordinatorContributeSubmitsOnce(t *testing.T) {
	c := newBatchAttestationCoordinator()
	key := batchAttestationKey{slot: 64, committeeIndex: 3, dataRoot: [32]byte{1}}
	data := &ethpb.AttestationData{Slot: key.slot}
	resp := &ethpb.AttestResponse{AttestationDataRoot: []byte{1, 2, 3}}
	var submitCount atomic.Int32
	submit := func(_ context.Context, snapshot batchAttestationSnapshot) (*ethpb.AttestResponse, error) {
		submitCount.Add(1)
		require.Equal(t, 2, len(snapshot.contributions))
		assert.Equal(t, primitives.ValidatorIndex(10), snapshot.batcher)
		assert.Equal(t, 0, snapshot.contributions[0].AttesterCommitteePos)
		assert.Equal(t, 1, snapshot.contributions[1].AttesterCommitteePos)
		return resp, nil
	}

	first := batchAttestationRequest{
		key:             key,
		expected:        2,
		committeeLength: 8,
		batcher:         10,
		data:            data,
		contribution: BatchContribution{
			AttesterIndex:        11,
			AttesterCommitteePos: 1,
			AttestationSignature: []byte{1},
			Seal:                 []byte{2},
		},
	}
	second := first
	second.contribution = BatchContribution{
		AttesterIndex:        10,
		AttesterCommitteePos: 0,
		AttestationSignature: []byte{3},
		Seal:                 []byte{4},
	}

	type result struct {
		resp    *ethpb.AttestResponse
		handled bool
		err     error
	}
	results := make(chan result, 1)
	go func() {
		got, handled, err := c.contribute(t.Context(), first, submit)
		results <- result{resp: got, handled: handled, err: err}
	}()

	require.Eventually(t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return len(c.entries) == 1
	}, time.Second, time.Millisecond)

	got, handled, err := c.contribute(t.Context(), second, submit)
	require.NoError(t, err)
	assert.Equal(t, true, handled)
	assert.DeepEqual(t, resp, got)

	firstResult := <-results
	require.NoError(t, firstResult.err)
	assert.Equal(t, true, firstResult.handled)
	assert.DeepEqual(t, resp, firstResult.resp)
	assert.Equal(t, int32(1), submitCount.Load())
}

func TestBatchAttestationCoordinatorContributeTimesOut(t *testing.T) {
	c := newBatchAttestationCoordinator()
	var submitCount atomic.Int32
	resp, handled, err := c.contribute(t.Context(), batchAttestationRequest{
		key:             batchAttestationKey{slot: 64, committeeIndex: 3, dataRoot: [32]byte{1}},
		expected:        2,
		committeeLength: 8,
		data:            &ethpb.AttestationData{Slot: 64},
		contribution: BatchContribution{
			AttesterIndex:        10,
			AttesterCommitteePos: 0,
			AttestationSignature: []byte{1},
			Seal:                 []byte{2},
		},
	}, func(context.Context, batchAttestationSnapshot) (*ethpb.AttestResponse, error) {
		submitCount.Add(1)
		return &ethpb.AttestResponse{}, nil
	})
	require.NoError(t, err)
	assert.Equal(t, false, handled)
	assert.Equal(t, (*ethpb.AttestResponse)(nil), resp)
	assert.Equal(t, int32(0), submitCount.Load())
}

func TestLocalBatchAttesterDuties(t *testing.T) {
	pk1 := bytesutil.ToBytes48([]byte{1})
	pk2 := bytesutil.ToBytes48([]byte{2})
	pk3 := bytesutil.ToBytes48([]byte{3})
	v := &validator{
		duties: testDutyStore(
			&ethpb.ValidatorDuty{
				PublicKey:               pk1[:],
				AttesterSlot:            10,
				CommitteeIndex:          5,
				CommitteeLength:         16,
				ValidatorIndex:          20,
				ValidatorCommitteeIndex: 3,
			},
			&ethpb.ValidatorDuty{
				PublicKey:               pk2[:],
				AttesterSlot:            10,
				CommitteeIndex:          5,
				CommitteeLength:         16,
				ValidatorIndex:          12,
				ValidatorCommitteeIndex: 1,
			},
			&ethpb.ValidatorDuty{
				PublicKey:       pk3[:],
				AttesterSlot:    11,
				CommitteeIndex:  5,
				CommitteeLength: 16,
				ValidatorIndex:  3,
			},
		),
	}

	duties := v.localBatchAttesterDuties(10, 5)
	require.Equal(t, 2, len(duties))
	assert.Equal(t, pk2, duties[0].pubKey)
	assert.Equal(t, primitives.ValidatorIndex(12), duties[0].validatorIndex)
	assert.Equal(t, uint64(1), duties[0].validatorCommitteeIndex)
	assert.Equal(t, pk1, duties[1].pubKey)
	assert.Equal(t, primitives.ValidatorIndex(20), duties[1].validatorIndex)
	assert.Equal(t, uint64(3), duties[1].validatorCommitteeIndex)
}
