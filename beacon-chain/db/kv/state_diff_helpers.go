package kv

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	statenative "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/hdiff"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/math"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	pkgerrors "github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

var (
	offsetKey                   = []byte("offset")
	exponentsKey                = []byte("exponents")
	ErrSlotBeforeOffset         = errors.New("slot is before state-diff root offset")
	errExponentsMetadataMissing = errors.New("state diff exponents metadata not found")
)

func encodeStateDiffExponents(exponents []int) ([]byte, error) {
	if len(exponents) == 0 {
		return nil, errors.New("state diff exponents cannot be empty")
	}
	if len(exponents) > 255 {
		return nil, fmt.Errorf("state diff exponents length %d exceeds max 255", len(exponents))
	}
	encoded := make([]byte, len(exponents)+1)
	encoded[0] = byte(len(exponents))
	for i, exp := range exponents {
		if exp < flags.MinStateDiffExponent || exp > flags.MaxStateDiffExponent {
			return nil, fmt.Errorf("state diff exponent out of range for encoding: got %d, expected between %d and %d", exp, flags.MinStateDiffExponent, flags.MaxStateDiffExponent)
		}
		encoded[i+1] = byte(exp)
	}
	return encoded, nil
}

func decodeStateDiffExponents(encoded []byte) ([]int, error) {
	if len(encoded) == 0 {
		return nil, errors.New("state diff exponents missing length prefix")
	}
	count := int(encoded[0])
	if count == 0 {
		return nil, errors.New("state diff exponents length cannot be zero")
	}
	if len(encoded) != count+1 {
		return nil, fmt.Errorf("state diff exponents length mismatch: expected %d got %d", count, len(encoded)-1)
	}
	exponents := make([]int, count)
	for i := range count {
		exponents[i] = int(encoded[i+1])
	}
	return exponents, nil
}

func formatStateDiffExponents(exponents []int) string {
	if len(exponents) == 0 {
		return ""
	}
	parts := make([]string, len(exponents))
	for i, exp := range exponents {
		parts[i] = fmt.Sprintf("%d", exp)
	}
	return strings.Join(parts, ",")
}

func (s *Store) loadStateDiffExponents() ([]int, error) {
	var encoded []byte
	err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}
		value := bucket.Get(exponentsKey)
		if value == nil {
			return errExponentsMetadataMissing
		}
		encoded = make([]byte, len(value))
		copy(encoded, value)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return decodeStateDiffExponents(encoded)
}

func makeKeyForStateDiffTree(level int, slot uint64) []byte {
	buf := make([]byte, 16)
	buf[0] = byte(level)
	binary.LittleEndian.PutUint64(buf[1:], slot)
	return buf
}

func (s *Store) getAnchorState(ctx context.Context, offset uint64, lvl int, slot primitives.Slot) (anchor state.ReadOnlyBeaconState, err error) {
	if lvl <= 0 || lvl > len(flags.Get().StateDiffExponents) {
		return nil, errors.New("invalid value for level")
	}

	if uint64(slot) < offset {
		return nil, ErrSlotBeforeOffset
	}
	relSlot := uint64(slot) - offset
	prevExp := flags.Get().StateDiffExponents[lvl-1]
	if prevExp < flags.MinStateDiffExponent || prevExp >= 64 {
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
	if anchor != nil && anchor.Slot() == anchorSlot {
		return anchor, nil
	}
	if anchor != nil {
		log.WithField("level", anchorLvl).
			WithField("expectedSlot", anchorSlot).
			WithField("cachedSlot", anchor.Slot()).
			Warn("Cached state-diff anchor slot mismatch; reloading anchor from database")
	}

	// If not, load it from the database.
	anchor, err = s.stateByDiff(ctx, anchorSlot)
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
	if uint64(slot) < offset {
		return -1
	}
	rel := uint64(slot) - offset
	for i, exp := range flags.Get().StateDiffExponents {
		if exp < flags.MinStateDiffExponent || exp >= 64 {
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

func (s *Store) loadOffset() (uint64, error) {
	var offset uint64
	err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}
		offsetBytes := bucket.Get(offsetKey)
		if offsetBytes == nil {
			return errors.New("state diff offset not found")
		}
		if len(offsetBytes) != 8 {
			return fmt.Errorf("state diff offset has invalid length %d", len(offsetBytes))
		}
		offset = binary.LittleEndian.Uint64(offsetBytes)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return offset, nil
}

// hasStateDiffOffset checks if the state-diff offset has been set in the database.
// This is used to detect if an existing database has state-diff enabled.
func (s *Store) hasStateDiffOffset() (bool, error) {
	var hasOffset bool
	err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return nil
		}
		hasOffset = bucket.Get(offsetKey) != nil
		return nil
	})
	return hasOffset, err
}

// initializeStateDiff sets up the state-diff schema for a new database.
// This should be called during checkpoint sync or genesis sync.
func (s *Store) initializeStateDiff(slot primitives.Slot, initialState state.ReadOnlyBeaconState) error {
	// Return early if the feature is not set
	if !features.Get().EnableStateDiff {
		return nil
	}
	// Only reinitialize if the offset is different
	if s.stateDiffCache != nil {
		if s.stateDiffCache.getOffset() == uint64(slot) {
			log.WithField("offset", slot).Warning("Ignoring state diff cache reinitialization")
			return nil
		}
	}
	exponentsBytes, err := encodeStateDiffExponents(flags.Get().StateDiffExponents)
	if err != nil {
		return pkgerrors.Wrap(err, "failed to encode state diff exponents")
	}

	// Write metadata directly to the database (without using cache which doesn't exist yet).
	err = s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}

		offsetBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(offsetBytes, uint64(slot))
		if err := bucket.Put(offsetKey, offsetBytes); err != nil {
			return err
		}
		return bucket.Put(exponentsKey, exponentsBytes)
	})
	if err != nil {
		return pkgerrors.Wrap(err, "failed to set offset")
	}

	// Create the state diff cache (this will read the offset from the database).
	sdCache, err := newStateDiffCache(s)
	if err != nil {
		return pkgerrors.Wrap(err, "failed to create state diff cache")
	}
	s.stateDiffCache = sdCache

	// Save the initial state as a full snapshot.
	if err := s.saveFullSnapshot(initialState); err != nil {
		return pkgerrors.Wrap(err, "failed to save initial snapshot")
	}

	log.WithField("offset", slot).Info("Initialized state-diff cache")
	return nil
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
	lastSeenAnchorRelSlot := baseAnchorSlot - offset
	for i, exp := range exponents[1 : lvl+1] {
		span := math.PowerOf2(uint64(exp))
		diffSlot := rel / span * span
		if diffSlot == lastSeenAnchorRelSlot {
			continue
		}
		level := i + 1
		if s.stateDiffCache != nil && !s.stateDiffCache.levelHasData(level) {
			continue
		}
		diffChainItems = append(diffChainItems, diffItem{level: level, slot: diffSlot + offset})
		lastSeenAnchorRelSlot = diffSlot
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
