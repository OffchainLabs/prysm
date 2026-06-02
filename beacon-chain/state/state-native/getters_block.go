package state_native

import (
	customtypes "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/custom-types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// ErrProposerDependentRootUnderflow is returned by ProposerDependentRoot when
// the proposal epoch is less than 2, in which case the spec falls back to the
// genesis block root — callers must supply that themselves.
var ErrProposerDependentRootUnderflow = errors.New("proposer dependent root: epoch < 2")

// LatestBlockHeader stored within the beacon state.
func (b *BeaconState) LatestBlockHeader() *ethpb.BeaconBlockHeader {
	if b.latestBlockHeader == nil {
		return nil
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.latestBlockHeaderVal()
}

// latestBlockHeaderVal stored within the beacon state.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) latestBlockHeaderVal() *ethpb.BeaconBlockHeader {
	if b.latestBlockHeader == nil {
		return nil
	}

	hdr := &ethpb.BeaconBlockHeader{
		Slot:          b.latestBlockHeader.Slot,
		ProposerIndex: b.latestBlockHeader.ProposerIndex,
	}

	parentRoot := make([]byte, len(b.latestBlockHeader.ParentRoot))
	bodyRoot := make([]byte, len(b.latestBlockHeader.BodyRoot))
	stateRoot := make([]byte, len(b.latestBlockHeader.StateRoot))

	copy(parentRoot, b.latestBlockHeader.ParentRoot)
	copy(bodyRoot, b.latestBlockHeader.BodyRoot)
	copy(stateRoot, b.latestBlockHeader.StateRoot)
	hdr.ParentRoot = parentRoot
	hdr.BodyRoot = bodyRoot
	hdr.StateRoot = stateRoot
	return hdr
}

// BlockRoots kept track of in the beacon state.
func (b *BeaconState) BlockRoots() [][]byte {
	b.lock.RLock()
	defer b.lock.RUnlock()

	roots := b.blockRootsVal()
	if roots == nil {
		return nil
	}
	return roots.Slice()
}

func (b *BeaconState) blockRootsVal() customtypes.BlockRoots {
	if b.blockRootsMultiValue == nil {
		return nil
	}
	return b.blockRootsMultiValue.Value(b)
}

// BlockRootAtIndex retrieves a specific block root based on an
// input index value.
func (b *BeaconState) BlockRootAtIndex(idx uint64) ([]byte, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.blockRootsMultiValue == nil {
		return []byte{}, nil
	}
	r, err := b.blockRootsMultiValue.At(b, idx)
	if err != nil {
		return nil, err
	}
	return r[:], nil
}

// ProposerDependentRoot is the spec's get_proposer_dependent_root(state, epoch(slot)) =
// state.block_roots[start_slot(epoch(slot)-1) - 1]. Returns
// ErrProposerDependentRootUnderflow when the proposal epoch is < 2; the spec's
// fallback to the genesis block root is the caller's responsibility.
func (b *BeaconState) ProposerDependentRoot(slot primitives.Slot) ([32]byte, error) {
	epoch := slots.ToEpoch(slot)
	if epoch < 2 {
		return [32]byte{}, ErrProposerDependentRootUnderflow
	}
	boundary, err := slots.EpochStart(epoch - 1)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "epoch start")
	}
	target := boundary - 1
	b.lock.RLock()
	stateSlot := b.slot
	b.lock.RUnlock()
	if target >= stateSlot || stateSlot > target+params.BeaconConfig().SlotsPerHistoricalRoot {
		return [32]byte{}, errors.Errorf("slot %d out of bounds", target)
	}
	rootBytes, err := b.BlockRootAtIndex(uint64(target % params.BeaconConfig().SlotsPerHistoricalRoot))
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "block root at slot")
	}
	return bytesutil.ToBytes32(rootBytes), nil
}
