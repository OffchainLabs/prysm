package p2p

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/cache"
	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/flags"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	ecdsaprysm "github.com/prysmaticlabs/prysm/v5/crypto/ecdsa"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestStartDiscV5_FindPeersWithSubnet(t *testing.T) {
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
	const genesisValidatorRootStr = "0xdeadbeefcafecafedeadbeefcafecafedeadbeefcafecafedeadbeefcafecafe"
	genesisValidatorsRoot, err := hex.DecodeString(genesisValidatorRootStr[2:])
	require.NoError(t, err)

	// Create a context.
	ctx := context.Background()

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
	services := make([]*Service, 0, 3)

	for i := 1; i <= 3; i++ {
		subnet := uint64(i)
		service, err := NewService(ctx, &Config{
			Discv5BootStrapAddrs: []string{bootNodeENR},
			MaxPeers:             30,
			UDPPort:              uint(2000 + i),
			TCPPort:              uint(3000 + i),
			QUICPort:             uint(3000 + i),
			PingInterval:         testPingInterval,
			DisableLivenessCheck: true,
		})

		require.NoError(t, err)

		service.genesisTime = genesisTime
		service.genesisValidatorsRoot = genesisValidatorsRoot

		nodeForkDigest, err := service.currentForkDigest()
		require.NoError(t, err)
		require.Equal(t, true, nodeForkDigest == bootNodeForkDigest, "fork digest of the node doesn't match the boot node")

		// Start the service.
		service.Start()

		// Set the ENR `attnets`, used by Prysm to filter peers by subnet.
		bitV := bitfield.NewBitvector64()
		bitV.SetBitAt(subnet, true)
		entry := enr.WithEntry(attSubnetEnrKey, &bitV)
		service.dv5Listener.LocalNode().Set(entry)

		// Join and subscribe to the subnet, needed by libp2p.
		topic, err := service.pubsub.Join(fmt.Sprintf(AttestationSubnetTopicFormat, bootNodeForkDigest, subnet) + "/ssz_snappy")
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
	}

	service, err := NewService(ctx, cfg)
	require.NoError(t, err)

	service.genesisTime = genesisTime
	service.genesisValidatorsRoot = genesisValidatorsRoot

	service.Start()
	defer func() {
		err := service.Stop()
		require.NoError(t, err)
	}()

	// Look up 3 different subnets.
	exists := make([]bool, 0, 3)
	for i := 1; i <= 3; i++ {
		subnet := uint64(i)
		topic := fmt.Sprintf(AttestationSubnetTopicFormat, bootNodeForkDigest, subnet)

		exist := false

		// This for loop is used to ensure we don't get stuck in `FindPeersWithSubnet`.
		// Read the documentation of `FindPeersWithSubnet` for more details.
		for j := 0; j < 3; j++ {
			ctxWithTimeOut, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			exist, err = service.FindPeersWithSubnet(ctxWithTimeOut, topic, subnet, 1)
			require.NoError(t, err)

			if exist {
				break
			}
		}

		require.NoError(t, err)
		exists = append(exists, exist)

	}

	// Check if all peers are found.
	for _, exist := range exists {
		require.Equal(t, true, exist, "Peer with subnet doesn't exist")
	}
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
			assert.NoError(t, err)

			priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
			assert.NoError(t, err)

			convertedKey, err := ecdsaprysm.ConvertFromInterfacePrivKey(priv)
			assert.NoError(t, err)

			localNode := enode.NewLocalNode(db, convertedKey)
			record := tt.record(localNode)

			got, err := attSubnets(record)
			if (err != nil) != tt.wantErr {
				t.Errorf("syncSubnets() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				assert.ErrorContains(t, tt.errContains, err)
			}

			want := make(map[uint64]bool, len(tt.want))
			for _, subnet := range tt.want {
				want[subnet] = true
			}

			if !reflect.DeepEqual(got, want) {
				t.Errorf("syncSubnets() got = %v, want %v", got, want)
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
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("syncSubnets() got = %v, want %v", got, tt.want)
			}
		})
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
