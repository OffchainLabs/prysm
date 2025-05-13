package kv

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/config/params"
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
	anchor, ok := anchorCache[anchorLvl]
	if ok {
		return anchor, nil
	}

	// If not, load it from the database.
	anchor, err = s.StateDiff(context.Background(), anchorSlot)
	if err != nil {
		return nil, err
	}

	// Save it in the cache.
	anchorCache[anchorLvl] = anchor
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
	return offset, s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}

		offsetBytes := bucket.Get(offsetKey)
		if offsetBytes != nil {
			offset = binary.BigEndian.Uint64(offsetBytes)
			return nil
		}

		offset = uint64(slot)
		offsetBytes = make([]byte, 8)
		binary.BigEndian.PutUint64(offsetBytes, offset)
		if err := bucket.Put(offsetKey, offsetBytes); err != nil {
			return err
		}
		return nil
	})
}

func (s *Store) getOffset() (offset uint64, err error) {
	return offset, s.db.View(func(tx *bbolt.Tx) error {
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
}

func keyForSnapshot(v int) ([]byte, error) {
	switch v {
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
	default:
		return nil, fmt.Errorf("unsupported version %s", version.String(v))
	}
}

func (s *Store) decodeStateSnapshot(enc []byte) (state.BeaconState, error) {
	switch {
	case HasElectraKey(enc):
		var electraState ethpb.BeaconStateElectra
		if err := electraState.UnmarshalSSZ(enc[len(ElectraKey):]); err != nil {
			return nil, err
		}
		return state_native.InitializeFromProtoElectra(&electraState)
	case hasDenebKey(enc):
		var denebState ethpb.BeaconStateDeneb
		if err := denebState.UnmarshalSSZ(enc[len(denebKey):]); err != nil {
			return nil, err
		}
		return state_native.InitializeFromProtoDeneb(&denebState)
	case hasCapellaKey(enc):
		var capellaState ethpb.BeaconStateCapella
		if err := capellaState.UnmarshalSSZ(enc[len(capellaKey):]); err != nil {
			return nil, err
		}
		return state_native.InitializeFromProtoCapella(&capellaState)
	case hasBellatrixKey(enc):
		var bellatrixState ethpb.BeaconStateBellatrix
		if err := bellatrixState.UnmarshalSSZ(enc[len(bellatrixKey):]); err != nil {
			return nil, err
		}
		return state_native.InitializeFromProtoBellatrix(&bellatrixState)
	case hasAltairKey(enc):
		var altairState ethpb.BeaconStateAltair
		if err := altairState.UnmarshalSSZ(enc[len(altairKey):]); err != nil {
			return nil, err
		}
		return state_native.InitializeFromProtoAltair(&altairState)
	default:
		return nil, fmt.Errorf("unsupported encoding %x", enc)
	}
}
