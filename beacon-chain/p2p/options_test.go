package p2p

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"os"
	"path"
	"testing"

	gethCrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	mock "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	ecdsaprysm "github.com/prysmaticlabs/prysm/v5/crypto/ecdsa"
	"github.com/prysmaticlabs/prysm/v5/network"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestPrivateKeyLoading(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	file, err := os.CreateTemp(t.TempDir(), "key")
	require.NoError(t, err)
	key, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
	require.NoError(t, err, "Could not generate key")
	raw, err := key.Raw()
	if err != nil {
		panic(err)
	}
	out := hex.EncodeToString(raw)

	err = os.WriteFile(file.Name(), []byte(out), params.BeaconIoConfig().ReadWritePermissions)
	require.NoError(t, err, "Could not write key to file")
	log.WithField("file", file.Name()).WithField("key", out).Info("Wrote key to file")
	cfg := &Config{
		PrivateKey: file.Name(),
	}
	pKey, err := privKey(cfg)
	require.NoError(t, err, "Could not apply option")
	newPkey, err := ecdsaprysm.ConvertToInterfacePrivkey(pKey)
	require.NoError(t, err)
	rawBytes, err := key.Raw()
	require.NoError(t, err)
	newRaw, err := newPkey.Raw()
	require.NoError(t, err)
	assert.DeepEqual(t, rawBytes, newRaw, "Private keys do not match")
}

func TestPrivateKeyLoading_StaticPrivateKey(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	tempDir := t.TempDir()

	cfg := &Config{
		StaticPeerID: true,
		DataDir:      tempDir,
	}
	pKey, err := privKey(cfg)
	require.NoError(t, err, "Could not apply option")

	newPkey, err := ecdsaprysm.ConvertToInterfacePrivkey(pKey)
	require.NoError(t, err)

	retrievedKey, err := privKeyFromFile(path.Join(tempDir, keyPath))
	require.NoError(t, err)
	retrievedPKey, err := ecdsaprysm.ConvertToInterfacePrivkey(retrievedKey)
	require.NoError(t, err)

	rawBytes, err := retrievedPKey.Raw()
	require.NoError(t, err)
	newRaw, err := newPkey.Raw()
	require.NoError(t, err)
	assert.DeepEqual(t, rawBytes, newRaw, "Private keys do not match")
}

func TestIPV6Support(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	key, err := gethCrypto.GenerateKey()
	require.NoError(t, err)
	db, err := enode.OpenDB("")
	if err != nil {
		log.Error("could not open node's peer database")
	}
	lNode := enode.NewLocalNode(db, key)
	mockIPV6 := net.IP{0xff, 0x02, 0xAA, 0, 0x1F, 0, 0x2E, 0, 0, 0x36, 0x45, 0, 0, 0, 0, 0x02}
	lNode.Set(enr.IP(mockIPV6))
	mas, err := retrieveMultiAddrsFromNode(lNode.Node())
	if err != nil {
		t.Fatal(err)
	}

	for _, ma := range mas {
		ipv6Exists := false
		for _, p := range ma.Protocols() {
			if p.Name == "ip4" {
				t.Error("Got ip4 address instead of ip6")
			}
			if p.Name == "ip6" {
				ipv6Exists = true
			}
		}
		if !ipv6Exists {
			t.Error("Multiaddress did not have ipv6 protocol")
		}
	}
}

func TestDefaultMultiplexers(t *testing.T) {
	var cfg libp2p.Config
	_ = cfg
	p2pCfg := &Config{
		UDPPort:       2000,
		TCPPort:       3000,
		QUICPort:      3000,
		StateNotifier: &mock.MockStateNotifier{},
	}
	svc := &Service{cfg: p2pCfg}
	var err error
	svc.privKey, err = privKey(svc.cfg)
	assert.NoError(t, err)
	ipAddr := network.IPAddr()
	opts, err := svc.buildOptions(ipAddr, svc.privKey)
	assert.NoError(t, err)

	err = cfg.Apply(append(opts, libp2p.FallbackDefaults)...)
	assert.NoError(t, err)

	assert.Equal(t, protocol.ID("/yamux/1.0.0"), cfg.Muxers[0].ID)
	assert.Equal(t, protocol.ID("/mplex/6.7.0"), cfg.Muxers[1].ID)
}

func TestMultiAddressBuilderWithID(t *testing.T) {
	testCases := []struct {
		name     string
		ip       net.IP
		protocol internetProtocol
		port     uint
		id       string

		expectedMultiaddrStr string
	}{
		{
			name:     "UDP",
			ip:       net.IPv4(192, 168, 0, 1),
			protocol: udp,
			port:     5678,
			id:       "0025080212210204fb1ebb1aa467527d34306a4794a5171d6516405e720b909b7f816d63aef96a",

			expectedMultiaddrStr: "/ip4/192.168.0.1/udp/5678/p2p/16Uiu2HAkum7hhuMpWqFj3yNLcmQBGmThmqw2ohaCRThXQuKU9ohs",
		},
		{
			name:     "TCP",
			ip:       net.IPv4(192, 168, 0, 1),
			protocol: tcp,
			port:     5678,
			id:       "0025080212210204fb1ebb1aa467527d34306a4794a5171d6516405e720b909b7f816d63aef96a",

			expectedMultiaddrStr: "/ip4/192.168.0.1/tcp/5678/p2p/16Uiu2HAkum7hhuMpWqFj3yNLcmQBGmThmqw2ohaCRThXQuKU9ohs",
		},
		{
			name:     "QUIC",
			ip:       net.IPv4(192, 168, 0, 1),
			protocol: quic,
			port:     5678,
			id:       "0025080212210204fb1ebb1aa467527d34306a4794a5171d6516405e720b909b7f816d63aef96a",

			expectedMultiaddrStr: "/ip4/192.168.0.1/udp/5678/quic-v1/p2p/16Uiu2HAkum7hhuMpWqFj3yNLcmQBGmThmqw2ohaCRThXQuKU9ohs",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			id, err := hex.DecodeString(tt.id)
			require.NoError(t, err)

			actualMultiaddr, err := multiAddressBuilderWithID(tt.ip, tt.protocol, tt.port, peer.ID(id))
			require.NoError(t, err)

			actualMultiaddrStr := actualMultiaddr.String()
			require.Equal(t, tt.expectedMultiaddrStr, actualMultiaddrStr)
		})
	}
}
