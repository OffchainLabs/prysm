package validator

import (
	"testing"

	p2pmock "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestSubmitSignedExecutionPayloadBid_OK(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	p2p := &p2pmock.MockBroadcaster{}
	vs := &Server{
		SyncChecker: &mockSync.Sync{IsSyncing: false},
		P2P:         p2p,
	}

	req := &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			ParentBlockHash:       make([]byte, 32),
			ParentBlockRoot:       make([]byte, 32),
			BlockHash:             make([]byte, 32),
			PrevRandao:            make([]byte, 32),
			FeeRecipient:          make([]byte, 20),
			GasLimit:              30_000_000,
			BuilderIndex:          1,
			Slot:                  10,
			Value:                 100,
			ExecutionRequestsRoot: make([]byte, 32),
		},
		Signature: make([]byte, 96),
	}

	resp, err := vs.SubmitSignedExecutionPayloadBid(t.Context(), req)
	require.NoError(t, err)
	require.DeepEqual(t, &emptypb.Empty{}, resp)
	assert.Equal(t, true, p2p.BroadcastCalled.Load())
	require.Equal(t, 1, len(p2p.BroadcastMessages))
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
