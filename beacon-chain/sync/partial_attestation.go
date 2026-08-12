package sync

import (
	"context"
	"strconv"
	"strings"

	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
)

// This file implements the sync-service side of the partial attestation
// broadcaster: replaying bundled attestations through the classic gossip
// validation pipeline, which owns every check.

const attestationTopicInfix = "beacon_attestation_"

// attestationSubnetFromTopic extracts the subnet id from a full
// beacon_attestation_{subnet_id} topic string.
func attestationSubnetFromTopic(topic string) (uint64, error) {
	idx := strings.Index(topic, attestationTopicInfix)
	if idx == -1 {
		return 0, errors.New("not an attestation subnet topic")
	}
	sub := topic[idx+len(attestationTopicInfix):]
	if end := strings.Index(sub, "/"); end != -1 {
		sub = sub[:end]
	}
	return strconv.ParseUint(sub, 10, 64)
}

// processPartialAttestation replays one bundled attestation through the
// classic gossip validator, pools it if accepted, and rebroadcasts it on
// classic gossip for non-partial peers. It reports whether the attestation
// was accepted; a non-nil error means it was rejected.
func (s *Service) processPartialAttestation(
	topic string, att *eth.SingleAttestation,
) (bool, error) {
	ctx, cancel := context.WithTimeout(s.ctx, pubsubMessageTimeout)
	defer cancel()

	subnet, err := attestationSubnetFromTopic(topic)
	if err != nil {
		return false, errors.Wrap(err, "extract subnet from topic")
	}

	attForPool, result, err := s.validateGossipAttestation(ctx, att, topic)
	if result == pubsub.ValidationReject {
		if err == nil {
			err = errors.New("validation failed")
		}
		return false, errors.Wrap(err, "bundled attestation rejected")
	}
	if result != pubsub.ValidationAccept {
		// Ignored: already seen or not currently verifiable. Not an error.
		return false, nil
	}
	if err := s.committeeIndexBeaconAttestationSubscriber(ctx, attForPool); err != nil {
		log.WithError(err).Error("Could not save partial attestation to pool")
	}
	if err := s.cfg.p2p.BroadcastAttestation(ctx, subnet, att); err != nil {
		log.WithError(err).Debug("Could not rebroadcast partial attestation")
	}
	return true, nil
}
