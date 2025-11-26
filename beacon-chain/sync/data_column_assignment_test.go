package sync

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// mockP2PForDAS wraps TestP2P to provide a known custody group count for any peer.
type mockP2PForDAS struct {
	*p2ptest.TestP2P
	custodyGroupCount uint64
}

func (m *mockP2PForDAS) CustodyGroupCountFromPeer(_ peer.ID) uint64 {
	return m.custodyGroupCount
}

// testDASSetup provides test fixtures for DAS peer assignment tests.
type testDASSetup struct {
	t          *testing.T
	cache      *DASPeerCache
	p2pService *mockP2PForDAS
	peers      []*p2ptest.TestP2P
	peerIDs    []peer.ID
}

// createSecp256k1Key generates a secp256k1 private key from a seed offset.
// These keys are compatible with ConvertPeerIDToNodeID.
func createSecp256k1Key(offset int) crypto.PrivKey {
	privateKeyBytes := make([]byte, 32)
	for i := range 32 {
		privateKeyBytes[i] = byte(offset + i)
	}
	privKey, err := crypto.UnmarshalSecp256k1PrivateKey(privateKeyBytes)
	if err != nil {
		panic(err)
	}
	return privKey
}

// setupDASTest creates a test setup with the specified number of connected peers.
// custodyGroupCount is the custody count returned for all peers.
func setupDASTest(t *testing.T, peerCount int, custodyGroupCount uint64) *testDASSetup {
	params.SetupTestConfigCleanup(t)

	// Create main p2p service with secp256k1 key
	testP2P := p2ptest.NewTestP2P(t, libp2p.Identity(createSecp256k1Key(0)))
	mockP2P := &mockP2PForDAS{
		TestP2P:           testP2P,
		custodyGroupCount: custodyGroupCount,
	}
	cache := NewDASPeerCache(mockP2P)

	peers := make([]*p2ptest.TestP2P, peerCount)
	peerIDs := make([]peer.ID, peerCount)

	for i := range peerCount {
		// Use offset starting at 100 to avoid collision with main p2p service
		peers[i] = p2ptest.NewTestP2P(t, libp2p.Identity(createSecp256k1Key(100+i*50)))
		peers[i].Connect(testP2P)
		peerIDs[i] = peers[i].PeerID()
	}

	return &testDASSetup{
		t:          t,
		cache:      cache,
		p2pService: mockP2P,
		peers:      peers,
		peerIDs:    peerIDs,
	}
}

// getActualCustodyColumns returns the columns actually custodied by the test peers.
// This queries the same peerdas.Info logic used by the production code.
func (s *testDASSetup) getActualCustodyColumns() peerdas.ColumnIndices {
	result := peerdas.NewColumnIndices()
	custodyCount := s.p2pService.custodyGroupCount

	for _, pid := range s.peerIDs {
		nodeID, err := p2p.ConvertPeerIDToNodeID(pid)
		if err != nil {
			continue
		}
		info, _, err := peerdas.Info(nodeID, custodyCount)
		if err != nil {
			continue
		}
		for col := range info.CustodyColumns {
			result.Set(col)
		}
	}
	return result
}

func TestNewPicker(t *testing.T) {
	custodyReq := params.BeaconConfig().CustodyRequirement

	t.Run("valid peers with columns", func(t *testing.T) {
		setup := setupDASTest(t, 3, custodyReq)
		toCustody := setup.getActualCustodyColumns()
		require.NotEqual(t, 0, toCustody.Count(), "test peers should custody some columns")

		picker, err := setup.cache.NewPicker(setup.peerIDs, toCustody, time.Millisecond)
		require.NoError(t, err)
		require.NotNil(t, picker)
		require.Equal(t, 3, len(picker.scores))
	})

	t.Run("empty peer list", func(t *testing.T) {
		setup := setupDASTest(t, 0, custodyReq)
		toCustody := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1})

		picker, err := setup.cache.NewPicker(setup.peerIDs, toCustody, time.Millisecond)
		require.NoError(t, err)
		require.NotNil(t, picker)
		require.Equal(t, 0, len(picker.scores))
	})

	t.Run("empty custody columns", func(t *testing.T) {
		setup := setupDASTest(t, 2, custodyReq)
		toCustody := peerdas.NewColumnIndices()

		picker, err := setup.cache.NewPicker(setup.peerIDs, toCustody, time.Millisecond)
		require.NoError(t, err)
		require.NotNil(t, picker)
		// With empty toCustody, peers are still added to scores but have no custodied columns
		require.Equal(t, 2, len(picker.scores))
	})
}

func TestForColumns(t *testing.T) {
	custodyReq := params.BeaconConfig().CustodyRequirement

	t.Run("basic selection returns covering peer", func(t *testing.T) {
		setup := setupDASTest(t, 3, custodyReq)
		toCustody := setup.getActualCustodyColumns()
		require.NotEqual(t, 0, toCustody.Count(), "test peers must custody some columns")

		picker, err := setup.cache.NewPicker(setup.peerIDs, toCustody, time.Millisecond)
		require.NoError(t, err)

		// Request columns that we know peers custody
		needed := toCustody

		pid, cols, err := picker.ForColumns(needed, nil)
		require.NoError(t, err)
		require.NotEmpty(t, pid)
		require.NotEmpty(t, cols)

		// Verify the returned columns are a subset of what was needed
		for _, col := range cols {
			require.Equal(t, true, needed.Has(col), "returned column should be in needed set")
		}
	})

	t.Run("skip busy peers", func(t *testing.T) {
		setup := setupDASTest(t, 2, custodyReq)
		toCustody := setup.getActualCustodyColumns()
		require.NotEqual(t, 0, toCustody.Count(), "test peers must custody some columns")

		picker, err := setup.cache.NewPicker(setup.peerIDs, toCustody, time.Millisecond)
		require.NoError(t, err)

		// Mark first peer as busy
		busy := map[peer.ID]bool{setup.peerIDs[0]: true}

		pid, _, err := picker.ForColumns(toCustody, busy)
		require.NoError(t, err)
		// Should not return the busy peer
		require.NotEqual(t, setup.peerIDs[0], pid)
	})

	t.Run("rate limiting respects reqInterval", func(t *testing.T) {
		setup := setupDASTest(t, 1, custodyReq)
		toCustody := setup.getActualCustodyColumns()
		require.NotEqual(t, 0, toCustody.Count(), "test peers must custody some columns")

		// Use a long interval so the peer can't be picked twice
		picker, err := setup.cache.NewPicker(setup.peerIDs, toCustody, time.Hour)
		require.NoError(t, err)

		// First call should succeed
		pid, _, err := picker.ForColumns(toCustody, nil)
		require.NoError(t, err)
		require.NotEmpty(t, pid)

		// Second call should fail due to rate limiting
		_, _, err = picker.ForColumns(toCustody, nil)
		require.ErrorIs(t, err, ErrNoPeersCoverNeeded)
	})

	t.Run("no peers available returns error", func(t *testing.T) {
		setup := setupDASTest(t, 2, custodyReq)
		toCustody := setup.getActualCustodyColumns()
		require.NotEqual(t, 0, toCustody.Count(), "test peers must custody some columns")

		picker, err := setup.cache.NewPicker(setup.peerIDs, toCustody, time.Millisecond)
		require.NoError(t, err)

		// Mark all peers as busy
		busy := map[peer.ID]bool{
			setup.peerIDs[0]: true,
			setup.peerIDs[1]: true,
		}

		_, _, err = picker.ForColumns(toCustody, busy)
		require.ErrorIs(t, err, ErrNoPeersCoverNeeded)
	})

	t.Run("empty needed columns returns error", func(t *testing.T) {
		setup := setupDASTest(t, 2, custodyReq)
		toCustody := setup.getActualCustodyColumns()

		picker, err := setup.cache.NewPicker(setup.peerIDs, toCustody, time.Millisecond)
		require.NoError(t, err)

		// Request empty set of columns
		needed := peerdas.NewColumnIndices()
		_, _, err = picker.ForColumns(needed, nil)
		require.ErrorIs(t, err, ErrNoPeersCoverNeeded)
	})
}

func TestForBlocks(t *testing.T) {
	custodyReq := params.BeaconConfig().CustodyRequirement

	t.Run("returns available peer", func(t *testing.T) {
		setup := setupDASTest(t, 3, custodyReq)
		toCustody := setup.getActualCustodyColumns()

		picker, err := setup.cache.NewPicker(setup.peerIDs, toCustody, time.Millisecond)
		require.NoError(t, err)

		pid, err := picker.ForBlocks(nil)
		require.NoError(t, err)
		require.NotEmpty(t, pid)
	})

	t.Run("skips busy peers", func(t *testing.T) {
		setup := setupDASTest(t, 3, custodyReq)
		toCustody := setup.getActualCustodyColumns()

		picker, err := setup.cache.NewPicker(setup.peerIDs, toCustody, time.Millisecond)
		require.NoError(t, err)

		// Mark first two peers as busy
		busy := map[peer.ID]bool{
			setup.peerIDs[0]: true,
			setup.peerIDs[1]: true,
		}

		pid, err := picker.ForBlocks(busy)
		require.NoError(t, err)
		require.NotEmpty(t, pid)
		// Verify returned peer is not busy
		require.Equal(t, false, busy[pid], "returned peer should not be busy")
	})

	t.Run("all peers busy returns error", func(t *testing.T) {
		setup := setupDASTest(t, 2, custodyReq)
		toCustody := setup.getActualCustodyColumns()

		picker, err := setup.cache.NewPicker(setup.peerIDs, toCustody, time.Millisecond)
		require.NoError(t, err)

		// Mark all peers as busy
		busy := map[peer.ID]bool{
			setup.peerIDs[0]: true,
			setup.peerIDs[1]: true,
		}

		_, err = picker.ForBlocks(busy)
		require.ErrorIs(t, err, ErrNoPeersAvailable)
	})

	t.Run("no peers returns error", func(t *testing.T) {
		setup := setupDASTest(t, 0, custodyReq)
		toCustody := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2, 3})

		picker, err := setup.cache.NewPicker(setup.peerIDs, toCustody, time.Millisecond)
		require.NoError(t, err)

		_, err = picker.ForBlocks(nil)
		require.ErrorIs(t, err, ErrNoPeersAvailable)
	})
}
