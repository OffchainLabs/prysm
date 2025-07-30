package sync

import (
	"context"
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v6/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/network/forks"
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

type (
	// wrappedVal represents a gossip validator which also returns an error along with the result.
	wrappedVal func(context.Context, peer.ID, *pubsub.Message) (pubsub.ValidationResult, error)

	// subHandler represents handler for a given subscription.
	subHandler func(context.Context, proto.Message) error

	// parameters used for the `subscribeWithParameters` function.
	subscribeParameters struct {
		topicFormat string
		validate    wrappedVal
		handle      subHandler
		digest      [4]byte

		// getSubnetsToJoin is a function that returns all subnets the node should join.
		getSubnetsToJoin func(currentSlot primitives.Slot) map[uint64]bool

		// getSubnetsRequiringPeers is a function that returns all subnets that require peers to be found
		// but for which no subscriptions are needed.
		getSubnetsRequiringPeers func(currentSlot primitives.Slot) map[uint64]bool
	}

	// parameters used for the `subscribeToSubnets` function.
	subscribeToSubnetsParameters struct {
		subscriptionBySubnet  map[uint64]*pubsub.Subscription
		topicFormat           string
		digest                [4]byte
		genesisValidatorsRoot [fieldparams.RootLength]byte
		genesisTime           time.Time
		currentSlot           primitives.Slot
		validate              wrappedVal
		handle                subHandler
		getSubnetsToJoin      func(currentSlot primitives.Slot) map[uint64]bool
	}
)

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

// Register PubSub subscribers
func (s *Service) registerSubscribers(epoch primitives.Epoch, digest [4]byte) {
	s.subscribe(
		p2p.BlockSubnetTopicFormat,
		s.validateBeaconBlockPubSub,
		s.beaconBlockSubscriber,
		digest,
	)
	s.subscribe(
		p2p.AggregateAndProofSubnetTopicFormat,
		s.validateAggregateAndProof,
		s.beaconAggregateProofSubscriber,
		digest,
	)
	s.subscribe(
		p2p.ExitSubnetTopicFormat,
		s.validateVoluntaryExit,
		s.voluntaryExitSubscriber,
		digest,
	)
	s.subscribe(
		p2p.ProposerSlashingSubnetTopicFormat,
		s.validateProposerSlashing,
		s.proposerSlashingSubscriber,
		digest,
	)
	s.subscribe(
		p2p.AttesterSlashingSubnetTopicFormat,
		s.validateAttesterSlashing,
		s.attesterSlashingSubscriber,
		digest,
	)
	s.subscribeWithParameters(subscribeParameters{
		topicFormat:              p2p.AttestationSubnetTopicFormat,
		validate:                 s.validateCommitteeIndexBeaconAttestation,
		handle:                   s.committeeIndexBeaconAttestationSubscriber,
		digest:                   digest,
		getSubnetsToJoin:         s.persistentAndAggregatorSubnetIndices,
		getSubnetsRequiringPeers: attesterSubnetIndices,
	})

	// New gossip topic in Altair
	if params.BeaconConfig().AltairForkEpoch <= epoch {
		s.subscribe(
			p2p.SyncContributionAndProofSubnetTopicFormat,
			s.validateSyncContributionAndProof,
			s.syncContributionAndProofSubscriber,
			digest,
		)

		s.subscribeWithParameters(subscribeParameters{
			topicFormat:      p2p.SyncCommitteeSubnetTopicFormat,
			validate:         s.validateSyncCommitteeMessage,
			handle:           s.syncCommitteeMessageSubscriber,
			digest:           digest,
			getSubnetsToJoin: s.activeSyncSubnetIndices,
		})

		if features.Get().EnableLightClient {
			s.subscribe(
				p2p.LightClientOptimisticUpdateTopicFormat,
				s.validateLightClientOptimisticUpdate,
				s.lightClientOptimisticUpdateSubscriber,
				digest,
			)
			s.subscribe(
				p2p.LightClientFinalityUpdateTopicFormat,
				s.validateLightClientFinalityUpdate,
				s.lightClientFinalityUpdateSubscriber,
				digest,
			)
		}
	}

	// New gossip topic in Capella
	if params.BeaconConfig().CapellaForkEpoch <= epoch {
		s.subscribe(
			p2p.BlsToExecutionChangeSubnetTopicFormat,
			s.validateBlsToExecutionChange,
			s.blsToExecutionChangeSubscriber,
			digest,
		)
	}

	// New gossip topic in Deneb, removed in Electra
	if params.BeaconConfig().DenebForkEpoch <= epoch && epoch < params.BeaconConfig().ElectraForkEpoch {
		s.subscribeWithParameters(subscribeParameters{
			topicFormat: p2p.BlobSubnetTopicFormat,
			validate:    s.validateBlob,
			handle:      s.blobSubscriber,
			digest:      digest,
			getSubnetsToJoin: func(primitives.Slot) map[uint64]bool {
				return mapFromCount(params.BeaconConfig().BlobsidecarSubnetCount)
			},
		})
	}

	// New gossip topic in Electra, removed in Fulu
	if params.BeaconConfig().ElectraForkEpoch <= epoch && epoch < params.BeaconConfig().FuluForkEpoch {
		s.subscribeWithParameters(subscribeParameters{
			topicFormat: p2p.BlobSubnetTopicFormat,
			validate:    s.validateBlob,
			handle:      s.blobSubscriber,
			digest:      digest,
			getSubnetsToJoin: func(currentSlot primitives.Slot) map[uint64]bool {
				return mapFromCount(params.BeaconConfig().BlobsidecarSubnetCountElectra)
			},
		})
	}

	// New gossip topic in Fulu.
	if params.BeaconConfig().FuluForkEpoch <= epoch {
		s.subscribeWithParameters(subscribeParameters{
			topicFormat:      p2p.DataColumnSubnetTopicFormat,
			validate:         s.validateDataColumn,
			handle:           s.dataColumnSubscriber,
			digest:           digest,
			getSubnetsToJoin: s.dataColumnSubnetIndices,
		})
	}
}

// subscribe to a given topic with a given validator and subscription handler.
// The base protobuf message is used to initialize new messages for decoding.
func (s *Service) subscribe(topic string, validator wrappedVal, handle subHandler, digest [4]byte) *pubsub.Subscription {
	genRoot := s.cfg.clock.GenesisValidatorsRoot()
	_, e, err := forks.RetrieveForkDataFromDigest(digest, genRoot[:])
	if err != nil {
		// Impossible condition as it would mean digest does not exist.
		panic(err) // lint:nopanic -- Impossible condition.
	}
	base := p2p.GossipTopicMappings(topic, e)
	if base == nil {
		// Impossible condition as it would mean topic does not exist.
		panic(fmt.Sprintf("%s is not mapped to any message in GossipTopicMappings", topic)) // lint:nopanic -- Impossible condition.
	}
	return s.subscribeWithBase(s.addDigestToTopic(topic, digest), validator, handle)
}

func (s *Service) subscribeWithBase(topic string, validator wrappedVal, handle subHandler) *pubsub.Subscription {
	topic += s.cfg.p2p.Encoding().ProtocolSuffix()
	log := log.WithField("topic", topic)

	// Do not resubscribe already seen subscriptions.
	ok := s.subHandler.topicExists(topic)
	if ok {
		log.WithField("topic", topic).Debug("Provided topic already has an active subscription running")
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

// pruneSubscriptions unsubscribes from topics we are currently subscribed to but that are
// not in the list of wanted subnets.
// This function mutates the `subscriptionBySubnet` map, which is used to keep track of the current subscriptions.
func (s *Service) pruneSubscriptions(
	subscriptionBySubnet map[uint64]*pubsub.Subscription,
	wantedSubnets map[uint64]bool,
	topicFormat string,
	digest [4]byte,
) {
	for subnet, subscription := range subscriptionBySubnet {
		if subscription == nil {
			// Should not happen, but just in case.
			delete(subscriptionBySubnet, subnet)
			continue
		}

		if wantedSubnets[subnet] {
			// Nothing to prune.
			continue
		}

		// We are subscribed to a subnet that is no longer wanted.
		subscription.Cancel()
		fullTopic := fmt.Sprintf(topicFormat, digest, subnet) + s.cfg.p2p.Encoding().ProtocolSuffix()
		s.unSubscribeFromTopic(fullTopic)
		delete(subscriptionBySubnet, subnet)
	}
}

// subscribeToSubnets subscribes to needed subnets and unsubscribe from unneeded ones.
// This functions mutates the `subscriptionBySubnet` map, which is used to keep track of the current subscriptions.
func (s *Service) subscribeToSubnets(p subscribeToSubnetsParameters) error {
	// Do not subscribe if not synced.
	if s.chainStarted.IsSet() && s.cfg.initialSync.Syncing() {
		return nil
	}

	// Check the validity of the digest.
	valid, err := isDigestValid(p.digest, p.genesisTime, p.genesisValidatorsRoot)
	if err != nil {
		return errors.Wrap(err, "is digest valid")
	}

	// Unsubscribe from all subnets if digest is not valid. It's likely to be the case after a hard fork.
	if !valid {
		wantedSubnets := map[uint64]bool{}
		s.pruneSubscriptions(p.subscriptionBySubnet, wantedSubnets, p.topicFormat, p.digest)
		return errInvalidDigest
	}

	// Retrieve the subnets we want to join.
	subnetsToJoin := p.getSubnetsToJoin(p.currentSlot)

	// Remove subscriptions that are no longer wanted.
	s.pruneSubscriptions(p.subscriptionBySubnet, subnetsToJoin, p.topicFormat, p.digest)

	// Subscribe to wanted and not already registered subnets.
	for subnet := range subnetsToJoin {
		subnetTopic := fmt.Sprintf(p.topicFormat, p.digest, subnet)

		if _, exists := p.subscriptionBySubnet[subnet]; !exists {
			subscription := s.subscribeWithBase(subnetTopic, p.validate, p.handle)
			p.subscriptionBySubnet[subnet] = subscription
		}
	}

	return nil
}

// subscribeWithParameters subscribes to a list of subnets.
func (s *Service) subscribeWithParameters(p subscribeParameters) {
	minimumPeersPerSubnet := flags.Get().MinimumPeersPerSubnet
	subscriptionBySubnet := make(map[uint64]*pubsub.Subscription)
	genesisValidatorsRoot := s.cfg.clock.GenesisValidatorsRoot()
	genesisTime := s.cfg.clock.GenesisTime()
	secondsPerSlot := params.BeaconConfig().SecondsPerSlot
	secondsPerSlotDuration := time.Duration(secondsPerSlot) * time.Second
	currentSlot := s.cfg.clock.CurrentSlot()
	neededSubnets := computeAllNeededSubnets(currentSlot, p.getSubnetsToJoin, p.getSubnetsRequiringPeers)

	shortTopicFormat := p.topicFormat
	shortTopicFormatLen := len(shortTopicFormat)
	if shortTopicFormatLen >= 3 && shortTopicFormat[shortTopicFormatLen-3:] == "_%d" {
		shortTopicFormat = shortTopicFormat[:shortTopicFormatLen-3]
	}

	shortTopic := fmt.Sprintf(shortTopicFormat, p.digest)

	parameters := subscribeToSubnetsParameters{
		subscriptionBySubnet:  subscriptionBySubnet,
		topicFormat:           p.topicFormat,
		digest:                p.digest,
		genesisValidatorsRoot: genesisValidatorsRoot,
		genesisTime:           genesisTime,
		currentSlot:           currentSlot,
		validate:              p.validate,
		handle:                p.handle,
		getSubnetsToJoin:      p.getSubnetsToJoin,
	}

	err := s.subscribeToSubnets(parameters)
	if err != nil {
		log.WithError(err).Error("Could not subscribe to subnets")
	}

	// Subscribe to expected subnets and search for peers if needed at every slot.
	go func() {
		func() {
			ctx, cancel := context.WithTimeout(s.ctx, secondsPerSlotDuration)
			defer cancel()

			if err := s.cfg.p2p.FindAndDialPeersWithSubnets(ctx, p.topicFormat, p.digest, minimumPeersPerSubnet, neededSubnets); err != nil && !errors.Is(err, context.DeadlineExceeded) {
				log.WithError(err).Debug("Could not find peers with subnets")
			}
		}()

		slotTicker := slots.NewSlotTicker(genesisTime, secondsPerSlot)
		defer slotTicker.Done()

		for {
			select {
			case currentSlot := <-slotTicker.C():
				parameters.currentSlot = currentSlot
				if err := s.subscribeToSubnets(parameters); err != nil {
					if errors.Is(err, errInvalidDigest) {
						log.WithField("topics", shortTopic).Debug("Digest is invalid, stopping subscription")
						return
					}

					log.WithError(err).Error("Could not subscribe to subnets")
					continue
				}

				func() {
					ctx, cancel := context.WithTimeout(s.ctx, secondsPerSlotDuration)
					defer cancel()

					if err := s.cfg.p2p.FindAndDialPeersWithSubnets(ctx, p.topicFormat, p.digest, minimumPeersPerSubnet, neededSubnets); err != nil && !errors.Is(err, context.DeadlineExceeded) {
						log.WithError(err).Debug("Could not find peers with subnets")
					}
				}()

			case <-s.ctx.Done():
				return
			}
		}
	}()

	// Warn the user if we are not subscribed to enough peers in the subnets.
	go func() {
		log := log.WithField("minimum", minimumPeersPerSubnet)
		logTicker := time.NewTicker(5 * time.Minute)
		defer logTicker.Stop()

		for {
			select {
			case <-logTicker.C:
				subnetsToFindPeersIndex := computeAllNeededSubnets(currentSlot, p.getSubnetsToJoin, p.getSubnetsRequiringPeers)

				isSubnetWithMissingPeers := false
				// Find new peers for wanted subnets if needed.
				for index := range subnetsToFindPeersIndex {
					topic := fmt.Sprintf(p.topicFormat, p.digest, index)

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
					log.WithField("topic", shortTopic).Info("All subnets have enough connected peers")
				}

			case <-s.ctx.Done():
				return
			}
		}
	}()
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
func (s *Service) connectedPeersCount(subnetTopic string) int {
	topic := subnetTopic + s.cfg.p2p.Encoding().ProtocolSuffix()
	peersWithSubnet := s.cfg.p2p.PubSub().ListPeers(topic)
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
	beaconConfig := params.BeaconConfig()

	if flags.Get().SubscribeAllDataSubnets {
		return beaconConfig.DataColumnSidecarSubnetCount, nil
	}

	// Compute the validators custody requirement.
	validatorsCustodyRequirement, err := s.validatorsCustodyRequirement()
	if err != nil {
		return 0, errors.Wrap(err, "validators custody requirement")
	}

	custodyGroupCount, err := s.cfg.p2p.CustodyGroupCount()
	if err != nil {
		return 0, errors.Wrap(err, "custody group count")
	}

	return max(beaconConfig.SamplesPerSlot, validatorsCustodyRequirement, custodyGroupCount), nil
}

func (s *Service) persistentAndAggregatorSubnetIndices(currentSlot primitives.Slot) map[uint64]bool {
	if flags.Get().SubscribeToAllSubnets {
		return mapFromCount(params.BeaconConfig().AttestationSubnetCount)
	}

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
	genRoot := s.cfg.clock.GenesisValidatorsRoot()
	return forks.CreateForkDigest(s.cfg.clock.GenesisTime(), genRoot[:])
}

// Checks if the provided digest matches up with the current supposed digest.
func isDigestValid(digest [4]byte, genesis time.Time, genValRoot [32]byte) (bool, error) {
	retDigest, err := forks.CreateForkDigest(genesis, genValRoot[:])
	if err != nil {
		return false, err
	}
	isNextEpoch, err := forks.IsForkNextEpoch(genesis, genValRoot[:])
	if err != nil {
		return false, err
	}
	// In the event there is a fork the next epoch,
	// we skip the check, as we subscribe subnets an
	// epoch in advance.
	if isNextEpoch {
		return true, nil
	}
	return retDigest == digest, nil
}

// computeAllNeededSubnets computes the subnets we want to join
// and the subnets for which we want to find peers.
func computeAllNeededSubnets(
	currentSlot primitives.Slot,
	getSubnetsToJoin func(currentSlot primitives.Slot) map[uint64]bool,
	getSubnetsRequiringPeers func(currentSlot primitives.Slot) map[uint64]bool,
) map[uint64]bool {
	// Retrieve the subnets we want to join.
	subnetsToJoin := getSubnetsToJoin(currentSlot)

	// Retrieve the subnets we want to find peers into.
	subnetsRequiringPeers := make(map[uint64]bool)
	if getSubnetsRequiringPeers != nil {
		subnetsRequiringPeers = getSubnetsRequiringPeers(currentSlot)
	}

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
