package sync

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers"
	testp2p "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	p2ptypes "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/consensus-types/wrapper"
	leakybucket "github.com/OffchainLabs/prysm/v6/container/leaky-bucket"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestFetchDataColumnSidecars(t *testing.T) {
	numberOfColumns := params.BeaconConfig().NumberOfColumns
	// Slot 1: All needed sidecars are available in storage ==> Retrieval from storage only.
	// Slot 2: No commitment ==> Nothing to do.
	// Slot 3: Some sidecars are in the storage, other have to be retrieved from peers ==> Retrieval from storage and peers.
	// Slot 4: Some sidecars are in the storage, other have to be retrieved from peers but peers do not deliver all requested sidecars ==> Retrieval from storage and peers then reconstruction.
	// Slot 5: Some sidecars are in the storage, other have to be retrieved from peers ==> Retrieval from storage and peers but peers do not respond all needed on first attempt and respond needed sidecars on second attempt ==> Retrieval from storage and peers.
	// Slot 6: Some sidecars are in the storage, other have to be retrieved from peers ==> Retrieval from storage and peers but peers do not respond all needed on first attempt and respond not needed sidecars on second attempt ==> Retrieval from storage and peers then reconstruction.
	// Slot 7: Some sidecars are in the storage, other have to be retrieved from peers but peers do not send anything ==> Still missing.

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 0
	cfg.BlobSchedule = []params.BlobScheduleEntry{{Epoch: 0, MaxBlobsPerBlock: 10}}
	params.OverrideBeaconConfig(cfg)

	// Start the trusted setup.
	err := kzg.Start()
	require.NoError(t, err)

	storage := filesystem.NewEphemeralDataColumnStorage(t)

	ctxMap, err := ContextByteVersionsForValRoot(params.BeaconConfig().GenesisValidatorsRoot)
	require.NoError(t, err)

	const blobCount = 3
	indices := map[uint64]bool{31: true, 81: true, 106: true}

	// Block 1
	block1, _, verifiedSidecars1 := util.GenerateTestFuluBlockWithSidecars(t, blobCount, util.WithSlot(1))
	root1 := block1.Root()

	toStore1 := make([]blocks.VerifiedRODataColumn, 0, len(indices))
	for index := range indices {
		sidecar := verifiedSidecars1[index]
		toStore1 = append(toStore1, sidecar)
	}

	err = storage.Save(toStore1)
	require.NoError(t, err)

	// Block 2
	block2, _, _ := util.GenerateTestFuluBlockWithSidecars(t, 0, util.WithSlot(2))

	// Block 3
	block3, _, verifiedSidecars3 := util.GenerateTestFuluBlockWithSidecars(t, blobCount, util.WithSlot(3))
	root3 := block3.Root()
	toStore3 := []blocks.VerifiedRODataColumn{verifiedSidecars3[106]}

	err = storage.Save(toStore3)
	require.NoError(t, err)

	// Block 4
	minimumColumnsCountToReconstruct := peerdas.MinimumColumnCountToReconstruct()
	block4, _, verifiedSidecars4 := util.GenerateTestFuluBlockWithSidecars(t, blobCount, util.WithSlot(4))
	root4 := block4.Root()

	toStoreCount := minimumColumnsCountToReconstruct - 1
	toStore4 := make([]blocks.VerifiedRODataColumn, 0, toStoreCount)

	for i := uint64(0); uint64(len(toStore4)) < toStoreCount; i++ {
		sidecar := verifiedSidecars4[minimumColumnsCountToReconstruct+i]
		if sidecar.Index == 81 {
			continue
		}

		toStore4 = append(toStore4, sidecar)
	}

	err = storage.Save(toStore4)
	require.NoError(t, err)

	// Block 5
	block5, _, verifiedSidecars5 := util.GenerateTestFuluBlockWithSidecars(t, blobCount, util.WithSlot(5))
	root5 := block5.Root()
	toStore5 := []blocks.VerifiedRODataColumn{verifiedSidecars5[106]}

	err = storage.Save(toStore5)
	require.NoError(t, err)

	// Block 6
	block6, _, verifiedSidecars6 := util.GenerateTestFuluBlockWithSidecars(t, blobCount, util.WithSlot(6))
	root6 := block6.Root()
	toStore6 := []blocks.VerifiedRODataColumn{verifiedSidecars6[106]}

	err = storage.Save(toStore6)
	require.NoError(t, err)

	// Block 7
	block7, _, verifiedSidecars7 := util.GenerateTestFuluBlockWithSidecars(t, blobCount, util.WithSlot(7))
	root7 := block7.Root()
	toStore7 := []blocks.VerifiedRODataColumn{verifiedSidecars7[106]}

	err = storage.Save(toStore7)
	require.NoError(t, err)

	// Peers
	byRangeProtocol := fmt.Sprintf("%s/ssz_snappy", p2p.RPCDataColumnSidecarsByRangeTopicV1)
	byRootProtocol := fmt.Sprintf("%s/ssz_snappy", p2p.RPCDataColumnSidecarsByRootTopicV1)

	privateKeyBytes := [32]byte{1}
	privateKey, err := crypto.UnmarshalSecp256k1PrivateKey(privateKeyBytes[:])
	require.NoError(t, err)

	p2p, other := testp2p.NewTestP2P(t), testp2p.NewTestP2P(t, libp2p.Identity(privateKey))
	p2p.Peers().SetConnectionState(other.PeerID(), peers.Connected)
	p2p.Connect(other)

	p2p.Peers().SetChainState(other.PeerID(), &ethpb.StatusV2{
		HeadSlot: 8,
	})

	p2p.Peers().SetMetadata(other.PeerID(), wrapper.WrappedMetadataV2(&ethpb.MetaDataV2{
		CustodyGroupCount: 128,
	}))

	clock := startup.NewClock(time.Now(), [fieldparams.RootLength]byte{})

	gs := startup.NewClockSynchronizer()
	err = gs.SetClock(startup.NewClock(time.Unix(4113849600, 0), [fieldparams.RootLength]byte{}))
	require.NoError(t, err)

	waiter := verification.NewInitializerWaiter(gs, nil, nil)
	initializer, err := waiter.WaitForInitializer(t.Context())
	require.NoError(t, err)

	newDataColumnsVerifier := newDataColumnsVerifierFromInitializer(initializer)

	other.SetStreamHandler(byRangeProtocol, func(stream network.Stream) {
		expectedRequest := &ethpb.DataColumnSidecarsByRangeRequest{
			StartSlot: 3,
			Count:     5,
			Columns:   []uint64{31, 81},
		}

		actualRequest := new(ethpb.DataColumnSidecarsByRangeRequest)
		err := other.Encoding().DecodeWithMaxLength(stream, actualRequest)
		assert.NoError(t, err)
		assert.DeepEqual(t, expectedRequest, actualRequest)

		err = WriteDataColumnSidecarChunk(stream, clock, other.Encoding(), verifiedSidecars3[31].DataColumnSidecar)
		assert.NoError(t, err)

		err = WriteDataColumnSidecarChunk(stream, clock, other.Encoding(), verifiedSidecars3[81].DataColumnSidecar)
		assert.NoError(t, err)

		err = WriteDataColumnSidecarChunk(stream, clock, other.Encoding(), verifiedSidecars4[81].DataColumnSidecar)
		assert.NoError(t, err)

		err = WriteDataColumnSidecarChunk(stream, clock, other.Encoding(), verifiedSidecars5[31].DataColumnSidecar)
		assert.NoError(t, err)

		err = WriteDataColumnSidecarChunk(stream, clock, other.Encoding(), verifiedSidecars6[31].DataColumnSidecar)
		assert.NoError(t, err)

		err = stream.CloseWrite()
		assert.NoError(t, err)
	})

	other.SetStreamHandler(byRootProtocol, func(stream network.Stream) {
		allBut31And81And106 := make([]uint64, 0, numberOfColumns-3)
		allBut31And106 := make([]uint64, 0, numberOfColumns-2)
		allBut106 := make([]uint64, 0, numberOfColumns-1)
		for i := range numberOfColumns {
			if !map[uint64]bool{31: true, 81: true, 106: true}[i] {
				allBut31And81And106 = append(allBut31And81And106, i)
			}
			if !map[uint64]bool{31: true, 106: true}[i] {
				allBut31And106 = append(allBut31And106, i)
			}

			if i != 106 {
				allBut106 = append(allBut106, i)
			}
		}

		expectedRequest := &p2ptypes.DataColumnsByRootIdentifiers{
			{
				BlockRoot: root7[:],
				Columns:   allBut106,
			},
			{
				BlockRoot: root5[:],
				Columns:   allBut31And106,
			},
			{
				BlockRoot: root6[:],
				Columns:   allBut31And106,
			},
		}

		actualRequest := new(p2ptypes.DataColumnsByRootIdentifiers)
		err := other.Encoding().DecodeWithMaxLength(stream, actualRequest)
		assert.NoError(t, err)
		assert.DeepEqual(t, expectedRequest, actualRequest)

		err = WriteDataColumnSidecarChunk(stream, clock, other.Encoding(), verifiedSidecars5[81].DataColumnSidecar)
		assert.NoError(t, err)

		for _, index := range allBut31And81And106 {
			err = WriteDataColumnSidecarChunk(stream, clock, other.Encoding(), verifiedSidecars6[index].DataColumnSidecar)
			assert.NoError(t, err)
		}

		err = stream.CloseWrite()
		assert.NoError(t, err)
	})

	params := DataColumnSidecarsParams{
		Ctx:         t.Context(),
		Tor:         clock,
		P2P:         p2p,
		RateLimiter: leakybucket.NewCollector(1., 10, time.Second, false /* deleteEmptyBuckets */),
		CtxMap:      ctxMap,
		Storage:     storage,
		NewVerifier: newDataColumnsVerifier,
	}

	expectedResult := map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn{
		root1: {verifiedSidecars1[31], verifiedSidecars1[81], verifiedSidecars1[106]},
		// no root2 (no commitments in this block)
		root3: {verifiedSidecars3[106], verifiedSidecars3[31], verifiedSidecars3[81]},
		root4: {verifiedSidecars4[31], verifiedSidecars4[81], verifiedSidecars4[106]},
		root5: {verifiedSidecars5[106], verifiedSidecars5[31], verifiedSidecars5[81]},
		root6: {verifiedSidecars6[31], verifiedSidecars6[81], verifiedSidecars6[106]},
		root7: {verifiedSidecars7[106]},
	}

	expectedMissingIndicesBYRoots := map[[fieldparams.RootLength]byte]map[uint64]bool{
		root7: {31: true, 81: true},
	}

	blocks := []blocks.ROBlock{block1, block2, block3, block4, block5, block6, block7}
	actualResult, actualMissingRoots, err := FetchDataColumnSidecars(params, blocks, indices)
	require.NoError(t, err)

	require.Equal(t, len(expectedResult), len(actualResult))
	for root := range expectedResult {
		require.Equal(t, len(expectedResult[root]), len(actualResult[root]))
		for i := range expectedResult[root] {
			require.DeepSSZEqual(t, expectedResult[root][i], actualResult[root][i])
		}
	}

	require.Equal(t, len(expectedMissingIndicesBYRoots), len(actualMissingRoots))
	for root, expectedMissingIndices := range expectedMissingIndicesBYRoots {
		actualMissingIndices := actualMissingRoots[root]
		require.Equal(t, len(expectedMissingIndices), len(actualMissingIndices))
		for index := range expectedMissingIndices {
			require.Equal(t, true, actualMissingIndices[index])
		}
	}
}

func TestSelectPeers(t *testing.T) {
	const (
		count = 3
		seed  = 42
	)

	params := DataColumnSidecarsParams{
		Ctx:         t.Context(),
		RateLimiter: leakybucket.NewCollector(1., 10, time.Second, false /* deleteEmptyBuckets */),
	}

	randomSource := rand.New(rand.NewSource(seed))

	indicesByRootByPeer := map[peer.ID]map[[fieldparams.RootLength]byte]map[uint64]bool{
		"peer1": {
			{1}: {12: true, 13: true},
			{2}: {13: true, 14: true, 15: true},
			{3}: {14: true, 15: true},
		},
		"peer2": {
			{1}: {13: true, 14: true},
			{2}: {13: true, 14: true, 15: true},
			{3}: {14: true, 16: true},
		},
	}

	expected := map[peer.ID]map[[fieldparams.RootLength]byte]map[uint64]bool{
		"peer1": {
			{1}: {12: true},
			{3}: {15: true},
		},
		"peer2": {
			{1}: {13: true, 14: true},
			{2}: {13: true, 14: true, 15: true},
			{3}: {14: true, 16: true},
		},
	}

	actual, err := selectPeers(params, randomSource, count, indicesByRootByPeer)

	require.NoError(t, err)
	require.Equal(t, len(expected), len(actual))
	for peerID := range expected {
		require.Equal(t, len(expected[peerID]), len(actual[peerID]))
		for root := range expected[peerID] {
			require.Equal(t, len(expected[peerID][root]), len(actual[peerID][root]))
			for indices := range expected[peerID][root] {
				require.Equal(t, expected[peerID][root][indices], actual[peerID][root][indices])
			}
		}
	}
}

func TestUpdateResults(t *testing.T) {
	_, verifiedSidecars := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{
		{Slot: 1, Index: 12, Column: [][]byte{{1}, {2}, {3}}},
		{Slot: 1, Index: 13, Column: [][]byte{{1}, {2}, {3}}},
		{Slot: 2, Index: 13, Column: [][]byte{{1}, {2}, {3}}},
		{Slot: 2, Index: 14, Column: [][]byte{{1}, {2}, {3}}},
	})

	missingIndicesByRoot := map[[fieldparams.RootLength]byte]map[uint64]bool{
		verifiedSidecars[0].BlockRoot(): {12: true, 13: true},
		verifiedSidecars[2].BlockRoot(): {13: true, 14: true, 15: true},
	}

	expectedMissingIndicesByRoot := map[[fieldparams.RootLength]byte]map[uint64]bool{
		verifiedSidecars[2].BlockRoot(): {15: true},
	}

	expectedVerifiedSidecarsByRoot := map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn{
		verifiedSidecars[0].BlockRoot(): {verifiedSidecars[0], verifiedSidecars[1]},
		verifiedSidecars[2].BlockRoot(): {verifiedSidecars[2], verifiedSidecars[3]},
	}

	actualVerifiedSidecarsByRoot := updateResults(verifiedSidecars, missingIndicesByRoot)
	require.DeepEqual(t, expectedMissingIndicesByRoot, missingIndicesByRoot)
	require.DeepEqual(t, expectedVerifiedSidecarsByRoot, actualVerifiedSidecarsByRoot)
}

func TestFetchDataColumnSidecarsFromPeers(t *testing.T) {
	const count = 4

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	clock := startup.NewClock(time.Now(), [fieldparams.RootLength]byte{})
	ctxMap, err := ContextByteVersionsForValRoot(params.BeaconConfig().GenesisValidatorsRoot)
	require.NoError(t, err)

	kzgCommitmentsInclusionProof := make([][]byte, 0, count)
	for range count {
		kzgCommitmentsInclusionProof = append(kzgCommitmentsInclusionProof, make([]byte, 32))
	}

	expectedResponseSidecarPb := &ethpb.DataColumnSidecar{
		Index: 2,
		SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
			Header: &ethpb.BeaconBlockHeader{
				Slot:       1,
				ParentRoot: make([]byte, fieldparams.RootLength),
				StateRoot:  make([]byte, fieldparams.RootLength),
				BodyRoot:   make([]byte, fieldparams.RootLength),
			},
			Signature: make([]byte, fieldparams.BLSSignatureLength),
		},
		KzgCommitmentsInclusionProof: kzgCommitmentsInclusionProof,
	}

	expectedResponseSidecar, err := blocks.NewRODataColumn(expectedResponseSidecarPb)
	require.NoError(t, err)

	slotByRoot := map[[fieldparams.RootLength]byte]primitives.Slot{
		{1}: 1,
		{3}: 3,
		{4}: 4,
		{7}: 7,
	}

	slotsWithCommitments := map[primitives.Slot]bool{
		1: true,
		3: true,
		4: true,
		7: true,
	}

	expectedRequest := &ethpb.DataColumnSidecarsByRangeRequest{
		StartSlot: 1,
		Count:     7,
		Columns:   []uint64{1, 2},
	}

	protocol := fmt.Sprintf("%s/ssz_snappy", p2p.RPCDataColumnSidecarsByRangeTopicV1)
	p2p, other := testp2p.NewTestP2P(t), testp2p.NewTestP2P(t)
	p2p.Connect(other)

	other.SetStreamHandler(protocol, func(stream network.Stream) {
		receivedRequest := new(ethpb.DataColumnSidecarsByRangeRequest)
		err := other.Encoding().DecodeWithMaxLength(stream, receivedRequest)
		assert.NoError(t, err)
		assert.DeepEqual(t, expectedRequest, receivedRequest)

		err = WriteDataColumnSidecarChunk(stream, clock, other.Encoding(), expectedResponseSidecarPb)
		assert.NoError(t, err)

		err = stream.CloseWrite()
		assert.NoError(t, err)
	})

	indicesByRootByPeer := map[peer.ID]map[[fieldparams.RootLength]byte]map[uint64]bool{
		other.PeerID(): {
			{1}: {1: true, 2: true},
			{3}: {1: true, 2: true},
			{4}: {1: true, 2: true},
			{7}: {1: true, 2: true},
		},
	}

	params := DataColumnSidecarsParams{
		Ctx:         t.Context(),
		Tor:         clock,
		P2P:         p2p,
		CtxMap:      ctxMap,
		RateLimiter: leakybucket.NewCollector(1., 1, time.Second, false /* deleteEmptyBuckets */),
	}

	expectedResponse := map[peer.ID][]blocks.RODataColumn{
		other.PeerID(): {expectedResponseSidecar},
	}

	actualResponse := fetchDataColumnSidecarsFromPeers(params, slotByRoot, slotsWithCommitments, indicesByRootByPeer)
	require.Equal(t, len(expectedResponse), len(actualResponse))

	for peerID := range expectedResponse {
		require.DeepSSZEqual(t, expectedResponse[peerID], actualResponse[peerID])
	}
}

func TestSendDataColumnSidecarsRequest(t *testing.T) {
	const count = 4

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	kzgCommitmentsInclusionProof := make([][]byte, 0, count)
	for range count {
		kzgCommitmentsInclusionProof = append(kzgCommitmentsInclusionProof, make([]byte, 32))
	}

	expectedResponsePb := &ethpb.DataColumnSidecar{
		Index: 2,
		SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
			Header: &ethpb.BeaconBlockHeader{
				Slot:       1,
				ParentRoot: make([]byte, fieldparams.RootLength),
				StateRoot:  make([]byte, fieldparams.RootLength),
				BodyRoot:   make([]byte, fieldparams.RootLength),
			},
			Signature: make([]byte, fieldparams.BLSSignatureLength),
		},
		KzgCommitmentsInclusionProof: kzgCommitmentsInclusionProof,
	}

	expectedResponse, err := blocks.NewRODataColumn(expectedResponsePb)
	require.NoError(t, err)

	clock := startup.NewClock(time.Now(), params.BeaconConfig().GenesisValidatorsRoot)
	ctxMap, err := ContextByteVersionsForValRoot(params.BeaconConfig().GenesisValidatorsRoot)
	require.NoError(t, err)

	t.Run("contiguous", func(t *testing.T) {
		indicesByRoot := map[[fieldparams.RootLength]byte]map[uint64]bool{
			{1}: {1: true, 2: true},
			{3}: {1: true, 2: true},
			{4}: {1: true, 2: true},
			{7}: {1: true, 2: true},
		}

		slotByRoot := map[[fieldparams.RootLength]byte]primitives.Slot{
			{1}: 1,
			{3}: 3,
			{4}: 4,
			{7}: 7,
		}

		slotsWithCommitments := map[primitives.Slot]bool{
			1: true,
			3: true,
			4: true,
			7: true,
		}

		expectedRequest := &ethpb.DataColumnSidecarsByRangeRequest{
			StartSlot: 1,
			Count:     7,
			Columns:   []uint64{1, 2},
		}

		protocol := fmt.Sprintf("%s/ssz_snappy", p2p.RPCDataColumnSidecarsByRangeTopicV1)
		p2p, other := testp2p.NewTestP2P(t), testp2p.NewTestP2P(t)
		p2p.Connect(other)

		other.SetStreamHandler(protocol, func(stream network.Stream) {
			receivedRequest := new(ethpb.DataColumnSidecarsByRangeRequest)
			err := other.Encoding().DecodeWithMaxLength(stream, receivedRequest)
			assert.NoError(t, err)
			assert.DeepEqual(t, expectedRequest, receivedRequest)

			err = WriteDataColumnSidecarChunk(stream, clock, other.Encoding(), expectedResponsePb)
			assert.NoError(t, err)

			err = stream.CloseWrite()
			assert.NoError(t, err)
		})

		params := DataColumnSidecarsParams{
			Ctx:         t.Context(),
			Tor:         clock,
			P2P:         p2p,
			CtxMap:      ctxMap,
			RateLimiter: leakybucket.NewCollector(1., 1, time.Second, false /* deleteEmptyBuckets */),
		}

		actualResponse, err := sendDataColumnSidecarsRequest(params, slotByRoot, slotsWithCommitments, other.PeerID(), indicesByRoot)
		require.NoError(t, err)
		require.DeepEqual(t, expectedResponse, actualResponse[0])
	})

	t.Run("non contiguous", func(t *testing.T) {
		indicesByRoot := map[[fieldparams.RootLength]byte]map[uint64]bool{
			expectedResponse.BlockRoot(): {1: true, 2: true},
			{4}:                          {1: true, 2: true},
			{7}:                          {1: true, 2: true},
		}

		slotByRoot := map[[fieldparams.RootLength]byte]primitives.Slot{
			expectedResponse.BlockRoot(): 1,
			{4}:                          4,
			{7}:                          7,
		}

		slotsWithCommitments := map[primitives.Slot]bool{
			1: true,
			3: true,
			4: true,
			7: true,
		}

		roots := [...][fieldparams.RootLength]byte{expectedResponse.BlockRoot(), {4}, {7}}

		expectedRequest := &p2ptypes.DataColumnsByRootIdentifiers{
			{
				BlockRoot: roots[1][:],
				Columns:   []uint64{1, 2},
			},
			{
				BlockRoot: roots[2][:],
				Columns:   []uint64{1, 2},
			},
			{
				BlockRoot: roots[0][:],
				Columns:   []uint64{1, 2},
			},
		}

		protocol := fmt.Sprintf("%s/ssz_snappy", p2p.RPCDataColumnSidecarsByRootTopicV1)
		p2p, other := testp2p.NewTestP2P(t), testp2p.NewTestP2P(t)
		p2p.Connect(other)

		other.SetStreamHandler(protocol, func(stream network.Stream) {
			receivedRequest := new(p2ptypes.DataColumnsByRootIdentifiers)
			err := other.Encoding().DecodeWithMaxLength(stream, receivedRequest)
			assert.NoError(t, err)
			assert.DeepSSZEqual(t, *expectedRequest, *receivedRequest)

			err = WriteDataColumnSidecarChunk(stream, clock, other.Encoding(), expectedResponsePb)
			assert.NoError(t, err)

			err = stream.CloseWrite()
			assert.NoError(t, err)
		})

		params := DataColumnSidecarsParams{
			Ctx:         t.Context(),
			Tor:         clock,
			P2P:         p2p,
			CtxMap:      ctxMap,
			RateLimiter: leakybucket.NewCollector(1., 1, time.Second, false /* deleteEmptyBuckets */),
		}

		actualResponse, err := sendDataColumnSidecarsRequest(params, slotByRoot, slotsWithCommitments, other.PeerID(), indicesByRoot)
		require.NoError(t, err)
		require.DeepEqual(t, expectedResponse, actualResponse[0])
	})
}

func TestBuildByRangeRequests(t *testing.T) {
	const nullBatchSize = 0

	t.Run("empty", func(t *testing.T) {
		actual, err := buildByRangeRequests(nil, nil, nil, nullBatchSize)
		require.NoError(t, err)

		require.Equal(t, 0, len(actual))
	})

	t.Run("missing Root", func(t *testing.T) {
		indicesByRoot := map[[fieldparams.RootLength]byte]map[uint64]bool{
			{1}: {1: true, 2: true},
		}

		_, err := buildByRangeRequests(nil, nil, indicesByRoot, nullBatchSize)
		require.NotNil(t, err)
	})

	t.Run("indices differ", func(t *testing.T) {
		indicesByRoot := map[[fieldparams.RootLength]byte]map[uint64]bool{
			{1}: {1: true, 2: true},
			{2}: {1: true, 2: true},
			{3}: {2: true, 3: true},
		}

		slotByRoot := map[[fieldparams.RootLength]byte]primitives.Slot{
			{1}: 1,
			{2}: 2,
			{3}: 3,
		}

		actual, err := buildByRangeRequests(slotByRoot, nil, indicesByRoot, nullBatchSize)
		require.NoError(t, err)
		require.Equal(t, 0, len(actual))
	})

	t.Run("slots non contiguous", func(t *testing.T) {
		indicesByRoot := map[[fieldparams.RootLength]byte]map[uint64]bool{
			{1}: {1: true, 2: true},
			{2}: {1: true, 2: true},
		}

		slotByRoot := map[[fieldparams.RootLength]byte]primitives.Slot{
			{1}: 1,
			{2}: 3,
		}

		slotsWithCommitments := map[primitives.Slot]bool{
			1: true,
			2: true,
			3: true,
		}

		actual, err := buildByRangeRequests(slotByRoot, slotsWithCommitments, indicesByRoot, nullBatchSize)
		require.NoError(t, err)
		require.Equal(t, 0, len(actual))
	})

	t.Run("nominal", func(t *testing.T) {
		const batchSize = 3

		indicesByRoot := map[[fieldparams.RootLength]byte]map[uint64]bool{
			{1}: {1: true, 2: true},
			{3}: {1: true, 2: true},
			{4}: {1: true, 2: true},
			{7}: {1: true, 2: true},
		}

		slotByRoot := map[[fieldparams.RootLength]byte]primitives.Slot{
			{1}: 1,
			{3}: 3,
			{4}: 4,
			{7}: 7,
		}

		slotsWithCommitments := map[primitives.Slot]bool{
			1: true,
			3: true,
			4: true,
			7: true,
		}

		expected := []*ethpb.DataColumnSidecarsByRangeRequest{
			{
				StartSlot: 1,
				Count:     3,
				Columns:   []uint64{1, 2},
			},
			{
				StartSlot: 4,
				Count:     3,
				Columns:   []uint64{1, 2},
			},
			{
				StartSlot: 7,
				Count:     1,
				Columns:   []uint64{1, 2},
			},
		}

		actual, err := buildByRangeRequests(slotByRoot, slotsWithCommitments, indicesByRoot, batchSize)
		require.NoError(t, err)
		require.DeepEqual(t, expected, actual)
	})
}

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
	require.DeepSSZEqual(t, expected, actual)
}

func TestVerifyDataColumnSidecarsByPeer(t *testing.T) {
	err := kzg.Start()
	require.NoError(t, err)

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.FuluForkEpoch = 0
	cfg.BlobSchedule = []params.BlobScheduleEntry{{Epoch: 0, MaxBlobsPerBlock: 2}}
	params.OverrideBeaconConfig(cfg)

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
			[fieldparams.RootLength]byte{2}: {10: true},
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
	// Fixed seed.
	const seed = 43
	randomSource := rand.New(rand.NewSource(seed))

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
		require.Equal(t, peer1, pid)

		pid, err = randomPeer(t.Context(), randomSource, collector, count, indicesByRootByPeer)
		require.NoError(t, err)
		require.Equal(t, peer2, pid)
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

func TestComputeTotalCount(t *testing.T) {
	input := map[[fieldparams.RootLength]byte]map[uint64]bool{
		[fieldparams.RootLength]byte{1}: {1: true, 3: true},
		[fieldparams.RootLength]byte{2}: {2: true},
	}

	const expected = 3
	actual := computeTotalCount(input)
	require.Equal(t, expected, actual)
}
