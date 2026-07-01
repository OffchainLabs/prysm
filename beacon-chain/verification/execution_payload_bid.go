package verification

import (
	"bytes"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// ExecutionPayloadBidGossipRequirements defines the list of requirements for gossip execution payload bids.
var ExecutionPayloadBidGossipRequirements = []Requirement{
	RequireBidCurrentOrNextSlot,
	RequireBidBuilderActive,
	RequireBidBuilderVersionValid,
	RequireBidExecutionPaymentZero,
	RequireBidFeeRecipientMatches,
	RequireBidBlobKzgCommitmentsLimit,
	RequireBidPrevRandaoValid,
	RequireBidParentBlockRootSeen,
	RequireBidSlotHigherThanParent,
	RequireBidParentBlockHashValid,
	RequireBidGasLimitCompatible,
	RequireBidBuilderCanCover,
	RequireBidSignatureValid,
}

// GossipExecutionPayloadBidRequirements is a requirement list for gossip execution payload bids.
var GossipExecutionPayloadBidRequirements = requirementList(ExecutionPayloadBidGossipRequirements)

var (
	ErrBidSlotNotCurrentOrNext      = errors.New("bid slot is not current or next")
	ErrBidBuilderNotActive          = errors.New("builder is not active")
	ErrBidBuilderVersionInvalid     = errors.New("builder is not a payload builder")
	ErrBidExecutionPaymentNonZero   = errors.New("execution payment is non-zero")
	ErrBidFeeRecipientMismatch      = errors.New("fee recipient does not match proposer preferences")
	ErrBidTooManyBlobKzgCommitments = errors.New("bid has too many blob KZG commitments")
	ErrBidPrevRandaoMismatch        = errors.New("bid prev randao does not match state randao mix")
	ErrBidGasLimitIncompatible      = errors.New("bid gas limit is incompatible with parent and target")
	ErrBidParentBlockRootNotSeen    = errors.New("parent block root not seen")
	ErrBidSlotNotHigherThanParent   = errors.New("bid slot is not higher than parent block slot")
	ErrBidParentBlockHashMismatch   = errors.New("parent block hash does not match forkchoice")
	ErrBidBuilderCannotCover        = errors.New("builder cannot cover bid")
)

// payloadBuilderVersion is PAYLOAD_BUILDER_VERSION: the builder version byte
// required for an execution payload builder (EIP-7732).
const payloadBuilderVersion = 0

var _ ExecutionPayloadBidVerifier = &BidVerifier{}

// BidVerifier is a read-only verifier for execution payload bids.
type BidVerifier struct {
	*sharedResources
	results *results
	b       interfaces.ROSignedExecutionPayloadBid
}

// VerifyCurrentOrNextSlot verifies the bid slot is for the current or next slot.
func (v *BidVerifier) VerifyCurrentOrNextSlot() (err error) {
	defer v.record(RequireBidCurrentOrNextSlot, &err)

	bid, err := v.b.Bid()
	if err != nil {
		return errors.Wrap(err, "failed to get bid")
	}
	currentSlot := v.clock.CurrentSlot()
	if bid.Slot() != currentSlot && bid.Slot() != currentSlot+1 {
		return fmt.Errorf("%w: got %d want %d or %d", ErrBidSlotNotCurrentOrNext, bid.Slot(), currentSlot, currentSlot+1)
	}
	return nil
}

// VerifyBuilderActive verifies the bid builder index refers to an active builder.
func (v *BidVerifier) VerifyBuilderActive(st state.ReadOnlyBeaconState) (err error) {
	defer v.record(RequireBidBuilderActive, &err)

	bid, err := v.b.Bid()
	if err != nil {
		return errors.Wrap(err, "failed to get bid")
	}
	active, err := st.IsActiveBuilder(bid.BuilderIndex())
	if err != nil {
		return errors.Wrap(err, "builder active check failed")
	}
	if !active {
		return fmt.Errorf("%w: builder=%d", ErrBidBuilderNotActive, bid.BuilderIndex())
	}
	return nil
}

// VerifyBuilderVersion verifies the bid builder's version is PAYLOAD_BUILDER_VERSION.
func (v *BidVerifier) VerifyBuilderVersion(st state.ReadOnlyBeaconState) (err error) {
	defer v.record(RequireBidBuilderVersionValid, &err)

	bid, err := v.b.Bid()
	if err != nil {
		return errors.Wrap(err, "failed to get bid")
	}
	builder, err := st.Builder(bid.BuilderIndex())
	if err != nil {
		return errors.Wrap(err, "failed to get builder")
	}
	if len(builder.Version) == 0 || builder.Version[0] != payloadBuilderVersion {
		return fmt.Errorf("%w: builder=%d version=%x", ErrBidBuilderVersionInvalid, bid.BuilderIndex(), builder.Version)
	}
	return nil
}

// VerifyBlobKzgCommitmentsLimit verifies the bid blob KZG commitment count does
// not exceed the per-block limit for the bid's slot.
func (v *BidVerifier) VerifyBlobKzgCommitmentsLimit() (err error) {
	defer v.record(RequireBidBlobKzgCommitmentsLimit, &err)

	bid, err := v.b.Bid()
	if err != nil {
		return errors.Wrap(err, "failed to get bid")
	}
	maxBlobs := uint64(params.BeaconConfig().MaxBlobsPerBlockAtEpoch(slots.ToEpoch(bid.Slot())))
	if count := bid.BlobKzgCommitmentCount(); count > maxBlobs {
		return fmt.Errorf("%w: got %d max %d", ErrBidTooManyBlobKzgCommitments, count, maxBlobs)
	}
	return nil
}

// VerifyPrevRandao verifies the bid prev_randao matches the state's RANDAO mix
// for the current epoch.
func (v *BidVerifier) VerifyPrevRandao(st state.ReadOnlyBeaconState) (err error) {
	defer v.record(RequireBidPrevRandaoValid, &err)

	bid, err := v.b.Bid()
	if err != nil {
		return errors.Wrap(err, "failed to get bid")
	}
	mix, err := helpers.RandaoMix(st, slots.ToEpoch(st.Slot()))
	if err != nil {
		return errors.Wrap(err, "failed to get randao mix")
	}
	prevRandao := bid.PrevRandao()
	if !bytes.Equal(prevRandao[:], mix) {
		return fmt.Errorf("%w: bid=%#x state=%#x", ErrBidPrevRandaoMismatch, prevRandao, mix)
	}
	return nil
}

// VerifyExecutionPaymentZero verifies the bid execution payment is zero.
// Bids with non-zero execution_payment indicate trusted EL payments and
// MUST NOT be broadcast on the gossip network.
func (v *BidVerifier) VerifyExecutionPaymentZero() (err error) {
	defer v.record(RequireBidExecutionPaymentZero, &err)

	bid, err := v.b.Bid()
	if err != nil {
		return errors.Wrap(err, "failed to get bid")
	}
	if bid.ExecutionPayment() != 0 {
		return fmt.Errorf("%w: builder=%d slot=%d payment=%d", ErrBidExecutionPaymentNonZero, bid.BuilderIndex(), bid.Slot(), bid.ExecutionPayment())
	}
	return nil
}

// VerifyFeeRecipientMatches verifies the bid fee recipient matches the expected proposer preferences value.
func (v *BidVerifier) VerifyFeeRecipientMatches(expected []byte) (err error) {
	defer v.record(RequireBidFeeRecipientMatches, &err)

	bid, err := v.b.Bid()
	if err != nil {
		return errors.Wrap(err, "failed to get bid")
	}
	bidFeeRecipient := bid.FeeRecipient()
	if !bytes.Equal(expected, bidFeeRecipient[:]) {
		return fmt.Errorf("%w: bid=%#x expected=%#x", ErrBidFeeRecipientMismatch, bidFeeRecipient, expected)
	}
	return nil
}

// VerifyGasLimitTargetCompatible verifies the bid gas limit is compatible with
// the parent payload's gas limit and the proposer's target via the EIP-1559
// elasticity rule.
func (v *BidVerifier) VerifyGasLimitTargetCompatible(parentGasLimit, targetGasLimit uint64) (err error) {
	defer v.record(RequireBidGasLimitCompatible, &err)

	bid, err := v.b.Bid()
	if err != nil {
		return errors.Wrap(err, "failed to get bid")
	}
	if !isGasLimitTargetCompatible(parentGasLimit, bid.GasLimit(), targetGasLimit) {
		return fmt.Errorf("%w: bid=%d parent=%d target=%d", ErrBidGasLimitIncompatible, bid.GasLimit(), parentGasLimit, targetGasLimit)
	}
	return nil
}

// isGasLimitTargetCompatible reports whether gasLimit is compatible with
// targetGasLimit under the EIP-1559 transition rule from parentGasLimit.
//
//	<spec fn="is_gas_limit_target_compatible" fork="gloas" hash="3fa22023">
//	def is_gas_limit_target_compatible(
//	    parent_gas_limit: uint64, gas_limit: uint64, target_gas_limit: uint64
//	) -> bool:
//	    """
//	    Check if ``gas_limit`` is compatible with ``target_gas_limit`` under the
//	    EIP-1559 transition rule from ``parent_gas_limit``.
//	    """
//	    max_gas_limit_difference = max(parent_gas_limit // 1024, 1) - 1
//	    min_gas_limit = parent_gas_limit - max_gas_limit_difference
//	    max_gas_limit = parent_gas_limit + max_gas_limit_difference
//
//	    if target_gas_limit >= min_gas_limit and target_gas_limit <= max_gas_limit:
//	        return gas_limit == target_gas_limit
//	    if target_gas_limit > max_gas_limit:
//	        return gas_limit == max_gas_limit
//	    return gas_limit == min_gas_limit
//	</spec>
func isGasLimitTargetCompatible(parentGasLimit, gasLimit, targetGasLimit uint64) bool {
	maxDiff := max(parentGasLimit/1024, 1) - 1
	minLimit := parentGasLimit - maxDiff
	maxLimit := parentGasLimit + maxDiff
	return gasLimit == min(max(targetGasLimit, minLimit), maxLimit)
}

// VerifyParentBlockRootSeen verifies the parent beacon block root is known.
func (v *BidVerifier) VerifyParentBlockRootSeen(parentSeen func([32]byte) bool) (err error) {
	defer v.record(RequireBidParentBlockRootSeen, &err)

	bid, err := v.b.Bid()
	if err != nil {
		return errors.Wrap(err, "failed to get bid")
	}
	root := bid.ParentBlockRoot()
	if parentSeen != nil && parentSeen(root) {
		return nil
	}
	return fmt.Errorf("%w: root=%#x", ErrBidParentBlockRootNotSeen, root)
}

// VerifyBidSlotHigherThanParent verifies the bid slot is greater than the slot of its parent block.
func (v *BidVerifier) VerifyBidSlotHigherThanParent(parentSlot primitives.Slot) (err error) {
	defer v.record(RequireBidSlotHigherThanParent, &err)

	bid, err := v.b.Bid()
	if err != nil {
		return errors.Wrap(err, "failed to get bid")
	}
	if bid.Slot() <= parentSlot {
		return fmt.Errorf("%w: bid=%d parent=%d", ErrBidSlotNotHigherThanParent, bid.Slot(), parentSlot)
	}
	return nil
}

// VerifyParentBlockHash verifies the parent execution block hash matches forkchoice for the bid parent root.
func (v *BidVerifier) VerifyParentBlockHash(resolveBlockHash func([32]byte) ([32]byte, error)) (err error) {
	defer v.record(RequireBidParentBlockHashValid, &err)

	bid, err := v.b.Bid()
	if err != nil {
		return errors.Wrap(err, "failed to get bid")
	}
	if resolveBlockHash == nil {
		return fmt.Errorf("%w: no parent block hash resolver", ErrBidParentBlockHashMismatch)
	}
	parentHash, err := resolveBlockHash(bid.ParentBlockRoot())
	if err != nil {
		return errors.Wrap(err, "failed to resolve parent block hash")
	}
	if parentHash != bid.ParentBlockHash() {
		return fmt.Errorf("%w: bid=%#x forkchoice=%#x", ErrBidParentBlockHashMismatch, bid.ParentBlockHash(), parentHash)
	}
	return nil
}

// VerifyBuilderCanCoverBid verifies the builder has enough balance to cover the bid value.
func (v *BidVerifier) VerifyBuilderCanCoverBid(st state.ReadOnlyBeaconState) (err error) {
	defer v.record(RequireBidBuilderCanCover, &err)

	bid, err := v.b.Bid()
	if err != nil {
		return errors.Wrap(err, "failed to get bid")
	}
	ok, err := st.CanBuilderCoverBid(bid.BuilderIndex(), bid.Value())
	if err != nil {
		return errors.Wrap(err, "builder balance check failed")
	}
	if !ok {
		return fmt.Errorf("%w: builder=%d amount=%d", ErrBidBuilderCannotCover, bid.BuilderIndex(), bid.Value())
	}
	return nil
}

// VerifySignature verifies the bid signature against the builder's public key.
func (v *BidVerifier) VerifySignature(st state.ReadOnlyBeaconState) (err error) {
	defer v.record(RequireBidSignatureValid, &err)
	return gloas.ValidatePayloadBidSignature(st, v.b)
}

// SatisfyRequirement allows the caller to manually mark a requirement as satisfied.
func (v *BidVerifier) SatisfyRequirement(req Requirement) {
	v.record(req, nil)
}

func (v *BidVerifier) record(req Requirement, err *error) {
	if err == nil || *err == nil {
		v.results.record(req, nil)
		return
	}

	v.results.record(req, *err)
}
