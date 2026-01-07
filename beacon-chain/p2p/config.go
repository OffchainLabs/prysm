package p2p

import (
	"net"
	"time"

	statefeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	"github.com/sirupsen/logrus"
)

// This is the default queue size used if we have specified an invalid one.
const defaultPubsubQueueSize = 600

// Config for the p2p service. These parameters are set from application level flags
// to initialize the p2p service.
type Config struct {
	NoDiscovery           bool
	EnableUPnP            bool
	StaticPeerID          bool
	DisableLivenessCheck  bool
	StaticPeers           []string
	Discv5BootStrapAddrs  []string
	RelayNodeAddr         string
	LocalIP               string
	HostAddress           string
	HostDNS               string
	PrivateKey            string
	DataDir               string
	DiscoveryDir          string
	QUICPort              uint
	TCPPort               uint
	UDPPort               uint
	PingInterval          time.Duration
	MaxPeers              uint
	QueueSize             uint
	AllowListCIDR         string
	DenyListCIDR          []string
	IPColocationWhitelist []*net.IPNet
	StateNotifier         statefeed.Notifier
	DB                    db.ReadOnlyDatabaseWithSeqNum
	StateGen              stategen.StateManager
	ClockWaiter           startup.ClockWaiter
}

// validateConfig validates whether the provided config has valid values and sets
// the invalid ones to default.
func validateConfig(cfg *Config) {
	if cfg.QueueSize > 0 {
		return
	}

	log.WithFields(logrus.Fields{
		"queueSize": cfg.QueueSize,
		"default":   defaultPubsubQueueSize,
	}).Warning("Invalid pubsub queue size, setting the queue size to the default value")

	cfg.QueueSize = defaultPubsubQueueSize
}
