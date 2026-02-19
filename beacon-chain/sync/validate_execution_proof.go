package sync

import (
	"context"
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

func (s *Service) validateExecutionProof(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	// Always accept our own messages.
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}

	// Ignore messages during initial sync.
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, errors.New("initial sync in progress")
	}

	// Reject messages with a nil topic.
	if msg.Topic == nil {
		return pubsub.ValidationReject, p2p.ErrInvalidTopic
	}

	// Decode the message, reject if it fails.
	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		log.WithError(err).Error("Failed to decode message")
		return pubsub.ValidationReject, err
	}

	// Reject messages that are not of the expected type.
	signedExecutionProof, ok := m.(*ethpb.SignedExecutionProof)
	if !ok {
		log.WithField("message", m).Error("Message is not of type *ethpb.SignedExecutionProof")
		return pubsub.ValidationReject, errWrongMessage
	}

	executionProof := signedExecutionProof.Message

	// [IGNORE] The proof's corresponding new payload request
	// (identified by `proof.message.public_input.new_payload_request_root`)
	// has been seen (via gossip or non-gossip sources)
	// (a client MAY queue proofs for processing once the new payload request is
	// retrieved).
	newPayloadRequestRoot := bytesutil.ToBytes32(executionProof.PublicInput.NewPayloadRequestRoot)
	ok, blockRootEpoch := s.hasSeenNewPayloadRequest(newPayloadRequestRoot)
	if !ok {
		return pubsub.ValidationIgnore, fmt.Errorf("new payload request root %#x not seen", newPayloadRequestRoot)
	}

	blockRoot, blockEpoch := blockRootEpoch.root, blockRootEpoch.epoch

	// Convert to ROSignedExecutionProof.
	roSignedProof, err := blocks.NewROSignedExecutionProof(signedExecutionProof, blockRoot, blockEpoch)
	if err != nil {
		return pubsub.ValidationReject, err
	}

	// [IGNORE] The proof is the first proof received for the tuple
	// `(proof.message.public_input.new_payload_request_root, proof.message.proof_type, proof.prover_pubkey)`
	// -- i.e. the first valid or invalid proof for `proof.message.proof_type` from `proof.prover_pubkey`.
	if s.hasSeenProof(&roSignedProof) {
		return pubsub.ValidationIgnore, errors.New("proof already seen for (newPayloadRequestRoot, proofType, proverPubkey) tuple")
	}

	// Mark the proof as seen regardless of whether it is valid or not,
	// to prevent processing multiple proofs with the same
	// (new payload request root, proof type, prover pubkey) tuple.
	defer s.setSeenProof(&roSignedProof)

	// Create the verifier with gossip requirements.
	verifier := s.newSignedExecutionProofsVerifier([]blocks.ROSignedExecutionProof{roSignedProof}, verification.GossipSignedExecutionProofRequirements)

	// Run verifications.
	// [REJECT] `proof.prover_pubkey` is associated with an active validator.
	if err := verifier.IsFromActiveValidator(); err != nil {
		return pubsub.ValidationReject, err
	}

	// [REJECT] `proof.signature` is valid with respect to the prover's public key.
	if err := verifier.ValidProverSignature(); err != nil {
		return pubsub.ValidationReject, err
	}

	// [REJECT] `proof.message.proof_data` is non-empty.
	if err := verifier.ProofDataNonEmpty(); err != nil {
		return pubsub.ValidationReject, err
	}

	// [REJECT] `proof.message.proof_data` is not larger than MAX_PROOF_SIZE.
	if err := verifier.ProofDataNotTooLarge(); err != nil {
		return pubsub.ValidationReject, err
	}

	// [REJECT] `proof.message` is a valid execution proof.
	if err := verifier.ProofVerified(); err != nil {
		return pubsub.ValidationReject, err
	}

	// [IGNORE] The proof is the first proof received for the tuple
	// `(proof.message.public_input.new_payload_request_root, proof.message.proof_type)`
	// -- i.e. the first valid proof for `proof.message.proof_type` from any prover.
	if s.hasSeenValidProof(&roSignedProof) {
		return pubsub.ValidationIgnore, errors.New("valid proof already seen for (newPayloadRequestRoot, proofType) tuple")
	}

	// Get verified proofs.
	verifiedProofs, err := verifier.VerifiedROSignedExecutionProofs()
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	log.WithFields(logrus.Fields{
		"blockRoot": fmt.Sprintf("%#x", roSignedProof.BlockRoot()),
		"type":      roSignedProof.Message.ProofType,
	}).Debug("Accepted execution proof")

	// Set validator data to the verified proof.
	msg.ValidatorData = verifiedProofs[0]
	return pubsub.ValidationAccept, nil
}

// hasSeenProof returns true if the proof with the same new payload request root, proof type and prover pubkey has been seen before, false otherwise.
func (s *Service) hasSeenProof(roSignedProof *blocks.ROSignedExecutionProof) bool {
	key := computeProofCacheKey(roSignedProof)
	_, ok := s.seenProofCache.Get(string(key))

	return ok
}

// setSeenProof marks the proof with the given new payload request root, proof type and prover pubkey as seen before.
func (s *Service) setSeenProof(roSignedProof *blocks.ROSignedExecutionProof) {
	key := computeProofCacheKey(roSignedProof)
	s.seenProofCache.Add(string(key), true)
}

// hasSeenValidProof returns true if a proof with the same new payload request root and proof type has been seen before, false otherwise.
func (s *Service) hasSeenValidProof(roSignedProof *blocks.ROSignedExecutionProof) bool {
	key := computeValidProofCacheKey(*roSignedProof)
	_, ok := s.seenValidProofCache.Get(string(key))

	return ok
}

// setSeenValidProof marks a proof with the given new payload request root and proof type as seen before.
func (s *Service) setSeenValidProof(roSignedProof *blocks.ROSignedExecutionProof) {
	key := computeValidProofCacheKey(*roSignedProof)
	s.seenValidProofCache.Add(string(key), true)
}

func computeProofCacheKey(roSignedProof *blocks.ROSignedExecutionProof) []byte {
	executionProof := roSignedProof.Message

	key := make([]byte, 0, 41)
	key = append(key, executionProof.PublicInput.NewPayloadRequestRoot...)
	key = append(key, executionProof.ProofType...)
	key = append(key, bytesutil.Uint64ToBytesBigEndian(uint64(roSignedProof.GetValidatorIndex()))...)

	return key
}

func computeValidProofCacheKey(roSignedProof blocks.ROSignedExecutionProof) []byte {
	executionProof := roSignedProof.Message

	key := make([]byte, 0, 33)
	key = append(key, executionProof.PublicInput.NewPayloadRequestRoot...)
	key = append(key, executionProof.ProofType...)

	return key
}
