package client

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	lru "github.com/hashicorp/golang-lru"
	"go.uber.org/mock/gomock"
)

func testLocalSelector(t *testing.T, v *validator) *localSelector {
	t.Helper()
	cache, err := lru.New(10)
	require.NoError(t, err)
	return newLocalSelector(v, cache)
}

func TestLocalSelector_ClaimAggregateSlot(t *testing.T) {
	cache, err := lru.New(10)
	require.NoError(t, err)
	s := newLocalSelector(&validator{}, cache)

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

	cache, err := lru.New(10)
	require.NoError(t, err)
	s := newLocalSelector(v, cache)

	var pubKey [fieldparams.BLSPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(),
		gomock.Any(),
	).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

	slot := primitives.Slot(1)
	idx := primitives.ValidatorIndex(0)

	proof1, err := s.AttestationSelectionProof(t.Context(), slot, pubKey, idx)
	require.NoError(t, err)
	require.NotNil(t, proof1)

	// Second call should return cached proof without additional signing.
	proof2, err := s.AttestationSelectionProof(t.Context(), slot, pubKey, idx)
	require.NoError(t, err)
	assert.DeepEqual(t, proof1, proof2)
}

func TestLocalSelector_RefreshSelectionProofs_ClearsCache(t *testing.T) {
	cache, err := lru.New(10)
	require.NoError(t, err)
	s := newLocalSelector(&validator{}, cache)

	key := attSelectionKey{slot: 1, index: 0}
	s.proofCache[key] = []byte("cached")

	require.NoError(t, s.RefreshSelectionProofs(t.Context(), nil))
	assert.Equal(t, 0, len(s.proofCache), "proof cache should be cleared")
}

func TestDistributedSelector_ClaimAggregateSlot_AlwaysTrue(t *testing.T) {
	s := &distributedSelector{}

	assert.Equal(t, true, s.ClaimAggregateSlot(0, 0))
	assert.Equal(t, true, s.ClaimAggregateSlot(0, 0))
	assert.Equal(t, true, s.ClaimAggregateSlot(99, 99))
}

func TestDistributedSelector_SyncCommitteeAggregators_ReturnsAll(t *testing.T) {
	s := &distributedSelector{}
	pubkeys := [][fieldparams.BLSPubkeyLength]byte{{1}, {2}, {3}}

	result, err := s.SyncCommitteeAggregators(t.Context(), 0, pubkeys)
	require.NoError(t, err)
	assert.DeepEqual(t, pubkeys, result)
}
