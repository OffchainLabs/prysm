//go:build minimal

package validator

import (
	"testing"

	mockChain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	dbutil "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	p2pmock "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"google.golang.org/protobuf/types/known/emptypb"
)

func testSignedBid(slot primitives.Slot, parentRoot [32]byte) *ethpb.SignedExecutionPayloadBid {
	return &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			ParentBlockHash:       make([]byte, 32),
			ParentBlockRoot:       parentRoot[:],
			BlockHash:             make([]byte, 32),
			PrevRandao:            make([]byte, 32),
			FeeRecipient:          make([]byte, 20),
			GasLimit:              30_000_000,
			BuilderIndex:          1,
			Slot:                  slot,
			Value:                 100,
			ExecutionRequestsRoot: make([]byte, 32),
		},
		Signature: make([]byte, 96),
	}
}

func TestSubmitSignedExecutionPayloadBid_OK(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	ctx := t.Context()
	st, _ := util.DeterministicGenesisStateGloas(t, 64)
	parentRoot := [32]byte{'p'}
	require.NoError(t, transition.UpdateNextSlotCache(ctx, parentRoot[:], st))

	db := dbutil.SetupDB(t)
	genesisRoot := [32]byte{'g'}
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesisRoot))

	prefs := cache.NewProposerPreferencesCache()
	prefs.Add(cache.ProposerPreference{DependentRoot: genesisRoot}, 1)

	p2p := &p2pmock.MockBroadcaster{}
	bidCache := cache.NewHighestExecutionPayloadBidCache()
	vs := &Server{
		SyncChecker:              &mockSync.Sync{IsSyncing: false},
		P2P:                      p2p,
		BeaconDB:                 db,
		ProposerPreferencesCache: prefs,
		HighestBidCache:          bidCache,
		OperationNotifier:        &mockChain.MockOperationNotifier{},
		ForkchoiceFetcher: &mockChain.ChainService{
			ForkchoiceGasLimits: map[[32]byte]uint64{parentRoot: 30_000_000},
		},
		NewExecutionPayloadBidVerifier: func(interfaces.ROSignedExecutionPayloadBid, []verification.Requirement) verification.ExecutionPayloadBidVerifier {
			return &fakeBidVerifier{}
		},
	}

	req := testSignedBid(1, parentRoot)
	resp, err := vs.SubmitSignedExecutionPayloadBid(ctx, req)
	require.NoError(t, err)
	require.DeepEqual(t, &emptypb.Empty{}, resp)
	assert.Equal(t, true, p2p.BroadcastCalled.Load())
	require.Equal(t, 1, len(p2p.BroadcastMessages))

	cached, ok := bidCache.Get(1, [32]byte{}, parentRoot)
	require.Equal(t, true, ok)
	require.Equal(t, req, cached)
}

func TestSubmitSignedExecutionPayloadBid_NoParentState(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	p2p := &p2pmock.MockBroadcaster{}
	vs := &Server{
		SyncChecker: &mockSync.Sync{IsSyncing: false},
		P2P:         p2p,
		NewExecutionPayloadBidVerifier: func(interfaces.ROSignedExecutionPayloadBid, []verification.Requirement) verification.ExecutionPayloadBidVerifier {
			return &fakeBidVerifier{}
		},
	}

	req := testSignedBid(1, [32]byte{'u', 'n', 'k', 'n', 'o', 'w', 'n'})
	_, err := vs.SubmitSignedExecutionPayloadBid(t.Context(), req)
	require.ErrorContains(t, "unavailable", err)
	assert.Equal(t, false, p2p.BroadcastCalled.Load())
}

func TestSubmitSignedExecutionPayloadBid_NoVerifier(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	p2p := &p2pmock.MockBroadcaster{}
	vs := &Server{
		SyncChecker: &mockSync.Sync{IsSyncing: false},
		P2P:         p2p,
	}

	req := testSignedBid(1, [32]byte{'p'})
	_, err := vs.SubmitSignedExecutionPayloadBid(t.Context(), req)
	require.ErrorContains(t, "verifier not ready", err)
	assert.Equal(t, false, p2p.BroadcastCalled.Load())
}

func TestSubmitSignedExecutionPayloadBid_NilRequest(t *testing.T) {
	vs := &Server{
		SyncChecker: &mockSync.Sync{IsSyncing: false},
	}
	_, err := vs.SubmitSignedExecutionPayloadBid(t.Context(), nil)
	require.ErrorContains(t, "nil", err)
}

func TestSubmitSignedExecutionPayloadBid_NilMessage(t *testing.T) {
	vs := &Server{
		SyncChecker: &mockSync.Sync{IsSyncing: false},
	}
	_, err := vs.SubmitSignedExecutionPayloadBid(t.Context(), &ethpb.SignedExecutionPayloadBid{})
	require.ErrorContains(t, "nil", err)
}

func TestSubmitSignedExecutionPayloadBid_Syncing(t *testing.T) {
	vs := &Server{
		SyncChecker: &mockSync.Sync{IsSyncing: true},
	}
	req := &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{Slot: 10},
	}
	_, err := vs.SubmitSignedExecutionPayloadBid(t.Context(), req)
	require.ErrorContains(t, "Syncing", err)
}

func TestSubmitSignedExecutionPayloadBid_PreGloas(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 100
	params.OverrideBeaconConfig(cfg)

	vs := &Server{
		SyncChecker: &mockSync.Sync{IsSyncing: false},
	}
	req := &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{Slot: 10},
	}
	_, err := vs.SubmitSignedExecutionPayloadBid(t.Context(), req)
	require.ErrorContains(t, "not supported before Gloas", err)
}
