package gloas

import (
	"bytes"
	"context"
	"fmt"

	requests "github.com/OffchainLabs/prysm/v7/beacon-chain/core/requests"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// ProcessExecutionPayload processes the signed execution payload envelope for the Gloas fork.
//
//	<spec fn="process_execution_payload" fork="gloas" hash="36bd3af3">
//	def process_execution_payload(
//	    state: BeaconState,
//	    # [Modified in Gloas:EIP7732]
//	    # Removed `body`
//	    # [New in Gloas:EIP7732]
//	    signed_envelope: SignedExecutionPayloadEnvelope,
//	    execution_engine: ExecutionEngine,
//	    # [New in Gloas:EIP7732]
//	    verify: bool = True,
//	) -> None:
//	    envelope = signed_envelope.message
//	    payload = envelope.payload
//
//	    # Verify signature
//	    if verify:
//	        assert verify_execution_payload_envelope_signature(state, signed_envelope)
//
//	    # Cache latest block header state root
//	    previous_state_root = hash_tree_root(state)
//	    if state.latest_block_header.state_root == Root():
//	        state.latest_block_header.state_root = previous_state_root
//
//	    # Verify consistency with the beacon block
//	    assert envelope.beacon_block_root == hash_tree_root(state.latest_block_header)
//	    assert envelope.slot == state.slot
//
//	    # Verify consistency with the committed bid
//	    committed_bid = state.latest_execution_payload_bid
//	    assert envelope.builder_index == committed_bid.builder_index
//	    assert committed_bid.prev_randao == payload.prev_randao
//
//	    # Verify consistency with expected withdrawals
//	    assert hash_tree_root(payload.withdrawals) == hash_tree_root(state.payload_expected_withdrawals)
//
//	    # Verify the gas_limit
//	    assert committed_bid.gas_limit == payload.gas_limit
//	    # Verify the block hash
//	    assert committed_bid.block_hash == payload.block_hash
//	    # Verify consistency of the parent hash with respect to the previous execution payload
//	    assert payload.parent_hash == state.latest_block_hash
//	    # Verify timestamp
//	    assert payload.timestamp == compute_time_at_slot(state, state.slot)
//	    # Verify the execution payload is valid
//	    versioned_hashes = [
//	        kzg_commitment_to_versioned_hash(commitment)
//	        # [Modified in Gloas:EIP7732]
//	        for commitment in committed_bid.blob_kzg_commitments
//	    ]
//	    requests = envelope.execution_requests
//	    assert execution_engine.verify_and_notify_new_payload(
//	        NewPayloadRequest(
//	            execution_payload=payload,
//	            versioned_hashes=versioned_hashes,
//	            parent_beacon_block_root=state.latest_block_header.parent_root,
//	            execution_requests=requests,
//	        )
//	    )
//
//	    def for_ops(operations: Sequence[Any], fn: Callable[[BeaconState, Any], None]) -> None:
//	        for operation in operations:
//	            fn(state, operation)
//
//	    for_ops(requests.deposits, process_deposit_request)
//	    for_ops(requests.withdrawals, process_withdrawal_request)
//	    for_ops(requests.consolidations, process_consolidation_request)
//
//	    # Queue the builder payment
//	    payment = state.builder_pending_payments[SLOTS_PER_EPOCH + state.slot % SLOTS_PER_EPOCH]
//	    amount = payment.withdrawal.amount
//	    if amount > 0:
//	        state.builder_pending_withdrawals.append(payment.withdrawal)
//	    state.builder_pending_payments[SLOTS_PER_EPOCH + state.slot % SLOTS_PER_EPOCH] = (
//	        BuilderPendingPayment()
//	    )
//
//	    # Cache the execution payload hash
//	    state.execution_payload_availability[state.slot % SLOTS_PER_HISTORICAL_ROOT] = 0b1
//	    state.latest_block_hash = payload.block_hash
//
//	    # Verify the state root
//	    if verify:
//	        assert envelope.state_root == hash_tree_root(state)
//	</spec>
func ProcessExecutionPayload(
	ctx context.Context,
	st state.BeaconState,
	signedEnvelope interfaces.ROSignedExecutionPayloadEnvelope,
) error {
	if err := VerifyExecutionPayloadEnvelopeSignature(st, signedEnvelope); err != nil {
		return errors.Wrap(err, "signature verification failed")
	}

	envelope, err := signedEnvelope.Envelope()
	if err != nil {
		return errors.Wrap(err, "could not get envelope from signed envelope")
	}

	if err := ApplyExecutionPayload(ctx, st, envelope); err != nil {
		return err
	}

	r, err := st.HashTreeRoot(ctx)
	if err != nil {
		return errors.Wrap(err, "could not get hash tree root")
	}
	if r != envelope.StateRoot() {
		return fmt.Errorf("state root mismatch: expected %#x, got %#x", envelope.StateRoot(), r)
	}

	return nil
}

// ApplyExecutionPayload applies the execution payload envelope to the state and performs the same
// consistency checks as the full processing path. This keeps the post-payload state root computation
// on a shared code path, even though some bid/payload checks are not strictly required for the root itself.
func ApplyExecutionPayload(
	ctx context.Context,
	st state.BeaconState,
	envelope interfaces.ROExecutionPayloadEnvelope,
) error {
	latestHeader := st.LatestBlockHeader()
	if len(latestHeader.StateRoot) == 0 || bytes.Equal(latestHeader.StateRoot, make([]byte, 32)) {
		previousStateRoot, err := st.HashTreeRoot(ctx)
		if err != nil {
			return errors.Wrap(err, "could not compute state root")
		}
		latestHeader.StateRoot = previousStateRoot[:]
		if err := st.SetLatestBlockHeader(latestHeader); err != nil {
			return errors.Wrap(err, "could not set latest block header")
		}
	}

	blockHeaderRoot, err := latestHeader.HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not compute block header root")
	}

	beaconBlockRoot := envelope.BeaconBlockRoot()
	if !bytes.Equal(beaconBlockRoot[:], blockHeaderRoot[:]) {
		return errors.Errorf("envelope beacon block root does not match state latest block header root: envelope=%#x, header=%#x", beaconBlockRoot, blockHeaderRoot)
	}

	if envelope.Slot() != st.Slot() {
		return errors.Errorf("envelope slot does not match state slot: envelope=%d, state=%d", envelope.Slot(), st.Slot())
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

	if err := processExecutionRequests(ctx, st, envelope.ExecutionRequests()); err != nil {
		return errors.Wrap(err, "could not process execution requests")
	}

	if err := st.QueueBuilderPayment(); err != nil {
		return errors.Wrap(err, "could not queue builder payment")
	}

	if err := st.SetExecutionPayloadAvailability(st.Slot(), true); err != nil {
		return errors.Wrap(err, "could not set execution payload availability")
	}

	if err := st.SetLatestBlockHash([32]byte(payload.BlockHash())); err != nil {
		return errors.Wrap(err, "could not set latest block hash")
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

// processExecutionRequests processes deposits, withdrawals, and consolidations from execution requests.
// Spec v1.7.0-alpha.0 (pseudocode):
// for op in requests.deposits: process_deposit_request(state, op)
// for op in requests.withdrawals: process_withdrawal_request(state, op)
// for op in requests.consolidations: process_consolidation_request(state, op)
func processExecutionRequests(ctx context.Context, st state.BeaconState, rqs *enginev1.ExecutionRequests) error {
	if err := processDepositRequests(ctx, st, rqs.Deposits); err != nil {
		return errors.Wrap(err, "could not process deposit requests")
	}

	var err error
	st, err = requests.ProcessWithdrawalRequests(ctx, st, rqs.Withdrawals)
	if err != nil {
		return errors.Wrap(err, "could not process withdrawal requests")
	}
	err = requests.ProcessConsolidationRequests(ctx, st, rqs.Consolidations)
	if err != nil {
		return errors.Wrap(err, "could not process consolidation requests")
	}
	return nil
}

// VerifyExecutionPayloadEnvelopeSignature verifies the BLS signature on a signed execution payload envelope.
// <spec fn="verify_execution_payload_envelope_signature" fork="gloas" style="full" hash="49483ae2">
// def verify_execution_payload_envelope_signature(
//
//	state: BeaconState, signed_envelope: SignedExecutionPayloadEnvelope
//
// ) -> bool:
//
//	builder_index = signed_envelope.message.builder_index
//	if builder_index == BUILDER_INDEX_SELF_BUILD:
//	    validator_index = state.latest_block_header.proposer_index
//	    pubkey = state.validators[validator_index].pubkey
//	else:
//	    pubkey = state.builders[builder_index].pubkey
//
//	signing_root = compute_signing_root(
//	    signed_envelope.message, get_domain(state, DOMAIN_BEACON_BUILDER)
//	)
//	return bls.Verify(pubkey, signing_root, signed_envelope.signature)
//
// </spec>
func VerifyExecutionPayloadEnvelopeSignature(st state.BeaconState, signedEnvelope interfaces.ROSignedExecutionPayloadEnvelope) error {
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
