package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/dustin/go-humanize"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

// executionProofSubscriber records an execution proof that passed gossip
// validation, including verification by the proof engine, following EIP-8025
// `on_execution_proof`.
func (s *Service) executionProofSubscriber(ctx context.Context, msg proto.Message) error {
	proof, ok := msg.(blocks.ROSignedExecutionProofEnvelope)
	if !ok {
		return fmt.Errorf("message was not type blocks.ROSignedExecutionProofEnvelope, type=%T", msg)
	}

	blockRoot := proof.BeaconBlockRoot()

	// Logged before the proof is recorded, which may validate the payload and log it.
	fields := logrus.Fields{
		"blockRoot": fmt.Sprintf("%#x", bytesutil.Trunc(blockRoot[:])),
		"proofType": proof.ProofType(),
		"prover":    proof.ValidatorIndex,
		"proofSize": humanize.Bytes(uint64(len(proof.Message.ProofData))),
	}

	if slot, err := s.cfg.chain.RecentBlockSlot(blockRoot); err == nil {
		fields["slot"] = slot

		if since, err := slots.SinceSlotStart(slot, s.cfg.clock.GenesisTime(), time.Now()); err == nil {
			fields["sinceSlotStart"] = since
			executionProofVerifiedSinceSlotStart.WithLabelValues(proof.ProofType().String()).Observe(since.Seconds())
		}
	}

	log.WithFields(fields).Debug("Received and verified execution proof")

	err := s.cfg.chain.ReceiveExecutionProof(ctx, blockRoot, proof.ProofType())
	if errors.Is(err, blockchain.ErrExecutionProofTypeKnown) {
		// Another proof of this type won the race for this block.
		return nil
	}
	if err != nil {
		return fmt.Errorf("receive execution proof: %w", err)
	}

	s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.ExecutionProofReceived,
		Data: &opfeed.ExecutionProofReceivedData{Proof: proof.SignedExecutionProofEnvelope},
	})

	return nil
}
