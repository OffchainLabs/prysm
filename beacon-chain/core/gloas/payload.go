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
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// ProcessExecutionPayload processes the signed execution payload envelope for the Gloas fork.
// Spec v1.7.0-alpha.0 (pseudocode):
// def process_execution_payload(
//
//	state: BeaconState,
//	signed_envelope: SignedExecutionPayloadEnvelope,
//	execution_engine: ExecutionEngine,
//	verify: bool = True,
//
// ) -> None:
//
//	envelope = signed_envelope.message
//	payload = envelope.payload
//
//	if verify:
//	    assert verify_execution_payload_envelope_signature(state, signed_envelope)
//
//	previous_state_root = hash_tree_root(state)
//	if state.latest_block_header.state_root == Root():
//	    state.latest_block_header.state_root = previous_state_root
//
//	assert envelope.beacon_block_root == hash_tree_root(state.latest_block_header)
//	assert envelope.slot == state.slot
//
//	committed_bid = state.latest_execution_payload_bid
//	assert envelope.builder_index == committed_bid.builder_index
//	assert committed_bid.blob_kzg_commitments_root == hash_tree_root(envelope.blob_kzg_commitments)
//	assert committed_bid.prev_randao == payload.prev_randao
//
//	assert hash_tree_root(payload.withdrawals) == hash_tree_root(state.payload_expected_withdrawals)
//
//	assert committed_bid.gas_limit == payload.gas_limit
//	assert committed_bid.block_hash == payload.block_hash
//	assert payload.parent_hash == state.latest_block_hash
//	assert payload.timestamp == compute_time_at_slot(state, state.slot)
//	assert (
//	    len(envelope.blob_kzg_commitments)
//	    <= get_blob_parameters(get_current_epoch(state)).max_blobs_per_block
//	)
//	versioned_hashes = [
//	    kzg_commitment_to_versioned_hash(commitment) for commitment in envelope.blob_kzg_commitments
//	]
//	requests = envelope.execution_requests
//	assert execution_engine.verify_and_notify_new_payload(
//	    NewPayloadRequest(
//	        execution_payload=payload,
//	        versioned_hashes=versioned_hashes,
//	        parent_beacon_block_root=state.latest_block_header.parent_root,
//	        execution_requests=requests,
//	    )
//	)
//
//	for op in requests.deposits: process_deposit_request(state, op)
//	for op in requests.withdrawals: process_withdrawal_request(state, op)
//	for op in requests.consolidations: process_consolidation_request(state, op)
//
//	payment = state.builder_pending_payments[SLOTS_PER_EPOCH + state.slot % SLOTS_PER_EPOCH]
//	amount = payment.withdrawal.amount
//	if amount > 0:
//	    state.builder_pending_withdrawals.append(payment.withdrawal)
//	state.builder_pending_payments[SLOTS_PER_EPOCH + state.slot % SLOTS_PER_EPOCH] = (
//	    BuilderPendingPayment()
//	)
//
//	state.execution_payload_availability[state.slot % SLOTS_PER_HISTORICAL_ROOT] = 0b1
//	state.latest_block_hash = payload.block_hash
//
//	if verify:
//	    assert envelope.state_root == hash_tree_root(state)
func ProcessExecutionPayload(
	ctx context.Context,
	st state.BeaconState,
	signedEnvelope interfaces.ROSignedExecutionPayloadEnvelope,
) error {
	if err := verifyExecutionPayloadEnvelopeSignature(st, signedEnvelope); err != nil {
		return errors.Wrap(err, "signature verification failed")
	}

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
	envelope, err := signedEnvelope.Envelope()
	if err != nil {
		return errors.Wrap(err, "could not get envelope from signed envelope")
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

	envelopeBlobCommitments := envelope.BlobKzgCommitments()
	envelopeBlobRoot, err := ssz.KzgCommitmentsRoot(envelopeBlobCommitments)
	if err != nil {
		return errors.Wrap(err, "could not compute envelope blob KZG commitments root")
	}
	committedBlobRoot := latestBid.BlobKzgCommitmentsRoot()
	if !bytes.Equal(committedBlobRoot[:], envelopeBlobRoot[:]) {
		return errors.Errorf("committed bid blob KZG commitments root does not match envelope: bid=%#x, envelope=%#x", committedBlobRoot, envelopeBlobRoot)
	}

	payload, err := envelope.Execution()
	if err != nil {
		return errors.Wrap(err, "could not get execution payload from envelope")
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

	latestBidPrevRandao := latestBid.PrevRandao()
	if !bytes.Equal(payload.PrevRandao(), latestBidPrevRandao[:]) {
		return errors.Errorf("payload prev randao does not match committed bid prev randao: payload=%#x, bid=%#x", payload.PrevRandao(), latestBidPrevRandao)
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

	cfg := params.BeaconConfig()
	maxBlobsPerBlock := cfg.MaxBlobsPerBlock(envelope.Slot())
	if len(envelopeBlobCommitments) > maxBlobsPerBlock {
		return errors.Errorf("too many blob KZG commitments: got=%d, max=%d", len(envelopeBlobCommitments), maxBlobsPerBlock)
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

	r, err := st.HashTreeRoot(ctx)
	if err != nil {
		return errors.Wrap(err, "could not get hash tree root")
	}
	if r != envelope.StateRoot() {
		return fmt.Errorf("state root mismatch: expected %#x, got %#x", envelope.StateRoot(), r)
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

// verifyExecutionPayloadEnvelopeSignature verifies the BLS signature on a signed execution payload envelope.
// Spec v1.7.0-alpha.0 (pseudocode):
// builder_index = signed_envelope.message.builder_index
// if builder_index == BUILDER_INDEX_SELF_BUILD:
//
//	validator_index = state.latest_block_header.proposer_index
//	pubkey = state.validators[validator_index].pubkey
//
// else:
//
//	pubkey = state.builders[builder_index].pubkey
//
// signing_root = compute_signing_root(
//
//	signed_envelope.message, get_domain(state, DOMAIN_BEACON_BUILDER)
//
// )
// return bls.Verify(pubkey, signing_root, signed_envelope.signature)
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
