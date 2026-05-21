package validator

import (
	"context"
	"sync"
	"testing"
	"time"

	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	payloadattestation "github.com/OffchainLabs/prysm/v7/beacon-chain/operations/payloadattestation"
	p2pmock "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"google.golang.org/protobuf/types/known/emptypb"
)

type payloadAttestationBlockReceiver struct {
	*chainMock.ChainService
	received bool
}

func (r *payloadAttestationBlockReceiver) ReceivePayloadAttestationMessage(_ context.Context, _ *ethpb.PayloadAttestationMessage) error {
	r.received = true
	return nil
}

func TestPayloadAttestationData_OK(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	slot := primitives.Slot(7)
	root := bytesutil.PadTo([]byte{0xAA}, 32)
	chain := &chainMock.ChainService{
		Slot: &slot,
		Root: root,
		MockCanonicalRoots: map[primitives.Slot][32]byte{
			slot: bytesutil.ToBytes32(root),
		},
		MockCanonicalFull: map[primitives.Slot]bool{
			slot: false,
		},
	}
	vs := &Server{
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
		TimeFetcher:       chain,
		HeadFetcher:       chain,
		ForkchoiceFetcher: chain,
		CoreService:       &core.Service{GenesisTimeFetcher: chain, ForkchoiceFetcher: chain},
	}

	resp, err := vs.PayloadAttestationData(t.Context(), &ethpb.PayloadAttestationDataRequest{Slot: slot})
	require.NoError(t, err)
	require.DeepEqual(t, root, resp.BeaconBlockRoot)
	assert.Equal(t, slot, resp.Slot)
	assert.Equal(t, false, resp.PayloadPresent)
	assert.Equal(t, false, resp.BlobDataAvailable)
}

func TestPayloadAttestationData_CachedPerSlot(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	slot := primitives.Slot(7)
	root := bytesutil.PadTo([]byte{0xAA}, 32)
	chain := &chainMock.ChainService{
		Slot: &slot,
		Root: root,
		MockCanonicalRoots: map[primitives.Slot][32]byte{
			slot: bytesutil.ToBytes32(root),
		},
		MockCanonicalFull: map[primitives.Slot]bool{slot: false},
	}
	vs := &Server{
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
		TimeFetcher:       chain,
		HeadFetcher:       chain,
		ForkchoiceFetcher: chain,
		CoreService:       &core.Service{GenesisTimeFetcher: chain, ForkchoiceFetcher: chain},
	}

	first, err := vs.PayloadAttestationData(t.Context(), &ethpb.PayloadAttestationDataRequest{Slot: slot})
	require.NoError(t, err)
	require.DeepEqual(t, root, first.BeaconBlockRoot)

	// Mutate the underlying mock; a second call at the same slot must hit the cache
	// and return the original response, ignoring the mutation.
	newRoot := bytesutil.PadTo([]byte{0xBB}, 32)
	chain.Root = newRoot
	chain.MockCanonicalRoots[slot] = bytesutil.ToBytes32(newRoot)
	chain.MockCanonicalFull[slot] = true

	second, err := vs.PayloadAttestationData(t.Context(), &ethpb.PayloadAttestationDataRequest{Slot: slot})
	require.NoError(t, err)
	assert.Equal(t, true, first == second) // same pointer = served from cache
	require.DeepEqual(t, root, second.BeaconBlockRoot)
	assert.Equal(t, false, second.PayloadPresent)

	// Advance to a new slot; cache must be bypassed and the fresh values returned.
	nextSlot := slot + 1
	chain.Slot = &nextSlot
	chain.BlockSlot = nextSlot
	chain.MockCanonicalRoots[nextSlot] = bytesutil.ToBytes32(newRoot)
	chain.MockCanonicalFull[nextSlot] = true
	chain.MockPayloadEarly = map[[32]byte]bool{bytesutil.ToBytes32(newRoot): true}

	third, err := vs.PayloadAttestationData(t.Context(), &ethpb.PayloadAttestationDataRequest{Slot: nextSlot})
	require.NoError(t, err)
	assert.Equal(t, false, first == third)
	require.DeepEqual(t, newRoot, third.BeaconBlockRoot)
	assert.Equal(t, nextSlot, third.Slot)
	assert.Equal(t, true, third.PayloadPresent)
}

func TestPayloadAttestationData_ConcurrentSingleflight(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	slot := primitives.Slot(7)
	root := bytesutil.PadTo([]byte{0xAA}, 32)
	chain := &chainMock.ChainService{
		Slot: &slot,
		Root: root,
		MockCanonicalRoots: map[primitives.Slot][32]byte{
			slot: bytesutil.ToBytes32(root),
		},
		MockCanonicalFull: map[primitives.Slot]bool{slot: false},
	}
	vs := &Server{
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
		TimeFetcher:       chain,
		HeadFetcher:       chain,
		ForkchoiceFetcher: chain,
		CoreService:       &core.Service{GenesisTimeFetcher: chain, ForkchoiceFetcher: chain},
	}

	const callers = 16
	results := make([]*ethpb.PayloadAttestationData, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			resp, err := vs.PayloadAttestationData(t.Context(), &ethpb.PayloadAttestationDataRequest{Slot: slot})
			require.NoError(t, err)
			results[i] = resp
		}(i)
	}
	close(start)
	wg.Wait()

	// All concurrent callers must receive the exact same pointer — proves the
	// burst was deduplicated rather than each computing independently.
	for i := 1; i < callers; i++ {
		assert.Equal(t, true, results[0] == results[i])
	}
}

func TestPayloadAttestationData_BeforeDeadline(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	slot := primitives.Slot(0)
	chain := &chainMock.ChainService{
		Slot:    &slot,
		Genesis: time.Now(),
		Root:    bytesutil.PadTo([]byte{0xAA}, 32),
	}
	vs := &Server{
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
		TimeFetcher:       chain,
		HeadFetcher:       chain,
		ForkchoiceFetcher: chain,
	}

	_, err := vs.PayloadAttestationData(t.Context(), &ethpb.PayloadAttestationDataRequest{Slot: slot})
	require.ErrorContains(t, "PTC deadline not yet reached", err)
	assert.Equal(t, (*ethpb.PayloadAttestationData)(nil), vs.payloadAttestationData.Load())
}

func TestPayloadAttestationData_SlotMismatch(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	current := primitives.Slot(10)
	requested := primitives.Slot(9)
	chain := &chainMock.ChainService{Slot: &current, Root: bytesutil.PadTo([]byte{0x01}, 32)}
	vs := &Server{
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
		TimeFetcher:       chain,
		HeadFetcher:       chain,
		ForkchoiceFetcher: chain,
		CoreService:       &core.Service{GenesisTimeFetcher: chain, ForkchoiceFetcher: chain},
	}

	_, err := vs.PayloadAttestationData(t.Context(), &ethpb.PayloadAttestationDataRequest{Slot: requested})
	require.ErrorContains(t, "only available for current slot", err)
}

func TestSubmitPayloadAttestation_OK(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	slot := primitives.Slot(0)
	root := bytesutil.PadTo([]byte{0xBB}, 32)
	st, _ := util.DeterministicGenesisStateGloas(t, 64)
	ptc, err := st.PayloadCommitteeReadOnly(slot)
	require.NoError(t, err)
	require.NotEqual(t, 0, len(ptc))

	chain := &chainMock.ChainService{
		Slot:      &slot,
		State:     st,
		BlockSlot: slot,
	}
	p2p := &p2pmock.MockBroadcaster{}
	receiver := &payloadAttestationBlockReceiver{ChainService: chain}

	vs := &Server{
		SyncChecker:                &mockSync.Sync{IsSyncing: false},
		TimeFetcher:                chain,
		HeadFetcher:                chain,
		ForkchoiceFetcher:          chain,
		P2P:                        p2p,
		BlockReceiver:              receiver,
		PayloadAttestationReceiver: receiver,
		PayloadAttestationPool:     payloadattestation.NewPool(),
		OperationNotifier:          chain.OperationNotifier(),
	}

	msg := &ethpb.PayloadAttestationMessage{
		ValidatorIndex: ptc[0],
		Data: &ethpb.PayloadAttestationData{
			BeaconBlockRoot: root,
			Slot:            slot,
		},
		Signature: make([]byte, 96),
	}

	resp, err := vs.SubmitPayloadAttestation(t.Context(), msg)
	require.NoError(t, err)
	require.DeepEqual(t, &emptypb.Empty{}, resp)
	assert.Equal(t, true, p2p.BroadcastCalled.Load())
	assert.Equal(t, true, receiver.received)
}

func TestSubmitPayloadAttestation_Syncing(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	slot := primitives.Slot(12)
	root := bytesutil.PadTo([]byte{0xCC}, 32)
	chain := &chainMock.ChainService{
		Slot:      &slot,
		BlockSlot: slot,
	}
	vs := &Server{
		SyncChecker:                &mockSync.Sync{IsSyncing: true},
		TimeFetcher:                chain,
		ForkchoiceFetcher:          chain,
		P2P:                        &p2pmock.MockBroadcaster{},
		BlockReceiver:              chain,
		PayloadAttestationReceiver: chain,
	}

	msg := &ethpb.PayloadAttestationMessage{
		ValidatorIndex: 1,
		Data: &ethpb.PayloadAttestationData{
			BeaconBlockRoot: root,
			Slot:            slot,
		},
		Signature: make([]byte, 96),
	}
	_, err := vs.SubmitPayloadAttestation(t.Context(), msg)
	require.ErrorContains(t, "not ready to respond", err)
}
