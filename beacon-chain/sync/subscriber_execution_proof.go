package sync

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

// executionProofSubscriber stores an execution proof that passed gossip
// validation, following EIP-8025 `on_execution_proof`.
func (s *Service) executionProofSubscriber(_ context.Context, msg proto.Message) error {
	signed, ok := msg.(*ethpb.SignedExecutionProofEnvelope)
	if !ok {
		return errWrongMessage
	}

	envelope, err := blocks.NewROSignedExecutionProofEnvelope(signed)
	if err != nil {
		return fmt.Errorf("new RO signed execution proof envelope: %w", err)
	}

	// TODO: We should transform a non verified envelope into a verified one ONLY in the verification package.
	// Gossip validation accepted this envelope, which includes the proof node
	// having verified the proof itself.
	proof := blocks.NewVerifiedROSignedExecutionProofEnvelope(envelope)

	blockRoot := proof.BeaconBlockRoot()
	if !s.cfg.executionProofCache.Save(proof) {
		// Another proof of this type won the race for this block.
		return nil
	}

	log.WithFields(logrus.Fields{
		"blockRoot": fmt.Sprintf("%#x", bytesutil.Trunc(blockRoot[:])),
		"proofType": proof.ProofType(),
		"prover":    proof.ValidatorIndex,
		"proofSize": len(proof.Message.ProofData),
	}).Debug("Stored verified execution proof")

	return nil
}
