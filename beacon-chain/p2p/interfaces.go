package p2p

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/encoder"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1/metadata"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"google.golang.org/protobuf/proto"
)

type (
	// P2P represents the full p2p interface composed of all of the sub-interfaces.
	P2P interface {
		Broadcaster
		SetStreamHandler
		PubSubProvider
		PubSubTopicUser
		SenderEncoder
		PeerManager
		ConnectionHandler
		PeersProvider
		MetadataProvider
		CustodyManager
	}

	// Accessor provides access to the Broadcaster, PeerManager and CustodyManager interfaces.
	Accessor interface {
		Broadcaster
		PeerManager
		CustodyManager
	}

	// Broadcaster broadcasts messages to peers over the p2p pubsub protocol.
	Broadcaster interface {
		Broadcast(context.Context, proto.Message) error
		BroadcastAttestation(ctx context.Context, subnet uint64, att ethpb.Att) error
		BroadcastSyncCommitteeMessage(ctx context.Context, subnet uint64, sMsg *ethpb.SyncCommitteeMessage) error
		BroadcastBlob(ctx context.Context, subnet uint64, blob *ethpb.BlobSidecar) error
		BroadcastLightClientOptimisticUpdate(ctx context.Context, update interfaces.LightClientOptimisticUpdate) error
		BroadcastLightClientFinalityUpdate(ctx context.Context, update interfaces.LightClientFinalityUpdate) error
		BroadcastDataColumnSidecars(ctx context.Context, sidecars []blocks.VerifiedRODataColumn) error
	}

	// SetStreamHandler configures p2p to handle streams of a certain topic ID.
	SetStreamHandler interface {
		SetStreamHandler(topic string, handler network.StreamHandler)
	}

	// PubSubTopicUser provides way to join, use and leave PubSub topics.
	PubSubTopicUser interface {
		JoinTopic(topic string, opts ...pubsub.TopicOpt) (*pubsub.Topic, error)
		LeaveTopic(topic string) error
		PublishToTopic(ctx context.Context, topic string, data []byte, opts ...pubsub.PubOpt) error
		SubscribeToTopic(topic string, opts ...pubsub.SubOpt) (*pubsub.Subscription, error)
	}

	// ConnectionHandler configures p2p to handle connections with a peer.
	ConnectionHandler interface {
		AddConnectionHandler(f func(ctx context.Context, id peer.ID) error,
			j func(ctx context.Context, id peer.ID) error)
		AddDisconnectionHandler(f func(ctx context.Context, id peer.ID) error)
		connmgr.ConnectionGater
	}

	// SenderEncoder allows sending functionality from libp2p as well as encoding for requests and responses.
	SenderEncoder interface {
		EncodingProvider
		Sender
	}

	// EncodingProvider provides p2p network encoding.
	EncodingProvider interface {
		Encoding() encoder.NetworkEncoding
	}

	// PubSubProvider provides the p2p pubsub protocol.
	PubSubProvider interface {
		PubSub() *pubsub.PubSub
	}

	// PeerManager abstracts some peer management methods from libp2p.
	PeerManager interface {
		Disconnect(peer.ID) error
		PeerID() peer.ID
		Host() host.Host
		ENR() *enr.Record
		NodeID() enode.ID
		DiscoveryAddresses() ([]multiaddr.Multiaddr, error)
		RefreshPersistentSubnets()
		FindAndDialPeersWithSubnets(ctx context.Context, topicFormat string, digest [fieldparams.VersionLength]byte, minimumPeersPerSubnet int, subnets map[uint64]bool) error
		AddPingMethod(reqFunc func(ctx context.Context, id peer.ID) error)
	}

	// Sender abstracts the sending functionality from libp2p.
	Sender interface {
		Send(context.Context, interface{}, string, peer.ID) (network.Stream, error)
	}

	// PeersProvider abstracts obtaining our current list of known peers status.
	PeersProvider interface {
		Peers() *peers.Status
	}

	// MetadataProvider returns the metadata related information for the local peer.
	MetadataProvider interface {
		Metadata() metadata.Metadata
		MetadataSeq() uint64
	}

	// CustodyManager abstracts some data columns related methods.
	CustodyManager interface {
		EarliestAvailableSlot(ctx context.Context) (primitives.Slot, error)
		CustodyGroupCount(ctx context.Context) (uint64, error)
		UpdateCustodyInfo(earliestAvailableSlot primitives.Slot, custodyGroupCount uint64) (primitives.Slot, uint64, error)
		UpdateEarliestAvailableSlot(earliestAvailableSlot primitives.Slot) error
		CustodyGroupCountFromPeer(peer.ID) uint64
	}
)
