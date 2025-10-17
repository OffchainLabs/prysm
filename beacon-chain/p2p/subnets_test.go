package p2p

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	testDB "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers/scorers"
	testp2p "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v6/config/params"
	ecdsaprysm "github.com/OffchainLabs/prysm/v6/crypto/ecdsa"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/prysmaticlabs/go-bitfield"
)

func TestStartDiscV5_FindAndDialPeersWithSubnet(t *testing.T) {
	// Topology of this test:
	//
	//
	// Node 1 (subscribed to subnet 1)  --\
	//									  |
	// Node 2 (subscribed to subnet 2)  --+--> BootNode (not subscribed to any subnet) <------- Node 0 (not subscribed to any subnet)
	//									  |
	// Node 3 (subscribed to subnet 3)  --/
	//
	// The purpose of this test is to ensure that the "Node 0" (connected only to the boot node) is able to
	// find and connect to a node already subscribed to a specific subnet.
	// In our case: The node i is subscribed to subnet i, with i = 1, 2, 3

	const subnetCount = 3
	const minimumPeersPerSubnet = 1
	ctx := t.Context()

	// Use shorter period for testing.
	currentPeriod := pollingPeriod
	pollingPeriod = 1 * time.Second
	defer func() {
		pollingPeriod = currentPeriod
	}()

	// Create flags.
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().InitializeForkSchedule()
	gFlags := new(flags.GlobalFlags)
	gFlags.MinimumPeersPerSubnet = 1
	flags.Init(gFlags)

	params.BeaconNetworkConfig().MinimumPeersInSubnetSearch = 1

	// Reset config.
	defer flags.Init(new(flags.GlobalFlags))

	// First, generate a bootstrap node.
	ipAddr, pkey := createAddrAndPrivKey(t)
	genesisTime := time.Now()

	bootNodeService := &Service{
		cfg:                   &Config{UDPPort: 2000, TCPPort: 3000, QUICPort: 3000, DisableLivenessCheck: true, PingInterval: testPingInterval},
		genesisTime:           genesisTime,
		genesisValidatorsRoot: params.BeaconConfig().GenesisValidatorsRoot[:],
		custodyInfo:           &custodyInfo{},
	}

	bootNodeForkDigest, err := bootNodeService.currentForkDigest()
	require.NoError(t, err)

	bootListener, err := bootNodeService.createListener(ipAddr, pkey)
	require.NoError(t, err)
	defer bootListener.Close()

	// Allow bootnode's table to have its initial refresh. This allows
	// inbound nodes to be added in.
	time.Sleep(5 * time.Second)

	bootNodeENR := bootListener.Self().String()

	// Create 3 nodes, each subscribed to a different subnet.
	// Each node is connected to the bootstrap node.
	services := make([]*Service, 0, subnetCount)
	db := testDB.SetupDB(t)

	for i := uint64(1); i <= subnetCount; i++ {
		service, err := NewService(ctx, &Config{
			Discv5BootStrapAddrs: []string{bootNodeENR},
			MaxPeers:             0, // Set to 0 to ensure that peers are discovered via subnets search, and not generic peers discovery.
			UDPPort:              uint(2000 + i),
			TCPPort:              uint(3000 + i),
			QUICPort:             uint(3000 + i),
			PingInterval:         testPingInterval,
			DisableLivenessCheck: true,
			DB:                   db,
		})

		require.NoError(t, err)

		service.genesisTime = genesisTime
		service.genesisValidatorsRoot = params.BeaconConfig().GenesisValidatorsRoot[:]
		service.custodyInfo = &custodyInfo{}

		nodeForkDigest, err := service.currentForkDigest()
		require.NoError(t, err)
		require.Equal(t, true, nodeForkDigest == bootNodeForkDigest, "fork digest of the node doesn't match the boot node")

		// Start the service.
		service.Start()

		// Set the ENR `attnets`, used by Prysm to filter peers by subnet.
		bitV := bitfield.NewBitvector64()
		bitV.SetBitAt(i, true)
		entry := enr.WithEntry(attSubnetEnrKey, &bitV)
		service.dv5Listener.LocalNode().Set(entry)

		// Join and subscribe to the subnet, needed by libp2p.
		topicName := fmt.Sprintf(AttestationSubnetTopicFormat, bootNodeForkDigest, i) + "/ssz_snappy"
		topic, err := service.pubsub.Join(topicName)
		require.NoError(t, err)

		_, err = topic.Subscribe()
		require.NoError(t, err)

		// Memoize the service.
		services = append(services, service)
	}

	// Stop the services.
	defer func() {
		for _, service := range services {
			err := service.Stop()
			require.NoError(t, err)
		}
	}()

	cfg := &Config{
		Discv5BootStrapAddrs: []string{bootNodeENR},
		PingInterval:         testPingInterval,
		DisableLivenessCheck: true,
		MaxPeers:             30,
		UDPPort:              2010,
		TCPPort:              3010,
		QUICPort:             3010,
		DB:                   db,
	}

	service, err := NewService(t.Context(), cfg)
	require.NoError(t, err)

	service.genesisTime = genesisTime
	service.genesisValidatorsRoot = params.BeaconConfig().GenesisValidatorsRoot[:]
	service.custodyInfo = &custodyInfo{}

	service.Start()
	defer func() {
		err := service.Stop()
		require.NoError(t, err)
	}()

	subnets := map[uint64]bool{1: true, 2: true, 3: true}
	defectiveSubnets := service.defectiveSubnets(AttestationSubnetTopicFormat, bootNodeForkDigest, minimumPeersPerSubnet, subnets)
	require.Equal(t, subnetCount, len(defectiveSubnets))

	ctxWithTimeOut, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err = service.FindAndDialPeersWithSubnets(ctxWithTimeOut, AttestationSubnetTopicFormat, bootNodeForkDigest, minimumPeersPerSubnet, subnets)
	require.NoError(t, err)

	defectiveSubnets = service.defectiveSubnets(AttestationSubnetTopicFormat, bootNodeForkDigest, minimumPeersPerSubnet, subnets)
	require.Equal(t, 0, len(defectiveSubnets))
}

func Test_AttSubnets(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	tests := []struct {
		name        string
		record      func(localNode *enode.LocalNode) *enr.Record
		want        []uint64
		wantErr     bool
		errContains string
	}{
		{
			name: "valid record",
			record: func(localNode *enode.LocalNode) *enr.Record {
				localNode = initializeAttSubnets(localNode)
				return localNode.Node().Record()
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "too small subnet",
			record: func(localNode *enode.LocalNode) *enr.Record {
				entry := enr.WithEntry(attSubnetEnrKey, []byte{})
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want:        []uint64{},
			wantErr:     true,
			errContains: "invalid bitvector provided, it has a size of",
		},
		{
			name: "half sized subnet",
			record: func(localNode *enode.LocalNode) *enr.Record {
				entry := enr.WithEntry(attSubnetEnrKey, make([]byte, 4))
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want:        []uint64{},
			wantErr:     true,
			errContains: "invalid bitvector provided, it has a size of",
		},
		{
			name: "too large subnet",
			record: func(localNode *enode.LocalNode) *enr.Record {
				entry := enr.WithEntry(attSubnetEnrKey, make([]byte, byteCount(int(attestationSubnetCount))+1))
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want:        []uint64{},
			wantErr:     true,
			errContains: "invalid bitvector provided, it has a size of",
		},
		{
			name: "very large subnet",
			record: func(localNode *enode.LocalNode) *enr.Record {
				entry := enr.WithEntry(attSubnetEnrKey, make([]byte, byteCount(int(attestationSubnetCount))+100))
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want:        []uint64{},
			wantErr:     true,
			errContains: "invalid bitvector provided, it has a size of",
		},
		{
			name: "single subnet",
			record: func(localNode *enode.LocalNode) *enr.Record {
				bitV := bitfield.NewBitvector64()
				bitV.SetBitAt(0, true)
				entry := enr.WithEntry(attSubnetEnrKey, bitV.Bytes())
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want:    []uint64{0},
			wantErr: false,
		},
		{
			name: "multiple subnets",
			record: func(localNode *enode.LocalNode) *enr.Record {
				bitV := bitfield.NewBitvector64()
				for i := uint64(0); i < bitV.Len(); i++ {
					// Keep only odd subnets.
					if (i+1)%2 == 0 {
						continue
					}
					bitV.SetBitAt(i, true)
				}
				bitV.SetBitAt(0, true)
				entry := enr.WithEntry(attSubnetEnrKey, bitV.Bytes())
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want: []uint64{0, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20,
				22, 24, 26, 28, 30, 32, 34, 36, 38, 40, 42, 44, 46, 48,
				50, 52, 54, 56, 58, 60, 62},
			wantErr: false,
		},
		{
			name: "all subnets",
			record: func(localNode *enode.LocalNode) *enr.Record {
				bitV := bitfield.NewBitvector64()
				for i := uint64(0); i < bitV.Len(); i++ {
					bitV.SetBitAt(i, true)
				}
				entry := enr.WithEntry(attSubnetEnrKey, bitV.Bytes())
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want: []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
				21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49,
				50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := enode.OpenDB("")
			require.NoError(t, err)

			priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
			require.NoError(t, err)

			convertedKey, err := ecdsaprysm.ConvertFromInterfacePrivKey(priv)
			require.NoError(t, err)

			localNode := enode.NewLocalNode(db, convertedKey)
			record := tt.record(localNode)

			got, err := attestationSubnets(record)
			if (err != nil) != tt.wantErr {
				t.Errorf("attestationSubnets() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				require.ErrorContains(t, tt.errContains, err)
			}

			require.Equal(t, len(tt.want), len(got))
			for _, subnet := range tt.want {
				require.Equal(t, true, got[subnet])
			}
		})
	}
}

func Test_SyncSubnets(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	tests := []struct {
		name        string
		record      func(t *testing.T) *enr.Record
		want        []uint64
		wantErr     bool
		errContains string
	}{
		{
			name: "valid record",
			record: func(t *testing.T) *enr.Record {
				db, err := enode.OpenDB("")
				assert.NoError(t, err)
				priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
				assert.NoError(t, err)
				convertedKey, err := ecdsaprysm.ConvertFromInterfacePrivKey(priv)
				assert.NoError(t, err)
				localNode := enode.NewLocalNode(db, convertedKey)
				localNode = initializeSyncCommSubnets(localNode)
				return localNode.Node().Record()
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "too small subnet",
			record: func(t *testing.T) *enr.Record {
				db, err := enode.OpenDB("")
				assert.NoError(t, err)
				priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
				assert.NoError(t, err)
				convertedKey, err := ecdsaprysm.ConvertFromInterfacePrivKey(priv)
				assert.NoError(t, err)
				localNode := enode.NewLocalNode(db, convertedKey)
				entry := enr.WithEntry(syncCommsSubnetEnrKey, []byte{})
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want:        []uint64{},
			wantErr:     true,
			errContains: "invalid bitvector provided, it has a size of",
		},
		{
			name: "too large subnet",
			record: func(t *testing.T) *enr.Record {
				db, err := enode.OpenDB("")
				assert.NoError(t, err)
				priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
				assert.NoError(t, err)
				convertedKey, err := ecdsaprysm.ConvertFromInterfacePrivKey(priv)
				assert.NoError(t, err)
				localNode := enode.NewLocalNode(db, convertedKey)
				entry := enr.WithEntry(syncCommsSubnetEnrKey, make([]byte, byteCount(int(syncCommsSubnetCount))+1))
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want:        []uint64{},
			wantErr:     true,
			errContains: "invalid bitvector provided, it has a size of",
		},
		{
			name: "very large subnet",
			record: func(t *testing.T) *enr.Record {
				db, err := enode.OpenDB("")
				assert.NoError(t, err)
				priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
				assert.NoError(t, err)
				convertedKey, err := ecdsaprysm.ConvertFromInterfacePrivKey(priv)
				assert.NoError(t, err)
				localNode := enode.NewLocalNode(db, convertedKey)
				entry := enr.WithEntry(syncCommsSubnetEnrKey, make([]byte, byteCount(int(syncCommsSubnetCount))+100))
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want:        []uint64{},
			wantErr:     true,
			errContains: "invalid bitvector provided, it has a size of",
		},
		{
			name: "single subnet",
			record: func(t *testing.T) *enr.Record {
				db, err := enode.OpenDB("")
				assert.NoError(t, err)
				priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
				assert.NoError(t, err)
				convertedKey, err := ecdsaprysm.ConvertFromInterfacePrivKey(priv)
				assert.NoError(t, err)
				localNode := enode.NewLocalNode(db, convertedKey)
				bitV := bitfield.Bitvector4{byte(0x00)}
				bitV.SetBitAt(0, true)
				entry := enr.WithEntry(syncCommsSubnetEnrKey, bitV.Bytes())
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want:    []uint64{0},
			wantErr: false,
		},
		{
			name: "multiple subnets",
			record: func(t *testing.T) *enr.Record {
				db, err := enode.OpenDB("")
				assert.NoError(t, err)
				priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
				assert.NoError(t, err)
				convertedKey, err := ecdsaprysm.ConvertFromInterfacePrivKey(priv)
				assert.NoError(t, err)
				localNode := enode.NewLocalNode(db, convertedKey)
				bitV := bitfield.Bitvector4{byte(0x00)}
				for i := uint64(0); i < bitV.Len(); i++ {
					// skip 2 subnets
					if (i+1)%2 == 0 {
						continue
					}
					bitV.SetBitAt(i, true)
				}
				bitV.SetBitAt(0, true)
				entry := enr.WithEntry(syncCommsSubnetEnrKey, bitV.Bytes())
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want:    []uint64{0, 2},
			wantErr: false,
		},
		{
			name: "all subnets",
			record: func(t *testing.T) *enr.Record {
				db, err := enode.OpenDB("")
				assert.NoError(t, err)
				priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
				assert.NoError(t, err)
				convertedKey, err := ecdsaprysm.ConvertFromInterfacePrivKey(priv)
				assert.NoError(t, err)
				localNode := enode.NewLocalNode(db, convertedKey)
				bitV := bitfield.Bitvector4{byte(0x00)}
				for i := uint64(0); i < bitV.Len(); i++ {
					bitV.SetBitAt(i, true)
				}
				entry := enr.WithEntry(syncCommsSubnetEnrKey, bitV.Bytes())
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want:    []uint64{0, 1, 2, 3},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := syncSubnets(tt.record(t))
			if (err != nil) != tt.wantErr {
				t.Errorf("syncSubnets() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				assert.ErrorContains(t, tt.errContains, err)
			}

			require.Equal(t, len(tt.want), len(got))
			for _, subnet := range tt.want {
				require.Equal(t, true, got[subnet])
			}
		})
	}
}

func TestDataColumnSubnets(t *testing.T) {
	const cgc = 3

	var (
		nodeID enode.ID
		record enr.Record
	)

	record.Set(peerdas.Cgc(cgc))

	expected := map[uint64]bool{1: true, 87: true, 102: true}
	actual, err := dataColumnSubnets(nodeID, &record)
	assert.NoError(t, err)

	require.Equal(t, len(expected), len(actual))
	for subnet := range expected {
		require.Equal(t, true, actual[subnet])
	}
}

func TestSubnetComputation(t *testing.T) {
	db, err := enode.OpenDB("")
	require.NoError(t, err)
	defer db.Close()

	priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
	require.NoError(t, err)

	convertedKey, err := ecdsaprysm.ConvertFromInterfacePrivKey(priv)
	require.NoError(t, err)

	localNode := enode.NewLocalNode(db, convertedKey)
	beaconConfig := params.BeaconConfig()

	t.Run("standard", func(t *testing.T) {
		retrievedSubnets, err := computeSubscribedSubnets(localNode.ID(), 1000)
		require.NoError(t, err)
		require.Equal(t, beaconConfig.SubnetsPerNode, uint64(len(retrievedSubnets)))
		require.Equal(t, retrievedSubnets[0]+1, retrievedSubnets[1])
	})

	t.Run("subscribed to all", func(t *testing.T) {
		gFlags := new(flags.GlobalFlags)
		gFlags.SubscribeToAllSubnets = true
		flags.Init(gFlags)
		defer flags.Init(new(flags.GlobalFlags))

		retrievedSubnets, err := computeSubscribedSubnets(localNode.ID(), 1000)
		require.NoError(t, err)
		require.Equal(t, beaconConfig.AttestationSubnetCount, uint64(len(retrievedSubnets)))
		for i := range beaconConfig.AttestationSubnetCount {
			require.Equal(t, i, retrievedSubnets[i])
		}
	})

}

func TestInitializePersistentSubnets(t *testing.T) {
	cache.SubnetIDs.EmptyAllCaches()
	defer cache.SubnetIDs.EmptyAllCaches()

	db, err := enode.OpenDB("")
	assert.NoError(t, err)
	defer db.Close()
	priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
	assert.NoError(t, err)
	convertedKey, err := ecdsaprysm.ConvertFromInterfacePrivKey(priv)
	assert.NoError(t, err)
	localNode := enode.NewLocalNode(db, convertedKey)

	assert.NoError(t, initializePersistentSubnets(localNode.ID(), 10000))
	subs, ok, expTime := cache.SubnetIDs.GetPersistentSubnets()
	assert.Equal(t, true, ok)
	assert.Equal(t, 2, len(subs))
	assert.Equal(t, true, expTime.After(time.Now()))
}

func TestFindPeersWithSubnets_NodeDeduplication(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cache.SubnetIDs.EmptyAllCaches()
	defer cache.SubnetIDs.EmptyAllCaches()

	ctx := context.Background()
	db := testDB.SetupDB(t)

	localNode1 := createTestNodeWithID(t, "node1")
	localNode2 := createTestNodeWithID(t, "node2")
	localNode3 := createTestNodeWithID(t, "node3")

	// Create different sequence versions of node1 with subnet 1
	setNodeSubnets(localNode1, []uint64{1})
	setNodeSeq(localNode1, 1)
	node1_seq1_subnet1 := localNode1.Node()
	setNodeSeq(localNode1, 2)
	node1_seq2_subnet1 := localNode1.Node() // Same ID, higher seq
	setNodeSeq(localNode1, 3)
	node1_seq3_subnet1 := localNode1.Node() // Same ID, even higher seq

	// Node2 with different sequences and subnets
	setNodeSubnets(localNode2, []uint64{1})
	node2_seq1_subnet1 := localNode2.Node()
	setNodeSubnets(localNode2, []uint64{2}) // Different subnet
	setNodeSeq(localNode2, 2)
	node2_seq2_subnet2 := localNode2.Node()

	// Node3 with multiple subnets
	setNodeSubnets(localNode3, []uint64{1, 2})
	node3_seq1_subnet1_2 := localNode3.Node()

	tests := []struct {
		name             string
		nodes            []*enode.Node
		defectiveSubnets map[uint64]int
		expectedCount    int
		description      string
		eval             func(t *testing.T, result []*enode.Node) // Custom validation function
	}{
		{
			name: "No duplicates - unique nodes with same subnet",
			nodes: []*enode.Node{
				node2_seq1_subnet1,
				node3_seq1_subnet1_2,
			},
			defectiveSubnets: map[uint64]int{1: 2},
			expectedCount:    2,
			description:      "Should return all unique nodes subscribed to subnet",
			eval:             nil, // No special validation needed
		},
		{
			name: "Duplicate with lower seq first - should replace",
			nodes: []*enode.Node{
				node1_seq1_subnet1,
				node1_seq2_subnet1, // Higher seq, should replace
				node2_seq1_subnet1, // Different node to ensure we process enough nodes
			},
			defectiveSubnets: map[uint64]int{1: 2}, // Need 2 peers for subnet 1
			expectedCount:    2,
			description:      "Should replace with higher seq node for same subnet",
			eval: func(t *testing.T, result []*enode.Node) {
				found := false
				for _, node := range result {
					if node.ID() == node1_seq2_subnet1.ID() && node.Seq() == node1_seq2_subnet1.Seq() {
						found = true
						break
					}
				}
				require.Equal(t, true, found, "Should have node with higher seq")
			},
		},
		{
			name: "Duplicate with higher seq first - should keep existing",
			nodes: []*enode.Node{
				node1_seq3_subnet1, // Higher seq
				node1_seq2_subnet1, // Lower seq, should be skipped (continue branch)
				node1_seq1_subnet1, // Even lower seq, should also be skipped (continue branch)
				node2_seq1_subnet1, // Different node
			},
			defectiveSubnets: map[uint64]int{1: 2},
			expectedCount:    2,
			description:      "Should keep existing node with higher seq and skip lower seq duplicates",
			eval: func(t *testing.T, result []*enode.Node) {
				found := false
				for _, node := range result {
					if node.ID() == node1_seq3_subnet1.ID() && node.Seq() == node1_seq3_subnet1.Seq() {
						found = true
						break
					}
				}
				require.Equal(t, true, found, "Should have node with highest seq")
			},
		},
		{
			name: "Multiple updates for same node",
			nodes: []*enode.Node{
				node1_seq1_subnet1,
				node1_seq2_subnet1, // Should replace seq1
				node1_seq3_subnet1, // Should replace seq2
				node2_seq1_subnet1, // Different node
			},
			defectiveSubnets: map[uint64]int{1: 2},
			expectedCount:    2,
			description:      "Should keep updating to highest seq",
			eval: func(t *testing.T, result []*enode.Node) {
				found := false
				for _, node := range result {
					if node.ID() == node1_seq3_subnet1.ID() && node.Seq() == node1_seq3_subnet1.Seq() {
						found = true
						break
					}
				}
				require.Equal(t, true, found, "Should have node with highest seq")
			},
		},
		{
			name: "Duplicate with equal seq in subnets - should skip",
			nodes: []*enode.Node{
				node1_seq2_subnet1, // First occurrence
				node1_seq2_subnet1, // Same exact node instance, should be skipped (continue branch)
				node2_seq1_subnet1, // Different node
			},
			defectiveSubnets: map[uint64]int{1: 2},
			expectedCount:    2,
			description:      "Should skip duplicate with equal sequence number in subnet search",
			eval: func(t *testing.T, result []*enode.Node) {
				foundNode1 := false
				foundNode2 := false
				node1Count := 0
				for _, node := range result {
					if node.ID() == node1_seq2_subnet1.ID() {
						require.Equal(t, node1_seq2_subnet1.Seq(), node.Seq(), "Node1 should have expected seq")
						foundNode1 = true
						node1Count++
					}
					if node.ID() == node2_seq1_subnet1.ID() {
						foundNode2 = true
					}
				}
				require.Equal(t, true, foundNode1, "Should have node1")
				require.Equal(t, true, foundNode2, "Should have node2")
				require.Equal(t, 1, node1Count, "Should have exactly one instance of node1")
			},
		},
		{
			name: "Mix with different subnets",
			nodes: []*enode.Node{
				node2_seq1_subnet1,
				node2_seq2_subnet2, // Higher seq but different subnet
				node3_seq1_subnet1_2,
			},
			defectiveSubnets: map[uint64]int{1: 2, 2: 1},
			expectedCount:    2, // node2 (latest) and node3
			description:      "Should handle nodes with different subnet subscriptions",
			eval:             nil, // Basic count validation is sufficient
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gFlags := new(flags.GlobalFlags)
			gFlags.MinimumPeersPerSubnet = 1
			flags.Init(gFlags)
			defer flags.Init(new(flags.GlobalFlags))

			fakePeer := testp2p.NewTestP2P(t)

			s := &Service{
				cfg: &Config{
					MaxPeers: 30,
					DB:       db,
				},
				genesisTime:           time.Now(),
				genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
				peers: peers.NewStatus(ctx, &peers.StatusConfig{
					PeerLimit:    30,
					ScorerParams: &scorers.Config{},
				}),
				host: fakePeer.BHost,
			}

			localNode := createTestNodeRandom(t)

			mockIter := testp2p.NewMockIterator(tt.nodes)
			s.dv5Listener = testp2p.NewMockListener(localNode, mockIter)

			digest, err := s.currentForkDigest()
			require.NoError(t, err)

			ctxWithTimeout, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
			defer cancel()

			result, err := s.findPeersWithSubnets(
				ctxWithTimeout,
				AttestationSubnetTopicFormat,
				digest,
				1,
				tt.defectiveSubnets,
			)

			require.NoError(t, err, tt.description)
			require.Equal(t, tt.expectedCount, len(result), tt.description)

			if tt.eval != nil {
				tt.eval(t, result)
			}
		})
	}
}

func TestFindPeersWithSubnets_FilterPeerRemoval(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cache.SubnetIDs.EmptyAllCaches()
	defer cache.SubnetIDs.EmptyAllCaches()

	ctx := context.Background()
	db := testDB.SetupDB(t)

	localNode1 := createTestNodeWithID(t, "node1")
	localNode2 := createTestNodeWithID(t, "node2")
	localNode3 := createTestNodeWithID(t, "node3")

	// Create versions of node1 with subnet 1
	setNodeSubnets(localNode1, []uint64{1})
	setNodeSeq(localNode1, 1)
	node1_seq1_valid_subnet1 := localNode1.Node()

	// Create bad version (higher seq)
	setNodeSeq(localNode1, 2)
	node1_seq2_bad_subnet1 := localNode1.Node()

	// Create another valid version
	setNodeSeq(localNode1, 3)
	node1_seq3_valid_subnet1 := localNode1.Node()

	// Node2 with subnet 1
	setNodeSubnets(localNode2, []uint64{1})
	node2_seq1_valid_subnet1 := localNode2.Node()

	// Node3 with subnet 1 and 2
	setNodeSubnets(localNode3, []uint64{1, 2})
	node3_seq1_valid_subnet1_2 := localNode3.Node()

	tests := []struct {
		name             string
		nodes            []*enode.Node
		defectiveSubnets map[uint64]int
		expectedCount    int
		description      string
		eval             func(t *testing.T, result []*enode.Node)
	}{
		{
			name: "Valid node in subnet followed by bad version - should remove",
			nodes: []*enode.Node{
				node1_seq1_valid_subnet1, // First add valid node with subnet 1
				node1_seq2_bad_subnet1,   // Invalid version with higher seq - should delete
				node2_seq1_valid_subnet1, // Different valid node with subnet 1
			},
			defectiveSubnets: map[uint64]int{1: 2}, // Need 2 peers for subnet 1
			expectedCount:    1,                    // Only node2 should remain
			description:      "Should remove node from map when bad version arrives, even if it has required subnet",
			eval: func(t *testing.T, result []*enode.Node) {
				foundNode1 := false
				foundNode2 := false
				for _, node := range result {
					if node.ID() == node1_seq1_valid_subnet1.ID() {
						foundNode1 = true
					}
					if node.ID() == node2_seq1_valid_subnet1.ID() {
						foundNode2 = true
					}
				}
				require.Equal(t, false, foundNode1, "Node1 should have been removed despite having subnet")
				require.Equal(t, true, foundNode2, "Node2 should be present")
			},
		},
		{
			name: "Bad node with subnet stays bad even with higher seq",
			nodes: []*enode.Node{
				node1_seq2_bad_subnet1,   // First bad node - not added
				node1_seq3_valid_subnet1, // Higher seq but same bad peer ID
				node2_seq1_valid_subnet1, // Different valid node
			},
			defectiveSubnets: map[uint64]int{1: 2},
			expectedCount:    1, // Only node2 (node1 remains bad)
			description:      "Bad peer with subnet remains bad even with higher seq",
			eval: func(t *testing.T, result []*enode.Node) {
				foundNode1 := false
				foundNode2 := false
				for _, node := range result {
					if node.ID() == node1_seq3_valid_subnet1.ID() {
						foundNode1 = true
					}
					if node.ID() == node2_seq1_valid_subnet1.ID() {
						foundNode2 = true
					}
				}
				require.Equal(t, false, foundNode1, "Node1 should remain bad despite having subnet")
				require.Equal(t, true, foundNode2, "Node2 should be present")
			},
		},
		{
			name: "Mixed valid and bad nodes with subnets",
			nodes: []*enode.Node{
				node1_seq1_valid_subnet1,   // Add valid node1 with subnet
				node2_seq1_valid_subnet1,   // Add valid node2 with subnet
				node1_seq2_bad_subnet1,     // Invalid update for node1 - should remove
				node3_seq1_valid_subnet1_2, // Add valid node3 with multiple subnets
			},
			defectiveSubnets: map[uint64]int{1: 3}, // Need 3 peers for subnet 1
			expectedCount:    2,                    // Only node2 and node3 should remain
			description:      "Should handle removal of nodes with subnets when they become bad",
			eval: func(t *testing.T, result []*enode.Node) {
				foundNode1 := false
				foundNode2 := false
				foundNode3 := false
				for _, node := range result {
					if node.ID() == node1_seq1_valid_subnet1.ID() {
						foundNode1 = true
					}
					if node.ID() == node2_seq1_valid_subnet1.ID() {
						foundNode2 = true
					}
					if node.ID() == node3_seq1_valid_subnet1_2.ID() {
						foundNode3 = true
					}
				}
				require.Equal(t, false, foundNode1, "Node1 should have been removed")
				require.Equal(t, true, foundNode2, "Node2 should be present")
				require.Equal(t, true, foundNode3, "Node3 should be present")
			},
		},
		{
			name: "Node with subnet marked bad stays bad for all sequences",
			nodes: []*enode.Node{
				node1_seq1_valid_subnet1, // Add valid node1 with subnet
				node1_seq2_bad_subnet1,   // Bad update - should remove and mark bad
				node1_seq3_valid_subnet1, // Higher seq but still same bad peer ID
				node2_seq1_valid_subnet1, // Different valid node
			},
			defectiveSubnets: map[uint64]int{1: 2},
			expectedCount:    1, // Only node2 (node1 stays bad)
			description:      "Once marked bad, subnet peer stays bad for all sequences",
			eval: func(t *testing.T, result []*enode.Node) {
				foundNode1 := false
				foundNode2 := false
				for _, node := range result {
					if node.ID() == node1_seq3_valid_subnet1.ID() {
						foundNode1 = true
					}
					if node.ID() == node2_seq1_valid_subnet1.ID() {
						foundNode2 = true
					}
				}
				require.Equal(t, false, foundNode1, "Node1 should stay bad")
				require.Equal(t, true, foundNode2, "Node2 should be present")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize flags for subnet operations
			gFlags := new(flags.GlobalFlags)
			gFlags.MinimumPeersPerSubnet = 1
			flags.Init(gFlags)
			defer flags.Init(new(flags.GlobalFlags))

			// Create test P2P instance
			fakePeer := testp2p.NewTestP2P(t)

			// Create mock service
			s := &Service{
				cfg: &Config{
					MaxPeers: 30,
					DB:       db,
				},
				genesisTime:           time.Now(),
				genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
				peers: peers.NewStatus(ctx, &peers.StatusConfig{
					PeerLimit:    30,
					ScorerParams: &scorers.Config{},
				}),
				host: fakePeer.BHost,
			}

			// Mark specific node versions as "bad" to simulate filterPeer failures
			for _, node := range tt.nodes {
				if node == node1_seq2_bad_subnet1 {
					// Get peer ID from the node to mark it as bad
					peerData, _, _ := convertToAddrInfo(node)
					if peerData != nil {
						s.peers.Add(node.Record(), peerData.ID, nil, network.DirUnknown)
						// Mark as bad peer - this will make filterPeer return false
						s.peers.Scorers().BadResponsesScorer().Increment(peerData.ID)
						s.peers.Scorers().BadResponsesScorer().Increment(peerData.ID)
						s.peers.Scorers().BadResponsesScorer().Increment(peerData.ID)
					}
				}
			}

			localNode := createTestNodeRandom(t)

			mockIter := testp2p.NewMockIterator(tt.nodes)
			s.dv5Listener = testp2p.NewMockListener(localNode, mockIter)

			digest, err := s.currentForkDigest()
			require.NoError(t, err)

			ctxWithTimeout, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
			defer cancel()

			result, err := s.findPeersWithSubnets(
				ctxWithTimeout,
				AttestationSubnetTopicFormat,
				digest,
				1,
				tt.defectiveSubnets,
			)

			require.NoError(t, err, tt.description)
			require.Equal(t, tt.expectedCount, len(result), tt.description)

			if tt.eval != nil {
				tt.eval(t, result)
			}
		})
	}
}

// callbackIterator allows us to execute callbacks at specific points during iteration
type callbackIteratorForSubnets struct {
	nodes     []*enode.Node
	index     int
	callbacks map[int]func() // map from index to callback function
}

func (c *callbackIteratorForSubnets) Next() bool {
	// Execute callback before checking if we can continue (if one exists)
	if callback, exists := c.callbacks[c.index]; exists {
		callback()
	}

	return c.index < len(c.nodes)
}

func (c *callbackIteratorForSubnets) Node() *enode.Node {
	if c.index >= len(c.nodes) {
		return nil
	}

	node := c.nodes[c.index]
	c.index++
	return node
}

func (c *callbackIteratorForSubnets) Close() {
	// Nothing to clean up for this simple implementation
}

func TestFindPeersWithSubnets_received_bad_existing_node(t *testing.T) {
	// This test successfully triggers delete(nodeByNodeID, node.ID()) in subnets.go by:
	// 1. Processing node1_seq1 first (passes filterPeer, gets added to map
	// 2. Callback marks peer as bad before processing node1_seq2"
	// 3. Processing node1_seq2 (fails filterPeer, triggers delete since ok=true
	params.SetupTestConfigCleanup(t)
	cache.SubnetIDs.EmptyAllCaches()
	defer cache.SubnetIDs.EmptyAllCaches()

	ctx := context.Background()
	db := testDB.SetupDB(t)

	// Create LocalNode with same ID but different sequences
	localNode1 := createTestNodeWithID(t, "testnode")
	setNodeSubnets(localNode1, []uint64{1})
	node1_seq1 := localNode1.Node() // Get current node
	currentSeq := node1_seq1.Seq()
	setNodeSeq(localNode1, currentSeq+1) // Increment sequence by 1
	node1_seq2 := localNode1.Node()      // This should have higher seq

	// Additional node to ensure we have enough peers to process
	localNode2 := createTestNodeWithID(t, "othernode")
	setNodeSubnets(localNode2, []uint64{1})
	node2 := localNode2.Node()

	gFlags := new(flags.GlobalFlags)
	gFlags.MinimumPeersPerSubnet = 1
	flags.Init(gFlags)
	defer flags.Init(new(flags.GlobalFlags))

	fakePeer := testp2p.NewTestP2P(t)

	service := &Service{
		cfg: &Config{
			MaxPeers: 30,
			DB:       db,
		},
		genesisTime:           time.Now(),
		genesisValidatorsRoot: bytesutil.PadTo([]byte{'A'}, 32),
		peers: peers.NewStatus(ctx, &peers.StatusConfig{
			PeerLimit:    30,
			ScorerParams: &scorers.Config{},
		}),
		host: fakePeer.BHost,
	}

	// Create iterator with callback that marks peer as bad before processing node1_seq2
	iter := &callbackIteratorForSubnets{
		nodes: []*enode.Node{node1_seq1, node1_seq2, node2},
		index: 0,
		callbacks: map[int]func(){
			1: func() { // Before processing node1_seq2 (index 1)
				// Mark peer as bad before processing node1_seq2
				peerData, _, _ := convertToAddrInfo(node1_seq2)
				if peerData != nil {
					service.peers.Add(node1_seq2.Record(), peerData.ID, nil, network.DirUnknown)
					// Mark as bad peer - need enough increments to exceed threshold (6)
					for i := 0; i < 10; i++ {
						service.peers.Scorers().BadResponsesScorer().Increment(peerData.ID)
					}
				}
			},
		},
	}

	localNode := createTestNodeRandom(t)
	service.dv5Listener = testp2p.NewMockListener(localNode, iter)

	digest, err := service.currentForkDigest()
	require.NoError(t, err)

	// Run findPeersWithSubnets - node1_seq1 gets processed first, then callback marks peer bad, then node1_seq2 fails
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	result, err := service.findPeersWithSubnets(
		ctxWithTimeout,
		AttestationSubnetTopicFormat,
		digest,
		1,
		map[uint64]int{1: 2}, // Need 2 peers for subnet 1
	)

	require.NoError(t, err)
	require.Equal(t, 1, len(result))
	require.Equal(t, localNode2.Node().ID(), result[0].ID()) // only node2 should remain
}
