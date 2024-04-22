package sync

import (
	"fmt"
	"reflect"
	"strings"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

var errNilPubsubMessage = errors.New("nil pubsub message")
var errInvalidTopic = errors.New("invalid topic format")

func (s *Service) decodePubsubMessage(msg *pubsub.Message) (ssz.Unmarshaler, error) {
	if msg == nil || msg.Topic == nil || *msg.Topic == "" {
		return nil, errNilPubsubMessage
	}
	topic := *msg.Topic
	fDigest, err := p2p.ExtractGossipDigest(topic)
	if err != nil {
		return nil, errors.Wrapf(err, "extraction failed for topic: %s", topic)
	}
	topic = strings.TrimSuffix(topic, s.cfg.p2p.Encoding().ProtocolSuffix())
	topic, err = s.replaceForkDigest(topic)
	if err != nil {
		return nil, err
	}
	// Specially handle subnet messages.
	switch {
	case strings.Contains(topic, p2p.GossipAttestationMessage):
		topic = p2p.GossipTypeMapping[reflect.TypeOf(&ethpb.Attestation{})]
		// Given that both sync message related subnets have the same message name, we have to
		// differentiate them below.
	case strings.Contains(topic, p2p.GossipSyncCommitteeMessage) && !strings.Contains(topic, p2p.SyncContributionAndProofSubnetTopicFormat):
		topic = p2p.GossipTypeMapping[reflect.TypeOf(&ethpb.SyncCommitteeMessage{})]
	case strings.Contains(topic, p2p.GossipBlobSidecarMessage):
		topic = p2p.GossipTypeMapping[reflect.TypeOf(&ethpb.BlobSidecar{})]
	case strings.Contains(topic, p2p.GossipDataColumnSidecarMessage):
		topic = p2p.GossipTypeMapping[reflect.TypeOf(&ethpb.DataColumnSidecar{})]
	}

	base := p2p.GossipTopicMappings(topic, 0)
	if base == nil {
		return nil, p2p.ErrMessageNotMapped
	}
	m, ok := base.(ssz.Unmarshaler)
	if !ok {
		return nil, errors.Errorf("message of %T does not support marshaller interface", base)
	}
	// Handle different message types across forks.
	dt, err := extractValidDataTypeFromTopic(topic, fDigest[:], s.cfg.clock)
	if err != nil {
		return nil, err
	}
	if dt != nil {
		m = dt
	}
	if err := s.cfg.p2p.Encoding().DecodeGossip(msg.Data, m); err != nil {
		return nil, err
	}
	return m, nil
}

// Replaces our fork digest with the formatter.
func (*Service) replaceForkDigest(topic string) (string, error) {
	subStrings := strings.Split(topic, "/")
	if len(subStrings) != 4 {
		return "", errInvalidTopic
	}
	subStrings[2] = "%x"
	return strings.Join(subStrings, "/"), nil
}

func extractValidDataTypeFromTopic(topic string, digest []byte, clock *startup.Clock) (ssz.Unmarshaler, error) {
	switch topic {
	case p2p.BlockSubnetTopicFormat:
		return extractDataTypeFromTypeMap(types.BlockMap, digest, clock)
	case p2p.AttestationSubnetTopicFormat:
		return extractDataTypeFromTypeMap(types.AttestationMap, digest, clock)
	case p2p.AggregateAndProofSubnetTopicFormat:
		return extractDataTypeFromTypeMap(types.AggregateAttestationMap, digest, clock)
	}
	return nil, nil
}

func extractDataTypeFromTypeMap[T any](typeMap map[[4]byte]func() (T, error), digest []byte, tor blockchain.TemporalOracle) (T, error) {
	var zero T

	if len(digest) == 0 {
		f, ok := typeMap[bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion)]
		if !ok {
			return zero, fmt.Errorf("no %T type exists for the genesis fork version", zero)
		}
		return f()
	}
	if len(digest) != forkDigestLength {
		return zero, errors.Errorf("invalid digest returned, wanted a length of %d but received %d", forkDigestLength, len(digest))
	}
	vRoot := tor.GenesisValidatorsRoot()
	for k, f := range typeMap {
		rDigest, err := signing.ComputeForkDigest(k[:], vRoot[:])
		if err != nil {
			return zero, err
		}
		if rDigest == bytesutil.ToBytes4(digest) {
			return f()
		}
	}
	return zero, errors.Wrapf(
		ErrNoValidDigest,
		"could not extract %T data type, saw digest=%#x, genesis=%v, vr=%#x",
		zero,
		digest,
		tor.GenesisTime(),
		tor.GenesisValidatorsRoot(),
	)
}
