package p2p

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/altair"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/crypto/hash"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

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

func (s *Service) internalBroadcastAttestation(
	ctx context.Context,
	subnet uint64,
	att ethpb.Att,
	forkDigest [fieldparams.VersionLength]byte,
) {
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
			ok, err := s.FindPeersWithSubnet(ctx, attestationToTopic(subnet, forkDigest), subnet, 1)
			if err != nil {
				return err
			}
			if ok {
				savedAttestationBroadcasts.Inc()
				return nil
			}
			return errors.New("failed to find peers for subnet")
		}(); err != nil {
			log.WithError(err).Error("Failed to find peers")
			tracing.AnnotateError(span, err)
		}
	}
	// In the event our attestation is outdated and beyond the
	// acceptable threshold, we exit early and do not broadcast it.
	currSlot := slots.CurrentSlot(uint64(s.genesisTime.Unix()))
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
	// to ensure that we can re-use the same subnet locker.
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
			ok, err := s.FindPeersWithSubnet(ctx, syncCommitteeToTopic(subnet, forkDigest), subnet, 1)
			if err != nil {
				return err
			}
			if ok {
				savedSyncCommitteeBroadcasts.Inc()
				return nil
			}
			return errors.New("failed to find peers for subnet")
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

func (s *Service) internalBroadcastBlob(
	ctx context.Context,
	subnet uint64,
	blobSidecar *ethpb.BlobSidecar,
	forkDigest [fieldparams.VersionLength]byte,
) {
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
			ok, err := s.FindPeersWithSubnet(ctx, blobSubnetToTopic(subnet, forkDigest), subnet, 1)
			if err != nil {
				return err
			}
			if ok {
				blobSidecarBroadcasts.Inc()
				return nil
			}
			return errors.New("failed to find peers for subnet")
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

// BroadcastDataColumn broadcasts a data column to the p2p network, the message is assumed to be
// broadcasted to the current fork and to the input column subnet.
// TODO: Add tests
func (s *Service) BroadcastDataColumn(
	ctx context.Context,
	root [fieldparams.RootLength]byte,
	columnSubnet uint64,
	dataColumnSidecar *ethpb.DataColumnSidecar,
) error {
	// Add tracing to the function.
	ctx, span := trace.StartSpan(ctx, "p2p.BroadcastBlob")
	defer span.End()

	// Ensure the data column sidecar is not nil.
	if dataColumnSidecar == nil {
		return errors.Errorf("attempted to broadcast nil data column sidecar at subnet %d", columnSubnet)
	}

	// Retrieve the current fork digest.
	forkDigest, err := s.currentForkDigest()
	if err != nil {
		err := errors.Wrap(err, "current fork digest")
		tracing.AnnotateError(span, err)
		return err
	}

	// Non-blocking broadcast, with attempts to discover a column subnet peer if none available.
	go s.internalBroadcastDataColumn(ctx, root, columnSubnet, dataColumnSidecar, forkDigest)

	return nil
}

func (s *Service) internalBroadcastDataColumn(
	ctx context.Context,
	root [fieldparams.RootLength]byte,
	columnSubnet uint64,
	dataColumnSidecar *ethpb.DataColumnSidecar,
	forkDigest [fieldparams.VersionLength]byte,
) {
	// Add tracing to the function.
	_, span := trace.StartSpan(ctx, "p2p.internalBroadcastDataColumn")
	defer span.End()

	// Increase the number of broadcast attempts.
	dataColumnSidecarBroadcastAttempts.Inc()

	// Clear parent context / deadline.
	ctx = trace.NewContext(context.Background(), span)

	// Define a one-slot length context timeout.
	oneSlot := time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second
	ctx, cancel := context.WithTimeout(ctx, oneSlot)
	defer cancel()

	// Build the topic corresponding to this column subnet and this fork digest.
	topic := dataColumnSubnetToTopic(columnSubnet, forkDigest)

	// Compute the wrapped subnet index.
	wrappedSubIdx := columnSubnet + dataColumnSubnetVal

	// Check if we have peers with this subnet.
	hasPeer := func() bool {
		s.subnetLocker(wrappedSubIdx).RLock()
		defer s.subnetLocker(wrappedSubIdx).RUnlock()

		return s.hasPeerWithSubnet(topic)
	}()

	// If no peers are found, attempt to find peers with this subnet.
	if !hasPeer {
		if err := func() error {
			s.subnetLocker(wrappedSubIdx).Lock()
			defer s.subnetLocker(wrappedSubIdx).Unlock()

			ok, err := s.FindPeersWithSubnet(ctx, topic, columnSubnet, 1 /*threshold*/)
			if err != nil {
				return errors.Wrap(err, "find peers for subnet")
			}

			if ok {
				return nil
			}
			return errors.New("failed to find peers for subnet")
		}(); err != nil {
			log.WithError(err).Error("Failed to find peers")
			tracing.AnnotateError(span, err)
		}
	}

	// Broadcast the data column sidecar to the network.
	if err := s.broadcastObject(ctx, dataColumnSidecar, topic); err != nil {
		log.WithError(err).Error("Failed to broadcast data column sidecar")
		tracing.AnnotateError(span, err)
	}

	header := dataColumnSidecar.SignedBlockHeader.GetHeader()
	slot := header.GetSlot()

	slotStartTime, err := slots.ToTime(uint64(s.genesisTime.Unix()), slot)
	if err != nil {
		log.WithError(err).Error("Failed to convert slot to time")
	}

	log.WithFields(logrus.Fields{
		"slot":               slot,
		"timeSinceSlotStart": time.Since(slotStartTime),
		"root":               fmt.Sprintf("%#x", root),
		"columnSubnet":       columnSubnet,
	}).Debug("Broadcasted data column sidecar")

	// Increase the number of successful broadcasts.
	dataColumnSidecarBroadcasts.Inc()
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

func dataColumnSubnetToTopic(subnet uint64, forkDigest [fieldparams.VersionLength]byte) string {
	return fmt.Sprintf(DataColumnSubnetTopicFormat, forkDigest, subnet)
}
