package kv

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	bolt "go.etcd.io/bbolt"
)

// MetadataSeqNum retrieves the p2p metadata sequence number from the database.
// It returns 0 and ErrNotFoundMetadataSeqNum if the key does not exist.
func (s *Store) MetadataSeqNum(ctx context.Context) (uint64, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.MetadataSeqNum")
	defer span.End()

	var seqNum uint64
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(chainMetadataBucket)
		val := bkt.Get(metadataSequenceNumberKey)
		if val == nil {
			return ErrNotFoundMetadataSeqNum
		}

		seqNum = bytesutil.BytesToUint64BigEndian(val)
		return nil
	})

	return seqNum, err
}

// SaveMetadataSeqNum saves the p2p metadata sequence number to the database.
func (s *Store) SaveMetadataSeqNum(ctx context.Context, seqNum uint64) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.SaveMetadataSeqNum")
	defer span.End()

	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(chainMetadataBucket)
		val := bytesutil.Uint64ToBytesBigEndian(seqNum)
		return bkt.Put(metadataSequenceNumberKey, val)
	})
}
