package p2p

import (
	"reflect"

	p2ptypes "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

const (
	// SchemaVersionV1 specifies the schema version for our rpc protocol ID.
	SchemaVersionV1 = "/1"

	// SchemaVersionV2 specifies the next schema version for our rpc protocol ID.
	SchemaVersionV2 = "/2"

	// SchemaVersionV3 specifies the next schema version for our rpc protocol ID.
	SchemaVersionV3 = "/3"
)

const (
	// Specifies the protocol prefix for all our Req/Resp topics.
	protocolPrefix = "/eth2/beacon_chain/req"

	// StatusMessageName specifies the name for the status message topic.
	StatusMessageName = "/status"

	// GoodbyeMessageName specifies the name for the goodbye message topic.
	GoodbyeMessageName = "/goodbye"

	// BeaconBlocksByRangeMessageName specifies the name for the beacon blocks by range message topic.
	BeaconBlocksByRangeMessageName = "/beacon_blocks_by_range"

	// BeaconBlocksByRootsMessageName specifies the name for the beacon blocks by root message topic.
	BeaconBlocksByRootsMessageName = "/beacon_blocks_by_root"

	// PingMessageName Specifies the name for the ping message topic.
	PingMessageName = "/ping"

	// MetadataMessageName specifies the name for the metadata message topic.
	MetadataMessageName = "/metadata"

	// BlobSidecarsByRangeName is the name for the BlobSidecarsByRange v1 message topic.
	BlobSidecarsByRangeName = "/blob_sidecars_by_range"

	// BlobSidecarsByRootName is the name for the BlobSidecarsByRoot v1 message topic.
	BlobSidecarsByRootName = "/blob_sidecars_by_root"

	// LightClientBootstrapName is the name for the LightClientBootstrap message topic,
	LightClientBootstrapName = "/light_client_bootstrap"

	// LightClientUpdatesByRangeName is the name for the LightClientUpdatesByRange topic.
	LightClientUpdatesByRangeName = "/light_client_updates_by_range"

	// LightClientFinalityUpdateName is the name for the LightClientFinalityUpdate topic.
	LightClientFinalityUpdateName = "/light_client_finality_update"

	// LightClientOptimisticUpdateName is the name for the LightClientOptimisticUpdate topic.
	LightClientOptimisticUpdateName = "/light_client_optimistic_update"

	// DataColumnSidecarsByRootName is the name for the DataColumnSidecarsByRoot v1 message topic.
	DataColumnSidecarsByRootName = "/data_column_sidecars_by_root"

	// DataColumnSidecarsByRangeName is the name for the DataColumnSidecarsByRange v1 message topic.
	DataColumnSidecarsByRangeName = "/data_column_sidecars_by_range"
)

const (
	// V1 RPC Topics
	// RPCStatusTopicV1 defines the v1 topic for the status rpc method.
	RPCStatusTopicV1 = protocolPrefix + StatusMessageName + SchemaVersionV1
	// RPCGoodByeTopicV1 defines the v1 topic for the goodbye rpc method.
	RPCGoodByeTopicV1 = protocolPrefix + GoodbyeMessageName + SchemaVersionV1
	// RPCBlocksByRangeTopicV1 defines v1 the topic for the blocks by range rpc method.
	RPCBlocksByRangeTopicV1 = protocolPrefix + BeaconBlocksByRangeMessageName + SchemaVersionV1
	// RPCBlocksByRootTopicV1 defines the v1 topic for the blocks by root rpc method.
	RPCBlocksByRootTopicV1 = protocolPrefix + BeaconBlocksByRootsMessageName + SchemaVersionV1
	// RPCPingTopicV1 defines the v1 topic for the ping rpc method.
	RPCPingTopicV1 = protocolPrefix + PingMessageName + SchemaVersionV1
	// RPCMetaDataTopicV1 defines the v1 topic for the metadata rpc method.
	RPCMetaDataTopicV1 = protocolPrefix + MetadataMessageName + SchemaVersionV1

	// RPCBlobSidecarsByRangeTopicV1 is a topic for requesting blob sidecars
	// in the slot range [start_slot, start_slot + count), leading up to the current head block as selected by fork choice.
	// /eth2/beacon_chain/req/blob_sidecars_by_range/1/ - New in deneb.
	RPCBlobSidecarsByRangeTopicV1 = protocolPrefix + BlobSidecarsByRangeName + SchemaVersionV1
	// RPCBlobSidecarsByRootTopicV1 is a topic for requesting blob sidecars by their block root.
	// /eth2/beacon_chain/req/blob_sidecars_by_root/1/ - New in deneb.
	RPCBlobSidecarsByRootTopicV1 = protocolPrefix + BlobSidecarsByRootName + SchemaVersionV1

	// RPCLightClientBootstrapTopicV1 is a topic for requesting a light client bootstrap.
	RPCLightClientBootstrapTopicV1 = protocolPrefix + LightClientBootstrapName + SchemaVersionV1
	// RPCLightClientUpdatesByRangeTopicV1 is a topic for requesting light client updates by range.
	RPCLightClientUpdatesByRangeTopicV1 = protocolPrefix + LightClientUpdatesByRangeName + SchemaVersionV1
	// RPCLightClientFinalityUpdateTopicV1 is a topic for requesting a light client finality update.
	RPCLightClientFinalityUpdateTopicV1 = protocolPrefix + LightClientFinalityUpdateName + SchemaVersionV1
	// RPCLightClientOptimisticUpdateTopicV1 is a topic for requesting a light client Optimistic update.
	RPCLightClientOptimisticUpdateTopicV1 = protocolPrefix + LightClientOptimisticUpdateName + SchemaVersionV1
	// RPCDataColumnSidecarsByRootTopicV1 is a topic for requesting data column sidecars by their block root.
	// /eth2/beacon_chain/req/data_column_sidecars_by_root/1 - New in Fulu.
	RPCDataColumnSidecarsByRootTopicV1 = protocolPrefix + DataColumnSidecarsByRootName + SchemaVersionV1
	// RPCDataColumnSidecarsByRangeTopicV1 is a topic for requesting data column sidecars by their slot.
	// /eth2/beacon_chain/req/data_column_sidecars_by_range/1 - New in Fulu.
	RPCDataColumnSidecarsByRangeTopicV1 = protocolPrefix + DataColumnSidecarsByRangeName + SchemaVersionV1

	// V2 RPC Topics
	// RPCStatusTopicV2 defines the v1 topic for the status rpc method.
	RPCStatusTopicV2 = protocolPrefix + StatusMessageName + SchemaVersionV2
	// RPCBlocksByRangeTopicV2 defines v2 the topic for the blocks by range rpc method.
	RPCBlocksByRangeTopicV2 = protocolPrefix + BeaconBlocksByRangeMessageName + SchemaVersionV2
	// RPCBlocksByRootTopicV2 defines the v2 topic for the blocks by root rpc method.
	RPCBlocksByRootTopicV2 = protocolPrefix + BeaconBlocksByRootsMessageName + SchemaVersionV2
	// RPCMetaDataTopicV2 defines the v2 topic for the metadata rpc method.
	RPCMetaDataTopicV2 = protocolPrefix + MetadataMessageName + SchemaVersionV2

	// V3 RPC Topics
	// RPCMetaDataTopicV3 defines the v3 topic for the metadata rpc method.
	RPCMetaDataTopicV3 = protocolPrefix + MetadataMessageName + SchemaVersionV3
)

// RPC errors for topic parsing.
const (
	invalidRPCMessageType = "provided message type doesn't have a registered mapping"
)

// RPCTopicMappings map the base message type to the rpc request.
var (
	RPCTopicMappings = map[string]interface{}{
		// RPC Status Message
		RPCStatusTopicV1: new(pb.Status),
		RPCStatusTopicV2: new(pb.StatusV2),

		// RPC Goodbye Message
		RPCGoodByeTopicV1: new(primitives.SSZUint64),

		// RPC Block By Range Message
		RPCBlocksByRangeTopicV1: new(pb.BeaconBlocksByRangeRequest),
		RPCBlocksByRangeTopicV2: new(pb.BeaconBlocksByRangeRequest),

		// RPC Block By Root Message
		RPCBlocksByRootTopicV1: new(p2ptypes.BeaconBlockByRootsReq),
		RPCBlocksByRootTopicV2: new(p2ptypes.BeaconBlockByRootsReq),

		// RPC Ping Message
		RPCPingTopicV1: new(primitives.SSZUint64),

		// RPC Metadata Message
		RPCMetaDataTopicV1: new(interface{}),
		RPCMetaDataTopicV2: new(interface{}),
		RPCMetaDataTopicV3: new(interface{}),

		// BlobSidecarsByRange v1 Message
		RPCBlobSidecarsByRangeTopicV1: new(pb.BlobSidecarsByRangeRequest),

		// BlobSidecarsByRoot v1 Message
		RPCBlobSidecarsByRootTopicV1: new(p2ptypes.BlobSidecarsByRootReq),

		// Light client
		RPCLightClientBootstrapTopicV1:        new([fieldparams.RootLength]byte),
		RPCLightClientUpdatesByRangeTopicV1:   new(pb.LightClientUpdatesByRangeRequest),
		RPCLightClientFinalityUpdateTopicV1:   new(interface{}),
		RPCLightClientOptimisticUpdateTopicV1: new(interface{}),

		// DataColumnSidecarsByRange v1 Message
		RPCDataColumnSidecarsByRangeTopicV1: new(pb.DataColumnSidecarsByRangeRequest),

		// DataColumnSidecarsByRoot v1 Message
		RPCDataColumnSidecarsByRootTopicV1: p2ptypes.DataColumnsByRootIdentifiers{},
	}

	// Maps all registered protocol prefixes.
	protocolMapping = map[string]bool{
		protocolPrefix: true,
	}

	// Maps all the protocol message names for the different rpc topics.
	messageMapping = map[string]bool{
		StatusMessageName:               true,
		GoodbyeMessageName:              true,
		BeaconBlocksByRangeMessageName:  true,
		BeaconBlocksByRootsMessageName:  true,
		PingMessageName:                 true,
		MetadataMessageName:             true,
		BlobSidecarsByRangeName:         true,
		BlobSidecarsByRootName:          true,
		LightClientBootstrapName:        true,
		LightClientUpdatesByRangeName:   true,
		LightClientFinalityUpdateName:   true,
		LightClientOptimisticUpdateName: true,
		DataColumnSidecarsByRootName:    true,
		DataColumnSidecarsByRangeName:   true,
	}

	// Maps all the RPC messages which are to updated in altair.
	altairMapping = map[string]string{
		BeaconBlocksByRangeMessageName: SchemaVersionV2,
		BeaconBlocksByRootsMessageName: SchemaVersionV2,
		MetadataMessageName:            SchemaVersionV2,
	}

	// Maps all the RPC messages which are to updated in fulu.
	fuluMapping = map[string]string{
		StatusMessageName:   SchemaVersionV2,
		MetadataMessageName: SchemaVersionV3,
	}

	versionMapping = map[string]bool{
		SchemaVersionV1: true,
		SchemaVersionV2: true,
		SchemaVersionV3: true,
	}

	// OmitContextBytesV1 keeps track of which RPC methods do not write context bytes in their v1 incarnations.
	// Phase0 did not have the notion of context bytes, which prefix wire-encoded values with a [4]byte identifier
	// to convey the schema for the receiver to use. These RPCs had a version bump to V2 when the context byte encoding
	// was introduced. For other RPC methods, context bytes are always required.
	OmitContextBytesV1 = map[string]bool{
		StatusMessageName:              true,
		GoodbyeMessageName:             true,
		BeaconBlocksByRangeMessageName: true,
		BeaconBlocksByRootsMessageName: true,
		PingMessageName:                true,
		MetadataMessageName:            true,
	}
)

// VerifyTopicMapping verifies that the topic and its accompanying
// message type is correct.
func VerifyTopicMapping(topic string, msg interface{}) error {
	msgType, ok := RPCTopicMappings[topic]
	if !ok {
		return errors.New("rpc topic is not registered currently")
	}
	receivedType := reflect.TypeOf(msg)
	registeredType := reflect.TypeOf(msgType)
	typeMatches := registeredType.AssignableTo(receivedType)

	if !typeMatches {
		return errors.Errorf("accompanying message type is incorrect for topic: wanted %v  but got %v",
			registeredType.String(), receivedType.String())
	}
	return nil
}

// TopicDeconstructor splits the provided topic to its logical sub-sections.
// It is assumed all input topics will follow the specific schema:
// /protocol-prefix/message-name/schema-version/...
// For the purposes of deconstruction, only the first 3 components are
// relevant.
func TopicDeconstructor(topic string) (string, string, string, error) {
	origTopic := topic
	protPrefix := ""
	message := ""
	version := ""

	// Iterate through all the relevant mappings to find the relevant prefixes,messages
	// and version for this topic.
	for k := range protocolMapping {
		keyLen := len(k)
		if keyLen > len(topic) {
			continue
		}
		if topic[:keyLen] == k {
			protPrefix = k
			topic = topic[keyLen:]
		}
	}

	if protPrefix == "" {
		return "", "", "", errors.Errorf("unable to find a valid protocol prefix for %s", origTopic)
	}

	for k := range messageMapping {
		keyLen := len(k)
		if keyLen > len(topic) {
			continue
		}
		if topic[:keyLen] == k {
			message = k
			topic = topic[keyLen:]
		}
	}

	if message == "" {
		return "", "", "", errors.Errorf("unable to find a valid message for %s", origTopic)
	}

	for k := range versionMapping {
		keyLen := len(k)
		if keyLen > len(topic) {
			continue
		}
		if topic[:keyLen] == k {
			version = k
			topic = topic[keyLen:]
		}
	}

	if version == "" {
		return "", "", "", errors.Errorf("unable to find a valid schema version for %s", origTopic)
	}

	return protPrefix, message, version, nil
}

// RPCTopic is a type used to denote and represent a req/resp topic.
type RPCTopic string

// ProtocolPrefix returns the protocol prefix of the rpc topic.
func (r RPCTopic) ProtocolPrefix() string {
	prefix, _, _, err := TopicDeconstructor(string(r))
	if err != nil {
		return ""
	}
	return prefix
}

// MessageType returns the message type of the rpc topic.
func (r RPCTopic) MessageType() string {
	_, message, _, err := TopicDeconstructor(string(r))
	if err != nil {
		return ""
	}
	return message
}

// Version returns the schema version of the rpc topic.
func (r RPCTopic) Version() string {
	_, _, version, err := TopicDeconstructor(string(r))
	if err != nil {
		return ""
	}
	return version
}

// TopicFromMessage constructs the rpc topic from the provided message
// type and epoch.
func TopicFromMessage(msg string, epoch primitives.Epoch) (string, error) {
	// Check if the topic is known.
	if !messageMapping[msg] {
		return "", errors.Errorf("%s: %s", invalidRPCMessageType, msg)
	}

	beaconConfig := params.BeaconConfig()

	// Check if the message is to be updated in fulu.
	if epoch >= beaconConfig.FuluForkEpoch {
		if version, ok := fuluMapping[msg]; ok {
			return protocolPrefix + msg + version, nil
		}
	}

	// Check if the message is to be updated in altair.
	if epoch >= beaconConfig.AltairForkEpoch {
		if version, ok := altairMapping[msg]; ok {
			return protocolPrefix + msg + version, nil
		}
	}

	return protocolPrefix + msg + SchemaVersionV1, nil
}
