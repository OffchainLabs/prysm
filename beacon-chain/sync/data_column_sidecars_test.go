package sync

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	testp2p "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	p2ptypes "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	leakybucket "github.com/OffchainLabs/prysm/v6/container/leaky-bucket"
	"github.com/OffchainLabs/prysm/v6/crypto/rand"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestBuildByRootRequest(t *testing.T) {
	root1 := [fieldparams.RootLength]byte{1}
	root2 := [fieldparams.RootLength]byte{2}

	input := map[[fieldparams.RootLength]byte]map[uint64]bool{
		root1: {1: true, 2: true},
		root2: {3: true},
	}

	expected := p2ptypes.DataColumnsByRootIdentifiers{
		{
			BlockRoot: root1[:],
			Columns:   []uint64{1, 2},
		},
		{
			BlockRoot: root2[:],
			Columns:   []uint64{3},
		},
	}

	actual := buildByRootRequest(input)
	require.DeepEqual(t, expected, actual)
}

func TestVerifyDataColumnSidecarsByPeer(t *testing.T) {
	err := kzg.Start()
	require.NoError(t, err)

	t.Run("nominal", func(t *testing.T) {
		const (
			start, stop = 0, 15
			blobCount   = 1
		)

		p2p := testp2p.NewTestP2P(t)

		// Setup test data and expectations
		_, roDataColumnSidecars, expected := util.GenerateTestFuluBlockWithSidecars(t, blobCount)

		roDataColumnsByPeer := map[peer.ID][]blocks.RODataColumn{
			"peer1": roDataColumnSidecars[start:5],
			"peer2": roDataColumnSidecars[5:9],
			"peer3": roDataColumnSidecars[9:stop],
		}
		gs := startup.NewClockSynchronizer()
		err := gs.SetClock(startup.NewClock(time.Unix(4113849600, 0), [fieldparams.RootLength]byte{}))
		require.NoError(t, err)

		waiter := verification.NewInitializerWaiter(gs, nil, nil)
		initializer, err := waiter.WaitForInitializer(t.Context())
		require.NoError(t, err)

		newDataColumnsVerifier := newDataColumnsVerifierFromInitializer(initializer)
		actual, err := verifyDataColumnSidecarsByPeer(p2p, newDataColumnsVerifier, roDataColumnsByPeer)
		require.NoError(t, err)

		require.Equal(t, stop-start, len(actual))

		for i := range actual {
			actualSidecar := actual[i]
			index := actualSidecar.Index
			expectedSidecar := expected[index]
			require.DeepEqual(t, expectedSidecar, actualSidecar)
		}
	})

	t.Run("one rogue peer", func(t *testing.T) {
		const (
			start, middle, stop = 0, 5, 15
			blobCount           = 1
		)

		p2p := testp2p.NewTestP2P(t)

		// Setup test data and expectations
		_, roDataColumnSidecars, expected := util.GenerateTestFuluBlockWithSidecars(t, blobCount)

		// Modify one sidecar to ensure proof verification fails.
		if roDataColumnSidecars[middle].KzgProofs[0][0] == 0 {
			roDataColumnSidecars[middle].KzgProofs[0][0]++
		} else {
			roDataColumnSidecars[middle].KzgProofs[0][0]--
		}

		roDataColumnsByPeer := map[peer.ID][]blocks.RODataColumn{
			"peer1": roDataColumnSidecars[start:middle],
			"peer2": roDataColumnSidecars[5:middle],
			"peer3": roDataColumnSidecars[middle:stop],
		}
		gs := startup.NewClockSynchronizer()
		err := gs.SetClock(startup.NewClock(time.Unix(4113849600, 0), [fieldparams.RootLength]byte{}))
		require.NoError(t, err)

		waiter := verification.NewInitializerWaiter(gs, nil, nil)
		initializer, err := waiter.WaitForInitializer(t.Context())
		require.NoError(t, err)

		newDataColumnsVerifier := newDataColumnsVerifierFromInitializer(initializer)
		actual, err := verifyDataColumnSidecarsByPeer(p2p, newDataColumnsVerifier, roDataColumnsByPeer)
		require.NoError(t, err)

		require.Equal(t, middle-start, len(actual))

		for i := range actual {
			actualSidecar := actual[i]
			index := actualSidecar.Index
			expectedSidecar := expected[index]
			require.DeepEqual(t, expectedSidecar, actualSidecar)
		}
	})
}

func TestComputeIndicesByRootByPeer(t *testing.T) {
	peerIdStrs := []string{
		"16Uiu2HAm3k5Npu6EaYWxiEvzsdLseEkjVyoVhvbxWEuyqdBgBBbq", // Custodies 89, 94, 97 & 122
		"16Uiu2HAmTwQPAwzTr6hTgBmKNecCfH6kP3Kbzxj36ZRyyQ46L6gf", // Custodies 1, 11, 37 & 86
		"16Uiu2HAmMDB5uUePTpN7737m78ehePfWPtBL9qMGdH8kCygjzNA8", // Custodies 2, 37, 38 & 68
		"16Uiu2HAmTAE5Vxf7Pgfk7eWpmCvVJdSba4C9xg4xkYuuvnVbgfFx", // Custodies 10, 29, 36 & 108
	}

	headSlotByPeer := map[string]primitives.Slot{
		"16Uiu2HAm3k5Npu6EaYWxiEvzsdLseEkjVyoVhvbxWEuyqdBgBBbq": 89,
		"16Uiu2HAmTwQPAwzTr6hTgBmKNecCfH6kP3Kbzxj36ZRyyQ46L6gf": 10,
		"16Uiu2HAmMDB5uUePTpN7737m78ehePfWPtBL9qMGdH8kCygjzNA8": 12,
		"16Uiu2HAmTAE5Vxf7Pgfk7eWpmCvVJdSba4C9xg4xkYuuvnVbgfFx": 9,
	}

	p2p := testp2p.NewTestP2P(t)
	peers := p2p.Peers()

	peerIDs := make([]peer.ID, 0, len(peerIdStrs))
	for _, peerIdStr := range peerIdStrs {
		peerID, err := peer.Decode(peerIdStr)
		require.NoError(t, err)

		peers.SetChainState(peerID, &ethpb.StatusV2{
			HeadSlot: headSlotByPeer[peerIdStr],
		})

		peerIDs = append(peerIDs, peerID)
	}

	slotByBlockRoot := map[[fieldparams.RootLength]byte]primitives.Slot{
		[fieldparams.RootLength]byte{1}: 8,
		[fieldparams.RootLength]byte{2}: 10,
		[fieldparams.RootLength]byte{3}: 9,
		[fieldparams.RootLength]byte{4}: 50,
	}

	indicesByBlockRoot := map[[fieldparams.RootLength]byte]map[uint64]bool{
		[fieldparams.RootLength]byte{1}: {3: true, 4: true, 5: true},
		[fieldparams.RootLength]byte{2}: {1: true, 10: true, 37: true, 80: true},
		[fieldparams.RootLength]byte{3}: {10: true, 38: true, 39: true, 40: true},
		[fieldparams.RootLength]byte{4}: {89: true, 108: true, 122: true},
	}

	expected := map[peer.ID]map[[fieldparams.RootLength]byte]map[uint64]bool{
		peerIDs[0]: {
			[fieldparams.RootLength]byte{4}: {89: true, 122: true},
		},
		peerIDs[1]: {
			[fieldparams.RootLength]byte{2}: {1: true, 37: true},
		},
		peerIDs[2]: {
			[fieldparams.RootLength]byte{2}: {37: true},
			[fieldparams.RootLength]byte{3}: {38: true},
		},
		peerIDs[3]: {
			[fieldparams.RootLength]byte{3}: {10: true},
		},
	}

	peerIDsMap := make(map[peer.ID]bool, len(peerIDs))
	for _, id := range peerIDs {
		peerIDsMap[id] = true
	}

	actual, err := computeIndicesByRootByPeer(p2p, slotByBlockRoot, indicesByBlockRoot, peerIDsMap)
	require.NoError(t, err)
	require.Equal(t, len(expected), len(actual))

	for peer, indicesByRoot := range expected {
		require.Equal(t, len(indicesByRoot), len(actual[peer]))
		for root, indices := range indicesByRoot {
			require.Equal(t, len(indices), len(actual[peer][root]))
			for index := range indices {
				require.Equal(t, actual[peer][root][index], true)
			}
		}
	}
}

func TestRandomPeer(t *testing.T) {
	randomSource := rand.NewGenerator()

	t.Run("no peers", func(t *testing.T) {
		pid, err := randomPeer(t.Context(), randomSource, leakybucket.NewCollector(4, 8, time.Second, false /* deleteEmptyBuckets */), 1, nil)
		require.NotNil(t, err)
		require.Equal(t, peer.ID(""), pid)
	})

	t.Run("context cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		indicesByRootByPeer := map[peer.ID]map[[fieldparams.RootLength]byte]map[uint64]bool{peer.ID("peer1"): {}}
		pid, err := randomPeer(ctx, randomSource, leakybucket.NewCollector(4, 8, time.Second, false /* deleteEmptyBuckets */), 1, indicesByRootByPeer)
		require.NotNil(t, err)
		require.Equal(t, peer.ID(""), pid)
	})

	t.Run("nominal", func(t *testing.T) {
		const count = 1
		collector := leakybucket.NewCollector(4, 8, time.Second, false /* deleteEmptyBuckets */)
		peer1, peer2, peer3 := peer.ID("peer1"), peer.ID("peer2"), peer.ID("peer3")

		indicesByRootByPeer := map[peer.ID]map[[fieldparams.RootLength]byte]map[uint64]bool{
			peer1: {},
			peer2: {},
			peer3: {},
		}

		pid, err := randomPeer(t.Context(), randomSource, collector, count, indicesByRootByPeer)
		require.NoError(t, err)
		require.Equal(t, true, map[peer.ID]bool{peer1: true, peer2: true, peer3: true}[pid])
	})
}

func TestCopyIndicesByRootByPeer(t *testing.T) {
	original := map[peer.ID]map[[fieldparams.RootLength]byte]map[uint64]bool{
		peer.ID("peer1"): {
			[fieldparams.RootLength]byte{1}: {1: true, 3: true},
			[fieldparams.RootLength]byte{2}: {2: true},
		},
		peer.ID("peer2"): {
			[fieldparams.RootLength]byte{1}: {1: true},
		},
	}

	copied := copyIndicesByRootByPeer(original)

	require.Equal(t, len(original), len(copied))
	for peer, indicesByRoot := range original {
		require.Equal(t, len(indicesByRoot), len(copied[peer]))
		for root, indices := range indicesByRoot {
			require.Equal(t, len(indices), len(copied[peer][root]))
			for index := range indices {
				require.Equal(t, copied[peer][root][index], true)
			}
		}
	}
}

func TestCompareIndices(t *testing.T) {
	left := map[uint64]bool{3: true, 5: true, 7: true}
	right := map[uint64]bool{5: true}
	require.Equal(t, false, compareIndices(left, right))

	left = map[uint64]bool{3: true, 5: true, 7: true}
	right = map[uint64]bool{3: true, 6: true, 7: true}
	require.Equal(t, false, compareIndices(left, right))

	left = map[uint64]bool{3: true, 5: true, 7: true}
	right = map[uint64]bool{5: true, 7: true, 3: true}
	require.Equal(t, true, compareIndices(left, right))
}

func TestSlortedSliceFromMap(t *testing.T) {
	input := map[uint64]bool{54: true, 23: true, 35: true}
	expected := []uint64{23, 35, 54}
	actual := sortedSliceFromMap(input)
	require.DeepEqual(t, expected, actual)
}

func TestComputeSlotByBlockRoot(t *testing.T) {
	const (
		count      = 3
		multiplier = 10
	)

	roBlocks := make([]blocks.ROBlock, 0, count)
	for i := range count {
		signedBlock := util.NewBeaconBlock()
		signedBlock.Block.Slot = primitives.Slot(i).Mul(multiplier)
		roSignedBlock, err := blocks.NewSignedBeaconBlock(signedBlock)
		require.NoError(t, err)

		roBlock, err := blocks.NewROBlockWithRoot(roSignedBlock, [fieldparams.RootLength]byte{byte(i)})
		require.NoError(t, err)

		roBlocks = append(roBlocks, roBlock)
	}

	expected := map[[fieldparams.RootLength]byte]primitives.Slot{
		[fieldparams.RootLength]byte{0}: primitives.Slot(0),
		[fieldparams.RootLength]byte{1}: primitives.Slot(10),
		[fieldparams.RootLength]byte{2}: primitives.Slot(20),
	}

	actual := computeSlotByBlockRoot(roBlocks)

	require.Equal(t, len(expected), len(actual))
	for k, v := range expected {
		require.Equal(t, v, actual[k])
	}
}

func TestComputeTotalCount(t *testing.T) {
	input := map[[fieldparams.RootLength]byte]map[uint64]bool{
		[fieldparams.RootLength]byte{1}: {1: true, 3: true},
		[fieldparams.RootLength]byte{2}: {2: true},
	}

	const expected = 3
	actual := computeTotalCount(input)
	require.Equal(t, expected, actual)
}
