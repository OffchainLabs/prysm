package doublylinkedtree

import (
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// SetPayloadExecutionValid records that the EL validated the payload of the beacon block, and therefore the payloads
// of all its ancestors.
// The caller MUST hold the forkchoice lock.
func (f *ForkChoice) SetPayloadExecutionValid(root [fieldparams.RootLength]byte) error {
	fn, err := f.fullNode(root)
	if err != nil {
		return fmt.Errorf("full node: %w", err)
	}

	for pn := fn; pn != nil; pn = pn.node.parent {
		if !pn.full {
			continue
		}

		if pn.executionValid {
			// Its ancestors are already marked.
			break
		}

		pn.executionValid = true
	}

	return nil
}

// IsExecutionValid reports whether the EL validated the payload the beacon block builds on: its own payload if
// revealed, otherwise the payload of its latest ancestor that has one.
// The caller MUST hold at least the forkchoice read lock.
func (f *ForkChoice) IsExecutionValid(root [fieldparams.RootLength]byte) bool {
	pn := f.store.fullNodeByRoot[root]
	if pn == nil {
		pn = f.store.emptyNodeByRoot[root]
	}

	for ; pn != nil; pn = pn.node.parent {
		if pn.full {
			return pn.executionValid
		}
	}

	return false
}

// PayloadValidationStatus reports whether the EL validated the payload of the beacon block, and the number of
// distinct proof types with a verified execution proof for it.
// The caller MUST hold at least the forkchoice read lock.
func (f *ForkChoice) PayloadValidationStatus(root [fieldparams.RootLength]byte) (executionValid bool, proofTypes int) {
	fn := f.store.fullNodeByRoot[root]
	if fn == nil {
		return false, 0
	}

	return fn.executionValid, len(fn.executionProofTypes)
}

// PayloadInsertionTime returns when the payload of the beacon block was inserted in forkchoice.
// The caller MUST hold at least the forkchoice read lock.
func (f *ForkChoice) PayloadInsertionTime(root [fieldparams.RootLength]byte) (time.Time, bool) {
	fn := f.store.fullNodeByRoot[root]
	if fn == nil {
		return time.Time{}, false
	}

	return fn.timestamp, true
}

// SetNewPayloadRequestRoot records the EIP-8025 new payload request root of the payload of the beacon block,
// which identifies the payload for execution proofs.
// The caller MUST hold the forkchoice lock.
func (f *ForkChoice) SetNewPayloadRequestRoot(root, newPayloadRequestRoot [fieldparams.RootLength]byte) error {
	fn, err := f.fullNode(root)
	if err != nil {
		return fmt.Errorf("full node: %w", err)
	}

	fn.newPayloadRequestRoot = newPayloadRequestRoot

	return nil
}

// NewPayloadRequestRoot returns the EIP-8025 new payload request root recorded for the payload of the beacon block.
// The caller MUST hold at least the forkchoice read lock.
func (f *ForkChoice) NewPayloadRequestRoot(root [fieldparams.RootLength]byte) ([fieldparams.RootLength]byte, bool) {
	fn := f.store.fullNodeByRoot[root]
	if fn == nil || fn.newPayloadRequestRoot == [fieldparams.RootLength]byte{} {
		return [fieldparams.RootLength]byte{}, false
	}

	return fn.newPayloadRequestRoot, true
}

// AddExecutionProofType records that a verified execution proof of this type is known for the payload of the
// beacon block. It returns forkchoice.ErrExecutionProofTypeKnown if the proof type was already known.
// The caller MUST hold the forkchoice lock.
func (f *ForkChoice) AddExecutionProofType(root [fieldparams.RootLength]byte, proofType ethpb.ProofType) error {
	fn, err := f.fullNode(root)
	if err != nil {
		return fmt.Errorf("full node: %w", err)
	}

	if fn.executionProofTypes[proofType] {
		return forkchoice.ErrExecutionProofTypeKnown
	}

	if fn.executionProofTypes == nil {
		fn.executionProofTypes = make(map[ethpb.ProofType]bool)
	}

	fn.executionProofTypes[proofType] = true

	return nil
}

// HasExecutionProofType reports whether a verified execution proof of this type is known for the payload of the
// beacon block.
// The caller MUST hold at least the forkchoice read lock.
func (f *ForkChoice) HasExecutionProofType(root [fieldparams.RootLength]byte, proofType ethpb.ProofType) bool {
	fn := f.store.fullNodeByRoot[root]
	return fn != nil && fn.executionProofTypes[proofType]
}

func (f *ForkChoice) fullNode(root [fieldparams.RootLength]byte) (*PayloadNode, error) {
	fn := f.store.fullNodeByRoot[root]
	if fn == nil {
		return nil, errors.Wrapf(ErrNilNode, "no full node found for root: %#x", bytesutil.Trunc(root[:]))
	}

	return fn, nil
}
