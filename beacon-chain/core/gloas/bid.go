package gloas

import (
	"errors"
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
)

// ProcessExecutionPayloadBid processes a signed execution payload bid in the Gloas fork.
// Spec v1.7.0-alpha.0 (pseudocode):
// process_execution_payload_bid(state: BeaconState, block: BeaconBlock):
//
//	signed_bid = block.body.signed_execution_payload_bid
//	bid = signed_bid.message
//	builder_index = bid.builder_index
//	amount = bid.value
//	if builder_index == BUILDER_INDEX_SELF_BUILD:
//	  assert amount == 0
//	  assert signed_bid.signature == G2_POINT_AT_INFINITY
//	else:
//	  assert is_active_builder(state, builder_index)
//	  assert can_builder_cover_bid(state, builder_index, amount)
//	  assert verify_execution_payload_bid_signature(state, signed_bid)
//	assert bid.slot == block.slot
//	assert bid.parent_block_hash == state.latest_block_hash
//	assert bid.parent_block_root == block.parent_root
//	assert bid.prev_randao == get_randao_mix(state, get_current_epoch(state))
//	if amount > 0:
//	  state.builder_pending_payments[...] = BuilderPendingPayment(weight=0, withdrawal=BuilderPendingWithdrawal(fee_recipient=bid.fee_recipient, amount=amount, builder_index=builder_index))
//	state.latest_execution_payload_bid = bid
func ProcessExecutionPayloadBid(st state.BeaconState, block interfaces.ReadOnlyBeaconBlock) error {
	signedBid, err := block.Body().SignedExecutionPayloadBid()
	if err != nil {
		return fmt.Errorf("failed to get signed execution payload bid: %w", err)
	}

	wrappedBid, err := blocks.WrappedROSignedExecutionPayloadBid(signedBid)
	if err != nil {
		return fmt.Errorf("failed to wrap signed bid: %w", err)
	}

	bid, err := wrappedBid.Bid()
	if err != nil {
		return fmt.Errorf("failed to get bid from wrapped bid: %w", err)
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
			return fmt.Errorf("builder active check failed: %w", err)
		}
		if !ok {
			return fmt.Errorf("builder %d is not active", builderIndex)
		}

		ok, err = st.CanBuilderCoverBid(builderIndex, amount)
		if err != nil {
			return fmt.Errorf("builder balance check failed: %w", err)
		}
		if !ok {
			return fmt.Errorf("builder %d cannot cover bid amount %d", builderIndex, amount)
		}

		if err := validatePayloadBidSignature(st, wrappedBid); err != nil {
			return fmt.Errorf("bid signature validation failed: %w", err)
		}
	}

	if err := validateBidConsistency(st, bid, block); err != nil {
		return fmt.Errorf("bid consistency validation failed: %w", err)
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
			return fmt.Errorf("failed to set pending payment: %w", err)
		}
	}

	if err := st.SetExecutionPayloadBid(bid); err != nil {
		return fmt.Errorf("failed to cache execution payload bid: %w", err)
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
		return fmt.Errorf("failed to get latest block hash: %w", err)
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
		return fmt.Errorf("failed to get randao mix: %w", err)
	}
	if bid.PrevRandao() != [32]byte(randaoMix) {
		return fmt.Errorf("bid prev randao mismatch: got %x, expected %x", bid.PrevRandao(), randaoMix)
	}

	return nil
}

// validatePayloadBidSignature verifies the BLS signature on a signed execution payload bid.
// It validates that the signature was created by the builder specified in the bid
// using the appropriate domain for the beacon builder.
func validatePayloadBidSignature(st state.ReadOnlyBeaconState, signedBid interfaces.ROSignedExecutionPayloadBid) error {
	bid, err := signedBid.Bid()
	if err != nil {
		return fmt.Errorf("failed to get bid: %w", err)
	}

	pubkey, err := st.BuilderPubkey(bid.BuilderIndex())
	if err != nil {
		return fmt.Errorf("failed to get builder pubkey: %w", err)
	}

	publicKey, err := bls.PublicKeyFromBytes(pubkey[:])
	if err != nil {
		return fmt.Errorf("invalid builder public key: %w", err)
	}

	signatureBytes := signedBid.Signature()
	signature, err := bls.SignatureFromBytes(signatureBytes[:])
	if err != nil {
		return fmt.Errorf("invalid signature format: %w", err)
	}

	currentEpoch := slots.ToEpoch(bid.Slot())
	domain, err := signing.Domain(
		st.Fork(),
		currentEpoch,
		params.BeaconConfig().DomainBeaconBuilder,
		st.GenesisValidatorsRoot(),
	)
	if err != nil {
		return fmt.Errorf("failed to compute signing domain: %w", err)
	}

	signingRoot, err := signedBid.SigningRoot(domain)
	if err != nil {
		return fmt.Errorf("failed to compute signing root: %w", err)
	}

	if !signature.Verify(publicKey, signingRoot[:]) {
		return fmt.Errorf("signature verification failed: %w", signing.ErrSigFailedToVerify)
	}

	return nil
}
