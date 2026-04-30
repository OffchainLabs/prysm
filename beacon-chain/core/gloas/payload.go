package gloas

import (
	"bytes"
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// VerifyExecutionPayloadEnvelope is a verification function called by fork-choice when
// importing a signed execution payload. It verifies the payload against the
// execution engine without processing execution requests or updating state.
// Actual state mutations are deferred to process_parent_execution_payload in
// the next block.
//
//	<spec fn="verify_execution_payload_envelope" fork="gloas" hash="0261931f">
//	def verify_execution_payload_envelope(
//	    state: BeaconState,
//	    signed_envelope: SignedExecutionPayloadEnvelope,
//	    execution_engine: ExecutionEngine,
//	) -> None:
//	    envelope = signed_envelope.message
//	    payload = envelope.payload
//
//	    # Verify signature
//	    assert verify_execution_payload_envelope_signature(state, signed_envelope)
//
//	    # Verify consistency with the beacon block
//	    header = copy(state.latest_block_header)
//	    header.state_root = hash_tree_root(state)
//	    assert envelope.beacon_block_root == hash_tree_root(header)
//	    assert envelope.parent_beacon_block_root == state.latest_block_header.parent_root
//
//	    # Verify consistency with the committed bid
//	    bid = state.latest_execution_payload_bid
//	    assert envelope.builder_index == bid.builder_index
//	    assert payload.prev_randao == bid.prev_randao
//	    assert payload.gas_limit == bid.gas_limit
//	    assert payload.block_hash == bid.block_hash
//	    assert hash_tree_root(envelope.execution_requests) == bid.execution_requests_root
//
//	    # Verify the execution payload is valid
//	    assert payload.slot_number == state.slot
//	    assert payload.parent_hash == state.latest_block_hash
//	    assert payload.timestamp == compute_time_at_slot(state, state.slot)
//	    assert hash_tree_root(payload.withdrawals) == hash_tree_root(state.payload_expected_withdrawals)
//	    assert execution_engine.verify_and_notify_new_payload(
//	        NewPayloadRequest(
//	            execution_payload=payload,
//	            versioned_hashes=[
//	                kzg_commitment_to_versioned_hash(commitment)
//	                for commitment in bid.blob_kzg_commitments
//	            ],
//	            parent_beacon_block_root=envelope.parent_beacon_block_root,
//	            execution_requests=envelope.execution_requests,
//	        )
//	    )
//	</spec>
func VerifyExecutionPayloadEnvelope(
	ctx context.Context,
	st state.BeaconState,
	signedEnvelope interfaces.ROSignedExecutionPayloadEnvelope,
) error {
	if err := verifyExecutionPayloadEnvelopeSignature(st, signedEnvelope); err != nil {
		return errors.Wrap(err, "signature verification failed")
	}

	envelope, err := signedEnvelope.Envelope()
	if err != nil {
		return errors.Wrap(err, "could not get envelope from signed envelope")
	}

	return validatePayloadConsistency(ctx, st, envelope)
}

// VerifyExecutionPayloadEnvelopeWithDeferredSig is the init-sync entry point: extract
// the signature for deferred batch verification and validate consistency.
// No state mutations are performed.
func VerifyExecutionPayloadEnvelopeWithDeferredSig(
	ctx context.Context,
	st state.BeaconState,
	signedEnvelope interfaces.ROSignedExecutionPayloadEnvelope,
) (*bls.SignatureBatch, error) {
	sigBatch, err := ExecutionPayloadEnvelopeSignatureBatch(st, signedEnvelope)
	if err != nil {
		return nil, errors.Wrap(err, "could not extract envelope signature batch")
	}

	envelope, err := signedEnvelope.Envelope()
	if err != nil {
		return nil, errors.Wrap(err, "could not get envelope from signed envelope")
	}

	if err := validatePayloadConsistency(ctx, st, envelope); err != nil {
		return nil, err
	}
	return sigBatch, nil
}

// validatePayloadConsistency checks that the envelope and payload are consistent
// with the beacon block header, the committed bid, and the current state.
func validatePayloadConsistency(ctx context.Context, st state.BeaconState, envelope interfaces.ROExecutionPayloadEnvelope) error {
	if envelope.Slot() != st.Slot() {
		return errors.Errorf("envelope slot does not match state slot: envelope=%d, state=%d", envelope.Slot(), st.Slot())
	}

	header := st.LatestBlockHeader()
	if header == nil {
		return errors.New("latest block header is nil")
	}
	envelopeParent := envelope.ParentBeaconBlockRoot()
	if !bytes.Equal(envelopeParent[:], header.ParentRoot) {
		return errors.Errorf("envelope parent beacon block root does not match state latest block header parent root: envelope=%#x, state=%#x", envelopeParent, header.ParentRoot)
	}

	latestBid, err := st.LatestExecutionPayloadBid()
	if err != nil {
		return errors.Wrap(err, "could not get latest execution payload bid")
	}
	if latestBid == nil {
		return errors.New("latest execution payload bid is nil")
	}
	if envelope.BuilderIndex() != latestBid.BuilderIndex() {
		return errors.Errorf("envelope builder index does not match committed bid builder index: envelope=%d, bid=%d", envelope.BuilderIndex(), latestBid.BuilderIndex())
	}

	// Verify execution_requests_root matches the bid commitment.
	executionRequestsRoot, err := envelope.ExecutionRequests().HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not compute execution requests root")
	}
	bidExecutionRequestsRoot := latestBid.ExecutionRequestsRoot()
	if executionRequestsRoot != bidExecutionRequestsRoot {
		return errors.Errorf("execution requests root mismatch: envelope=%#x, bid=%#x", executionRequestsRoot, bidExecutionRequestsRoot)
	}

	payload, err := envelope.Execution()
	if err != nil {
		return errors.Wrap(err, "could not get execution payload from envelope")
	}
	latestBidPrevRandao := latestBid.PrevRandao()
	if !bytes.Equal(payload.PrevRandao(), latestBidPrevRandao[:]) {
		return errors.Errorf("payload prev randao does not match committed bid prev randao: payload=%#x, bid=%#x", payload.PrevRandao(), latestBidPrevRandao)
	}

	withdrawals, err := payload.Withdrawals()
	if err != nil {
		return errors.Wrap(err, "could not get withdrawals from payload")
	}
	ok, err := st.WithdrawalsMatchPayloadExpected(withdrawals)
	if err != nil {
		return errors.Wrap(err, "could not validate payload withdrawals")
	}
	if !ok {
		return errors.New("payload withdrawals do not match expected withdrawals")
	}

	if latestBid.GasLimit() != payload.GasLimit() {
		return errors.Errorf("committed bid gas limit does not match payload gas limit: bid=%d, payload=%d", latestBid.GasLimit(), payload.GasLimit())
	}

	bidBlockHash := latestBid.BlockHash()
	payloadBlockHash := payload.BlockHash()
	if !bytes.Equal(bidBlockHash[:], payloadBlockHash) {
		return errors.Errorf("committed bid block hash does not match payload block hash: bid=%#x, payload=%#x", bidBlockHash, payloadBlockHash)
	}

	latestBlockHash, err := st.LatestBlockHash()
	if err != nil {
		return errors.Wrap(err, "could not get latest block hash")
	}
	if !bytes.Equal(payload.ParentHash(), latestBlockHash[:]) {
		return errors.Errorf("payload parent hash does not match state latest block hash: payload=%#x, state=%#x", payload.ParentHash(), latestBlockHash)
	}

	t, err := slots.StartTime(st.GenesisTime(), st.Slot())
	if err != nil {
		return errors.Wrap(err, "could not compute timestamp")
	}
	if payload.Timestamp() != uint64(t.Unix()) {
		return errors.Errorf("payload timestamp does not match expected timestamp: payload=%d, expected=%d", payload.Timestamp(), uint64(t.Unix()))
	}

	return nil
}

// ExecutionPayloadEnvelopeSignatureBatch extracts the BLS signature from a signed execution payload
// envelope as a SignatureBatch for deferred batch verification.
func ExecutionPayloadEnvelopeSignatureBatch(
	st state.BeaconState,
	signedEnvelope interfaces.ROSignedExecutionPayloadEnvelope,
) (*bls.SignatureBatch, error) {
	envelope, err := signedEnvelope.Envelope()
	if err != nil {
		return nil, fmt.Errorf("failed to get envelope: %w", err)
	}

	builderIdx := envelope.BuilderIndex()
	publicKey, err := envelopePublicKey(st, builderIdx)
	if err != nil {
		return nil, err
	}

	currentEpoch := slots.ToEpoch(envelope.Slot())
	domain, err := signing.Domain(
		st.Fork(),
		currentEpoch,
		params.BeaconConfig().DomainBeaconBuilder,
		st.GenesisValidatorsRoot(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to compute signing domain: %w", err)
	}

	signingRoot, err := signedEnvelope.SigningRoot(domain)
	if err != nil {
		return nil, fmt.Errorf("failed to compute signing root: %w", err)
	}

	signatureBytes := signedEnvelope.Signature()
	return &bls.SignatureBatch{
		Signatures:   [][]byte{signatureBytes[:]},
		PublicKeys:   []bls.PublicKey{publicKey},
		Messages:     [][32]byte{signingRoot},
		Descriptions: []string{"execution payload envelope signature"},
	}, nil
}

// verifyExecutionPayloadEnvelopeSignature verifies the BLS signature on a signed execution payload envelope.
//
//	<spec fn="verify_execution_payload_envelope_signature" fork="gloas" style="full" hash="49483ae2">
//	def verify_execution_payload_envelope_signature(
//	    state: BeaconState, signed_envelope: SignedExecutionPayloadEnvelope
//	) -> bool:
//	    builder_index = signed_envelope.message.builder_index
//	    if builder_index == BUILDER_INDEX_SELF_BUILD:
//	        validator_index = state.latest_block_header.proposer_index
//	        pubkey = state.validators[validator_index].pubkey
//	    else:
//	        pubkey = state.builders[builder_index].pubkey
//
//	    signing_root = compute_signing_root(
//	        signed_envelope.message, get_domain(state, DOMAIN_BEACON_BUILDER)
//	    )
//	    return bls.Verify(pubkey, signing_root, signed_envelope.signature)
//	</spec>
func verifyExecutionPayloadEnvelopeSignature(st state.BeaconState, signedEnvelope interfaces.ROSignedExecutionPayloadEnvelope) error {
	envelope, err := signedEnvelope.Envelope()
	if err != nil {
		return fmt.Errorf("failed to get envelope: %w", err)
	}

	builderIdx := envelope.BuilderIndex()
	publicKey, err := envelopePublicKey(st, builderIdx)
	if err != nil {
		return err
	}

	signatureBytes := signedEnvelope.Signature()
	signature, err := bls.SignatureFromBytes(signatureBytes[:])
	if err != nil {
		return fmt.Errorf("invalid signature format: %w", err)
	}

	currentEpoch := slots.ToEpoch(envelope.Slot())
	domain, err := signing.Domain(
		st.Fork(),
		currentEpoch,
		params.BeaconConfig().DomainBeaconBuilder,
		st.GenesisValidatorsRoot(),
	)
	if err != nil {
		return fmt.Errorf("failed to compute signing domain: %w", err)
	}

	signingRoot, err := signedEnvelope.SigningRoot(domain)
	if err != nil {
		return fmt.Errorf("failed to compute signing root: %w", err)
	}

	if !signature.Verify(publicKey, signingRoot[:]) {
		return fmt.Errorf("signature verification failed: %w", signing.ErrSigFailedToVerify)
	}

	return nil
}

func envelopePublicKey(st state.BeaconState, builderIdx primitives.BuilderIndex) (bls.PublicKey, error) {
	if builderIdx == params.BeaconConfig().BuilderIndexSelfBuild {
		return proposerPublicKey(st)
	}
	return builderPublicKey(st, builderIdx)
}

func proposerPublicKey(st state.BeaconState) (bls.PublicKey, error) {
	header := st.LatestBlockHeader()
	if header == nil {
		return nil, fmt.Errorf("latest block header is nil")
	}
	proposerPubkey := st.PubkeyAtIndex(header.ProposerIndex)
	publicKey, err := bls.PublicKeyFromBytes(proposerPubkey[:])
	if err != nil {
		return nil, fmt.Errorf("invalid proposer public key: %w", err)
	}
	return publicKey, nil
}

func builderPublicKey(st state.BeaconState, builderIdx primitives.BuilderIndex) (bls.PublicKey, error) {
	builder, err := st.Builder(builderIdx)
	if err != nil {
		return nil, fmt.Errorf("failed to get builder: %w", err)
	}
	if builder == nil {
		return nil, fmt.Errorf("builder at index %d not found", builderIdx)
	}
	publicKey, err := bls.PublicKeyFromBytes(builder.Pubkey)
	if err != nil {
		return nil, fmt.Errorf("invalid builder public key: %w", err)
	}
	return publicKey, nil
}
