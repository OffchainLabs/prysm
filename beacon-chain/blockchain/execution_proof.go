package blockchain

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/eip8025"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/proofengine"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
)

var (
	// ErrExecutionProofBlockUnknown is returned when an execution proof refers to an unknown beacon block.
	ErrExecutionProofBlockUnknown = errors.New("execution proof beacon block is unknown")

	// ErrExecutionProofPayloadUnavailable is returned when the payload an execution proof refers to is not imported.
	ErrExecutionProofPayloadUnavailable = errors.New("execution proof payload is not available")

	// ErrInvalidExecutionProofEnvelope is returned when an execution proof envelope fails validation against its state.
	ErrInvalidExecutionProofEnvelope = errors.New("invalid execution proof envelope")

	// ErrInvalidExecutionProof is returned when an execution proof fails verification.
	ErrInvalidExecutionProof = proofengine.ErrProofInvalid

	// ErrExecutionProofTypeKnown is returned when a verified execution proof of the same type is already known for the
	// payload.
	ErrExecutionProofTypeKnown = forkchoice.ErrExecutionProofTypeKnown

	errNoExecutionProofVerifier = errors.New("no execution proof verifier configured")
)

// ExecutionProofReceiver defines the methods of the chain service handling EIP-8025 execution proofs.
type ExecutionProofReceiver interface {
	CheckExecutionProofEnvelope(context.Context, blocks.ROSignedExecutionProofEnvelope) ([fieldparams.RootLength]byte, error)
	VerifyExecutionProof(ctx context.Context, newPayloadRequestRoot [fieldparams.RootLength]byte, proof blocks.ROSignedExecutionProofEnvelope) error
	HasExecutionProofType(root [fieldparams.RootLength]byte, proofType ethpb.ProofType) bool
	ReceiveExecutionProof(ctx context.Context, root [fieldparams.RootLength]byte, proofType ethpb.ProofType) error
	RecentBlockSlot(root [fieldparams.RootLength]byte) (primitives.Slot, error)
}

// ExecutionProofVerifier verifies EIP-8025 execution proofs against the SSZ encoding of their expected PublicInput.
type ExecutionProofVerifier interface {
	Verify(ctx context.Context, proofType ethpb.ProofType, proofData, publicInput []byte) error
}

// newPayloadRequestRoot returns the hash tree root of the EIP-8025 new payload request of a revealed payload, which
// identifies the payload for execution proofs. It returns nil, and no error, if EIP-8025 is disabled. The state is the
// one the envelope builds on: its execution payload bid MUST be the one the envelope fulfils.
func newPayloadRequestRoot(envelope interfaces.ROExecutionPayloadEnvelope, st state.BeaconState) (*[32]byte, error) {
	if !features.Get().EnableExecutionProofs {
		return nil, nil
	}

	bid, err := st.LatestExecutionPayloadBid()
	if err != nil {
		return nil, fmt.Errorf("latest execution payload bid: %w", err)
	}
	if bid == nil {
		return nil, errors.New("no execution payload bid in the state")
	}

	// The commitments come from the bid, so a bid of another block would silently yield a wrong root.
	if bid.BlockHash() != envelope.BlockHash() {
		return nil, fmt.Errorf("bid block hash %#x does not match envelope block hash %#x", bid.BlockHash(), envelope.BlockHash())
	}

	root, err := blocks.NewPayloadRequestRoot(envelope, bid.BlobKzgCommitments())
	if err != nil {
		return nil, fmt.Errorf("new payload request root: %w", err)
	}

	return &root, nil
}

// CheckExecutionProofEnvelope runs the checks shared by every path receiving an execution proof: the beacon block and
// its payload are known, and the envelope is valid against the state. It returns the new payload request root the
// proof must be verified against. The proof itself is not verified.
func (s *Service) CheckExecutionProofEnvelope(
	ctx context.Context,
	proof blocks.ROSignedExecutionProofEnvelope,
) ([fieldparams.RootLength]byte, error) {
	root := proof.BeaconBlockRoot()

	if !s.HasBlock(ctx, root) {
		return [32]byte{}, ErrExecutionProofBlockUnknown
	}

	requestRoot, ok := func() ([fieldparams.RootLength]byte, bool) {
		s.cfg.ForkChoiceStore.RLock()
		defer s.cfg.ForkChoiceStore.RUnlock()

		return s.cfg.ForkChoiceStore.NewPayloadRequestRoot(root)
	}()

	if !ok {
		return [32]byte{}, ErrExecutionProofPayloadUnavailable
	}

	slot, err := s.RecentBlockSlot(root)
	if err != nil {
		return [32]byte{}, fmt.Errorf("recent block slot: %w", err)
	}

	state, err := s.HeadStateReadOnly(ctx)
	if err != nil {
		return [32]byte{}, fmt.Errorf("head state: %w", err)
	}

	if err := eip8025.VerifyExecutionProofEnvelope(state, slots.ToEpoch(slot), proof); err != nil {
		return [32]byte{}, fmt.Errorf("%w: %w", ErrInvalidExecutionProofEnvelope, err)
	}

	return requestRoot, nil
}

// VerifyExecutionProof verifies the execution proof of an envelope that passed CheckExecutionProofEnvelope, against
// the new payload request root it returned. It returns an error wrapping ErrInvalidExecutionProof if the proof is
// invalid.
// https://github.com/ethereum/consensus-specs/blob/master/specs/_features/eip8025/beacon-chain.md#new-process_execution_proof
func (s *Service) VerifyExecutionProof(
	ctx context.Context,
	newPayloadRequestRoot [fieldparams.RootLength]byte,
	proof blocks.ROSignedExecutionProofEnvelope,
) error {
	if s.cfg.ExecutionProofVerifier == nil {
		return errNoExecutionProofVerifier
	}

	publicInput := proofengine.PublicInput(newPayloadRequestRoot)
	if err := s.cfg.ExecutionProofVerifier.Verify(ctx, proof.ProofType(), proof.Message.ProofData, publicInput); err != nil {
		return fmt.Errorf("verify execution proof: %w", err)
	}

	return nil
}

// HasExecutionProofType reports whether a verified execution proof of this type is known for the payload of the
// beacon block.
func (s *Service) HasExecutionProofType(root [fieldparams.RootLength]byte, proofType ethpb.ProofType) bool {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()

	return s.cfg.ForkChoiceStore.HasExecutionProofType(root, proofType)
}

// ReceiveExecutionProof records a verified execution proof of this type for the payload of the beacon block. It
// returns ErrExecutionProofTypeKnown if a proof of this type was already known.
// (The proof may be the last missing piece to consider the payload as valid.)
func (s *Service) ReceiveExecutionProof(ctx context.Context, root [fieldparams.RootLength]byte, proofType ethpb.ProofType) error {
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()

	if err := s.cfg.ForkChoiceStore.AddExecutionProofType(root, proofType); err != nil {
		return fmt.Errorf("add execution proof type: %w", err)
	}

	if insertedAt, ok := s.cfg.ForkChoiceStore.PayloadInsertionTime(root); ok {
		executionProofArrivalDelay.WithLabelValues(proofType.String()).Observe(time.Since(insertedAt).Seconds())
	}

	if err := s.setPayloadValidIfReady(ctx, root); err != nil {
		return fmt.Errorf("set payload valid if ready: %w", err)
	}

	return nil
}

// markPayloadExecutionValid records that the EL validated the Gloas payload of the beacon block, then sets the
// payload as valid if enough execution proofs are also known.
// The payload MUST have been inserted in forkchoice.
//
// Caller of the method MUST acquire a lock on forkchoice.
func (s *Service) markPayloadExecutionValid(ctx context.Context, root [fieldparams.RootLength]byte) error {
	if err := s.cfg.ForkChoiceStore.SetPayloadExecutionValid(root); err != nil {
		return fmt.Errorf("set payload execution valid: %w", err)
	}

	if err := s.setPayloadValidIfReady(ctx, root); err != nil {
		return fmt.Errorf("set payload valid if ready: %w", err)
	}

	return nil
}

// setPayloadValidIfReady sets the Gloas payload of the beacon block as valid in forkchoice once it is ready: the EL
// validated it and at least MinExecutionProofs distinct execution proof types are verified for it.
// Caller of the method MUST acquire a lock on forkchoice.
func (s *Service) setPayloadValidIfReady(ctx context.Context, root [fieldparams.RootLength]byte) error {
	executionValid, proofTypes := s.cfg.ForkChoiceStore.PayloadValidationStatus(root)
	if !executionValid || uint64(proofTypes) < s.cfg.MinExecutionProofs {
		return nil
	}

	wasOptimistic, err := s.cfg.ForkChoiceStore.IsOptimistic(root)
	if err != nil {
		return fmt.Errorf("is optimistic: %w", err)
	}

	// Execution proofs are assumed recursive: a proof of a payload also attests to the validity of all its ancestors.
	// This also validates the ancestors of the payload, and the empty blocks built on top of it.
	if err := s.cfg.ForkChoiceStore.SetOptimisticToValid(ctx, root); err != nil {
		return fmt.Errorf("set optimistic to valid: %w", err)
	}

	if err := s.refreshHeadOptimistic(); err != nil {
		return fmt.Errorf("refresh head optimistic: %w", err)
	}

	// Record and log only the transition from optimistic to valid, not the later proofs of an already valid payload. Without
	// EIP-8025, the payload is validated at import, which the envelope log already reports.
	if wasOptimistic && s.cfg.MinExecutionProofs > 0 {
		if insertedAt, ok := s.cfg.ForkChoiceStore.PayloadInsertionTime(root); ok {
			payloadProofValidationDelay.Observe(time.Since(insertedAt).Seconds())
		}

		log.WithFields(logrus.Fields{
			"blockRoot":      fmt.Sprintf("%#x", bytesutil.Trunc(root[:])),
			"proofTypeCount": proofTypes,
		}).Info("Payload validated by the EL and execution proofs")
	}

	return nil
}
