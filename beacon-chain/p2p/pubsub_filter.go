package p2p

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/encoder"
	"github.com/OffchainLabs/prysm/v6/config/params"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

var _ pubsub.SubscriptionFilter = (*Service)(nil)

// It is set at this limit to handle the possibility
// of double topic subscriptions at fork boundaries.
// -> BeaconBlock              * 2 = 2
// -> BeaconAggregateAndProof  * 2 = 2
// -> VoluntaryExit            * 2 = 2
// -> ProposerSlashing         * 2 = 2
// -> AttesterSlashing         * 2 = 2
// -> 64 Beacon Attestation    * 2 = 128
// -> SyncContributionAndProof * 2 = 2
// -> 4 SyncCommitteeSubnets   * 2 = 8
// -> BlsToExecutionChange     * 2 = 2
// -> 128 DataColumnSidecar    * 2 = 256
// -------------------------------------
// TOTAL                           = 406
// (Note: BlobSidecar is not included in this list since it is superseded by DataColumnSidecar)
const pubsubSubscriptionRequestLimit = 500

func (s *Service) setAllForkDigests() {
	entries := params.SortedNetworkScheduleEntries()
	s.allForkDigests = make(map[[4]byte]struct{}, len(entries))
	for _, entry := range entries {
		s.allForkDigests[entry.ForkDigest] = struct{}{}
	}
}

// CanSubscribe returns true if the topic is of interest and we could subscribe to it.
func (s *Service) CanSubscribe(topic string) bool {
	if !s.isInitialized() {
		return false
	}
	parts := strings.Split(topic, "/")
	if len(parts) != 5 {
		return false
	}
	// The topic must start with a slash, which means the first part will be empty.
	if parts[0] != "" {
		return false
	}
	if parts[1] != "eth2" {
		return false
	}

	var digest [4]byte
	dl, err := hex.Decode(digest[:], []byte(parts[2]))
	if err == nil && dl != 4 {
		err = fmt.Errorf("expected 4 bytes, got %d", dl)
	}
	if err != nil {
		log.WithError(err).WithField("topic", topic).WithField("digest", parts[2]).Error("CanSubscribe failed to parse message")
		return false
	}
	if _, ok := s.allForkDigests[digest]; !ok {
		log.WithField("topic", topic).WithField("digest", fmt.Sprintf("%#x", digest)).Error("CanSubscribe failed to find digest in allForkDigests")
		return false
	}

	if parts[4] != encoder.ProtocolSuffixSSZSnappy {
		return false
	}

	// Check the incoming topic matches any topic mapping. This includes a check for part[3].
	for gt := range gossipTopicMappings {
		if _, err := scanfcheck(strings.Join(parts[0:4], "/"), gt); err == nil {
			return true
		}
	}

	return false
}

// FilterIncomingSubscriptions is invoked for all RPCs containing subscription notifications.
// This method returns only the topics of interest and may return an error if the subscription
// request contains too many topics.
func (s *Service) FilterIncomingSubscriptions(peerID peer.ID, subs []*pubsubpb.RPC_SubOpts) ([]*pubsubpb.RPC_SubOpts, error) {
	if len(subs) > pubsubSubscriptionRequestLimit {
		subsCount := len(subs)
		log.WithFields(logrus.Fields{
			"peerID":             peerID,
			"subscriptionCounts": subsCount,
			"subscriptionLimit":  pubsubSubscriptionRequestLimit,
		}).Debug("Too many incoming subscriptions, filtering them")

		return nil, pubsub.ErrTooManySubscriptions
	}

	return pubsub.FilterSubscriptions(subs, s.CanSubscribe), nil
}

// scanfcheck uses fmt.Sscanf to check that a given string matches expected format. This method
// returns the number of formatting substitutions matched and error if the string does not match
// the expected format. Note: this method only accepts integer compatible formatting substitutions
// such as %d or %x.
func scanfcheck(input, format string) (int, error) {
	var t int
	// Sscanf requires argument pointers with the appropriate type to load the value from the input.
	// This method only checks that the input conforms to the format, the arguments are not used and
	// therefore we can reuse the same integer pointer.
	var cnt = strings.Count(format, "%")
	var args []interface{}
	for i := 0; i < cnt; i++ {
		args = append(args, &t)
	}
	return fmt.Sscanf(input, format, args...)
}
