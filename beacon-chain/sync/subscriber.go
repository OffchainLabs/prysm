package sync

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/messagehandler"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/common/hexutil"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const pubsubMessageTimeout = 30 * time.Second

var errInvalidDigest = errors.New("invalid digest")

// noopValidator is a no-op that only decodes the message, but does not check its contents.
func (s *Service) noopValidator(_ context.Context, _ peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		log.WithError(err).Debug("Could not decode message")
		return pubsub.ValidationReject, nil
	}
	msg.ValidatorData = m
	return pubsub.ValidationAccept, nil
}

func mapFromCount(count uint64) map[uint64]bool {
	result := make(map[uint64]bool, count)
	for item := range count {
		result[item] = true
	}

	return result
}

func mapFromSlice(slices ...[]uint64) map[uint64]bool {
	result := make(map[uint64]bool)

	for _, slice := range slices {
		for _, item := range slice {
			result[item] = true
		}
	}

	return result
}

func (s *Service) activeSyncSubnetIndices(currentSlot primitives.Slot) map[uint64]bool {
	if flags.Get().SubscribeToAllSubnets {
		return mapFromCount(params.BeaconConfig().SyncCommitteeSubnetCount)
	}

	currentEpoch := slots.ToEpoch(currentSlot)
	subscriptions := cache.SyncSubnetIDs.GetAllSubnets(currentEpoch)

	return mapFromSlice(subscriptions)
}

// Wrap the pubsub validator with a metric monitoring function. This function increments the
// appropriate counter if the particular message fails to validate.
func (s *Service) wrapAndReportValidation(topic string, v wrappedVal) (string, pubsub.ValidatorEx) {
	return topic, func(ctx context.Context, pid peer.ID, msg *pubsub.Message) (res pubsub.ValidationResult) {
		defer messagehandler.HandlePanic(ctx, msg)
		// Default: ignore any message that panics.
		res = pubsub.ValidationIgnore // nolint:wastedassign
		ctx, cancel := context.WithTimeout(ctx, pubsubMessageTimeout)
		defer cancel()
		messageReceivedCounter.WithLabelValues(topic).Inc()
		if msg.Topic == nil {
			messageFailedValidationCounter.WithLabelValues(topic).Inc()
			return pubsub.ValidationReject
		}
		// Ignore any messages received before chainstart.
		if s.chainStarted.IsNotSet() {
			messageIgnoredValidationCounter.WithLabelValues(topic).Inc()
			return pubsub.ValidationIgnore
		}
		retDigest, err := p2p.ExtractGossipDigest(topic)
		if err != nil {
			log.WithField("topic", topic).Errorf("Invalid topic format of pubsub topic: %v", err)
			return pubsub.ValidationIgnore
		}
		currDigest, err := s.currentForkDigest()
		if err != nil {
			log.WithField("topic", topic).Errorf("Unable to retrieve fork data: %v", err)
			return pubsub.ValidationIgnore
		}
		if currDigest != retDigest {
			log.WithField("topic", topic).Debugf("Received message from outdated fork digest %#x", retDigest)
			return pubsub.ValidationIgnore
		}
		b, err := v(ctx, pid, msg)
		// We do not penalize peers if we are hitting pubsub timeouts
		// trying to process those messages.
		if b == pubsub.ValidationReject && ctx.Err() != nil {
			b = pubsub.ValidationIgnore
		}
		if b == pubsub.ValidationReject {
			fields := logrus.Fields{
				"topic":        topic,
				"multiaddress": multiAddr(pid, s.cfg.p2p.Peers()),
				"peerID":       pid.String(),
				"agent":        agentString(pid, s.cfg.p2p.Host()),
				"gossipScore":  s.cfg.p2p.Peers().Scorers().GossipScorer().Score(pid),
			}
			if features.Get().EnableFullSSZDataLogging {
				fields["message"] = hexutil.Encode(msg.Data)
			}
			log.WithError(err).WithFields(fields).Debug("Gossip message was rejected")
			messageFailedValidationCounter.WithLabelValues(topic).Inc()
		}
		if b == pubsub.ValidationIgnore {
			if err != nil && !errorIsIgnored(err) {
				log.WithError(err).WithFields(logrus.Fields{
					"topic":        topic,
					"multiaddress": multiAddr(pid, s.cfg.p2p.Peers()),
					"peerID":       pid.String(),
					"agent":        agentString(pid, s.cfg.p2p.Host()),
					"gossipScore":  fmt.Sprintf("%.2f", s.cfg.p2p.Peers().Scorers().GossipScorer().Score(pid)),
				}).Debug("Gossip message was ignored")
			}
			messageIgnoredValidationCounter.WithLabelValues(topic).Inc()
		}
		return b
	}
}

func (s *Service) dataColumnSubnetIndices(primitives.Slot) map[uint64]bool {
	nodeID := s.cfg.p2p.NodeID()

	samplingSize, err := s.samplingSize()
	if err != nil {
		log.WithError(err).Error("Could not retrieve sampling size")
		return nil
	}

	// Compute the subnets to subscribe to.
	nodeInfo, _, err := peerdas.Info(nodeID, samplingSize)
	if err != nil {
		log.WithError(err).Error("Could not retrieve peer info")
		return nil
	}

	return nodeInfo.DataColumnsSubnets
}

// samplingSize computes the sampling size based on the samples per slot value,
// the validators custody requirement, and the custody group count.
// The custody group count is the source of truth and already includes supernode/semi-supernode logic.
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/das-core.md#custody-sampling
func (s *Service) samplingSize() (uint64, error) {
	cfg := params.BeaconConfig()

	// Compute the validators custody requirement.
	validatorsCustodyRequirement, err := s.validatorsCustodyRequirement()
	if err != nil {
		return 0, errors.Wrap(err, "validators custody requirement")
	}

	// Get custody group count - this is the source of truth and already reflects:
	// - Supernode mode: NUMBER_OF_CUSTODY_GROUPS
	// - Semi-supernode mode: half of NUMBER_OF_CUSTODY_GROUPS (or more if validators require)
	// - Regular mode: validator custody requirement
	custodyGroupCount, err := s.cfg.p2p.CustodyGroupCount(s.ctx)
	if err != nil {
		return 0, errors.Wrap(err, "custody group count")
	}

	// Sampling size should match custody to ensure we can serve what we advertise
	return max(cfg.SamplesPerSlot, validatorsCustodyRequirement, custodyGroupCount), nil
}

func (s *Service) persistentAndAggregatorSubnetIndices(currentSlot primitives.Slot) map[uint64]bool {
	persistentSubnetIndices := persistentSubnetIndices()
	aggregatorSubnetIndices := aggregatorSubnetIndices(currentSlot)

	// Combine subscriptions to get all requested subscriptions.
	return mapFromSlice(persistentSubnetIndices, aggregatorSubnetIndices)
}

// filters out required peers for the node to function, not
// pruning peers who are in our attestation subnets.
func (s *Service) filterNeededPeers(pids []peer.ID) []peer.ID {
	minimumPeersPerSubnet := flags.Get().MinimumPeersPerSubnet
	currentSlot := s.cfg.clock.CurrentSlot()

	// Exit early if nothing to filter.
	if len(pids) == 0 {
		return pids
	}

	digest, err := s.currentForkDigest()
	if err != nil {
		log.WithError(err).Error("Could not compute fork digest")
		return pids
	}

	wantedSubnets := make(map[uint64]bool)
	for subnet := range s.persistentAndAggregatorSubnetIndices(currentSlot) {
		wantedSubnets[subnet] = true
	}

	for subnet := range attesterSubnetIndices(currentSlot) {
		wantedSubnets[subnet] = true
	}

	topic := p2p.GossipTypeMapping[reflect.TypeFor[*ethpb.Attestation]()]

	alreadyProtected := make(map[string]struct{})

	// Map of peers in subnets
	peerMap := make(map[peer.ID]bool)
	for subnet := range wantedSubnets {
		subnetTopic := fmt.Sprintf(topic, digest, subnet) + s.cfg.p2p.Encoding().ProtocolSuffix()
		peers := s.cfg.p2p.PubSub().ListPeers(subnetTopic)
		if len(peers) > minimumPeersPerSubnet {
			// In the event we have more than the minimum, we can
			// mark the remaining as viable for pruning.
			peers = peers[:minimumPeersPerSubnet]
		}

		// Add peer to peer map.
		for _, peer := range peers {
			// Even if the peer ID has already been seen we still set it,
			// as the outcome is the same.
			peerMap[peer] = true
		}
		alreadyProtected[subnetTopic] = struct{}{}
	}

	dialer := s.cfg.p2p.GossipDialer()

	if dialer != nil {
		// ask the dialer for peers that should be protected from pruning.
		for _, pid := range dialer.ProtectedPeers(alreadyProtected) {
			peerMap[pid] = true
		}
	}

	// Clear out necessary peers from the peers to prune.
	newPeers := make([]peer.ID, 0, len(pids))

	for _, pid := range pids {
		if peerMap[pid] {
			continue
		}
		newPeers = append(newPeers, pid)
	}
	return newPeers
}

// Add fork digest to topic.
func (*Service) addDigestToTopic(topic string, digest [4]byte) string {
	if !strings.Contains(topic, "%x") {
		log.Error("Topic does not have appropriate formatter for digest")
	}
	return fmt.Sprintf(topic, digest)
}

// Add the digest and index to subnet topic.
func (*Service) addDigestAndIndexToTopic(topic string, digest [4]byte, idx uint64) string {
	if !strings.Contains(topic, "%x") {
		log.Error("Topic does not have appropriate formatter for digest")
	}
	return fmt.Sprintf(topic, digest, idx)
}

func (s *Service) currentForkDigest() ([4]byte, error) {
	return params.ForkDigest(s.cfg.clock.CurrentEpoch()), nil
}

// Checks if the provided digest matches up with the current supposed digest.
func isDigestValid(digest [4]byte, clock *startup.Clock) (bool, error) {
	current := clock.CurrentEpoch()
	// In the event there is a fork the next epoch,
	// we skip the check, as we subscribe subnets an
	// epoch in advance.
	if params.NextNetworkScheduleEntry(current).Epoch == current+1 {
		return true, nil
	}
	return params.ForkDigest(current) == digest, nil
}

func agentString(pid peer.ID, hst host.Host) string {
	rawVersion, storeErr := hst.Peerstore().Get(pid, "AgentVersion")
	agString, ok := rawVersion.(string)
	if storeErr != nil || !ok {
		agString = ""
	}
	return agString
}

func multiAddr(pid peer.ID, stat *peers.Status) string {
	addrs, err := stat.Address(pid)
	if err != nil || addrs == nil {
		return ""
	}
	return addrs.String()
}

func errorIsIgnored(err error) bool {
	if errors.Is(err, helpers.ErrTooLate) {
		return true
	}
	if errors.Is(err, altair.ErrTooLate) {
		return true
	}
	return false
}
