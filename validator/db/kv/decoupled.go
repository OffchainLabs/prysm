package kv

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	bolt "go.etcd.io/bbolt"
)

// Reservations older than this many slots serve no vote, the window is two slots.
const availableVoteRetention = 64

// The key's index value, settled when every recorded row was submitted, pending when the last one still has to go out.
var (
	finalitySettled = []byte{1}
	finalityPending = []byte{2}
)

// Spec: Λ read back as the rows this key signed, in signing order. A key the index knows without its history refuses to sign, lost history is not a fresh start.
func (s *Store) FinalityHistory(_ context.Context, pubKey [fieldparams.BLSPubkeyLength]byte) (history []*ethpb.AttestationDataDecoupled, pending, known bool, err error) {
	err = s.view(func(tx *bolt.Tx) error {
		mark := tx.Bucket(finalityKeysBucket).Get(pubKey[:])
		known, pending = mark != nil, bytes.Equal(mark, finalityPending)
		b := tx.Bucket(finalityHistoryBucket).Bucket(pubKey[:])
		if b == nil {
			if known {
				return errors.New("finality signing history is missing")
			}
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			d := &ethpb.AttestationDataDecoupled{}
			if err := d.UnmarshalSSZ(v); err != nil {
				return err
			}
			history = append(history, d)
			return nil
		})
	})
	return history, pending, known, err
}

func (s *Store) MarkFinalityKey(_ context.Context, pubKey [fieldparams.BLSPubkeyLength]byte) error {
	return s.update(func(tx *bolt.Tx) error {
		if _, err := tx.Bucket(finalityHistoryBucket).CreateBucketIfNotExists(pubKey[:]); err != nil {
			return err
		}
		return tx.Bucket(finalityKeysBucket).Put(pubKey[:], finalitySettled)
	})
}

// Written before the row is released for signing, and marked pending until its submission is confirmed.
func (s *Store) SaveFinalityAttestation(_ context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, d *ethpb.AttestationDataDecoupled) error {
	enc, err := d.MarshalSSZ()
	if err != nil {
		return err
	}
	return s.update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(finalityKeysBucket).Put(pubKey[:], finalityPending); err != nil {
			return err
		}
		b, err := tx.Bucket(finalityHistoryBucket).CreateBucketIfNotExists(pubKey[:])
		if err != nil {
			return err
		}
		seq, err := b.NextSequence()
		if err != nil {
			return err
		}
		return b.Put(bytesutil.Uint64ToBytesBigEndian(seq), enc)
	})
}

// Settles the key only while the submitted row is still the last one, an acknowledgment arriving after a newer row was reserved must not clear that row's pending mark.
func (s *Store) MarkFinalitySubmitted(_ context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, d *ethpb.AttestationDataDecoupled) error {
	enc, err := d.MarshalSSZ()
	if err != nil {
		return err
	}
	return s.update(func(tx *bolt.Tx) error {
		b := tx.Bucket(finalityHistoryBucket).Bucket(pubKey[:])
		if b == nil {
			return nil
		}
		if _, last := b.Cursor().Last(); !bytes.Equal(last, enc) {
			return nil
		}
		return tx.Bucket(finalityKeysBucket).Put(pubKey[:], finalitySettled)
	})
}

// Spec: one AC vote per slot. The first head reserved for a slot is the only one this key signs there, a retry with the same head passes.
func (s *Store) ReserveAvailableVote(_ context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, slot primitives.Slot, root [32]byte) error {
	return s.update(func(tx *bolt.Tx) error {
		b, err := tx.Bucket(availableVotesBucket).CreateBucketIfNotExists(pubKey[:])
		if err != nil {
			return err
		}
		key := bytesutil.SlotToBytesBigEndian(slot)
		if have := b.Get(key); have != nil {
			if !bytes.Equal(have, root[:]) {
				return fmt.Errorf("slot %d already reserved for head %#x", slot, have)
			}
			return nil
		}
		if err := b.Put(key, root[:]); err != nil {
			return err
		}
		var stale [][]byte
		c := b.Cursor()
		for k, _ := c.First(); k != nil && bytesutil.BytesToSlotBigEndian(k)+availableVoteRetention < slot; k, _ = c.Next() {
			stale = append(stale, k)
		}
		for _, k := range stale {
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}
