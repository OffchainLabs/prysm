package p2p

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	testDB "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v6/config/params"
	ecdsaprysm "github.com/OffchainLabs/prysm/v6/crypto/ecdsa"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p/core/crypto"
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

	// Define the genesis validators root, to ensure everybody is on the same network.
	const (
		genesisValidatorRootStr = "0xdeadbeefcafecafedeadbeefcafecafedeadbeefcafecafedeadbeefcafecafe"
		subnetCount             = 3
		minimumPeersPerSubnet   = 1
	)

	genesisValidatorsRoot, err := hex.DecodeString(genesisValidatorRootStr[2:])
	require.NoError(t, err)

	// Create a context.
	ctx := t.Context()

	// Use shorter period for testing.
	currentPeriod := pollingPeriod
	pollingPeriod = 1 * time.Second
	defer func() {
		pollingPeriod = currentPeriod
	}()

	// Create flags.
	params.SetupTestConfigCleanup(t)
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
		genesisValidatorsRoot: genesisValidatorsRoot,
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
		service.genesisValidatorsRoot = genesisValidatorsRoot
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

	service, err := NewService(ctx, cfg)
	require.NoError(t, err)

	service.genesisTime = genesisTime
	service.genesisValidatorsRoot = genesisValidatorsRoot
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
	assert.NoError(t, err)
	defer db.Close()
	priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
	assert.NoError(t, err)
	convertedKey, err := ecdsaprysm.ConvertFromInterfacePrivKey(priv)
	assert.NoError(t, err)
	localNode := enode.NewLocalNode(db, convertedKey)

	retrievedSubnets, err := computeSubscribedSubnets(localNode.ID(), 1000)
	assert.NoError(t, err)
	assert.Equal(t, retrievedSubnets[0]+1, retrievedSubnets[1])
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
