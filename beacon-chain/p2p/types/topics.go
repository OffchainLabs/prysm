package types

import (
	"encoding/hex"
	"strings"

	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/pkg/errors"
)

// ErrInvalidTopic is returned when a gossip topic string cannot be parsed.
var ErrInvalidTopic = errors.New("invalid topic format")

// Specifies the fixed size context length.
const digestLength = 4

// Specifies the prefix for any pubsub topic.
const gossipTopicPrefix = "/eth2/"

// ExtractGossipDigest extracts the relevant fork digest from the gossip topic.
// Topics are in the form of /eth2/{fork-digest}/{topic} and this method extracts the
// fork digest from the topic string to a 4 byte array.
func ExtractGossipDigest(topic string) ([4]byte, error) {
	// Ensure the topic prefix is correct.
	if len(topic) < len(gossipTopicPrefix)+1 || topic[:len(gossipTopicPrefix)] != gossipTopicPrefix {
		return [4]byte{}, ErrInvalidTopic
	}
	start := len(gossipTopicPrefix)
	end := strings.Index(topic[start:], "/")
	if end == -1 { // Ensure a topic suffix exists.
		return [4]byte{}, ErrInvalidTopic
	}
	end += start
	strDigest := topic[start:end]
	digest, err := hex.DecodeString(strDigest)
	if err != nil {
		return [4]byte{}, err
	}
	if len(digest) != digestLength {
		return [4]byte{}, errors.Errorf("invalid digest length wanted %d but got %d", digestLength, len(digest))
	}
	return bytesutil.ToBytes4(digest), nil
}
