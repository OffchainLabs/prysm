package core

import (
	"context"
	"testing"

	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	p2pmock "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

func TestSubmitSignedProposerPreferences(t *testing.T) {
	t.Run("empty request", func(t *testing.T) {
		rpcErr := (&Service{}).SubmitSignedProposerPreferences(t.Context(), nil)
		requireRPCError(t, rpcErr, BadRequest, "request is empty")
	})

	t.Run("syncing", func(t *testing.T) {
		service := &Service{SyncChecker: &mockSync.Sync{IsSyncing: true}}
		rpcErr := service.SubmitSignedProposerPreferences(t.Context(), signedProposerPreferencesRequest(0xcc, 32))
		requireRPCError(t, rpcErr, Unavailable, "not ready to respond")
	})

	t.Run("validation", func(t *testing.T) {
		t.Run("nil message", func(t *testing.T) {
			configureGloasFork(t)
			currentSlot := primitives.Slot(31)
			chain := &chainMock.ChainService{Slot: &currentSlot}
			service := &Service{
				SyncChecker:              &mockSync.Sync{},
				GenesisTimeFetcher:       chain,
				P2P:                      &p2pmock.MockBroadcaster{},
				ProposerPreferencesCache: cache.NewProposerPreferencesCache(),
				OperationNotifier:        chain.OperationNotifier(),
			}

			rpcErr := service.SubmitSignedProposerPreferences(t.Context(), &ethpb.SubmitSignedProposerPreferencesRequest{
				SignedProposerPreferences: []*ethpb.SignedProposerPreferences{nil},
			})
			requireRPCError(t, rpcErr, BadRequest, "message is nil")
		})

		t.Run("pre-fork", func(t *testing.T) {
			params.SetupTestConfigCleanup(t)
			cfg := params.BeaconConfig().Copy()
			cfg.GloasForkEpoch = 2
			params.OverrideBeaconConfig(cfg)
			currentSlot := primitives.Slot(31)
			chain := &chainMock.ChainService{Slot: &currentSlot}
			service := &Service{
				SyncChecker:              &mockSync.Sync{},
				GenesisTimeFetcher:       chain,
				P2P:                      &p2pmock.MockBroadcaster{},
				ProposerPreferencesCache: cache.NewProposerPreferencesCache(),
				OperationNotifier:        chain.OperationNotifier(),
			}

			rpcErr := service.SubmitSignedProposerPreferences(t.Context(), signedProposerPreferencesRequest(0xcc, 32))
			requireRPCError(t, rpcErr, BadRequest, "not supported before Gloas fork")
		})

		t.Run("passed slot", func(t *testing.T) {
			configureGloasFork(t)
			currentSlot := primitives.Slot(31)
			chain := &chainMock.ChainService{Slot: &currentSlot}
			service := &Service{
				SyncChecker:              &mockSync.Sync{},
				GenesisTimeFetcher:       chain,
				P2P:                      &p2pmock.MockBroadcaster{},
				ProposerPreferencesCache: cache.NewProposerPreferencesCache(),
				OperationNotifier:        chain.OperationNotifier(),
			}

			rpcErr := service.SubmitSignedProposerPreferences(t.Context(), signedProposerPreferencesRequest(0xcc, currentSlot))
			requireRPCError(t, rpcErr, BadRequest, "already passed")
		})

		t.Run("two epochs ahead", func(t *testing.T) {
			configureGloasFork(t)
			currentSlot := primitives.Slot(31)
			chain := &chainMock.ChainService{Slot: &currentSlot}
			service := &Service{
				SyncChecker:              &mockSync.Sync{},
				GenesisTimeFetcher:       chain,
				P2P:                      &p2pmock.MockBroadcaster{},
				ProposerPreferencesCache: cache.NewProposerPreferencesCache(),
				OperationNotifier:        chain.OperationNotifier(),
			}

			rpcErr := service.SubmitSignedProposerPreferences(t.Context(), signedProposerPreferencesRequest(0xcc, currentSlot+primitives.Slot(2*params.BeaconConfig().SlotsPerEpoch)))
			requireRPCError(t, rpcErr, BadRequest, "current or next epoch")
		})

		t.Run("dependent root length", func(t *testing.T) {
			configureGloasFork(t)
			currentSlot := primitives.Slot(31)
			chain := &chainMock.ChainService{Slot: &currentSlot}
			service := &Service{
				SyncChecker:              &mockSync.Sync{},
				GenesisTimeFetcher:       chain,
				P2P:                      &p2pmock.MockBroadcaster{},
				ProposerPreferencesCache: cache.NewProposerPreferencesCache(),
				OperationNotifier:        chain.OperationNotifier(),
			}
			req := signedProposerPreferencesRequest(0xcc, 32)
			req.SignedProposerPreferences[0].Message.DependentRoot = []byte{0xcc}

			rpcErr := service.SubmitSignedProposerPreferences(t.Context(), req)
			requireRPCError(t, rpcErr, BadRequest, "dependent_root must be 32 bytes")
		})
	})

	t.Run("success", func(t *testing.T) {
		configureGloasFork(t)
		currentSlot := primitives.Slot(31)
		chain := &chainMock.ChainService{Slot: &currentSlot}
		broadcaster := &p2pmock.MockBroadcaster{}
		preferencesCache := cache.NewProposerPreferencesCache()
		service := &Service{
			SyncChecker:              &mockSync.Sync{},
			GenesisTimeFetcher:       chain,
			P2P:                      broadcaster,
			ProposerPreferencesCache: preferencesCache,
			OperationNotifier:        chain.OperationNotifier(),
		}
		req := signedProposerPreferencesRequest(0xcc, 32)
		req.SignedProposerPreferences = append(req.SignedProposerPreferences, signedProposerPreferencesRequest(0xbb, 33).SignedProposerPreferences[0])

		requireNoRPCError(t, service.SubmitSignedProposerPreferences(t.Context(), req))
		require.Equal(t, 2, broadcaster.NumMessages())
		require.DeepEqual(t, []primitives.Epoch{1, 1}, broadcaster.BroadcastEpochs)
		preference, ok := preferencesCache.Get([32]byte{0xbb}, 33)
		require.Equal(t, true, ok)
		require.Equal(t, uint64(30_000_000), preference.TargetGasLimit)
	})

	t.Run("current epoch future slot", func(t *testing.T) {
		configureGloasFork(t)
		currentSlot := primitives.Slot(33)
		chain := &chainMock.ChainService{Slot: &currentSlot}
		service := &Service{
			SyncChecker:              &mockSync.Sync{},
			GenesisTimeFetcher:       chain,
			P2P:                      &p2pmock.MockBroadcaster{},
			ProposerPreferencesCache: cache.NewProposerPreferencesCache(),
			OperationNotifier:        chain.OperationNotifier(),
		}

		requireNoRPCError(t, service.SubmitSignedProposerPreferences(t.Context(), signedProposerPreferencesRequest(0xcc, 34)))
	})

	t.Run("duplicate", func(t *testing.T) {
		configureGloasFork(t)
		currentSlot := primitives.Slot(31)
		chain := &chainMock.ChainService{Slot: &currentSlot}
		broadcaster := &p2pmock.MockBroadcaster{}
		service := &Service{
			SyncChecker:              &mockSync.Sync{},
			GenesisTimeFetcher:       chain,
			P2P:                      broadcaster,
			ProposerPreferencesCache: cache.NewProposerPreferencesCache(),
			OperationNotifier:        chain.OperationNotifier(),
		}
		req := signedProposerPreferencesRequest(0xcc, 32)

		requireNoRPCError(t, service.SubmitSignedProposerPreferences(t.Context(), req))
		requireNoRPCError(t, service.SubmitSignedProposerPreferences(t.Context(), req))
		require.Equal(t, 1, broadcaster.NumMessages())
	})

	t.Run("broadcast failure is not cached", func(t *testing.T) {
		configureGloasFork(t)
		currentSlot := primitives.Slot(31)
		chain := &chainMock.ChainService{Slot: &currentSlot}
		preferencesCache := cache.NewProposerPreferencesCache()
		service := &Service{
			SyncChecker:              &mockSync.Sync{},
			GenesisTimeFetcher:       chain,
			P2P:                      &failingProposerPreferencesBroadcaster{MockBroadcaster: &p2pmock.MockBroadcaster{}},
			ProposerPreferencesCache: preferencesCache,
			OperationNotifier:        chain.OperationNotifier(),
		}
		req := signedProposerPreferencesRequest(0xcc, 32)

		requireRPCError(t, service.SubmitSignedProposerPreferences(t.Context(), req), Internal, "broadcast failed")
		require.Equal(t, false, preferencesCache.Has([32]byte{0xcc}, 32))
		broadcaster := &p2pmock.MockBroadcaster{}
		service.P2P = broadcaster
		requireNoRPCError(t, service.SubmitSignedProposerPreferences(t.Context(), req))
		require.Equal(t, 1, broadcaster.NumMessages())
		require.Equal(t, true, preferencesCache.Has([32]byte{0xcc}, 32))
	})
}

func signedProposerPreferencesRequest(root byte, slot primitives.Slot) *ethpb.SubmitSignedProposerPreferencesRequest {
	return &ethpb.SubmitSignedProposerPreferencesRequest{
		SignedProposerPreferences: []*ethpb.SignedProposerPreferences{{
			Message: &ethpb.ProposerPreferences{
				DependentRoot:  bytesutil.PadTo([]byte{root}, 32),
				ProposalSlot:   slot,
				ValidatorIndex: 2,
				FeeRecipient:   make([]byte, 20),
				TargetGasLimit: 30_000_000,
			},
			Signature: make([]byte, 96),
		}},
	}
}

type failingProposerPreferencesBroadcaster struct {
	*p2pmock.MockBroadcaster
}

func (*failingProposerPreferencesBroadcaster) BroadcastForEpoch(context.Context, proto.Message, primitives.Epoch) error {
	return errors.New("broadcast failed")
}
