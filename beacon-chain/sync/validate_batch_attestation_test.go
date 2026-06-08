package sync

import (
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestBatchDedup_SingleAttestersCoverBatchBits(t *testing.T) {
	s := &Service{
		seenBatchAttestationCache: lruwrpr.New(10),
	}
	key := batchDedupKey{slot: 1, committeeIndex: 2}
	aggregationBits := bitfield.NewBitlist(4)
	aggregationBits.SetBitAt(1, true)
	aggregationBits.SetBitAt(2, true)

	require.Equal(t, true, s.setSeenBatchAttester(key, 4, 1))

	entry := s.getOrCreateBatchDedupEntry(key, 4)
	entry.mu.Lock()
	assert.Equal(t, false, seenAttestersCoverAggregationBits(entry.seenAttesters, aggregationBits))
	entry.mu.Unlock()

	require.Equal(t, true, s.setSeenBatchAttester(key, 4, 2))
	entry.mu.Lock()
	assert.Equal(t, true, seenAttestersCoverAggregationBits(entry.seenAttesters, aggregationBits))
	entry.mu.Unlock()
}

func TestBatchDedup_BatchBitsCoverSingleAttesters(t *testing.T) {
	s := &Service{
		seenBatchAttestationCache: lruwrpr.New(10),
	}
	key := batchDedupKey{slot: 1, committeeIndex: 2}
	aggregationBits := bitfield.NewBitlist(4)
	aggregationBits.SetBitAt(0, true)
	aggregationBits.SetBitAt(3, true)

	entry := s.getOrCreateBatchDedupEntry(key, 4)
	entry.mu.Lock()
	entry.seenAttesters = orBitlists(entry.seenAttesters, aggregationBits, 4)
	entry.mu.Unlock()

	assert.Equal(t, true, s.hasSeenBatchAttester(key, 4, 0))
	assert.Equal(t, true, s.hasSeenBatchAttester(key, 4, 3))
	assert.Equal(t, false, s.setSeenBatchAttester(key, 4, 0))
	assert.Equal(t, true, s.setSeenBatchAttester(key, 4, 1))
}

func TestBatchDedup_BatchMarksSingleCache(t *testing.T) {
	s := &Service{
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
	}
	slot := primitives.Slot(7)
	committeeIndex := primitives.CommitteeIndex(3)
	attesters := []primitives.ValidatorIndex{11, 13}

	s.setSeenUnaggregatedAttesters(slot, committeeIndex, attesters)

	for _, attester := range attesters {
		key := generateUnaggregatedAttCacheKeyForAttester(slot, committeeIndex, uint64(attester))
		assert.Equal(t, true, s.hasSeenUnaggregatedAtt(key))
	}
}

func TestSingleAttesterCommitteePosition(t *testing.T) {
	committee := []primitives.ValidatorIndex{10, 11, 12}
	single := &ethpb.SingleAttestation{
		AttesterIndex: 11,
		Data:          &ethpb.AttestationData{Slot: 1},
	}
	pos, err := singleAttesterCommitteePosition(single, committee)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), pos)

	phase0 := &ethpb.Attestation{
		Data:            &ethpb.AttestationData{Slot: 1},
		AggregationBits: bitfield.Bitlist{0x05},
	}
	pos, err = singleAttesterCommitteePosition(phase0, committee)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), pos)
}
