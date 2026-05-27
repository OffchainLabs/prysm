package gloas

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// ProcessExecutionPayloadBid processes a signed execution payload bid in the Gloas fork.
//
//	<spec fn="process_execution_payload_bid" fork="gloas" hash="823c9f3a">
//	def process_execution_payload_bid(state: BeaconState, block: BeaconBlock) -> None:
//	    signed_bid = block.body.signed_execution_payload_bid
//	    bid = signed_bid.message
//	    builder_index = bid.builder_index
//	    amount = bid.value
//
//	    # For self-builds, amount must be zero regardless of withdrawal credential prefix
//	    if builder_index == BUILDER_INDEX_SELF_BUILD:
//	        assert amount == 0
//	        assert signed_bid.signature == bls.G2_POINT_AT_INFINITY
//	    else:
//	        # Verify that the builder is active
//	        assert is_active_builder(state, builder_index)
//	        # Verify that the builder has funds to cover the bid
//	        assert can_builder_cover_bid(state, builder_index, amount)
//	        # Verify that the bid signature is valid
//	        assert verify_execution_payload_bid_signature(state, signed_bid)
//
//	    # Verify commitments are under limit
//	    assert (
//	        len(bid.blob_kzg_commitments)
//	        <= get_blob_parameters(get_current_epoch(state)).max_blobs_per_block
//	    )
//
//	    # Verify that the bid is for the current slot
//	    assert bid.slot == block.slot
//	    # Verify that the bid is for the right parent block
//	    assert bid.parent_block_hash == state.latest_block_hash
//	    assert bid.parent_block_root == block.parent_root
//	    assert bid.prev_randao == get_randao_mix(state, get_current_epoch(state))
//
//	    # Record the pending payment if there is some payment
//	    if amount > 0:
//	        pending_payment = BuilderPendingPayment(
//	            weight=0,
//	            withdrawal=BuilderPendingWithdrawal(
//	                fee_recipient=bid.fee_recipient,
//	                amount=amount,
//	                builder_index=builder_index,
//	            ),
//	        )
//	        state.builder_pending_payments[SLOTS_PER_EPOCH + bid.slot % SLOTS_PER_EPOCH] = (
//	            pending_payment
//	        )
//
//	    # Cache the signed execution payload bid
//	    state.latest_execution_payload_bid = bid
//	</spec>
func ProcessExecutionPayloadBid(st state.BeaconState, block interfaces.ReadOnlyBeaconBlock) error {
	signedBid, err := block.Body().SignedExecutionPayloadBid()
	if err != nil {
		return errors.Wrap(err, "failed to get signed execution payload bid")
	}

	wrappedBid, err := blocks.WrappedROSignedExecutionPayloadBid(signedBid)
	if err != nil {
		return errors.Wrap(err, "failed to wrap signed bid")
	}

	bid, err := wrappedBid.Bid()
	if err != nil {
		return errors.Wrap(err, "failed to get bid from wrapped bid")
	}

	builderIndex := bid.BuilderIndex()
	amount := bid.Value()

	if builderIndex == params.BeaconConfig().BuilderIndexSelfBuild {
		if amount != 0 {
			return fmt.Errorf("self-build amount must be zero, got %d", amount)
		}
		if wrappedBid.Signature() != common.InfiniteSignature {
			return errors.New("self-build signature must be point at infinity")
		}
	} else {
		ok, err := st.IsActiveBuilder(builderIndex)
		if err != nil {
			return errors.Wrap(err, "builder active check failed")
		}
		if !ok {
			return fmt.Errorf("builder %d is not active", builderIndex)
		}

		ok, err = st.CanBuilderCoverBid(builderIndex, amount)
		if err != nil {
			return errors.Wrap(err, "builder balance check failed")
		}
		if !ok {
			return fmt.Errorf("builder %d cannot cover bid amount %d", builderIndex, amount)
		}

		if err := ValidatePayloadBidSignature(st, wrappedBid); err != nil {
			return errors.Wrap(err, "bid signature validation failed")
		}
	}

	maxBlobsPerBlock := params.BeaconConfig().MaxBlobsPerBlockAtEpoch(slots.ToEpoch(block.Slot()))
	commitmentCount := bid.BlobKzgCommitmentCount()
	if commitmentCount > uint64(maxBlobsPerBlock) {
		return fmt.Errorf("bid has %d blob KZG commitments over max %d", commitmentCount, maxBlobsPerBlock)
	}

	if err := validateBidConsistency(st, bid, block); err != nil {
		return errors.Wrap(err, "bid consistency validation failed")
	}

	if amount > 0 {
		feeRecipient := bid.FeeRecipient()
		pendingPayment := &ethpb.BuilderPendingPayment{
			Weight: 0,
			Withdrawal: &ethpb.BuilderPendingWithdrawal{
				FeeRecipient: feeRecipient[:],
				Amount:       amount,
				BuilderIndex: builderIndex,
			},
		}
		slotIndex := params.BeaconConfig().SlotsPerEpoch + (bid.Slot() % params.BeaconConfig().SlotsPerEpoch)
		if err := st.SetBuilderPendingPayment(slotIndex, pendingPayment); err != nil {
			return errors.Wrap(err, "failed to set pending payment")
		}
	}

	if err := st.SetExecutionPayloadBid(bid); err != nil {
		return errors.Wrap(err, "failed to cache execution payload bid")
	}

	return nil
}

// validateBidConsistency checks that the bid is consistent with the current beacon state.
func validateBidConsistency(st state.BeaconState, bid interfaces.ROExecutionPayloadBid, block interfaces.ReadOnlyBeaconBlock) error {
	if bid.Slot() != block.Slot() {
		return fmt.Errorf("bid slot %d does not match block slot %d", bid.Slot(), block.Slot())
	}

	latestBlockHash, err := st.LatestBlockHash()
	if err != nil {
		return errors.Wrap(err, "failed to get latest block hash")
	}
	if bid.ParentBlockHash() != latestBlockHash {
		return fmt.Errorf("bid parent block hash mismatch: got %x, expected %x",
			bid.ParentBlockHash(), latestBlockHash)
	}

	if bid.ParentBlockRoot() != block.ParentRoot() {
		return fmt.Errorf("bid parent block root mismatch: got %x, expected %x",
			bid.ParentBlockRoot(), block.ParentRoot())
	}

	randaoMix, err := helpers.RandaoMix(st, slots.ToEpoch(st.Slot()))
	if err != nil {
		return errors.Wrap(err, "failed to get randao mix")
	}
	if bid.PrevRandao() != [32]byte(randaoMix) {
		return fmt.Errorf("bid prev randao mismatch: got %x, expected %x", bid.PrevRandao(), randaoMix)
	}

	return nil
}

// ValidatePayloadBidSignature verifies the BLS signature on a signed execution payload bid.
// It validates that the signature was created by the builder specified in the bid
// using the appropriate domain for the beacon builder.
func ValidatePayloadBidSignature(st state.ReadOnlyBeaconState, signedBid interfaces.ROSignedExecutionPayloadBid) error {
	bid, err := signedBid.Bid()
	if err != nil {
		return errors.Wrap(err, "failed to get bid")
	}

	pubkey, err := st.BuilderPubkey(bid.BuilderIndex())
	if err != nil {
		return errors.Wrap(err, "failed to get builder pubkey")
	}

	publicKey, err := bls.PublicKeyFromBytes(pubkey[:])
	if err != nil {
		return errors.Wrap(err, "invalid builder public key")
	}

	signatureBytes := signedBid.Signature()
	signature, err := bls.SignatureFromBytes(signatureBytes[:])
	if err != nil {
		return errors.Wrap(err, "invalid signature format")
	}

	currentEpoch := slots.ToEpoch(bid.Slot())
	domain, err := signing.Domain(
		st.Fork(),
		currentEpoch,
		params.BeaconConfig().DomainBeaconBuilder,
		st.GenesisValidatorsRoot(),
	)
	if err != nil {
		return errors.Wrap(err, "failed to compute signing domain")
	}

	signingRoot, err := signedBid.SigningRoot(domain)
	if err != nil {
		return errors.Wrap(err, "failed to compute signing root")
	}

	if !signature.Verify(publicKey, signingRoot[:]) {
		return signing.ErrSigFailedToVerify
	}

	return nil
}
