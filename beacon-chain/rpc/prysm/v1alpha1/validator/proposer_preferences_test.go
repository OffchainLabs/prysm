package validator

import (
	"testing"

	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	p2pmock "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestSubmitSignedProposerPreferences_OK(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 1
	params.OverrideBeaconConfig(cfg)

	currentSlot := primitives.Slot(31)
	proposalSlot := currentSlot + 1
	chain := &chainMock.ChainService{Slot: &currentSlot}
	p2p := &p2pmock.MockBroadcaster{}
	cache := cache.NewProposerPreferencesCache()
	vs := &Server{
		SyncChecker:              &mockSync.Sync{IsSyncing: false},
		TimeFetcher:              chain,
		P2P:                      p2p,
		ProposerPreferencesCache: cache,
	}

	req := &ethpb.SubmitSignedProposerPreferencesRequest{
		SignedProposerPreferences: []*ethpb.SignedProposerPreferences{
			{
				Message: &ethpb.ProposerPreferences{
					ProposalSlot:   proposalSlot,
					ValidatorIndex: 2,
					FeeRecipient:   make([]byte, 20),
					GasLimit:       30_000_000,
				},
				Signature: make([]byte, 96),
			},
		},
	}

	resp, err := vs.SubmitSignedProposerPreferences(t.Context(), req)
	require.NoError(t, err)
	require.DeepEqual(t, &emptypb.Empty{}, resp)
	assert.Equal(t, true, p2p.BroadcastCalled.Load())
	pref, ok := cache.Get(proposalSlot)
	require.Equal(t, true, ok)
	require.DeepEqual(t, req.SignedProposerPreferences[0].Message.FeeRecipient, pref.FeeRecipient)
	require.Equal(t, req.SignedProposerPreferences[0].Message.GasLimit, pref.GasLimit)
}

func TestSubmitSignedProposerPreferences_Multiple(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 1
	params.OverrideBeaconConfig(cfg)

	currentSlot := primitives.Slot(31)
	chain := &chainMock.ChainService{Slot: &currentSlot}
	p2p := &p2pmock.MockBroadcaster{}
	c := cache.NewProposerPreferencesCache()
	vs := &Server{
		SyncChecker:              &mockSync.Sync{IsSyncing: false},
		TimeFetcher:              chain,
		P2P:                      p2p,
		ProposerPreferencesCache: c,
	}

	req := &ethpb.SubmitSignedProposerPreferencesRequest{
		SignedProposerPreferences: []*ethpb.SignedProposerPreferences{
			{
				Message: &ethpb.ProposerPreferences{
					ProposalSlot:   currentSlot + 1,
					ValidatorIndex: 2,
					FeeRecipient:   make([]byte, 20),
					GasLimit:       30_000_000,
				},
				Signature: make([]byte, 96),
			},
			{
				Message: &ethpb.ProposerPreferences{
					ProposalSlot:   currentSlot + 2,
					ValidatorIndex: 5,
					FeeRecipient:   make([]byte, 20),
					GasLimit:       25_000_000,
				},
				Signature: make([]byte, 96),
			},
		},
	}

	resp, err := vs.SubmitSignedProposerPreferences(t.Context(), req)
	require.NoError(t, err)
	require.DeepEqual(t, &emptypb.Empty{}, resp)

	_, ok := c.Get(currentSlot + 1)
	require.Equal(t, true, ok)
	pref2, ok := c.Get(currentSlot + 2)
	require.Equal(t, true, ok)
	require.Equal(t, uint64(25_000_000), pref2.GasLimit)
}

func TestSubmitSignedProposerPreferences_DuplicateSlot(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 1
	params.OverrideBeaconConfig(cfg)

	currentSlot := primitives.Slot(31)
	proposalSlot := currentSlot + 1
	chain := &chainMock.ChainService{Slot: &currentSlot}
	p2p := &p2pmock.MockBroadcaster{}
	c := cache.NewProposerPreferencesCache()
	c.Add(proposalSlot, make([]byte, 20), 30_000_000)
	vs := &Server{
		SyncChecker:              &mockSync.Sync{IsSyncing: false},
		TimeFetcher:              chain,
		P2P:                      p2p,
		ProposerPreferencesCache: c,
	}

	req := &ethpb.SubmitSignedProposerPreferencesRequest{
		SignedProposerPreferences: []*ethpb.SignedProposerPreferences{
			{
				Message: &ethpb.ProposerPreferences{
					ProposalSlot:   proposalSlot,
					ValidatorIndex: 2,
					FeeRecipient:   make([]byte, 20),
					GasLimit:       30_000_000,
				},
				Signature: make([]byte, 96),
			},
		},
	}

	resp, err := vs.SubmitSignedProposerPreferences(t.Context(), req)
	require.NoError(t, err)
	require.DeepEqual(t, &emptypb.Empty{}, resp)
	assert.Equal(t, false, p2p.BroadcastCalled.Load())
}

func TestSubmitSignedProposerPreferences_InvalidEpoch(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 1
	params.OverrideBeaconConfig(cfg)

	currentSlot := primitives.Slot(31)
	chain := &chainMock.ChainService{Slot: &currentSlot}
	vs := &Server{
		SyncChecker:              &mockSync.Sync{IsSyncing: false},
		TimeFetcher:              chain,
		P2P:                      &p2pmock.MockBroadcaster{},
		ProposerPreferencesCache: cache.NewProposerPreferencesCache(),
	}

	// Same epoch (current), not next epoch.
	req := &ethpb.SubmitSignedProposerPreferencesRequest{
		SignedProposerPreferences: []*ethpb.SignedProposerPreferences{
			{
				Message: &ethpb.ProposerPreferences{
					ProposalSlot:   currentSlot,
					ValidatorIndex: 2,
					FeeRecipient:   make([]byte, 20),
					GasLimit:       30_000_000,
				},
				Signature: make([]byte, 96),
			},
		},
	}
	_, err := vs.SubmitSignedProposerPreferences(t.Context(), req)
	require.ErrorContains(t, "next epoch", err)

	// Two epochs ahead.
	req.SignedProposerPreferences[0].Message.ProposalSlot = currentSlot + primitives.Slot(2*params.BeaconConfig().SlotsPerEpoch)
	_, err = vs.SubmitSignedProposerPreferences(t.Context(), req)
	require.ErrorContains(t, "next epoch", err)
}

func TestSubmitSignedProposerPreferences_Syncing(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 1
	params.OverrideBeaconConfig(cfg)

	currentSlot := primitives.Slot(31)
	chain := &chainMock.ChainService{Slot: &currentSlot}
	vs := &Server{
		SyncChecker:              &mockSync.Sync{IsSyncing: true},
		TimeFetcher:              chain,
		P2P:                      &p2pmock.MockBroadcaster{},
		ProposerPreferencesCache: cache.NewProposerPreferencesCache(),
	}

	req := &ethpb.SubmitSignedProposerPreferencesRequest{
		SignedProposerPreferences: []*ethpb.SignedProposerPreferences{
			{
				Message: &ethpb.ProposerPreferences{
					ProposalSlot:   currentSlot + 1,
					ValidatorIndex: 2,
					FeeRecipient:   make([]byte, 20),
					GasLimit:       30_000_000,
				},
				Signature: make([]byte, 96),
			},
		},
	}

	_, err := vs.SubmitSignedProposerPreferences(t.Context(), req)
	require.ErrorContains(t, "not ready to respond", err)
}
