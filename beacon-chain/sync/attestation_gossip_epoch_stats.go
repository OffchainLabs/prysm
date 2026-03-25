package sync

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// committeeAttGossipEpochStats aggregates committee-index (subnet) attestation gossip
// validation outcomes per clock epoch. It is safe for concurrent use from pubsub validators.
type committeeAttGossipEpochStats struct {
	mu          sync.Mutex
	initialized bool
	epoch       primitives.Epoch
	success     uint64
	nonSuccess  map[string]uint64
}

func (a *committeeAttGossipEpochStats) observe(clock *startup.Clock, res pubsub.ValidationResult, reasonKey string) {
	if clock == nil {
		return
	}
	cur := clock.CurrentEpoch()

	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.initialized {
		a.initialized = true
		a.epoch = cur
		a.nonSuccess = make(map[string]uint64)
	}

	a.advanceEpochsLocked(cur)

	if res == pubsub.ValidationAccept {
		a.success++
		return
	}
	if reasonKey == "" {
		reasonKey = "non_success_unknown"
	}
	a.nonSuccess[reasonKey]++
}

// rotateOnly flushes any fully completed epochs when the clock has moved forward, including
// epochs with no attestations (empty summary).
func (a *committeeAttGossipEpochStats) rotateOnly(clock *startup.Clock) {
	if clock == nil {
		return
	}
	cur := clock.CurrentEpoch()

	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.initialized {
		return
	}
	a.advanceEpochsLocked(cur)
}

// advanceEpochsLocked logs and clears stats for each epoch in [a.epoch, cur) then aligns a.epoch to cur.
// Caller must hold a.mu.
func (a *committeeAttGossipEpochStats) advanceEpochsLocked(cur primitives.Epoch) {
	for a.epoch < cur {
		a.logEpochLocked()
		a.epoch++
		a.success = 0
		a.nonSuccess = make(map[string]uint64)
	}
}

func (a *committeeAttGossipEpochStats) logEpochLocked() {
	fields := logrus.Fields{
		"epoch":        a.epoch,
		"successCount": a.success,
	}
	var total uint64
	reasons := make([]string, 0, len(a.nonSuccess))
	for k := range a.nonSuccess {
		reasons = append(reasons, k)
	}
	sort.Strings(reasons)
	for _, k := range reasons {
		total += a.nonSuccess[k]
	}
	fields["nonSuccessTotal"] = total
	if len(a.nonSuccess) > 0 {
		byReason := make(map[string]uint64, len(a.nonSuccess))
		for _, k := range reasons {
			byReason[k] = a.nonSuccess[k]
		}
		fields["nonSuccessByReason"] = byReason
	}
	log.WithFields(fields).Info("Committee subnet attestation gossip validation epoch summary")
}

func classifyCommitteeAttGossipNonSuccess(res pubsub.ValidationResult, err error) string {
	if res == pubsub.ValidationAccept {
		return ""
	}
	if err == nil {
		switch res {
		case pubsub.ValidationIgnore:
			return "ignore_unspecified"
		case pubsub.ValidationReject:
			return "reject_unspecified"
		default:
			return "non_success_unspecified"
		}
	}
	if res == pubsub.ValidationReject {
		return classifyCommitteeAttGossipReject(err)
	}
	return classifyCommitteeAttGossipIgnore(err)
}

func classifyCommitteeAttGossipReject(err error) string {
	if errors.Is(err, p2p.ErrInvalidTopic) {
		return "reject_invalid_topic"
	}
	if errors.Is(err, errWrongMessage) {
		return "reject_wrong_message"
	}
	msg := err.Error()
	switch {
	case strings.Contains(strings.ToLower(msg), "nil attestation"):
		return "reject_nil_attestation"
	case strings.Contains(msg, "does not match target epoch"):
		return "reject_slot_target_epoch_mismatch"
	case strings.Contains(msg, "subnet does not match"):
		return "reject_wrong_subnet_topic"
	case strings.Contains(msg, "committee index") && strings.Contains(msg, "must be 0"):
		return "reject_committee_index_electra"
	case strings.Contains(msg, "committee index") && strings.Contains(msg, "must be < 2"):
		return "reject_committee_index_gloas"
	case strings.Contains(msg, "committee bits"):
		return "reject_committee_bits"
	case strings.Contains(msg, "committee index") && strings.Contains(msg, ">="):
		return "reject_committee_index_out_of_range"
	case strings.Contains(msg, "attestation bitfield"):
		return "reject_attestation_bitfield"
	case strings.Contains(msg, "attester") && strings.Contains(msg, "not a member"):
		return "reject_attester_not_in_committee"
	case strings.Contains(msg, "FFG and LMD votes are not consistent"):
		return "reject_lmd_ffg_inconsistent"
	case strings.Contains(msg, "bad block root"):
		return "reject_bad_block_root"
	}
	if strings.Contains(strings.ToLower(msg), "signature") {
		return "reject_signature_verification"
	}
	return "reject_other"
}

func classifyCommitteeAttGossipIgnore(err error) string {
	if errors.Is(err, blockchain.ErrNotDescendantOfFinalized) {
		return "ignore_not_descendant_of_finalized"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not within attestation propagation range"):
		return "ignore_attestation_propagation_range"
	case strings.Contains(msg, "execution payload for attested block has not been seen"):
		return "ignore_payload_not_seen"
	case strings.Contains(msg, "could not process attestation slot"):
		return "ignore_attestation_slot_time"
	case strings.Contains(msg, "source check point"):
		return "ignore_source_checkpoint_mismatch"
	case strings.Contains(msg, "target check point"):
		return "ignore_target_checkpoint_mismatch"
	}
	return "ignore_other"
}

func committeeAttGossipSlotDuration() time.Duration {
	return time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second
}
