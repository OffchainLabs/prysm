package testing

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/config"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/x/simlibp2p"
	"github.com/marcopolo/simnet"
)

const (
	// simnetLatency is the default latency for the simulated network.
	simnetLatency = time.Millisecond

	// bandwidthLimit is the default bandwidth limit for the simulated network,
	// both for downlink and uplink.
	// Note: 10 Gbps is intended to be fast enough to not throttle the tests.
	bandwidthLimit = 10_000 * simlibp2p.OneMbps
)

// testSims maps test names to their SimNet so all TestP2P instances of a test, including its subtests.
// e.g., TestFoo and TestFoo/Bar share the same SimNet, so they can talk to each other.
var testSims sync.Map

// SimNet is a wrapper of simnet.Simnet which is an in-memory libp2p network for tests.
// Note that this is not a mock of libp2p, but a real libp2p stack (QUIC, TLS, muxing, gossipsub) over a simulated wire.
type SimNet struct {
	net   *simnet.Simnet
	count int
}

// NewSimNet starts a simulated network.
func NewSimNet(t *testing.T) *SimNet {
	nw := &simnet.Simnet{LatencyFunc: simnet.StaticLatency(simnetLatency)}
	nw.Start()
	t.Cleanup(nw.Close)
	return &SimNet{net: nw}
}

// newHost creates a libp2p host on the simulated network.
func (s *SimNet) newHost(t *testing.T, userOptions ...config.Option) host.Host {
	s.count++

	ip := simnet.IntToPublicIPv4(s.count)
	opts := []config.Option{
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/%s/udp/8000/quic-v1", ip)),
		simlibp2p.QUICSimnet(s.net, simnet.NodeBiDiLinkSettings{
			Downlink: simnet.LinkSettings{BitsPerSecond: bandwidthLimit},
			Uplink:   simnet.LinkSettings{BitsPerSecond: bandwidthLimit},
		}),
		// Identify address discovery stalls synctest.
		libp2p.DisableIdentifyAddressDiscovery(),
		libp2p.ResourceManager(&network.NullResourceManager{}),
	}
	h, err := libp2p.New(append(opts, userOptions...)...)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, h.Close()) })
	return h
}
