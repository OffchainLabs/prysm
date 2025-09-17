package p2p

import (
	"net"
	"time"

	statefeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
)

// This is the default queue size used if we have specified an invalid one.
const defaultPubsubQueueSize = 600
const (
	// defaultConnManagerPruneAbove sets the number of peers where ConnectionManager
	// will begin to internally prune peers. This value is set based on the internal
	// value of the libp2p DefaultConectionManager "high water mark". The "low water mark"
	// is the number of peers where ConnManager will stop pruning. This value is computed
	// by subtracting connManagerPruneAmount from the high water mark.
	defaultConnManagerPruneAbove = 192
	connManagerPruneAmount       = 32
)

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
	ClockWaiter           startup.ClockWaiter
}

// connManagerLowHigh picks the low and high water marks for the connection manager based
// on the MaxPeers setting. The high water mark will be at least the default high water mark
// (192), or MaxPeers + 32, whichever is higher. The low water mark is set to be 32 less than
// the high water mark. This is done to ensure the ConnManager never prunes peers that the
// node has connected to based on the MaxPeers setting.
func (cfg *Config) connManagerLowHigh() (int, int) {
	maxPeersPlusMargin := int(cfg.MaxPeers) + connManagerPruneAmount
	high := max(maxPeersPlusMargin, defaultConnManagerPruneAbove)
	low := high - connManagerPruneAmount
	return low, high
}

// validateConfig validates whether the values provided are accurate and will set
// the appropriate values for those that are invalid.
func validateConfig(cfg *Config) *Config {
	if cfg.QueueSize == 0 {
		log.Warnf("Invalid pubsub queue size of %d initialized, setting the quese size as %d instead", cfg.QueueSize, defaultPubsubQueueSize)
		cfg.QueueSize = defaultPubsubQueueSize
	}
	return cfg
}
