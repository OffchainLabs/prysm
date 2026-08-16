//go:build minimal

package validator

import (
	"testing"

	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	dbutil "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	p2pmock "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestSubmitSignedExecutionPayloadBidGRPC(t *testing.T) {
	t.Run("invalid argument", func(t *testing.T) {
		server := &Server{CoreService: &core.Service{}}

		_, err := server.SubmitSignedExecutionPayloadBid(t.Context(), nil)
		require.ErrorContains(t, "bid is nil", err)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("success", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)
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
		server := &Server{CoreService: &core.Service{
			SyncChecker:              &mockSync.Sync{},
			P2P:                      &p2pmock.MockBroadcaster{},
			BeaconDB:                 database,
			ProposerPreferencesCache: preferencesCache,
			HighestBidCache:          bidCache,
			OperationNotifier:        &chainMock.MockOperationNotifier{},
			ForkchoiceFetcher: &chainMock.ChainService{
				ForkchoiceGasLimits: map[[32]byte]uint64{parentRoot: 30_000_000},
			},
			NewExecutionPayloadBidVerifier: func(interfaces.ROSignedExecutionPayloadBid, []verification.Requirement) verification.ExecutionPayloadBidVerifier {
				return &fakeBidVerifier{}
			},
		}}
		req := &ethpb.SignedExecutionPayloadBid{
			Message: &ethpb.ExecutionPayloadBid{
				ParentBlockHash:       make([]byte, 32),
				ParentBlockRoot:       parentRoot[:],
				BlockHash:             make([]byte, 32),
				PrevRandao:            make([]byte, 32),
				FeeRecipient:          make([]byte, 20),
				GasLimit:              30_000_000,
				BuilderIndex:          1,
				Slot:                  1,
				Value:                 100,
				ExecutionRequestsRoot: make([]byte, 32),
			},
			Signature: make([]byte, 96),
		}

		response, err := server.SubmitSignedExecutionPayloadBid(ctx, req)
		require.NoError(t, err)
		require.DeepEqual(t, &emptypb.Empty{}, response)
		cached, ok := bidCache.Get(1, [32]byte{}, parentRoot)
		require.Equal(t, true, ok)
		require.Equal(t, req, cached)
	})
}
