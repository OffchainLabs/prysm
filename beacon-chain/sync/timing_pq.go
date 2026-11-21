package sync

import (
	"context"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq/timing"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

func pqValidationOutcome(result pubsub.ValidationResult) string {
	switch result {
	case pubsub.ValidationAccept:
		return "accepted"
	case pubsub.ValidationReject:
		return "rejected"
	}
	return "ignored"
}

func recordPQReceipt(ctx context.Context, msg *pubsub.Message, kind string, data *ethpb.AttestationData, committee primitives.CommitteeIndex, signers uint64, size int) {
	receipt := timing.ReceiptFrom(ctx)
	if receipt == nil {
		return
	}
	root, err := data.HashTreeRoot()
	if err != nil {
		return
	}
	var subnet any
	if _, rest, ok := strings.Cut(*msg.Topic, "beacon_attestation_"); ok {
		number, _, _ := strings.Cut(rest, "/")
		if n, err := strconv.ParseUint(number, 10, 64); err == nil {
			subnet = n
		}
	}
	receipt.Received(kind, []byte(msg.ID), uint64(data.Slot), uint64(committee), subnet, signers, size, root)
}
