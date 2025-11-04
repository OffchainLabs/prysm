package sync

import (
	"context"
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/messagehandler"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/common/hexutil"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

const pubsubMessageTimeout = 30 * time.Second

var errInvalidDigest = errors.New("invalid digest")

func familyLogFields(tf GossipsubTopicFamilyWithDynamicSubnets) logrus.Fields {
	nse := tf.NetworkScheduleEntry()
	return logrus.Fields{
		"topicFamily": fmt.Sprintf("%T", tf),
		"digest":      nse.ForkDigest,
		"forkEpoch":   nse.Epoch,
	}
}

// subnetTracker keeps track of which subnets we are subscribed to for a given
// dynamic topic family (attestations, sync-committee, data-column, etc.).
type subnetTracker struct {
	family        GossipsubTopicFamilyWithDynamicSubnets
	mu            sync.RWMutex
	subscriptions map[uint64]*pubsub.Subscription
}

func newSubnetTracker(tf GossipsubTopicFamilyWithDynamicSubnets) *subnetTracker {
	return &subnetTracker{
		family:        tf,
		subscriptions: make(map[uint64]*pubsub.Subscription),
	}
}

// unwanted takes a list of wanted subnets and returns a list of currently subscribed subnets that are not included.
func (t *subnetTracker) unwanted(wanted map[uint64]bool) []uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	unwanted := make([]uint64, 0, len(t.subscriptions))
	for subnet := range t.subscriptions {
		if wanted == nil || !wanted[subnet] {
			unwanted = append(unwanted, subnet)
		}
	}
	return unwanted
}

// missing takes a list of wanted subnets and returns a list of wanted subnets that are not currently tracked.
func (t *subnetTracker) missing(wanted map[uint64]bool) []uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	missing := make([]uint64, 0, len(wanted))
	for subnet := range wanted {
		if _, ok := t.subscriptions[subnet]; !ok {
			missing = append(missing, subnet)
		}
	}
	return missing
}

// cancelSubscription cancels and removes the subscription for a given subnet.
func (t *subnetTracker) cancelSubscription(subnet uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	defer delete(t.subscriptions, subnet)

	sub := t.subscriptions[subnet]
	if sub == nil {
		return
	}
	sub.Cancel()
}

// track asks subscriptionTracker to hold on to the subscription for a given subnet so
// that we can remember that it is tracked and cancel its context when it's time to unsubscribe.
func (t *subnetTracker) track(subnet uint64, sub *pubsub.Subscription) {
	if sub == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.subscriptions[subnet] = sub
}

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

func (s *Service) subscriptionRequestExpired(nse params.NetworkScheduleEntry) bool {
	next := params.NextNetworkScheduleEntry(nse.Epoch)
	return next.Epoch != nse.Epoch && s.cfg.clock.CurrentEpoch() > next.Epoch
}

func (s *Service) subscribe(topic string, validator wrappedVal, handle subHandler) *pubsub.Subscription {
	log := log.WithField("topic", topic)

	// Do not resubscribe already seen subscriptions.
	ok := s.subHandler.topicExists(topic)
	if ok {
		log.WithField("topic", topic).Error("Provided topic already has an active subscription running")
		return nil
	}

	if err := s.cfg.p2p.PubSub().RegisterTopicValidator(s.wrapAndReportValidation(topic, validator)); err != nil {
		log.WithError(err).Error("Could not register validator for topic")
		return nil
	}

	sub, err := s.cfg.p2p.SubscribeToTopic(topic)
	if err != nil {
		// Any error subscribing to a PubSub topic would be the result of a misconfiguration of
		// libp2p PubSub library or a subscription request to a topic that fails to match the topic
		// subscription filter.
		log.WithError(err).Error("Could not subscribe topic")
		return nil
	}

	s.subHandler.addTopic(sub.Topic(), sub)

	// Pipeline decodes the incoming subscription data, runs the validation, and handles the
	// message.
	pipeline := func(msg *pubsub.Message) {
		ctx, cancel := context.WithTimeout(s.ctx, pubsubMessageTimeout)
		defer cancel()

		ctx, span := trace.StartSpan(ctx, "sync.pubsub")
		defer span.End()

		defer func() {
			if r := recover(); r != nil {
				tracing.AnnotateError(span, fmt.Errorf("panic occurred: %v", r))
				log.WithField("error", r).
					WithField("recoveredAt", "subscribeWithBase").
					WithField("stack", string(debug.Stack())).
					Error("Panic occurred")
			}
		}()

		span.SetAttributes(trace.StringAttribute("topic", topic))

		if msg.ValidatorData == nil {
			log.Error("Received nil message on pubsub")
			messageFailedProcessingCounter.WithLabelValues(topic).Inc()
			return
		}

		if err := handle(ctx, msg.ValidatorData.(proto.Message)); err != nil {
			tracing.AnnotateError(span, err)
			log.WithError(err).Error("Could not handle p2p pubsub")
			messageFailedProcessingCounter.WithLabelValues(topic).Inc()
			return
		}
	}

	// The main message loop for receiving incoming messages from this subscription.
	messageLoop := func() {
		for {
			msg, err := sub.Next(s.ctx)
			if err != nil {
				// This should only happen when the context is cancelled or subscription is cancelled.
				if !errors.Is(err, pubsub.ErrSubscriptionCancelled) { // Only log a warning on unexpected errors.
					log.WithError(err).Warn("Subscription next failed")
				}
				// Cancel subscription in the event of an error, as we are
				// now exiting topic event loop.
				sub.Cancel()
				return
			}

			if msg.ReceivedFrom == s.cfg.p2p.PeerID() {
				continue
			}

			go pipeline(msg)
		}
	}

	go messageLoop()
	log.WithField("topic", topic).Info("Subscribed to")
	return sub
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

// pruneNotWanted unsubscribes from topics we are currently subscribed to but that are
// not in the list of wanted subnets.
func (s *Service) pruneNotWanted(t *subnetTracker, wantedSubnets map[uint64]bool) {
	for _, subnet := range t.unwanted(wantedSubnets) {
		t.cancelSubscription(subnet)
		s.unSubscribeFromTopic(t.family.GetFullTopicString(subnet))
	}
}

// subscribeToDynamicSubnetFamily subscribes to a list of subnets.
func (s *Service) subscribeToDynamicSubnetFamily(tf GossipsubTopicFamilyWithDynamicSubnets) *subnetTracker {
	tracker := newSubnetTracker(tf)
	go s.subscribeToSubnets(tf, tracker)
	return tracker
}

func (s *Service) subscribeToSubnets(tf GossipsubTopicFamilyWithDynamicSubnets, tracker *subnetTracker) {
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	go s.ensurePeers(ctx, tracker)
	go s.logMinimumPeersPerSubnet(ctx, tf)

	s.trySubscribeSubnets(tracker)

	slotTicker := slots.NewSlotTicker(s.cfg.clock.GenesisTime(), params.BeaconConfig().SecondsPerSlot)
	defer slotTicker.Done()
	for {
		select {
		case <-slotTicker.C():
			// Check if this subscribe request is still valid - we may have crossed another fork epoch while waiting for initial sync.
			if s.subscriptionRequestExpired(tf.NetworkScheduleEntry()) {
				// If we are already past the next fork epoch, do not subscribe to this topic.
				log.WithFields(logrus.Fields{
					"topicFamily":  fmt.Sprintf("%T", tf),
					"digest":       tf.NetworkScheduleEntry().ForkDigest,
					"epoch":        tf.NetworkScheduleEntry().Epoch,
					"currentEpoch": s.cfg.clock.CurrentEpoch(),
				}).Debug("Exiting topic subnet subscription loop")
				return
			}
			s.trySubscribeSubnets(tracker)
		case <-s.ctx.Done():
			return
		}
	}
}

// trySubscribeSubnets attempts to subscribe to any missing subnets that we should be subscribed to.
// Only if initial sync is complete.
func (s *Service) trySubscribeSubnets(t *subnetTracker) {
	subnetsToJoin := t.family.GetSubnetsToJoin(s.cfg.clock.CurrentSlot())
	s.pruneNotWanted(t, subnetsToJoin)
	for _, subnet := range t.missing(subnetsToJoin) {
		topic := t.family.GetFullTopicString(subnet)
		t.track(subnet, s.subscribe(topic, t.family.Validator(), t.family.Handler()))
	}
}

func (s *Service) ensurePeers(ctx context.Context, tracker *subnetTracker) {
	// Try once immediately so we don't have to wait until the next slot.
	s.tryEnsurePeers(ctx, tracker)

	oncePerSlot := slots.NewSlotTicker(s.cfg.clock.GenesisTime(), params.BeaconConfig().SecondsPerSlot)
	defer oncePerSlot.Done()
	for {
		select {
		case <-oncePerSlot.C():
			s.tryEnsurePeers(ctx, tracker)
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) tryEnsurePeers(ctx context.Context, tracker *subnetTracker) {
	timeout := (time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second) - 100*time.Millisecond
	minPeers := flags.Get().MinimumPeersPerSubnet
	neededSubnets := computeAllNeededSubnets(s.cfg.clock.CurrentSlot(), tracker.family)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	builder := func(idx uint64) string { return tracker.family.GetFullTopicString(idx) }
	err := s.cfg.p2p.FindAndDialPeersWithSubnets(ctx, builder, minPeers, neededSubnets)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		log.WithFields(familyLogFields(tracker.family)).WithError(err).Debug("Could not find peers with subnets")
	}
}

func (s *Service) logMinimumPeersPerSubnet(ctx context.Context, tf GossipsubTopicFamilyWithDynamicSubnets) {
	logFields := familyLogFields(tf)
	minimumPeersPerSubnet := flags.Get().MinimumPeersPerSubnet
	// Warn the user if we are not subscribed to enough peers in the subnets.
	log := log.WithField("minimum", minimumPeersPerSubnet)
	logTicker := time.NewTicker(5 * time.Minute)
	defer logTicker.Stop()

	for {
		select {
		case <-logTicker.C:
			currentSlot := s.cfg.clock.CurrentSlot()
			subnetsToFindPeersIndex := computeAllNeededSubnets(currentSlot, tf)

			isSubnetWithMissingPeers := false
			// Find new peers for wanted subnets if needed.
			for index := range subnetsToFindPeersIndex {
				topic := tf.GetFullTopicString(index)

				// Check if we have enough peers in the subnet. Skip if we do.
				if count := s.connectedPeersCount(topic); count < minimumPeersPerSubnet {
					isSubnetWithMissingPeers = true
					log.WithFields(logrus.Fields{
						"topic":  topic,
						"actual": count,
					}).Warning("Not enough connected peers")
				}
			}
			if !isSubnetWithMissingPeers {
				log.WithFields(logFields).Debug("All subnets have enough connected peers")
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) unSubscribeFromTopic(topic string) {
	log.WithField("topic", topic).Info("Unsubscribed from")
	if err := s.cfg.p2p.PubSub().UnregisterTopicValidator(topic); err != nil {
		log.WithError(err).Error("Could not unregister topic validator")
	}
	sub := s.subHandler.subForTopic(topic)
	if sub != nil {
		sub.Cancel()
	}
	s.subHandler.removeTopic(topic)
	if err := s.cfg.p2p.LeaveTopic(topic); err != nil {
		log.WithError(err).Error("Unable to leave topic")
	}
}

// connectedPeersCount counts how many peer for a given topic are connected to the node.
func (s *Service) connectedPeersCount(fullTopic string) int {
	peersWithSubnet := s.cfg.p2p.PubSub().ListPeers(fullTopic)
	return len(peersWithSubnet)
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
// the validators custody requirement, and whether the node is subscribed to all data subnets.
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/das-core.md#custody-sampling
func (s *Service) samplingSize() (uint64, error) {
	cfg := params.BeaconConfig()

	if flags.Get().SubscribeAllDataSubnets {
		return cfg.DataColumnSidecarSubnetCount, nil
	}

	// Compute the validators custody requirement.
	validatorsCustodyRequirement, err := s.validatorsCustodyRequirement()
	if err != nil {
		return 0, errors.Wrap(err, "validators custody requirement")
	}

	custodyGroupCount, err := s.cfg.p2p.CustodyGroupCount(s.ctx)
	if err != nil {
		return 0, errors.Wrap(err, "custody group count")
	}

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

	topic := p2p.GossipTypeMapping[reflect.TypeOf(&ethpb.Attestation{})]

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

// computeAllNeededSubnets computes the subnets we want to join
// and the subnets for which we want to find peers.
func computeAllNeededSubnets(
	currentSlot primitives.Slot,
	dtf GossipsubTopicFamilyWithDynamicSubnets,
) map[uint64]bool {
	// Retrieve the subnets we want to join.
	subnetsToJoin := dtf.GetSubnetsToJoin(currentSlot)

	// Retrieve the subnets we want to find peers into.
	subnetsRequiringPeers := dtf.GetSubnetsForBroadcast(currentSlot)

	// Combine the two maps to get all needed subnets.
	neededSubnets := make(map[uint64]bool, len(subnetsToJoin)+len(subnetsRequiringPeers))
	for subnet := range subnetsToJoin {
		neededSubnets[subnet] = true
	}
	for subnet := range subnetsRequiringPeers {
		neededSubnets[subnet] = true
	}

	return neededSubnets
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
