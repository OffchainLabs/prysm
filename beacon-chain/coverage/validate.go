package coverage

import (
	"bytes"

	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// ValidateEnvelopeAgainstBlock checks a stored blinded envelope against its
// canonical beacon block and that block's committed bid: slot, beacon root,
// parent beacon root, block hash, parent block hash, builder index, and
// execution-requests root must all match. It is shared by the coverage
// coordinator (before an index entry is published) and the by-range serving
// path (before an indexed item is streamed).
func ValidateEnvelopeAgainstBlock(env *ethpb.SignedBlindedExecutionPayloadEnvelope, blk interfaces.ReadOnlySignedBeaconBlock, root [32]byte) error {
	if env == nil || env.Message == nil {
		return errors.New("nil blinded execution payload envelope")
	}
	if blk == nil || blk.IsNil() {
		return errors.New("nil beacon block")
	}
	m := env.Message
	b := blk.Block()
	if m.Slot != b.Slot() {
		return errors.Errorf("envelope slot %d does not match block slot %d", m.Slot, b.Slot())
	}
	if bytesutil.ToBytes32(m.BeaconBlockRoot) != root {
		return errors.Errorf("envelope beacon block root %#x does not match block root %#x", m.BeaconBlockRoot, root)
	}
	if bytesutil.ToBytes32(m.ParentBeaconBlockRoot) != b.ParentRoot() {
		return errors.Errorf("envelope parent beacon block root %#x does not match block parent root %#x", m.ParentBeaconBlockRoot, b.ParentRoot())
	}
	bid, err := b.Body().SignedExecutionPayloadBid()
	if err != nil {
		return errors.Wrap(err, "could not get execution payload bid")
	}
	if bid == nil || bid.Message == nil {
		return errors.New("nil execution payload bid")
	}
	if !bytes.Equal(m.BlockHash, bid.Message.BlockHash) {
		return errors.Errorf("envelope block hash %#x does not match bid block hash %#x", m.BlockHash, bid.Message.BlockHash)
	}
	if !bytes.Equal(m.ParentBlockHash, bid.Message.ParentBlockHash) {
		return errors.Errorf("envelope parent block hash %#x does not match bid parent block hash %#x", m.ParentBlockHash, bid.Message.ParentBlockHash)
	}
	if m.BuilderIndex != bid.Message.BuilderIndex {
		return errors.Errorf("envelope builder index %d does not match bid builder index %d", m.BuilderIndex, bid.Message.BuilderIndex)
	}
	if m.ExecutionRequests == nil {
		return errors.New("nil envelope execution requests")
	}
	reqRoot, err := m.ExecutionRequests.HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not hash envelope execution requests")
	}
	if !bytes.Equal(reqRoot[:], bid.Message.ExecutionRequestsRoot) {
		return errors.Errorf("envelope execution requests root %#x does not match bid %#x", reqRoot, bid.Message.ExecutionRequestsRoot)
	}
	return nil
}
