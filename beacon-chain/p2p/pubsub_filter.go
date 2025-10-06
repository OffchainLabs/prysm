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
	"github.com/pkg/errors"
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

var (
	errNotReadyToSubscribe         = fmt.Errorf("not ready to subscribe, service is not initialized")
	errMissingLeadingSlash         = fmt.Errorf("topic is missing leading slash")
	errTopicMissingProtocolVersion = fmt.Errorf("topic is missing protocol version (eth2)")
	errTopicPathWrongPartCount     = fmt.Errorf("topic path has wrong part count")
	errDigestInvalid               = fmt.Errorf("digest is invalid")
	errDigestUnexpected            = fmt.Errorf("digest is unexpected")
	errSnappySuffixMissing         = fmt.Errorf("snappy suffix is missing")
	errTopicNotFound               = fmt.Errorf("topic not found in gossip topic mappings")
)

// CanSubscribe returns true if the topic is of interest and we could subscribe to it.
func (s *Service) CanSubscribe(topic string) bool {
	if err := s.checkSubscribable(topic); err != nil {
		if !errors.Is(err, errNotReadyToSubscribe) {
			logrus.WithError(err).WithField("topic", topic).Debug("CanSubscribe failed")
		}
		return false
	}
	return true
}

func (s *Service) checkSubscribable(topic string) error {
	if !s.isInitialized() {
		return errNotReadyToSubscribe
	}
	parts := strings.Split(topic, "/")
	if len(parts) != 5 {
		return errTopicPathWrongPartCount
	}
	// The topic must start with a slash, which means the first part will be empty.
	if parts[0] != "" {
		return errMissingLeadingSlash
	}
	protocol, rawDigest, suffix := parts[1], parts[2], parts[4]
	if protocol != "eth2" {
		return errTopicMissingProtocolVersion
	}
	if suffix != encoder.ProtocolSuffixSSZSnappy {
		return errSnappySuffixMissing
	}

	var digest [4]byte
	dl, err := hex.Decode(digest[:], []byte(rawDigest))
	if err != nil {
		return errors.Wrapf(errDigestInvalid, "%v", err)
	}
	if dl != 4 {
		return errors.Wrapf(errDigestInvalid, "wrong byte length")
	}
	if _, ok := s.allForkDigests[digest]; !ok {
		return errDigestUnexpected
	}

	// Check the incoming topic matches any topic mapping. This includes a check for part[3].
	for gt := range gossipTopicMappings {
		if _, err := scanfcheck(strings.Join(parts[0:4], "/"), gt); err == nil {
			return nil
		}
	}

	return errTopicNotFound
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

	return pubsub.FilterSubscriptions(subs, s.logCheckSubscribableError(peerID)), nil
}

func (s *Service) logCheckSubscribableError(pid peer.ID) func(string) bool {
	return func(topic string) bool {
		if err := s.checkSubscribable(topic); err != nil {
			if !errors.Is(err, errNotReadyToSubscribe) {
				log.WithError(err).WithFields(logrus.Fields{
					"peerID": pid,
					"topic":  topic,
				}).Trace("Peer subscription rejected")
			}
			return false
		}
		return true
	}
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
