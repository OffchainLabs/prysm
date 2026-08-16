//go:build minimal

package validator

import (
	"testing"

	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	p2pmock "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestSubmitSignedProposerPreferencesGRPC(t *testing.T) {
	t.Run("invalid argument", func(t *testing.T) {
		server := &Server{CoreService: &core.Service{}}

		_, err := server.SubmitSignedProposerPreferences(t.Context(), nil)
		require.ErrorContains(t, "request is empty", err)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("success", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 1
		params.OverrideBeaconConfig(cfg)
		currentSlot := primitives.Slot(cfg.SlotsPerEpoch - 1)
		proposalSlot := primitives.Slot(cfg.SlotsPerEpoch)
		chain := &chainMock.ChainService{Slot: &currentSlot}
		preferencesCache := cache.NewProposerPreferencesCache()
		server := &Server{CoreService: &core.Service{
			SyncChecker:              &mockSync.Sync{},
			GenesisTimeFetcher:       chain,
			P2P:                      &p2pmock.MockBroadcaster{},
			ProposerPreferencesCache: preferencesCache,
			OperationNotifier:        chain.OperationNotifier(),
		}}
		req := &ethpb.SubmitSignedProposerPreferencesRequest{
			SignedProposerPreferences: []*ethpb.SignedProposerPreferences{{
				Message: &ethpb.ProposerPreferences{
					DependentRoot:  bytesutil.PadTo([]byte{0xcc}, 32),
					ProposalSlot:   proposalSlot,
					ValidatorIndex: 2,
					FeeRecipient:   make([]byte, 20),
					TargetGasLimit: 30_000_000,
				},
				Signature: make([]byte, 96),
			}},
		}

		response, err := server.SubmitSignedProposerPreferences(t.Context(), req)
		require.NoError(t, err)
		require.DeepEqual(t, &emptypb.Empty{}, response)
		require.Equal(t, true, preferencesCache.Has([32]byte{0xcc}, proposalSlot))
	})
}
