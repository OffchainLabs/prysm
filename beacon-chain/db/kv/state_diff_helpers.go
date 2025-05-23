package kv

import (
	"context"
	"encoding/binary"
	"errors"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/hdiff"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/math"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"go.etcd.io/bbolt"
)

var (
	offsetKey           = []byte("offset")
	ErrSlotBeforeOffset = errors.New("slot is before root offset")
)

func makeKey(level int, slot uint64) []byte {
	buf := make([]byte, 1+8)
	buf[0] = byte(level)
	binary.BigEndian.PutUint64(buf[1:], slot)
	return buf
}

func (s *Store) getAnchorState(offset uint64, lvl int, slot primitives.Slot) (anchor state.ReadOnlyBeaconState, err error) {
	if lvl == 0 {
		return nil, errors.New("no anchor for level 0")
	}

	relSlot := uint64(slot) - offset
	prevExp := params.StateHierarchyExponents()[lvl-1]
	span := math.PowerOf2(prevExp)
	anchorSlot := primitives.Slot((relSlot / span * span) + offset)

	anchorLvl := computeLevel(offset, anchorSlot)
	if anchorLvl == -1 {
		return nil, errors.New("could not compute anchor level")
	}

	// Check if we have the anchor in cache.
	anchor = s.stateDiffCache.getAnchor(lvl)
	if anchor != nil {
		return anchor, nil
	}

	// If not, load it from the database.
	anchor, err = s.stateByDiff(context.Background(), anchorSlot)
	if err != nil {
		return nil, err
	}

	// Save it in the cache.
	s.stateDiffCache.setAnchor(anchorLvl, anchor)
	return anchor, nil
}

// ComputeLevel computes the level in the diff tree. Returns -1 in case slot should not be in tree.
func computeLevel(offset uint64, slot primitives.Slot) int {
	rel := uint64(slot) - offset
	for i, exp := range params.StateHierarchyExponents() {
		span := math.PowerOf2(exp)
		if rel%span == 0 {
			return i
		}
	}
	// If rel isn’t on any of the boundaries, we should ignore saving it.
	return -1
}

func (s *Store) loadOrInitOffset(slot primitives.Slot) (offset uint64, err error) {
	offset, err = s.stateDiffCache.getOffset()
	if err == nil {
		return offset, nil
	}

	err = s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}

		offsetBytes := bucket.Get(offsetKey)
		if offsetBytes != nil {
			offset = binary.LittleEndian.Uint64(offsetBytes)
			return nil
		}

		offset = uint64(slot)
		offsetBytes = make([]byte, 8)
		binary.LittleEndian.PutUint64(offsetBytes, offset)
		if err := bucket.Put(offsetKey, offsetBytes); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	// Save the offset in the cache.
	s.stateDiffCache.setOffset(offset)
	return offset, nil
}

func (s *Store) getOffset() (offset uint64, err error) {
	offset, err = s.stateDiffCache.getOffset()
	if err == nil {
		return offset, nil
	}

	err = s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}

		offsetBytes := bucket.Get(offsetKey)
		if offsetBytes != nil {
			offset = binary.BigEndian.Uint64(offsetBytes)
			return nil
		}
		return bbolt.ErrIncompatibleValue
	})
	if err != nil {
		return 0, err
	}

	// Save the offset in the cache.
	s.stateDiffCache.setOffset(offset)
	return offset, nil
}

func keyForSnapshot(v int) []byte {
	switch v {
	case version.Fulu:
		return fuluKey
	case version.Electra:
		return ElectraKey
	case version.Deneb:
		return denebKey
	case version.Capella:
		return capellaKey
	case version.Bellatrix:
		return bellatrixKey
	case version.Altair:
		return altairKey
	default:
		// Phase0
		return []byte{}
	}
}

func addKey(v int, bytes []byte) ([]byte, error) {
	key := keyForSnapshot(v)
	enc := make([]byte, len(key)+len(bytes))
	copy(enc, key)
	copy(enc[len(key):], bytes)
	return enc, nil
}

func (s *Store) decodeStateSnapshot(enc []byte) (state.BeaconState, error) {
	switch {
	case hasFuluKey(enc):
		var fuluState ethpb.BeaconStateElectra
		if err := fuluState.UnmarshalSSZ(enc[len(ElectraKey):]); err != nil {
			return nil, err
		}
		return state_native.InitializeFromProtoUnsafeFulu(&fuluState)
	case HasElectraKey(enc):
		var electraState ethpb.BeaconStateElectra
		if err := electraState.UnmarshalSSZ(enc[len(ElectraKey):]); err != nil {
			return nil, err
		}
		return state_native.InitializeFromProtoUnsafeElectra(&electraState)
	case hasDenebKey(enc):
		var denebState ethpb.BeaconStateDeneb
		if err := denebState.UnmarshalSSZ(enc[len(denebKey):]); err != nil {
			return nil, err
		}
		return state_native.InitializeFromProtoUnsafeDeneb(&denebState)
	case hasCapellaKey(enc):
		var capellaState ethpb.BeaconStateCapella
		if err := capellaState.UnmarshalSSZ(enc[len(capellaKey):]); err != nil {
			return nil, err
		}
		return state_native.InitializeFromProtoUnsafeCapella(&capellaState)
	case hasBellatrixKey(enc):
		var bellatrixState ethpb.BeaconStateBellatrix
		if err := bellatrixState.UnmarshalSSZ(enc[len(bellatrixKey):]); err != nil {
			return nil, err
		}
		return state_native.InitializeFromProtoUnsafeBellatrix(&bellatrixState)
	case hasAltairKey(enc):
		var altairState ethpb.BeaconStateAltair
		if err := altairState.UnmarshalSSZ(enc[len(altairKey):]); err != nil {
			return nil, err
		}
		return state_native.InitializeFromProtoUnsafeAltair(&altairState)
	default:
		var phase0State ethpb.BeaconState
		if err := phase0State.UnmarshalSSZ(enc); err != nil {
			return nil, err
		}
		return state_native.InitializeFromProtoUnsafePhase0(&phase0State)
	}
}

func (s *Store) getBaseAndDiffChain(offset uint64, slot primitives.Slot) (state.BeaconState, []hdiff.HdiffBytes, error) {
	rel := uint64(slot) - offset
	lvl := computeLevel(offset, slot)
	if lvl == -1 {
		return nil, nil, errors.New("slot not in tree")
	}

	exponents := params.StateHierarchyExponents()

	baseSpan := math.PowerOf2(exponents[0])
	baseAnchorSlot := (rel / baseSpan * baseSpan) + offset

	var diffChainIndices []uint64
	for i := 1; i <= lvl; i++ {
		span := math.PowerOf2(exponents[i])
		diffSlot := rel / span * span
		if diffSlot == baseAnchorSlot {
			continue
		}
		diffChainIndices = appendUnique(diffChainIndices, diffSlot+offset)
	}

	baseSnapshot, err := s.getFullSnapshot(0, baseAnchorSlot)
	if err != nil {
		return nil, nil, err
	}

	diffChain := make([]hdiff.HdiffBytes, 0, len(diffChainIndices))
	for _, diffSlot := range diffChainIndices {
		diff, err := s.getDiff(computeLevel(offset, primitives.Slot(diffSlot)), diffSlot)
		if err != nil {
			return nil, nil, err
		}
		diffChain = append(diffChain, diff)
	}

	return baseSnapshot, diffChain, nil
}

func appendUnique(s []uint64, v uint64) []uint64 {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
