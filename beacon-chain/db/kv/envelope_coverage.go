package kv

import (
	"bytes"
	"context"
	"crypto/sha256"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/iface"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/proto/dbval"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
)

// envelopeCoverageFormatVersion is the only supported EnvelopeCoverage layout.
const envelopeCoverageFormatVersion = 1

// EnvelopeCoverage reads the dedicated execution payload envelope coverage
// record. A missing record is the explicit UNINITIALIZED state and is
// reported as ErrNotFound.
func (s *Store) EnvelopeCoverage(ctx context.Context) (*dbval.EnvelopeCoverage, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.EnvelopeCoverage")
	defer span.End()

	cov := &dbval.EnvelopeCoverage{}
	err := s.db.View(func(tx *bolt.Tx) error {
		enc := tx.Bucket(chainMetadataBucket).Get(envelopeCoverageKey)
		if len(enc) == 0 {
			return errors.Wrap(ErrNotFound, "EnvelopeCoverage not found")
		}
		return proto.Unmarshal(enc, cov)
	})
	if err != nil {
		return nil, err
	}
	return cov, nil
}

// SaveEnvelopeCoverage writes the coverage record without touching the
// canonical-revealed slot index. It is used to seed an empty interval (for
// example by SaveOrigin, before any services exist); every mutation that also
// changes index contents must go through CommitEnvelopeCoverage.
func (s *Store) SaveEnvelopeCoverage(ctx context.Context, cov *dbval.EnvelopeCoverage) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.SaveEnvelopeCoverage")
	defer span.End()

	enc, err := validateAndEncodeEnvelopeCoverage(cov)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(chainMetadataBucket).Put(envelopeCoverageKey, enc)
	})
}

// CommitEnvelopeCoverage commits the coverage record together with exact
// range replacements of the canonical-revealed slot index in one Bolt
// transaction. For every replacement, all existing index keys in
// [Start, End) are deleted and exactly the supplied entries are written.
// Before each put, the same transaction rechecks that the stored primary
// envelope value still matches the entry's fingerprint (prepared by the
// coordinator outside the transaction); absence or replacement aborts the
// entire publication with ErrEnvelopeCoveragePrimaryMismatch.
func (s *Store) CommitEnvelopeCoverage(ctx context.Context, cov *dbval.EnvelopeCoverage, replacements []iface.EnvelopeIndexReplacement) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.CommitEnvelopeCoverage")
	defer span.End()

	enc, err := validateAndEncodeEnvelopeCoverage(cov)
	if err != nil {
		return err
	}
	for _, r := range replacements {
		if r.End < r.Start {
			return errors.Errorf("invalid envelope index replacement range [%d, %d)", r.Start, r.End)
		}
		for _, e := range r.Entries {
			if e.Slot < r.Start || e.Slot >= r.End {
				return errors.Errorf("envelope index entry slot %d outside replacement range [%d, %d)", e.Slot, r.Start, r.End)
			}
		}
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		idxBkt := tx.Bucket(revealedEnvelopeSlotIndexBucket)
		envBkt := tx.Bucket(executionPayloadEnvelopesBucket)
		for _, r := range replacements {
			if err := deleteRevealedEnvelopeIndexRange(idxBkt, r.Start, r.End); err != nil {
				return err
			}
			for _, e := range r.Entries {
				stored := envBkt.Get(e.Root[:])
				if stored == nil {
					return errors.Wrapf(iface.ErrEnvelopeCoveragePrimaryMismatch, "no stored envelope for root %#x at slot %d", e.Root, e.Slot)
				}
				if sha256.Sum256(stored) != e.PrimaryFingerprint {
					return errors.Wrapf(iface.ErrEnvelopeCoveragePrimaryMismatch, "stored envelope for root %#x at slot %d was replaced", e.Root, e.Slot)
				}
				if err := idxBkt.Put(bytesutil.SlotToBytesBigEndian(e.Slot), e.Root[:]); err != nil {
					return err
				}
			}
		}
		return tx.Bucket(chainMetadataBucket).Put(envelopeCoverageKey, enc)
	})
}

// deleteRevealedEnvelopeIndexRange removes every serving index key in
// [start, end) inside the caller's transaction. Keys are collected first and
// deleted afterwards to avoid mutating under an iterating cursor.
func deleteRevealedEnvelopeIndexRange(bkt *bolt.Bucket, start, end primitives.Slot) error {
	if end <= start {
		return nil
	}
	c := bkt.Cursor()
	endKey := bytesutil.SlotToBytesBigEndian(end)
	var keys [][]byte
	for k, _ := c.Seek(bytesutil.SlotToBytesBigEndian(start)); k != nil && bytes.Compare(k, endKey) < 0; k, _ = c.Next() {
		keys = append(keys, bytesutil.SafeCopyBytes(k))
	}
	for _, k := range keys {
		if err := bkt.Delete(k); err != nil {
			return err
		}
	}
	return nil
}

// RevealedEnvelopeRoots copies up to limit (slot, root) entries of the
// canonical-revealed slot index within [start, end) in ascending slot order,
// in one short read transaction.
func (s *Store) RevealedEnvelopeRoots(ctx context.Context, start, end primitives.Slot, limit uint64) ([]iface.RevealedEnvelopeSlotRoot, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.RevealedEnvelopeRoots")
	defer span.End()

	if end <= start || limit == 0 {
		return nil, nil
	}
	var out []iface.RevealedEnvelopeSlotRoot
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(revealedEnvelopeSlotIndexBucket).Cursor()
		endKey := bytesutil.SlotToBytesBigEndian(end)
		for k, v := c.Seek(bytesutil.SlotToBytesBigEndian(start)); k != nil && bytes.Compare(k, endKey) < 0; k, v = c.Next() {
			if len(v) != hashLength {
				return errors.Errorf("corrupt revealed envelope index value of length %d at slot %d", len(v), bytesutil.BytesToSlotBigEndian(k))
			}
			out = append(out, iface.RevealedEnvelopeSlotRoot{
				Slot: bytesutil.BytesToSlotBigEndian(k),
				Root: bytesutil.ToBytes32(v),
			})
			if uint64(len(out)) >= limit {
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// PruneRevealedEnvelopeIndexBelow deletes up to limit canonical-revealed
// index keys strictly below floor and reports how many were removed. It is
// used by the coordinator for bounded cleanup of keys that fell below the
// published lower bound; such keys are never served.
func (s *Store) PruneRevealedEnvelopeIndexBelow(ctx context.Context, floor primitives.Slot, limit int) (int, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.PruneRevealedEnvelopeIndexBelow")
	defer span.End()

	if limit <= 0 {
		return 0, nil
	}
	deleted := 0
	err := s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(revealedEnvelopeSlotIndexBucket)
		c := bkt.Cursor()
		floorKey := bytesutil.SlotToBytesBigEndian(floor)
		var keys [][]byte
		for k, _ := c.First(); k != nil && bytes.Compare(k, floorKey) < 0; k, _ = c.Next() {
			keys = append(keys, bytesutil.SafeCopyBytes(k))
			if len(keys) >= limit {
				break
			}
		}
		for _, k := range keys {
			if err := bkt.Delete(k); err != nil {
				return err
			}
			deleted++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

// ExecutionPayloadEnvelopeWithFingerprint retrieves the blinded envelope by
// beacon block root together with the SHA-256 fingerprint of its raw stored
// value. The fingerprint lets the coverage coordinator validate the envelope
// outside the combined commit and detect replacement inside it.
func (s *Store) ExecutionPayloadEnvelopeWithFingerprint(ctx context.Context, blockRoot [32]byte) (*ethpb.SignedBlindedExecutionPayloadEnvelope, [32]byte, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.ExecutionPayloadEnvelopeWithFingerprint")
	defer span.End()

	var env *ethpb.SignedBlindedExecutionPayloadEnvelope
	var fp [32]byte
	err := s.db.View(func(tx *bolt.Tx) error {
		enc := tx.Bucket(executionPayloadEnvelopesBucket).Get(blockRoot[:])
		if enc == nil {
			return errors.Wrap(ErrNotFound, "execution payload envelope not found")
		}
		fp = sha256.Sum256(enc)
		var err error
		env, err = decodeBlindedEnvelope(enc)
		return err
	})
	if err != nil {
		return nil, [32]byte{}, err
	}
	return env, fp, nil
}

// validateAndEncodeEnvelopeCoverage enforces the record invariants shared by
// every coverage write and returns the encoded value.
func validateAndEncodeEnvelopeCoverage(cov *dbval.EnvelopeCoverage) ([]byte, error) {
	if cov == nil {
		return nil, errors.New("cannot save nil EnvelopeCoverage")
	}
	if cov.FormatVersion != envelopeCoverageFormatVersion {
		return nil, errors.Errorf("unsupported EnvelopeCoverage format_version %d", cov.FormatVersion)
	}
	if cov.HighSlot < cov.LowSlot {
		return nil, errors.Errorf("invalid EnvelopeCoverage interval [%d, %d)", cov.LowSlot, cov.HighSlot)
	}
	if len(cov.HighAnchorRoot) != hashLength {
		return nil, errors.Errorf("invalid EnvelopeCoverage high_anchor_root length %d", len(cov.HighAnchorRoot))
	}
	return proto.Marshal(cov)
}
