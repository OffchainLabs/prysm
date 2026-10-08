package sync

import (
	"sync/atomic"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// TestDetectOrphanedOrigin_NoPeers pins the devnet regression: with --min-sync-peers=0 the
// detector fired on a genesis synced node that had no peers and no checkpoint origin at all.
func TestDetectOrphanedOrigin_NoPeers(t *testing.T) {
	resetFlags := flags.Get()
	flags.Init(&flags.GlobalFlags{MinimumSyncPeers: 0})
	defer flags.Init(resetFlags)

	ctx := t.Context()
	beaconDB := dbtest.SetupDB(t)
	r := &Service{
		cfg: &config{
			chain:    &mock.ChainService{FinalizedCheckPoint: &ethpb.Checkpoint{Epoch: 10, Root: bytesutil.PadTo([]byte("origin"), 32)}},
			beaconDB: beaconDB,
			p2p:      p2ptest.NewTestP2P(t),
			clock:    startup.NewClock(time.Now(), [32]byte{}),
		},
		ctx:                  ctx,
		orphanedOriginStreak: &atomic.Int64{},
	}

	for i := 0; i < 3; i++ {
		r.detectOrphanedOrigin(ctx)
	}
	require.NoError(t, r.Status())
	require.Equal(t, int64(0), r.orphanedOriginStreak.Load())
}

// TestDetectOrphanedOrigin_GenesisSynced covers a node with no checkpoint sync origin.
func TestDetectOrphanedOrigin_GenesisSynced(t *testing.T) {
	resetFlags := flags.Get()
	flags.Init(&flags.GlobalFlags{MinimumSyncPeers: 1})
	defer flags.Init(resetFlags)

	ctx := t.Context()
	beaconDB := dbtest.SetupDB(t)
	r := &Service{
		cfg: &config{
			chain:    &mock.ChainService{FinalizedCheckPoint: &ethpb.Checkpoint{Epoch: 10, Root: bytesutil.PadTo([]byte("origin"), 32)}},
			beaconDB: beaconDB,
			p2p:      p2ptest.NewTestP2P(t),
			clock:    startup.NewClock(time.Now(), [32]byte{}),
		},
		ctx:                  ctx,
		orphanedOriginStreak: &atomic.Int64{},
	}

	r.detectOrphanedOrigin(ctx)
	require.Equal(t, int64(0), r.orphanedOriginStreak.Load())
	require.NoError(t, r.Status())
}

// newOrphanTestService returns a service whose finalized checkpoint is finalizedRoot at epoch 10,
// with originRoot saved as the checkpoint sync origin and two connected peers that have finalized
// epoch 12 at a root unknown to us.
func newOrphanTestService(t *testing.T, originRoot, finalizedRoot [32]byte) *Service {
	ctx := t.Context()
	beaconDB := dbtest.SetupDB(t)
	require.NoError(t, beaconDB.SaveOriginCheckpointBlockRoot(ctx, originRoot))

	p := p2ptest.NewTestP2P(t)
	for _, id := range []peer.ID{"peer1", "peer2"} {
		p.Peers().Add(new(enr.Record), id, nil, network.DirOutbound)
		p.Peers().SetConnectionState(id, peers.Connected)
		p.Peers().SetChainState(id, &ethpb.StatusV2{
			FinalizedEpoch: 12,
			FinalizedRoot:  bytesutil.PadTo([]byte("unknown"), 32),
		})
	}
	return &Service{
		cfg: &config{
			chain:    &mock.ChainService{FinalizedCheckPoint: &ethpb.Checkpoint{Epoch: 10, Root: finalizedRoot[:]}},
			beaconDB: beaconDB,
			p2p:      p,
			clock:    startup.NewClock(time.Now(), [32]byte{}),
		},
		ctx:                  ctx,
		orphanedOriginStreak: &atomic.Int64{},
	}
}

// TestDetectOrphanedOrigin_FinalizedAtOrigin flags peers finalizing an unknown chain while our
// finalized checkpoint is still the origin.
func TestDetectOrphanedOrigin_FinalizedAtOrigin(t *testing.T) {
	resetFlags := flags.Get()
	flags.Init(&flags.GlobalFlags{MinimumSyncPeers: 1})
	defer flags.Init(resetFlags)

	origin := bytesutil.ToBytes32([]byte("origin"))
	r := newOrphanTestService(t, origin, origin)

	for range orphanedOriginStreakThreshold - 1 {
		r.detectOrphanedOrigin(t.Context())
	}
	require.NoError(t, r.Status())
	r.detectOrphanedOrigin(t.Context())
	require.ErrorIs(t, r.Status(), errOrphanedOrigin)
}

// TestDetectOrphanedOrigin_FinalizedPastOrigin covers a node that finalized past its origin and
// then fell behind its peers: the origin is canonical, so it must not be reported as orphaned.
func TestDetectOrphanedOrigin_FinalizedPastOrigin(t *testing.T) {
	resetFlags := flags.Get()
	flags.Init(&flags.GlobalFlags{MinimumSyncPeers: 1})
	defer flags.Init(resetFlags)

	origin := bytesutil.ToBytes32([]byte("origin"))
	finalized := bytesutil.ToBytes32([]byte("finalized"))
	r := newOrphanTestService(t, origin, finalized)

	for range orphanedOriginStreakThreshold + 1 {
		r.detectOrphanedOrigin(t.Context())
	}
	require.Equal(t, int64(0), r.orphanedOriginStreak.Load())
	require.NoError(t, r.Status())
}
