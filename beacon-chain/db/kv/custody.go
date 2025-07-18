package kv

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

// CustodyGroCustodyGroupCountupCount returns the custody group count stored in the database.
func (s *Store) CustodyGroupCount(ctx context.Context) (uint64, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.CustodyGroupCount")
	defer span.End()

	result := uint64(0)
	err := s.db.View(func(tx *bolt.Tx) error {
		// Retrieve the custody bucket.
		bucket := tx.Bucket(custodyBucket)
		if bucket == nil {
			return nil
		}

		// Retrieve the group count.
		bytes := bucket.Get(groupCountKey)
		if len(bytes) == 0 {
			return nil
		}

		result = bytesutil.BytesToUint64BigEndian(bytes)
		return nil
	})

	return result, err
}

// SaveCustodyGroupCount saves the custody group count to the database.
func (s *Store) SaveCustodyGroupCount(ctx context.Context, custodyGroupCount uint64) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.SetCustodyGroupCount")
	defer span.End()

	return s.db.Update(func(tx *bolt.Tx) error {
		// Retrieve the custody bucket.
		bucket, err := tx.CreateBucketIfNotExists(custodyBucket)
		if err != nil {
			return errors.Wrap(err, "create custody bucket")
		}

		// Store the custody group count.
		custodyGroupCountBytes := bytesutil.Uint64ToBytesBigEndian(custodyGroupCount)
		if err := bucket.Put(groupCountKey, custodyGroupCountBytes); err != nil {
			return errors.Wrap(err, "put custody group count")
		}

		return nil
	})
}

// SubscribedToAllDataSubnets checks in the database if the node is subscribed to all data subnets.
func (s *Store) SubscribedToAllDataSubnets(ctx context.Context) (bool, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.SubscribedToAllDataSubnets")
	defer span.End()

	result := false
	err := s.db.View(func(tx *bolt.Tx) error {
		// Retrieve the custody bucket.
		bucket := tx.Bucket(custodyBucket)
		if bucket == nil {
			return nil
		}

		// Retrieve the subscribe all data subnets flag.
		bytes := bucket.Get(subscribeAllDataSubnetsKey)
		if len(bytes) == 0 {
			return nil
		}

		if bytes[0] == 1 {
			result = true
		}

		return nil
	})

	return result, err
}

// SaveSubscribedToAllDataSubnets saves the subscription status to all data subnets in the database.
func (s *Store) SaveSubscribedToAllDataSubnets(ctx context.Context, subscribed bool) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.SaveSubscribedToAllDataSubnets")
	defer span.End()

	return s.db.Update(func(tx *bolt.Tx) error {
		// Retrieve the custody bucket.
		bucket, err := tx.CreateBucketIfNotExists(custodyBucket)
		if err != nil {
			return errors.Wrap(err, "create custody bucket")
		}

		// Store the subscription status.
		value := byte(0)
		if subscribed {
			value = 1
		}

		if err := bucket.Put(subscribeAllDataSubnetsKey, []byte{value}); err != nil {
			return errors.Wrap(err, "put subscribe all data subnets")
		}

		return nil
	})
}
