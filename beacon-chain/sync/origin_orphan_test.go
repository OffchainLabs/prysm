package sync

import (
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"sync/atomic"
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
