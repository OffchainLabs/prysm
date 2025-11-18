package kv

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	statenative "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/consensus-types/hdiff"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/math"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"go.etcd.io/bbolt"
)

var (
	offsetKey           = []byte("offset")
	ErrSlotBeforeOffset = errors.New("slot is before root offset")
)

func makeKeyForStateDiffTree(level int, slot uint64) []byte {
	buf := make([]byte, 16)
	buf[0] = byte(level)
	binary.LittleEndian.PutUint64(buf[1:], slot)
	return buf
}

func (s *Store) getAnchorState(offset uint64, lvl int, slot primitives.Slot) (anchor state.ReadOnlyBeaconState, err error) {
	if lvl <= 0 || lvl > len(flags.Get().StateDiffExponents) {
		return nil, errors.New("invalid value for level")
	}

	if uint64(slot) < offset {
		return nil, ErrSlotBeforeOffset
	}
	relSlot := uint64(slot) - offset
	prevExp := flags.Get().StateDiffExponents[lvl-1]
	if prevExp < 2 || prevExp >= 64 {
		return nil, fmt.Errorf("state diff exponent %d out of range for uint64", prevExp)
	}
	span := math.PowerOf2(uint64(prevExp))
	anchorSlot := primitives.Slot(uint64(slot) - relSlot%span)

	// anchorLvl can be [0, lvl-1]
	anchorLvl := computeLevel(offset, anchorSlot)
	if anchorLvl == -1 {
		return nil, errors.New("could not compute anchor level")
	}

	// Check if we have the anchor in cache.
	anchor = s.stateDiffCache.getAnchor(anchorLvl)
	if anchor != nil {
		return anchor, nil
	}

	// If not, load it from the database.
	anchor, err = s.stateByDiff(context.Background(), anchorSlot)
	if err != nil {
		return nil, err
	}

	// Save it in the cache.
	err = s.stateDiffCache.setAnchor(anchorLvl, anchor)
	if err != nil {
		return nil, err
	}
	return anchor, nil
}

// computeLevel computes the level in the diff tree. Returns -1 in case slot should not be in tree.
func computeLevel(offset uint64, slot primitives.Slot) int {
	rel := uint64(slot) - offset
	for i, exp := range flags.Get().StateDiffExponents {
		if exp < 2 || exp >= 64 {
			return -1
		}
		span := math.PowerOf2(uint64(exp))
		if rel%span == 0 {
			return i
		}
	}
	// If rel isn’t on any of the boundaries, we should ignore saving it.
	return -1
}

func (s *Store) setOffset(slot primitives.Slot) error {
	err := s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}

		offsetBytes := bucket.Get(offsetKey)
		if offsetBytes != nil {
			return fmt.Errorf("offset already set to %d", binary.LittleEndian.Uint64(offsetBytes))
		}

		offsetBytes = make([]byte, 8)
		binary.LittleEndian.PutUint64(offsetBytes, uint64(slot))
		if err := bucket.Put(offsetKey, offsetBytes); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Save the offset in the cache.
	s.stateDiffCache.setOffset(uint64(slot))
	return nil
}

func (s *Store) getOffset() uint64 {
	return s.stateDiffCache.getOffset()
}

func keyForSnapshot(v int) ([]byte, error) {
	switch v {
	case version.Fulu:
		return fuluKey, nil
	case version.Electra:
		return ElectraKey, nil
	case version.Deneb:
		return denebKey, nil
	case version.Capella:
		return capellaKey, nil
	case version.Bellatrix:
		return bellatrixKey, nil
	case version.Altair:
		return altairKey, nil
	case version.Phase0:
		return phase0Key, nil
	default:
		return nil, errors.New("unsupported fork")
	}
}

func addKey(v int, bytes []byte) ([]byte, error) {
	key, err := keyForSnapshot(v)
	if err != nil {
		return nil, err
	}
	enc := make([]byte, len(key)+len(bytes))
	copy(enc, key)
	copy(enc[len(key):], bytes)
	return enc, nil
}

func decodeStateSnapshot(enc []byte) (state.BeaconState, error) {
	switch {
	case hasFuluKey(enc):
		var fuluState ethpb.BeaconStateFulu
		if err := fuluState.UnmarshalSSZ(enc[len(fuluKey):]); err != nil {
			return nil, err
		}
		return statenative.InitializeFromProtoUnsafeFulu(&fuluState)
	case HasElectraKey(enc):
		var electraState ethpb.BeaconStateElectra
		if err := electraState.UnmarshalSSZ(enc[len(ElectraKey):]); err != nil {
			return nil, err
		}
		return statenative.InitializeFromProtoUnsafeElectra(&electraState)
	case hasDenebKey(enc):
		var denebState ethpb.BeaconStateDeneb
		if err := denebState.UnmarshalSSZ(enc[len(denebKey):]); err != nil {
			return nil, err
		}
		return statenative.InitializeFromProtoUnsafeDeneb(&denebState)
	case hasCapellaKey(enc):
		var capellaState ethpb.BeaconStateCapella
		if err := capellaState.UnmarshalSSZ(enc[len(capellaKey):]); err != nil {
			return nil, err
		}
		return statenative.InitializeFromProtoUnsafeCapella(&capellaState)
	case hasBellatrixKey(enc):
		var bellatrixState ethpb.BeaconStateBellatrix
		if err := bellatrixState.UnmarshalSSZ(enc[len(bellatrixKey):]); err != nil {
			return nil, err
		}
		return statenative.InitializeFromProtoUnsafeBellatrix(&bellatrixState)
	case hasAltairKey(enc):
		var altairState ethpb.BeaconStateAltair
		if err := altairState.UnmarshalSSZ(enc[len(altairKey):]); err != nil {
			return nil, err
		}
		return statenative.InitializeFromProtoUnsafeAltair(&altairState)
	case hasPhase0Key(enc):
		var phase0State ethpb.BeaconState
		if err := phase0State.UnmarshalSSZ(enc[len(phase0Key):]); err != nil {
			return nil, err
		}
		return statenative.InitializeFromProtoUnsafePhase0(&phase0State)
	default:
		return nil, errors.New("unsupported fork")
	}
}

func (s *Store) getBaseAndDiffChain(offset uint64, slot primitives.Slot) (state.BeaconState, []hdiff.HdiffBytes, error) {
	if uint64(slot) < offset {
		return nil, nil, ErrSlotBeforeOffset
	}
	rel := uint64(slot) - offset
	lvl := computeLevel(offset, slot)
	if lvl == -1 {
		return nil, nil, errors.New("slot not in tree")
	}

	exponents := flags.Get().StateDiffExponents

	baseSpan := math.PowerOf2(uint64(exponents[0]))
	baseAnchorSlot := uint64(slot) - rel%baseSpan

	type diffItem struct {
		level int
		slot  uint64
	}

	var diffChainItems []diffItem
	for i, exp := range exponents[1 : lvl+1] {
		span := math.PowerOf2(uint64(exp))
		diffSlot := rel / span * span
		if diffSlot == baseAnchorSlot {
			continue
		}
		diffChainItems = append(diffChainItems, diffItem{level: i + 1, slot: diffSlot + offset})
	}

	baseSnapshot, err := s.getFullSnapshot(baseAnchorSlot)
	if err != nil {
		return nil, nil, err
	}

	diffChain := make([]hdiff.HdiffBytes, 0, len(diffChainItems))
	for _, item := range diffChainItems {
		diff, err := s.getDiff(item.level, item.slot)
		if err != nil {
			return nil, nil, err
		}
		diffChain = append(diffChain, diff)
	}

	return baseSnapshot, diffChain, nil
}
