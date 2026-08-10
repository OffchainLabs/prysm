package core

import (
	"context"
	"testing"

	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	dbutil "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	p2pmock "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

func TestSubmitSignedExecutionPayloadBid(t *testing.T) {
	t.Run("validation", func(t *testing.T) {
		t.Run("nil request", func(t *testing.T) {
			rpcErr := (&Service{}).SubmitSignedExecutionPayloadBid(t.Context(), nil)
			requireRPCError(t, rpcErr, BadRequest, "bid is nil")
		})

		t.Run("nil message", func(t *testing.T) {
			rpcErr := (&Service{}).SubmitSignedExecutionPayloadBid(t.Context(), &ethpb.SignedExecutionPayloadBid{})
			requireRPCError(t, rpcErr, BadRequest, "bid is nil")
		})

		t.Run("syncing", func(t *testing.T) {
			service := &Service{SyncChecker: &mockSync.Sync{IsSyncing: true}}
			rpcErr := service.SubmitSignedExecutionPayloadBid(t.Context(), signedExecutionPayloadBid(1, [32]byte{'p'}))
			requireRPCError(t, rpcErr, Unavailable, "not ready to respond")
		})

		t.Run("pre-fork", func(t *testing.T) {
			params.SetupTestConfigCleanup(t)
			cfg := params.BeaconConfig().Copy()
			cfg.GloasForkEpoch = 100
			params.OverrideBeaconConfig(cfg)
			service := &Service{SyncChecker: &mockSync.Sync{}}

			rpcErr := service.SubmitSignedExecutionPayloadBid(t.Context(), signedExecutionPayloadBid(1, [32]byte{'p'}))
			requireRPCError(t, rpcErr, BadRequest, "not supported before Gloas")
		})

		t.Run("verifier not ready", func(t *testing.T) {
			configureGloasFork(t)
			service := &Service{SyncChecker: &mockSync.Sync{}}

			rpcErr := service.SubmitSignedExecutionPayloadBid(t.Context(), signedExecutionPayloadBid(1, [32]byte{'p'}))
			requireRPCError(t, rpcErr, Internal, "verifier not ready")
		})

		t.Run("parent state unavailable", func(t *testing.T) {
			configureGloasFork(t)
			service := &Service{
				SyncChecker: &mockSync.Sync{},
				NewExecutionPayloadBidVerifier: func(interfaces.ROSignedExecutionPayloadBid, []verification.Requirement) verification.ExecutionPayloadBidVerifier {
					return &acceptingExecutionPayloadBidVerifier{}
				},
			}

			rpcErr := service.SubmitSignedExecutionPayloadBid(t.Context(), signedExecutionPayloadBid(1, [32]byte{'u'}))
			requireRPCError(t, rpcErr, FailedPrecondition, "parent block root is unavailable")
		})
	})

	t.Run("proposer preferences unavailable", func(t *testing.T) {
		configureGloasFork(t)
		ctx := t.Context()
		st, _ := util.DeterministicGenesisStateGloas(t, 64)
		parentRoot := [32]byte{'n'}
		require.NoError(t, transition.UpdateNextSlotCache(ctx, parentRoot[:], st))
		database := dbutil.SetupDB(t)
		require.NoError(t, database.SaveGenesisBlockRoot(ctx, [32]byte{'g'}))
		service := &Service{
			SyncChecker:              &mockSync.Sync{},
			BeaconDB:                 database,
			ProposerPreferencesCache: cache.NewProposerPreferencesCache(),
			NewExecutionPayloadBidVerifier: func(interfaces.ROSignedExecutionPayloadBid, []verification.Requirement) verification.ExecutionPayloadBidVerifier {
				return &acceptingExecutionPayloadBidVerifier{}
			},
		}

		rpcErr := service.SubmitSignedExecutionPayloadBid(ctx, signedExecutionPayloadBid(1, parentRoot))
		requireRPCError(t, rpcErr, FailedPrecondition, "no proposer preferences")
	})

	t.Run("accepted bid", func(t *testing.T) {
		configureGloasFork(t)
		ctx := t.Context()
		st, _ := util.DeterministicGenesisStateGloas(t, 64)
		parentRoot := [32]byte{'p'}
		require.NoError(t, transition.UpdateNextSlotCache(ctx, parentRoot[:], st))
		database := dbutil.SetupDB(t)
		genesisRoot := [32]byte{'g'}
		require.NoError(t, database.SaveGenesisBlockRoot(ctx, genesisRoot))
		preferencesCache := cache.NewProposerPreferencesCache()
		preferencesCache.Add(cache.ProposerPreference{DependentRoot: genesisRoot}, 1)
		bidCache := cache.NewHighestExecutionPayloadBidCache()
		broadcaster := &p2pmock.MockBroadcaster{}
		service := &Service{
			SyncChecker:              &mockSync.Sync{},
			P2P:                      broadcaster,
			BeaconDB:                 database,
			ProposerPreferencesCache: preferencesCache,
			HighestBidCache:          bidCache,
			OperationNotifier:        &chainMock.MockOperationNotifier{},
			ForkchoiceFetcher: &chainMock.ChainService{
				ForkchoiceGasLimits: map[[32]byte]uint64{parentRoot: 30_000_000},
			},
			NewExecutionPayloadBidVerifier: func(interfaces.ROSignedExecutionPayloadBid, []verification.Requirement) verification.ExecutionPayloadBidVerifier {
				return &acceptingExecutionPayloadBidVerifier{}
			},
		}
		req := signedExecutionPayloadBid(1, parentRoot)

		requireNoRPCError(t, service.SubmitSignedExecutionPayloadBid(ctx, req))
		require.Equal(t, 1, broadcaster.NumMessages())
		cached, ok := bidCache.Get(1, [32]byte{}, parentRoot)
		require.Equal(t, true, ok)
		require.Equal(t, req, cached)

		t.Run("broadcast failure remains cached", func(t *testing.T) {
			failedReq := signedExecutionPayloadBid(1, parentRoot)
			failedReq.Message.Value = req.Message.Value + 1
			service.P2P = &failingExecutionPayloadBidBroadcaster{MockBroadcaster: &p2pmock.MockBroadcaster{}}

			rpcErr := service.SubmitSignedExecutionPayloadBid(ctx, failedReq)
			requireRPCError(t, rpcErr, Internal, "could not broadcast signed execution payload bid")
			cached, ok := bidCache.Get(1, [32]byte{}, parentRoot)
			require.Equal(t, true, ok)
			require.Equal(t, failedReq, cached)
		})
	})
}

func signedExecutionPayloadBid(slot primitives.Slot, parentRoot [32]byte) *ethpb.SignedExecutionPayloadBid {
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

type acceptingExecutionPayloadBidVerifier struct{}

func (*acceptingExecutionPayloadBidVerifier) VerifyCurrentOrNextSlot() error             { return nil }
func (*acceptingExecutionPayloadBidVerifier) VerifyBidSlotMatches(primitives.Slot) error { return nil }
func (*acceptingExecutionPayloadBidVerifier) VerifyBuilderActive(state.ReadOnlyBeaconState) error {
	return nil
}
func (*acceptingExecutionPayloadBidVerifier) VerifyBuilderVersion(state.ReadOnlyBeaconState) error {
	return nil
}
func (*acceptingExecutionPayloadBidVerifier) VerifyExecutionPaymentZero() error      { return nil }
func (*acceptingExecutionPayloadBidVerifier) VerifyFeeRecipientMatches([]byte) error { return nil }
func (*acceptingExecutionPayloadBidVerifier) VerifyBlobKzgCommitmentsLimit() error   { return nil }
func (*acceptingExecutionPayloadBidVerifier) VerifyPrevRandao(state.ReadOnlyBeaconState) error {
	return nil
}
func (*acceptingExecutionPayloadBidVerifier) VerifyParentBlockRootSeen(func([32]byte) bool) error {
	return nil
}
func (*acceptingExecutionPayloadBidVerifier) VerifyBidCompatibleWithHead(func(interfaces.ROExecutionPayloadBid) bool) error {
	return nil
}
func (*acceptingExecutionPayloadBidVerifier) VerifyBidSlotHigherThanParent(primitives.Slot) error {
	return nil
}
func (*acceptingExecutionPayloadBidVerifier) VerifyParentBlockHash(func([32]byte, [32]byte) bool) error {
	return nil
}
func (*acceptingExecutionPayloadBidVerifier) VerifyGasLimitTargetCompatible(uint64, uint64) error {
	return nil
}
func (*acceptingExecutionPayloadBidVerifier) VerifyBuilderCanCoverBid(state.ReadOnlyBeaconState) error {
	return nil
}
func (*acceptingExecutionPayloadBidVerifier) VerifySignature(state.ReadOnlyBeaconState) error {
	return nil
}
func (*acceptingExecutionPayloadBidVerifier) SatisfyRequirement(verification.Requirement) {}

type failingExecutionPayloadBidBroadcaster struct {
	*p2pmock.MockBroadcaster
}

func (*failingExecutionPayloadBidBroadcaster) Broadcast(context.Context, proto.Message) error {
	return errors.New("broadcast failed")
}
