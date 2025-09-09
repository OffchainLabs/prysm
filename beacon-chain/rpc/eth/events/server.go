// Package events defines a gRPC events service implementation,
// following the official API standards https://ethereum.github.io/beacon-apis/#/.
// This package includes the events endpoint.
package events

import (
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	opfeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/operation"
	statefeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
)

// Server defines a server implementation of the http events service,
// providing RPC endpoints to subscribe to events from the beacon node.
type Server struct {
	StateNotifier          statefeed.Notifier
	OperationNotifier      opfeed.Notifier
	HeadFetcher            blockchain.HeadFetcher
	ChainInfoFetcher       blockchain.ChainInfoFetcher
	TrackedValidatorsCache *cache.TrackedValidatorsCache
	KeepAliveInterval      time.Duration
	EventFeedDepth         int
	EventWriteTimeout      time.Duration
	StateGen               stategen.StateManager
}
