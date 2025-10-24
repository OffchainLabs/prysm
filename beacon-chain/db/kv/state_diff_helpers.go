package kv

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	statenative "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
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
	buf := make([]byte, 16)
	buf[0] = byte(level)
	binary.LittleEndian.PutUint64(buf[1:], slot)
	return buf
}

func (s *Store) getAnchorState(offset uint64, lvl int, slot primitives.Slot) (anchor state.ReadOnlyBeaconState, err error) {
	if lvl <= 0 || lvl >= len(flags.Get().StateDiffExponents) {
		return nil, errors.New("invalid value for level")
	}

	relSlot := uint64(slot) - offset
	prevExp := flags.Get().StateDiffExponents[lvl-1]
	span := math.PowerOf2(uint64(prevExp))
	anchorSlot := primitives.Slot((relSlot / span * span) + offset)

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
		var fuluState ethpb.BeaconStateFulu
		if err := fuluState.UnmarshalSSZ(enc[len(ElectraKey):]); err != nil {
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
	default:
		var phase0State ethpb.BeaconState
		if err := phase0State.UnmarshalSSZ(enc); err != nil {
			return nil, err
		}
		return statenative.InitializeFromProtoUnsafePhase0(&phase0State)
	}
}

func (s *Store) getBaseAndDiffChain(offset uint64, slot primitives.Slot) (state.BeaconState, []hdiff.HdiffBytes, error) {
	rel := uint64(slot) - offset
	lvl := computeLevel(offset, slot)
	if lvl == -1 {
		return nil, nil, errors.New("slot not in tree")
	}

	exponents := flags.Get().StateDiffExponents

	baseSpan := math.PowerOf2(uint64(exponents[0]))
	baseAnchorSlot := (rel / baseSpan * baseSpan) + offset

	var diffChainIndices []uint64
	for i := 1; i <= lvl; i++ {
		span := math.PowerOf2(uint64(exponents[i]))
		diffSlot := rel / span * span
		if diffSlot == baseAnchorSlot {
			continue
		}
		diffChainIndices = appendUnique(diffChainIndices, diffSlot+offset)
	}

	baseSnapshot, err := s.getFullSnapshot(baseAnchorSlot)
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
