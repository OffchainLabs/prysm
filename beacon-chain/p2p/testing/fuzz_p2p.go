package testing

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
	"github.com/libp2p/go-libp2p/core/control"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"google.golang.org/protobuf/proto"
)

// FakeP2P stack
type FakeP2P struct {
}

// NewFuzzTestP2P - Create a new fake p2p stack.
func NewFuzzTestP2P() *FakeP2P {
	return &FakeP2P{}
}

// Encoding -- fake.
func (*FakeP2P) Encoding() encoder.NetworkEncoding {
	return &encoder.SszNetworkEncoder{}
}

// AddConnectionHandler -- fake.
func (*FakeP2P) AddConnectionHandler(_, _ func(ctx context.Context, id peer.ID) error) {

}

// AddDisconnectionHandler -- fake.
func (*FakeP2P) AddDisconnectionHandler(_ func(ctx context.Context, id peer.ID) error) {
}

// AddPingMethod -- fake.
func (*FakeP2P) AddPingMethod(_ func(ctx context.Context, id peer.ID) error) {

}

// PeerID -- fake.
func (*FakeP2P) PeerID() peer.ID {
	return "fake"
}

// ENR returns the enr of the local peer.
func (*FakeP2P) ENR() *enr.Record {
	return new(enr.Record)
}

// NodeID returns the node id of the local peer.
func (*FakeP2P) NodeID() enode.ID {
	return enode.ID{}
}

// DiscoveryAddresses -- fake
func (*FakeP2P) DiscoveryAddresses() ([]multiaddr.Multiaddr, error) {
	return nil, nil
}

// FindAndDialPeersWithSubnets mocks the p2p func.
func (*FakeP2P) FindAndDialPeersWithSubnets(ctx context.Context, topicFormat string, digest [fieldparams.VersionLength]byte, minimumPeersPerSubnet int, subnets map[uint64]bool) error {
	return nil
}

// RefreshPersistentSubnets mocks the p2p func.
func (*FakeP2P) RefreshPersistentSubnets() {}

// LeaveTopic -- fake.
func (*FakeP2P) LeaveTopic(_ string) error {
	return nil
}

// Metadata -- fake.
func (*FakeP2P) Metadata() metadata.Metadata {
	return nil
}

// Peers -- fake.
func (*FakeP2P) Peers() *peers.Status {
	return nil
}

// PublishToTopic -- fake.
func (*FakeP2P) PublishToTopic(_ context.Context, _ string, _ []byte, _ ...pubsub.PubOpt) error {
	return nil
}

// Send -- fake.
func (*FakeP2P) Send(_ context.Context, _ interface{}, _ string, _ peer.ID) (network.Stream, error) {
	return nil, nil
}

// PubSub -- fake.
func (*FakeP2P) PubSub() *pubsub.PubSub {
	return nil
}

// MetadataSeq -- fake.
func (*FakeP2P) MetadataSeq() uint64 {
	return 0
}

// SetStreamHandler -- fake.
func (*FakeP2P) SetStreamHandler(_ string, _ network.StreamHandler) {

}

// SubscribeToTopic -- fake.
func (*FakeP2P) SubscribeToTopic(_ string, _ ...pubsub.SubOpt) (*pubsub.Subscription, error) {
	return nil, nil
}

// JoinTopic -- fake.
func (*FakeP2P) JoinTopic(_ string, _ ...pubsub.TopicOpt) (*pubsub.Topic, error) {
	return nil, nil
}

// Host -- fake.
func (*FakeP2P) Host() host.Host {
	return nil
}

// Disconnect -- fake.
func (*FakeP2P) Disconnect(_ peer.ID) error {
	return nil
}

// Broadcast -- fake.
func (*FakeP2P) Broadcast(_ context.Context, _ proto.Message) error {
	return nil
}

// BroadcastAttestation -- fake.
func (*FakeP2P) BroadcastAttestation(_ context.Context, _ uint64, _ ethpb.Att) error {
	return nil
}

// BroadcastSyncCommitteeMessage -- fake.
func (*FakeP2P) BroadcastSyncCommitteeMessage(_ context.Context, _ uint64, _ *ethpb.SyncCommitteeMessage) error {
	return nil
}

// BroadcastBlob -- fake.
func (*FakeP2P) BroadcastBlob(_ context.Context, _ uint64, _ *ethpb.BlobSidecar) error {
	return nil
}

// BroadcastLightClientOptimisticUpdate -- fake.
func (*FakeP2P) BroadcastLightClientOptimisticUpdate(_ context.Context, _ interfaces.LightClientOptimisticUpdate) error {
	return nil
}

// BroadcastLightClientFinalityUpdate -- fake.
func (*FakeP2P) BroadcastLightClientFinalityUpdate(_ context.Context, _ interfaces.LightClientFinalityUpdate) error {
	return nil
}

// BroadcastDataColumnSidecar -- fake.
func (*FakeP2P) BroadcastDataColumnSidecars(_ context.Context, _ []blocks.VerifiedRODataColumn) error {
	return nil
}

// InterceptPeerDial -- fake.
func (*FakeP2P) InterceptPeerDial(peer.ID) (allow bool) {
	return true
}

// InterceptAddrDial -- fake.
func (*FakeP2P) InterceptAddrDial(peer.ID, multiaddr.Multiaddr) (allow bool) {
	return true
}

// InterceptAccept -- fake.
func (*FakeP2P) InterceptAccept(_ network.ConnMultiaddrs) (allow bool) {
	return true
}

// InterceptSecured -- fake.
func (*FakeP2P) InterceptSecured(network.Direction, peer.ID, network.ConnMultiaddrs) (allow bool) {
	return true
}

// InterceptUpgraded -- fake.
func (*FakeP2P) InterceptUpgraded(network.Conn) (allow bool, reason control.DisconnectReason) {
	return true, 0
}

// EarliestAvailableSlot -- fake.
func (*FakeP2P) EarliestAvailableSlot(context.Context) (primitives.Slot, error) {
	return 0, nil
}

// CustodyGroupCount -- fake.
func (*FakeP2P) CustodyGroupCount(context.Context) (uint64, error) {
	return 0, nil
}

// UpdateCustodyInfo -- fake.
func (s *FakeP2P) UpdateCustodyInfo(earliestAvailableSlot primitives.Slot, custodyGroupCount uint64) (primitives.Slot, uint64, error) {
	return earliestAvailableSlot, custodyGroupCount, nil
}

// UpdateEarliestAvailableSlot -- fake.
func (*FakeP2P) UpdateEarliestAvailableSlot(earliestAvailableSlot primitives.Slot) error {
	return nil
}

// CustodyGroupCountFromPeer -- fake.
func (*FakeP2P) CustodyGroupCountFromPeer(peer.ID) uint64 {
	return 0
}
