package client

import (
	"sync"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"go.uber.org/mock/gomock"
)

func testLocalSelector(t *testing.T, v *validator) *localSelector {
	t.Helper()
	s, err := newLocalSelector(v)
	require.NoError(t, err)
	return s
}

func TestLocalSelector_ClaimAggregateSlot(t *testing.T) {
	s, err := newLocalSelector(&validator{})
	require.NoError(t, err)

	slot := primitives.Slot(5)
	committee := primitives.CommitteeIndex(2)

	assert.Equal(t, true, s.ClaimAggregateSlot(slot, committee), "first claim should succeed")
	assert.Equal(t, false, s.ClaimAggregateSlot(slot, committee), "duplicate claim should fail")
	assert.Equal(t, true, s.ClaimAggregateSlot(slot, primitives.CommitteeIndex(3)), "different committee should succeed")
	assert.Equal(t, true, s.ClaimAggregateSlot(slot+1, committee), "different slot should succeed")
}

func TestLocalSelector_AttestationSelectionProof_Memoized(t *testing.T) {
	v, m, validatorKey, finish := setup(t, false)
	defer finish()

	s, err := newLocalSelector(v)
	require.NoError(t, err)

	var pubKey [fieldparams.BLSPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	v.pubkeyToStatus = map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus{
		pubKey: {index: 0},
	}

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(),
		gomock.Any(),
	).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

	slot := primitives.Slot(1)

	proof1, err := s.AttestationSelectionProof(t.Context(), slot, pubKey)
	require.NoError(t, err)
	require.NotNil(t, proof1)

	// Second call should return cached proof without additional signing.
	proof2, err := s.AttestationSelectionProof(t.Context(), slot, pubKey)
	require.NoError(t, err)
	assert.DeepEqual(t, proof1, proof2)
}

func TestLocalSelector_AttestationSelectionProof_ConcurrentDedup(t *testing.T) {
	v, m, validatorKey, finish := setup(t, false)
	defer finish()

	s, err := newLocalSelector(v)
	require.NoError(t, err)

	var pubKey [fieldparams.BLSPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	v.pubkeyToStatus = map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus{
		pubKey: {index: 0},
	}

	// DomainData should only be called once despite concurrent callers.
	m.validatorClient.EXPECT().DomainData(
		gomock.Any(),
		gomock.Any(),
	).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil).Times(1)

	slot := primitives.Slot(1)
	const goroutines = 5

	var wg sync.WaitGroup
	results := make([][]byte, goroutines)
	errs := make([]error, goroutines)

	wg.Add(goroutines)
	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = s.AttestationSelectionProof(t.Context(), slot, pubKey)
		}(i)
	}
	wg.Wait()

	for i := range goroutines {
		require.NoError(t, errs[i], "goroutine %d failed", i)
		assert.DeepEqual(t, results[0], results[i], "all goroutines should get the same proof")
	}
}

func TestLocalSelector_RefreshSelectionProofs_ClearsCache(t *testing.T) {
	s, err := newLocalSelector(&validator{})
	require.NoError(t, err)

	key := attSelectionKey{slot: 1, index: 0}
	s.proofCache[key] = []byte("cached")

	require.NoError(t, s.RefreshSelectionProofs(t.Context()))
	assert.Equal(t, 0, len(s.proofCache), "proof cache should be cleared")
}

func TestDistributedSelector_ClaimAggregateSlot_AlwaysTrue(t *testing.T) {
	s := newDistributedSelector(&validator{})

	assert.Equal(t, true, s.ClaimAggregateSlot(0, 0))
	assert.Equal(t, true, s.ClaimAggregateSlot(0, 0))
	assert.Equal(t, true, s.ClaimAggregateSlot(99, 99))
}

func TestDistributedSelector_SyncCommitteeAggregators_ReturnsAll(t *testing.T) {
	s := newDistributedSelector(&validator{})
	pubkeys := [][fieldparams.BLSPubkeyLength]byte{{1}, {2}, {3}}

	result, err := s.SyncCommitteeAggregators(t.Context(), 0, pubkeys)
	require.NoError(t, err)
	assert.DeepEqual(t, pubkeys, result)
}
