package client

import (
	"context"
	"sync"
	"testing"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	validatormock "github.com/OffchainLabs/prysm/v7/testing/validator-mock"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/pkg/errors"
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

// newDistributedTestValidator builds a minimal validator wired to a mock client
// with genesisTime set so that time.Now() falls in the given epoch.
func newDistributedTestValidator(t *testing.T, epoch primitives.Epoch) (*validator, *validatormock.MockValidatorClient, keypair) {
	t.Helper()
	ctrl := gomock.NewController(t)
	client := validatormock.NewMockValidatorClient(ctrl)
	keys := randKeypair(t)

	secsPerSlot := params.BeaconConfig().SecondsPerSlot
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	slotInEpoch := primitives.Slot(epoch) * slotsPerEpoch
	genesis := time.Now().Add(-time.Duration(uint64(slotInEpoch)*secsPerSlot) * time.Second)

	v := &validator{
		km:              newMockKeymanager(t, keys),
		validatorClient: client,
		distributed:     true,
		duties:          &dutyStore{},
		genesisTime:     genesis,
		pubkeyToStatus: map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus{
			keys.pub: {publicKey: keys.pub[:], index: 200},
		},
	}
	v.aggSelector = newDistributedSelector(v)
	return v, client, keys
}

func TestDistributedSelector_EpochGuard(t *testing.T) {
	v, client, keys := newDistributedTestValidator(t, 2)
	ds := v.aggSelector.(*distributedSelector)

	slot := primitives.Slot(2) * params.BeaconConfig().SlotsPerEpoch
	ds.v.duties.SetFromCombinedDutiesResponse(&ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{{
			AttesterSlot:   slot,
			ValidatorIndex: 200,
			PublicKey:      keys.pub[:],
			Status:         ethpb.ValidatorStatus_ACTIVE,
		}},
	})

	sigDomain := make([]byte, 32)
	client.EXPECT().DomainData(gomock.Any(), gomock.Any()).
		Return(&ethpb.DomainResponse{SignatureDomain: sigDomain}, nil).Times(1)
	client.EXPECT().AggregatedSelections(gomock.Any(), gomock.Any()).
		Return([]iface.BeaconCommitteeSelection{{
			SelectionProof: make([]byte, 96),
			Slot:           slot,
			ValidatorIndex: 200,
		}}, nil).Times(1) // Only one RPC call despite two Refresh calls.

	require.NoError(t, ds.RefreshSelectionProofs(t.Context()))
	require.NoError(t, ds.RefreshSelectionProofs(t.Context()), "second call same epoch should be no-op")
	assert.Equal(t, 1, len(ds.attSelections))
}

func TestDistributedSelector_ReadyCh_BlocksUntilRefresh(t *testing.T) {
	v, client, keys := newDistributedTestValidator(t, 3)
	ds := v.aggSelector.(*distributedSelector)

	slot := primitives.Slot(3) * params.BeaconConfig().SlotsPerEpoch
	ds.v.duties.SetFromCombinedDutiesResponse(&ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{{
			AttesterSlot:   slot,
			ValidatorIndex: 200,
			PublicKey:      keys.pub[:],
			Status:         ethpb.ValidatorStatus_ACTIVE,
		}},
	})

	proof := make([]byte, 96)
	proof[0] = 0xAB

	sigDomain := make([]byte, 32)
	// Block during signing so the refresh is visibly in-flight.
	signingStarted := make(chan struct{})
	rpcDone := make(chan struct{})
	client.EXPECT().DomainData(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ *ethpb.DomainRequest) (*ethpb.DomainResponse, error) {
			close(signingStarted) // Signal that refresh is in-flight.
			<-rpcDone             // Block until test unblocks.
			return &ethpb.DomainResponse{SignatureDomain: sigDomain}, nil
		})
	client.EXPECT().AggregatedSelections(gomock.Any(), gomock.Any()).
		Return([]iface.BeaconCommitteeSelection{{
			SelectionProof: proof,
			Slot:           slot,
			ValidatorIndex: 200,
		}}, nil)

	// Start refresh in background.
	refreshErr := make(chan error, 1)
	go func() { refreshErr <- ds.RefreshSelectionProofs(t.Context()) }()

	// Wait until the refresh goroutine is past the epoch guard and in-flight.
	<-signingStarted

	// AttestationSelectionProof should block while refresh is in-flight.
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	_, err := ds.AttestationSelectionProof(ctx, slot, keys.pub)
	require.ErrorIs(t, err, context.DeadlineExceeded, "should block until refresh completes")

	// Unblock the refresh.
	close(rpcDone)
	require.NoError(t, <-refreshErr)

	// Now the channel is closed, proof should be available immediately.
	got, err := ds.AttestationSelectionProof(t.Context(), slot, keys.pub)
	require.NoError(t, err)
	assert.DeepEqual(t, proof, got)
}

func TestDistributedSelector_ErrorIsStickyWithinEpoch(t *testing.T) {
	v, client, keys := newDistributedTestValidator(t, 4)
	ds := v.aggSelector.(*distributedSelector)

	slot := primitives.Slot(4) * params.BeaconConfig().SlotsPerEpoch
	ds.v.duties.SetFromCombinedDutiesResponse(&ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{{
			AttesterSlot:   slot,
			ValidatorIndex: 200,
			PublicKey:      keys.pub[:],
			Status:         ethpb.ValidatorStatus_ACTIVE,
		}},
	})

	sigDomain := make([]byte, 32)
	client.EXPECT().DomainData(gomock.Any(), gomock.Any()).
		Return(&ethpb.DomainResponse{SignatureDomain: sigDomain}, nil).Times(1)

	refreshErr := errors.New("middleware down")
	client.EXPECT().AggregatedSelections(gomock.Any(), gomock.Any()).
		Return(nil, refreshErr)

	err := ds.RefreshSelectionProofs(t.Context())
	require.ErrorContains(t, "middleware down", err)
	assert.Equal(t, slots.ToEpoch(slot), ds.refreshedEpoch, "epoch guard should remain set for the failing epoch")

	// Same-epoch refreshes should not retry the middleware call.
	err = ds.RefreshSelectionProofs(t.Context())
	require.ErrorContains(t, "middleware down", err)

	_, err = ds.AttestationSelectionProof(t.Context(), slot, keys.pub)
	require.ErrorContains(t, "selection proofs unavailable", err)
	require.ErrorContains(t, "middleware down", err)
}

func TestDistributedSelector_SyncSubnetDedup(t *testing.T) {
	v, client, keys := newDistributedTestValidator(t, 1)
	ds := v.aggSelector.(*distributedSelector)

	sigDomain := make([]byte, 32)

	// SyncCommitteeSize=512, SyncCommitteeSubnetCount=4 → 128 per subnet.
	// Indices 0 and 127 both map to subnet 0, index 128 maps to subnet 1.
	indexRes := &ethpb.SyncSubcommitteeIndexResponse{
		Indices: []primitives.CommitteeIndex{0, 127, 128},
	}

	// DomainData called once per unique subnet (2 subnets).
	client.EXPECT().DomainData(gomock.Any(), gomock.Any()).
		Return(&ethpb.DomainResponse{SignatureDomain: sigDomain}, nil).Times(2)

	proofSubnet0 := []byte("aggregated-proof-subnet0")
	proofSubnet1 := []byte("aggregated-proof-subnet1")

	// AggregatedSyncSelections should receive exactly 2 selections (one per subnet).
	client.EXPECT().AggregatedSyncSelections(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, sels []iface.SyncCommitteeSelection) ([]iface.SyncCommitteeSelection, error) {
			require.Equal(t, 2, len(sels), "should deduplicate to 2 unique subnets")
			return []iface.SyncCommitteeSelection{
				{SelectionProof: proofSubnet0},
				{SelectionProof: proofSubnet1},
			}, nil
		})

	proofs, err := ds.SyncCommitteeSelectionProofs(t.Context(), 10, keys.pub, indexRes)
	require.NoError(t, err)
	require.Equal(t, 3, len(proofs), "should return a proof for each original index")
	// Indices 0 and 127 are both subnet 0 → same proof.
	assert.DeepEqual(t, proofSubnet0, proofs[0])
	assert.DeepEqual(t, proofSubnet0, proofs[1])
	// Index 128 is subnet 1 → different proof.
	assert.DeepEqual(t, proofSubnet1, proofs[2])
}
