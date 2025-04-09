package slasherkv

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	bolt "go.etcd.io/bbolt"
)

type endianness int

const (
	bigEndian endianness = iota
	littleEndian
)

func encodeEpochLittleEndian(epoch primitives.Epoch) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(epoch))
	return buf
}

func createAttestation(signingRootBucket, attRecordsBucket *bolt.Bucket, epoch primitives.Epoch, encoding endianness) error {
	// Encode the target epoch.
	var key []byte
	if encoding == bigEndian {
		key = encodeTargetEpoch(epoch)
	} else {
		key = encodeEpochLittleEndian(epoch)
	}

	// Encode the validator index.
	encodedValidatorIndex := encodeValidatorIndex(primitives.ValidatorIndex(epoch))

	// Write the attestation to the database.
	key = append(key, encodedValidatorIndex...)
	if err := signingRootBucket.Put(key, encodedValidatorIndex); err != nil {
		return err
	}

	err := attRecordsBucket.Put(encodedValidatorIndex, []byte("dummy"))

	return err
}

func createProposal(proposalBucket *bolt.Bucket, epoch primitives.Epoch, encoding endianness) error {
	// Get the slot for the epoch.
	slot := primitives.Slot(epoch) * params.BeaconConfig().SlotsPerEpoch

	// Encode the slot.
	key := make([]byte, 8)
	if encoding == bigEndian {
		binary.BigEndian.PutUint64(key, uint64(slot))
	} else {
		binary.LittleEndian.PutUint64(key, uint64(slot))
	}

	// Encode the validator index.
	encodedValidatorIndex := encodeValidatorIndex(primitives.ValidatorIndex(slot))

	// Write the proposal to the database.
	key = append(key, encodedValidatorIndex...)
	err := proposalBucket.Put(key, []byte("dummy"))

	return err
}

func TestMigrate(t *testing.T) {
	const (
		headEpoch       = primitives.Epoch(65000)
		maxPruningEpoch = primitives.Epoch(60000)
		batchSize       = 3
	)

	/*
		State of the DB before migration:
		=================================

		LE: Little-endian encoding
		BE: Big-endian encoding

		Attestations:
		-------------
		59000 (LE), 59100 (LE), 59200 (BE), 59300 (LE), 59400 (LE), 59500 (LE), 59600 (LE), 59700 (LE), 59800 (LE), 59900 (LE),
		60000 (LE), 60100 (LE), 60200 (LE), 60300 (LE), 60400 (BE), 60500 (LE), 60600 (LE), 60700 (LE), 60800 (LE), 60900 (LE)


		Proposals:
		----------
		59000*32 (LE), 59100*32 (LE), 59200*32 (BE), 59300*32 (LE), 59400*32 (LE), 59500*32 (LE), 59600*32 (LE), 59700*32 (LE), 59800*32 (LE), 59900*32 (LE),
		60000*32 (LE), 60100*32 (LE), 60200*32 (LE), 60300*32 (LE), 60400*32 (BE), 60500*32 (LE), 60600*32 (LE), 60700*32 (LE), 60800*32 (LE), 60900*32 (LE)


		State of the DB after migration:
		================================

		Attestations:
		-------------
		59200 (BE), 60100 (BE), 60200 (BE), 60300(BE), 60400 (BE), 60500 (BE), 60600 (BE), 60700 (BE), 60800 (BE), 60900 (BE)

		Proposals:
		----------
		59200*32 (BE), 60100*32 (BE), 60200*32 (BE), 60300*32 (BE), 60400*32 (BE), 60500*32 (BE), 60600*32 (BE), 60700*32 (BE), 60800*32 (BE), 60900*32 (BE)

	*/

	beforeLittleEndianEpochs := []primitives.Epoch{
		59000, 59100, 59300, 59400, 59500, 59600, 59700, 59800, 59900,
		60000, 60100, 60200, 60300, 60500, 60600, 60700, 60800, 60900,
	}

	beforeBigEndianEpochs := []primitives.Epoch{59200, 60400}

	afterBigEndianEpochs := []primitives.Epoch{
		59200, 60100, 60200, 60300, 60400, 60500, 60600, 60700, 60800, 60900,
	}

	// Create a new context.
	ctx := context.Background()

	// Setup a test database.
	beaconDB := setupDB(t)

	// Write attestations and proposals to the database.
	err := beaconDB.db.Update(func(tx *bolt.Tx) error {
		signingRootsBkt := tx.Bucket(attestationDataRootsBucket)
		attRecordsBkt := tx.Bucket(attestationRecordsBucket)
		proposalBkt := tx.Bucket(proposalRecordsBucket)

		// Create attestations with little-endian encoding.
		for _, epoch := range beforeLittleEndianEpochs {
			if err := createAttestation(signingRootsBkt, attRecordsBkt, epoch, littleEndian); err != nil {
				return err
			}
		}

		// Create attestations with big-endian encoding.
		for _, epoch := range beforeBigEndianEpochs {
			if err := createAttestation(signingRootsBkt, attRecordsBkt, epoch, bigEndian); err != nil {
				return err
			}
		}

		// Create proposals with little-endian encoding.
		for _, epoch := range beforeLittleEndianEpochs {
			if err := createProposal(proposalBkt, epoch, littleEndian); err != nil {
				return err
			}
		}

		// Create proposals with big-endian encoding.
		for _, epoch := range beforeBigEndianEpochs {
			if err := createProposal(proposalBkt, epoch, bigEndian); err != nil {
				return err
			}
		}

		return nil
	})

	require.NoError(t, err)

	// Migrate the database.
	err = beaconDB.Migrate(ctx, headEpoch, maxPruningEpoch, batchSize)
	require.NoError(t, err)

	// Check the state of the database after migration.
	err = beaconDB.db.View(func(tx *bolt.Tx) error {
		signingRootsBkt := tx.Bucket(attestationDataRootsBucket)
		attRecordsBkt := tx.Bucket(attestationRecordsBucket)
		proposalBkt := tx.Bucket(proposalRecordsBucket)

		// Check that all the expected attestations are in the store.
		for _, epoch := range afterBigEndianEpochs {
			// Check if the attestation exists.
			key := encodeTargetEpoch(epoch)
			encodedValidatorIndex := encodeValidatorIndex(primitives.ValidatorIndex(epoch))
			key = append(key, encodedValidatorIndex...)

			// Check the signing root bucket.
			indexedAtt := signingRootsBkt.Get(key)
			require.DeepSSZEqual(t, encodedValidatorIndex, indexedAtt)

			// Check the attestation records bucket.
			attestationRecord := attRecordsBkt.Get(encodedValidatorIndex)
			require.DeepSSZEqual(t, []byte("dummy"), attestationRecord)
		}

		// Check only the expected attestations are in the store.
		c := signingRootsBkt.Cursor()
		count := 0
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			count++
		}

		require.Equal(t, len(afterBigEndianEpochs), count)

		c = attRecordsBkt.Cursor()
		count = 0
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			count++
		}

		require.Equal(t, len(afterBigEndianEpochs), count)

		// Check that all the expected proposals are in the store.
		slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch

		for _, epoch := range afterBigEndianEpochs {
			// Check if the proposal exists.
			slot := primitives.Slot(epoch) * slotsPerEpoch
			key := make([]byte, 8)
			binary.BigEndian.PutUint64(key, uint64(slot))
			encodedValidatorIndex := encodeValidatorIndex(primitives.ValidatorIndex(slot))
			key = append(key, encodedValidatorIndex...)

			// Check the proposal bucket.
			proposal := proposalBkt.Get(key)
			require.DeepEqual(t, []byte("dummy"), proposal)
		}

		// Check only the expected proposals are in the store.
		c = proposalBkt.Cursor()
		count = 0
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			count++
		}

		require.Equal(t, len(afterBigEndianEpochs), count)

		return nil
	})

	require.NoError(t, err)
}
