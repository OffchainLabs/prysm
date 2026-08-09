package state

import "errors"

var (
	// ErrNilValidatorsInState returns when accessing validators in the state while the state has a
	// nil slice for the validators field.
	ErrNilValidatorsInState = errors.New("state has nil validator slice")

	// ErrFieldElementProofUnsupported indicates that the state's optimized element proof path
	// cannot prove the requested field element.
	ErrFieldElementProofUnsupported = errors.New("field element proof unsupported")
	// ErrProposerDependentRootUnderflow is returned by ProposerDependentRoot when
	// the proposal epoch is < 2; the spec's fallback to the genesis block root is
	// the caller's responsibility.
	ErrProposerDependentRootUnderflow = errors.New("proposer dependent root: epoch < 2")
	// ErrNoPayloadCommitteeAvailable returns when the state cannot resolve the PTC for the requested slot.
	ErrNoPayloadCommitteeAvailable = errors.New("no payload committee available for slot")
)
