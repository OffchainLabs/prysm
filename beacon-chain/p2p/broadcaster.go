package p2p

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/hash"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

const minimumPeersPerSubnetForBroadcast = 1

// ErrMessageNotMapped occurs on a Broadcast attempt when a message has not been defined in the
// GossipTypeMapping.
var ErrMessageNotMapped = errors.New("message type is not mapped to a PubSub topic")

// Broadcast a message to the p2p network, the message is assumed to be
// broadcasted to the current fork.
func (s *Service) Broadcast(ctx context.Context, msg proto.Message) error {
	ctx, span := trace.StartSpan(ctx, "p2p.Broadcast")
	defer span.End()

	twoSlots := time.Duration(2*params.BeaconConfig().SecondsPerSlot) * time.Second
	ctx, cancel := context.WithTimeout(ctx, twoSlots)
	defer cancel()

	forkDigest, err := s.currentForkDigest()
	if err != nil {
		err := errors.Wrap(err, "could not retrieve fork digest")
		tracing.AnnotateError(span, err)
		return err
	}

	topic, ok := GossipTypeMapping[reflect.TypeOf(msg)]
	if !ok {
		tracing.AnnotateError(span, ErrMessageNotMapped)
		return ErrMessageNotMapped
	}
	castMsg, ok := msg.(ssz.Marshaler)
	if !ok {
		return errors.Errorf("message of %T does not support marshaller interface", msg)
	}
	return s.broadcastObject(ctx, castMsg, fmt.Sprintf(topic, forkDigest))
}

// BroadcastAttestation broadcasts an attestation to the p2p network, the message is assumed to be
// broadcasted to the current fork.
func (s *Service) BroadcastAttestation(ctx context.Context, subnet uint64, att ethpb.Att) error {
	if att == nil {
		return errors.New("attempted to broadcast nil attestation")
	}
	ctx, span := trace.StartSpan(ctx, "p2p.BroadcastAttestation")
	defer span.End()
	forkDigest, err := s.currentForkDigest()
	if err != nil {
		err := errors.Wrap(err, "could not retrieve fork digest")
		tracing.AnnotateError(span, err)
		return err
	}

	// Non-blocking broadcast, with attempts to discover a subnet peer if none available.
	go s.internalBroadcastAttestation(ctx, subnet, att, forkDigest)

	return nil
}

// BroadcastSyncCommitteeMessage broadcasts a sync committee message to the p2p network, the message is assumed to be
// broadcasted to the current fork.
func (s *Service) BroadcastSyncCommitteeMessage(ctx context.Context, subnet uint64, sMsg *ethpb.SyncCommitteeMessage) error {
	if sMsg == nil {
		return errors.New("attempted to broadcast nil sync committee message")
	}
	ctx, span := trace.StartSpan(ctx, "p2p.BroadcastSyncCommitteeMessage")
	defer span.End()
	forkDigest, err := s.currentForkDigest()
	if err != nil {
		err := errors.Wrap(err, "could not retrieve fork digest")
		tracing.AnnotateError(span, err)
		return err
	}

	// Non-blocking broadcast, with attempts to discover a subnet peer if none available.
	go s.broadcastSyncCommittee(ctx, subnet, sMsg, forkDigest)

	return nil
}

func (s *Service) internalBroadcastAttestation(ctx context.Context, subnet uint64, att ethpb.Att, forkDigest [fieldparams.VersionLength]byte) {
	_, span := trace.StartSpan(ctx, "p2p.internalBroadcastAttestation")
	defer span.End()
	ctx = trace.NewContext(context.Background(), span) // clear parent context / deadline.

	oneEpoch := time.Duration(1*params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot)) * time.Second
	ctx, cancel := context.WithTimeout(ctx, oneEpoch)
	defer cancel()

	// Ensure we have peers with this subnet.
	s.subnetLocker(subnet).RLock()
	hasPeer := s.hasPeerWithSubnet(attestationToTopic(subnet, forkDigest))
	s.subnetLocker(subnet).RUnlock()

	span.SetAttributes(
		trace.BoolAttribute("hasPeer", hasPeer),
		trace.Int64Attribute("slot", int64(att.GetData().Slot)), // lint:ignore uintcast -- It's safe to do this for tracing.
		trace.Int64Attribute("subnet", int64(subnet)),           // lint:ignore uintcast -- It's safe to do this for tracing.
	)

	if !hasPeer {
		attestationBroadcastAttempts.Inc()
		if err := func() error {
			s.subnetLocker(subnet).Lock()
			defer s.subnetLocker(subnet).Unlock()

			if err := s.FindAndDialPeersWithSubnets(ctx, AttestationSubnetTopicFormat, forkDigest, minimumPeersPerSubnetForBroadcast, map[uint64]bool{subnet: true}); err != nil {
				return errors.Wrap(err, "find peers with subnets")
			}

			savedAttestationBroadcasts.Inc()
			return nil
		}(); err != nil {
			log.WithError(err).Error("Failed to find peers")
			tracing.AnnotateError(span, err)
		}
	}
	// In the event our attestation is outdated and beyond the
	// acceptable threshold, we exit early and do not broadcast it.
	currSlot := slots.CurrentSlot(s.genesisTime)
	if err := helpers.ValidateAttestationTime(att.GetData().Slot, s.genesisTime, params.BeaconConfig().MaximumGossipClockDisparityDuration()); err != nil {
		log.WithFields(logrus.Fields{
			"attestationSlot": att.GetData().Slot,
			"currentSlot":     currSlot,
		}).WithError(err).Debug("Attestation is too old to broadcast, discarding it")
		return
	}

	if err := s.broadcastObject(ctx, att, attestationToTopic(subnet, forkDigest)); err != nil {
		log.WithError(err).Error("Failed to broadcast attestation")
		tracing.AnnotateError(span, err)
	}
}

func (s *Service) broadcastSyncCommittee(ctx context.Context, subnet uint64, sMsg *ethpb.SyncCommitteeMessage, forkDigest [fieldparams.VersionLength]byte) {
	_, span := trace.StartSpan(ctx, "p2p.broadcastSyncCommittee")
	defer span.End()
	ctx = trace.NewContext(context.Background(), span) // clear parent context / deadline.

	oneSlot := time.Duration(1*params.BeaconConfig().SecondsPerSlot) * time.Second
	ctx, cancel := context.WithTimeout(ctx, oneSlot)
	defer cancel()

	// Ensure we have peers with this subnet.
	// This adds in a special value to the subnet
	// to ensure that we can reuse the same subnet locker.
	wrappedSubIdx := subnet + syncLockerVal
	s.subnetLocker(wrappedSubIdx).RLock()
	hasPeer := s.hasPeerWithSubnet(syncCommitteeToTopic(subnet, forkDigest))
	s.subnetLocker(wrappedSubIdx).RUnlock()

	span.SetAttributes(
		trace.BoolAttribute("hasPeer", hasPeer),
		trace.Int64Attribute("slot", int64(sMsg.Slot)), // lint:ignore uintcast -- It's safe to do this for tracing.
		trace.Int64Attribute("subnet", int64(subnet)),  // lint:ignore uintcast -- It's safe to do this for tracing.
	)

	if !hasPeer {
		syncCommitteeBroadcastAttempts.Inc()
		if err := func() error {
			s.subnetLocker(wrappedSubIdx).Lock()
			defer s.subnetLocker(wrappedSubIdx).Unlock()
			if err := s.FindAndDialPeersWithSubnets(ctx, SyncCommitteeSubnetTopicFormat, forkDigest, minimumPeersPerSubnetForBroadcast, map[uint64]bool{subnet: true}); err != nil {
				return errors.Wrap(err, "find peers with subnets")
			}

			savedSyncCommitteeBroadcasts.Inc()
			return nil
		}(); err != nil {
			log.WithError(err).Error("Failed to find peers")
			tracing.AnnotateError(span, err)
		}
	}
	// In the event our sync message is outdated and beyond the
	// acceptable threshold, we exit early and do not broadcast it.
	if err := altair.ValidateSyncMessageTime(sMsg.Slot, s.genesisTime, params.BeaconConfig().MaximumGossipClockDisparityDuration()); err != nil {
		log.WithError(err).Warn("Sync Committee Message is too old to broadcast, discarding it")
		return
	}

	if err := s.broadcastObject(ctx, sMsg, syncCommitteeToTopic(subnet, forkDigest)); err != nil {
		log.WithError(err).Error("Failed to broadcast sync committee message")
		tracing.AnnotateError(span, err)
	}
}

// BroadcastBlob broadcasts a blob to the p2p network, the message is assumed to be
// broadcasted to the current fork and to the input subnet.
func (s *Service) BroadcastBlob(ctx context.Context, subnet uint64, blob *ethpb.BlobSidecar) error {
	ctx, span := trace.StartSpan(ctx, "p2p.BroadcastBlob")
	defer span.End()
	if blob == nil {
		return errors.New("attempted to broadcast nil blob sidecar")
	}
	forkDigest, err := s.currentForkDigest()
	if err != nil {
		err := errors.Wrap(err, "could not retrieve fork digest")
		tracing.AnnotateError(span, err)
		return err
	}

	// Non-blocking broadcast, with attempts to discover a subnet peer if none available.
	go s.internalBroadcastBlob(ctx, subnet, blob, forkDigest)

	return nil
}

func (s *Service) internalBroadcastBlob(ctx context.Context, subnet uint64, blobSidecar *ethpb.BlobSidecar, forkDigest [fieldparams.VersionLength]byte) {
	_, span := trace.StartSpan(ctx, "p2p.internalBroadcastBlob")
	defer span.End()
	ctx = trace.NewContext(context.Background(), span) // clear parent context / deadline.

	oneSlot := time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second
	ctx, cancel := context.WithTimeout(ctx, oneSlot)
	defer cancel()

	wrappedSubIdx := subnet + blobSubnetLockerVal
	s.subnetLocker(wrappedSubIdx).RLock()
	hasPeer := s.hasPeerWithSubnet(blobSubnetToTopic(subnet, forkDigest))
	s.subnetLocker(wrappedSubIdx).RUnlock()

	if !hasPeer {
		blobSidecarBroadcastAttempts.Inc()
		if err := func() error {
			s.subnetLocker(wrappedSubIdx).Lock()
			defer s.subnetLocker(wrappedSubIdx).Unlock()

			if err := s.FindAndDialPeersWithSubnets(ctx, BlobSubnetTopicFormat, forkDigest, minimumPeersPerSubnetForBroadcast, map[uint64]bool{subnet: true}); err != nil {
				return errors.Wrap(err, "find peers with subnets")
			}

			blobSidecarBroadcasts.Inc()
			return nil
		}(); err != nil {
			log.WithError(err).Error("Failed to find peers")
			tracing.AnnotateError(span, err)
		}
	}

	if err := s.broadcastObject(ctx, blobSidecar, blobSubnetToTopic(subnet, forkDigest)); err != nil {
		log.WithError(err).Error("Failed to broadcast blob sidecar")
		tracing.AnnotateError(span, err)
	}
}

func (s *Service) BroadcastLightClientOptimisticUpdate(ctx context.Context, update interfaces.LightClientOptimisticUpdate) error {
	ctx, span := trace.StartSpan(ctx, "p2p.BroadcastLightClientOptimisticUpdate")
	defer span.End()

	if update == nil || update.IsNil() {
		return errors.New("attempted to broadcast nil light client optimistic update")
	}

	// add delay to ensure block has time to propagate
	slotStart, err := slots.StartTime(s.genesisTime, update.SignatureSlot())
	if err != nil {
		err := errors.Wrap(err, "could not compute slot start time")
		tracing.AnnotateError(span, err)
		return err
	}
	timeSinceSlotStart := time.Since(slotStart)
	expectedDelay := slots.ComponentDuration(primitives.BP(params.BeaconConfig().SyncMessageDueBPS))
	if timeSinceSlotStart < expectedDelay {
		waitDuration := expectedDelay - timeSinceSlotStart
		<-time.After(waitDuration)
	}

	digest := params.ForkDigest(slots.ToEpoch(update.AttestedHeader().Beacon().Slot))
	if err := s.broadcastObject(ctx, update, lcOptimisticToTopic(digest)); err != nil {
		log.WithError(err).Debug("Failed to broadcast light client optimistic update")
		err := errors.Wrap(err, "could not publish message")
		tracing.AnnotateError(span, err)
		return err
	}
	log.Debug("Successfully broadcast light client optimistic update")

	return nil
}

func (s *Service) BroadcastLightClientFinalityUpdate(ctx context.Context, update interfaces.LightClientFinalityUpdate) error {
	ctx, span := trace.StartSpan(ctx, "p2p.BroadcastLightClientFinalityUpdate")
	defer span.End()

	if update == nil || update.IsNil() {
		return errors.New("attempted to broadcast nil light client finality update")
	}

	// add delay to ensure block has time to propagate
	slotStart, err := slots.StartTime(s.genesisTime, update.SignatureSlot())
	if err != nil {
		err := errors.Wrap(err, "could not compute slot start time")
		tracing.AnnotateError(span, err)
		return err
	}
	timeSinceSlotStart := time.Since(slotStart)
	expectedDelay := slots.ComponentDuration(primitives.BP(params.BeaconConfig().SyncMessageDueBPS))
	if timeSinceSlotStart < expectedDelay {
		waitDuration := expectedDelay - timeSinceSlotStart
		<-time.After(waitDuration)
	}

	forkDigest := params.ForkDigest(slots.ToEpoch(update.AttestedHeader().Beacon().Slot))
	if err := s.broadcastObject(ctx, update, lcFinalityToTopic(forkDigest)); err != nil {
		log.WithError(err).Debug("Failed to broadcast light client finality update")
		err := errors.Wrap(err, "could not publish message")
		tracing.AnnotateError(span, err)
		return err
	}
	log.Debug("Successfully broadcast light client finality update")

	return nil
}

// BroadcastDataColumnSidecars broadcasts multiple data column sidecars to the p2p network, after ensuring
// there is at least one peer in each needed subnet. If not, it will attempt to find one before broadcasting.
// This function is non-blocking. It stops trying to broadcast a given sidecar when more than one slot has passed, or the context is
// cancelled (whichever comes first).
func (s *Service) BroadcastDataColumnSidecars(ctx context.Context, sidecars []blocks.VerifiedRODataColumn) error {
	// Increase the number of broadcast attempts.
	dataColumnSidecarBroadcastAttempts.Add(float64(len(sidecars)))

	// Retrieve the current fork digest.
	forkDigest, err := s.currentForkDigest()
	if err != nil {
		return errors.Wrap(err, "current fork digest")
	}

	go s.broadcastDataColumnSidecars(ctx, forkDigest, sidecars)

	return nil
}

// broadcastDataColumnSidecars broadcasts multiple data column sidecars to the p2p network, after ensuring
// there is at least one peer in each needed subnet. If not, it will attempt to find one before broadcasting.
// It returns when all broadcasts are complete, or the context is cancelled (whichever comes first).
func (s *Service) broadcastDataColumnSidecars(ctx context.Context, forkDigest [fieldparams.VersionLength]byte, sidecars []blocks.VerifiedRODataColumn) {
	type rootAndIndex struct {
		root  [fieldparams.RootLength]byte
		index uint64
	}

	var (
		wg      sync.WaitGroup
		timings sync.Map
	)

	logLevel := logrus.GetLevel()

	slotPerRoot := make(map[[fieldparams.RootLength]byte]primitives.Slot, 1)
	for _, sidecar := range sidecars {
		slotPerRoot[sidecar.BlockRoot()] = sidecar.Slot()

		wg.Go(func() {
			// Add tracing to the function.
			ctx, span := trace.StartSpan(s.ctx, "p2p.broadcastDataColumnSidecars")
			defer span.End()

			// Compute the subnet for this data column sidecar.
			subnet := peerdas.ComputeSubnetForDataColumnSidecar(sidecar.Index)

			// Build the topic corresponding to subnet column subnet and this fork digest.
			topic := dataColumnSubnetToTopic(subnet, forkDigest)

			// Compute the wrapped subnet index.
			wrappedSubIdx := subnet + dataColumnSubnetVal

			// Find peers if needed.
			if err := s.findPeersIfNeeded(ctx, wrappedSubIdx, DataColumnSubnetTopicFormat, forkDigest, subnet); err != nil {
				tracing.AnnotateError(span, err)
				log.WithError(err).Error("Cannot find peers if needed")
				return
			}

			// Broadcast the data column sidecar to the network.
			if err := s.broadcastObject(ctx, sidecar, topic); err != nil {
				tracing.AnnotateError(span, err)
				log.WithError(err).Error("Cannot broadcast data column sidecar")
				return
			}

			// Increase the number of successful broadcasts.
			dataColumnSidecarBroadcasts.Inc()

			// Record the timing for log purposes.
			if logLevel >= logrus.DebugLevel {
				root := sidecar.BlockRoot()
				timings.Store(rootAndIndex{root: root, index: sidecar.Index}, time.Now())
			}
		})
	}

	// Wait for all broadcasts to finish.
	wg.Wait()

	// The rest of this function is only for debug logging purposes.
	if logLevel < logrus.DebugLevel {
		return
	}

	type logInfo struct {
		durationMin time.Duration
		durationMax time.Duration
		indices     []uint64
	}

	logInfoPerRoot := make(map[[fieldparams.RootLength]byte]*logInfo, 1)

	timings.Range(func(key any, value any) bool {
		rootAndIndex, ok := key.(rootAndIndex)
		if !ok {
			log.Error("Could not cast key to rootAndIndex")
			return true
		}

		broadcastTime, ok := value.(time.Time)
		if !ok {
			log.Error("Could not cast value to time.Time")
			return true
		}

		slot, ok := slotPerRoot[rootAndIndex.root]
		if !ok {
			log.WithField("root", fmt.Sprintf("%#x", rootAndIndex.root)).Error("Could not find slot for root")
			return true
		}

		duration, err := slots.SinceSlotStart(slot, s.genesisTime, broadcastTime)
		if err != nil {
			log.WithError(err).Error("Could not compute duration since slot start")
			return true
		}

		info, ok := logInfoPerRoot[rootAndIndex.root]
		if !ok {
			logInfoPerRoot[rootAndIndex.root] = &logInfo{durationMin: duration, durationMax: duration, indices: []uint64{rootAndIndex.index}}
			return true
		}

		info.durationMin = min(info.durationMin, duration)
		info.durationMax = max(info.durationMax, duration)
		info.indices = append(info.indices, rootAndIndex.index)

		return true
	})

	for root, info := range logInfoPerRoot {
		slices.Sort(info.indices)

		log.WithFields(logrus.Fields{
			"root":                  fmt.Sprintf("%#x", root),
			"slot":                  slotPerRoot[root],
			"count":                 len(info.indices),
			"indices":               helpers.PrettySlice(info.indices),
			"timeSinceSlotStartMin": info.durationMin,
			"timeSinceSlotStartMax": info.durationMax,
		}).Debug("Broadcasted data column sidecars")
	}
}

func (s *Service) findPeersIfNeeded(
	ctx context.Context,
	wrappedSubIdx uint64,
	topicFormat string,
	forkDigest [fieldparams.VersionLength]byte,
	subnet uint64,
) error {
	// Sending a data column sidecar to only one peer is not ideal,
	// but it ensures at least one peer receives it.
	s.subnetLocker(wrappedSubIdx).Lock()
	defer s.subnetLocker(wrappedSubIdx).Unlock()

	// No peers found, attempt to find peers with this subnet.
	if err := s.FindAndDialPeersWithSubnets(ctx, topicFormat, forkDigest, minimumPeersPerSubnetForBroadcast, map[uint64]bool{subnet: true}); err != nil {
		return errors.Wrap(err, "find peers with subnet")
	}

	return nil
}

// method to broadcast messages to other peers in our gossip mesh.
func (s *Service) broadcastObject(ctx context.Context, obj ssz.Marshaler, topic string) error {
	ctx, span := trace.StartSpan(ctx, "p2p.broadcastObject")
	defer span.End()

	span.SetAttributes(trace.StringAttribute("topic", topic))

	buf := new(bytes.Buffer)
	if _, err := s.Encoding().EncodeGossip(buf, obj); err != nil {
		err := errors.Wrap(err, "could not encode message")
		tracing.AnnotateError(span, err)
		return err
	}

	if span.IsRecording() {
		id := hash.FastSum64(buf.Bytes())
		messageLen := int64(buf.Len())
		// lint:ignore uintcast -- It's safe to do this for tracing.
		iid := int64(id)
		span = trace.AddMessageSendEvent(span, iid, messageLen /*uncompressed*/, messageLen /*compressed*/)
	}
	if err := s.PublishToTopic(ctx, topic+s.Encoding().ProtocolSuffix(), buf.Bytes()); err != nil {
		err := errors.Wrap(err, "could not publish message")
		tracing.AnnotateError(span, err)
		return err
	}
	return nil
}

func attestationToTopic(subnet uint64, forkDigest [fieldparams.VersionLength]byte) string {
	return fmt.Sprintf(AttestationSubnetTopicFormat, forkDigest, subnet)
}

func syncCommitteeToTopic(subnet uint64, forkDigest [fieldparams.VersionLength]byte) string {
	return fmt.Sprintf(SyncCommitteeSubnetTopicFormat, forkDigest, subnet)
}

func blobSubnetToTopic(subnet uint64, forkDigest [fieldparams.VersionLength]byte) string {
	return fmt.Sprintf(BlobSubnetTopicFormat, forkDigest, subnet)
}

func lcOptimisticToTopic(forkDigest [4]byte) string {
	return fmt.Sprintf(LightClientOptimisticUpdateTopicFormat, forkDigest)
}

func lcFinalityToTopic(forkDigest [4]byte) string {
	return fmt.Sprintf(LightClientFinalityUpdateTopicFormat, forkDigest)
}

func dataColumnSubnetToTopic(subnet uint64, forkDigest [fieldparams.VersionLength]byte) string {
	return fmt.Sprintf(DataColumnSubnetTopicFormat, forkDigest, subnet)
}
