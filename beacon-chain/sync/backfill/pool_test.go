package backfill

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/libp2p/go-libp2p/core/peer"
)

type mockAssigner struct {
	err    error
	assign []peer.ID
}

// Assign satisfies the PeerAssigner interface so that mockAssigner can be used in tests
// in place of the concrete p2p implementation of PeerAssigner.
func (m mockAssigner) Assign(filter peers.AssignmentFilter) ([]peer.ID, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.assign, nil
}

var _ PeerAssigner = &mockAssigner{}

func mockNewBlobVerifier(_ blocks.ROBlob, _ []verification.Requirement) verification.BlobVerifier {
	return &verification.MockBlobVerifier{}
}

func TestPoolDetectAllEnded(t *testing.T) {
	nw := 5
	p2p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	ma := &mockAssigner{}
	pool := newP2PBatchWorkerPool(p2p, nw)
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	keys, err := st.PublicKeys()
	require.NoError(t, err)
	v, err := newBackfillVerifier(st.GenesisValidatorsRoot(), keys)
	require.NoError(t, err)

	ctxMap, err := sync.ContextByteVersionsForValRoot(bytesutil.ToBytes32(st.GenesisValidatorsRoot()))
	require.NoError(t, err)
	bfs := filesystem.NewEphemeralBlobStorage(t)
	wcfg := &workerCfg{c: startup.NewClock(time.Now(), [32]byte{}), nbv: mockNewBlobVerifier, v: v, cm: ctxMap, bfs: bfs}
	pool.spawn(ctx, nw, ma, wcfg)
	br := batcher{min: 10, size: 10}
	endSeq := br.before(0)
	require.Equal(t, batchEndSequence, endSeq.state)
	for i := 0; i < nw; i++ {
		pool.todo(endSeq)
	}
	b, err := pool.complete()
	require.ErrorIs(t, err, errEndSequence)
	require.Equal(t, b.end, endSeq.end)
}

type mockPool struct {
	spawnCalled  []int
	finishedChan chan batch
	finishedErr  chan error
	todoChan     chan batch
}

func (m *mockPool) spawn(_ context.Context, _ int, _ PeerAssigner, _ *workerCfg) {
}

func (m *mockPool) todo(b batch) {
	m.todoChan <- b
}

func (m *mockPool) complete() (batch, error) {
	select {
	case b := <-m.finishedChan:
		return b, nil
	case err := <-m.finishedErr:
		return batch{}, err
	}
}

var _ batchWorkerPool = &mockPool{}
