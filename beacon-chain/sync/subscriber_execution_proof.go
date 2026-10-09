package sync

import (
	"context"
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
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
	log.WithFields(logrus.Fields{
		"blockRoot": fmt.Sprintf("%#x", bytesutil.Trunc(blockRoot[:])),
		"proofType": proof.ProofType(),
		"prover":    proof.ValidatorIndex,
		"proofSize": humanize.Bytes(uint64(len(proof.Message.ProofData))),
	}).Debug("Received and verified execution proof")

	err := s.cfg.chain.ReceiveExecutionProof(ctx, blockRoot, proof.ProofType())
	if errors.Is(err, blockchain.ErrExecutionProofTypeKnown) {
		// Another proof of this type won the race for this block.
		return nil
	}
	if err != nil {
		return fmt.Errorf("receive execution proof: %w", err)
	}

	return nil
}
