package kv

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	bolt "go.etcd.io/bbolt"
)

// setupDB instantiates and returns a Store instance.
func setupDB(t testing.TB) *Store {
	db, err := NewKVStore(t.Context(), t.TempDir())
	require.NoError(t, err, "Failed to instantiate DB")
	t.Cleanup(func() {
		err := db.Close()
		if err != context.Canceled {
			require.NoError(t, err, "Failed to close database")
		}
	})
	return db
}

func TestStartStateDiff_ExponentMismatch(t *testing.T) {
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()
	setDefaultStateDiffExponents()

	store := setupDB(t)
	require.NoError(t, store.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}
		offsetBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(offsetBytes, 0)
		if err := bucket.Put(offsetKey, offsetBytes); err != nil {
			return err
		}
		encoded, err := encodeStateDiffExponents([]int{20, 10})
		if err != nil {
			return err
		}
		return bucket.Put(exponentsKey, encoded)
	}))

	ctx := t.Context()
	err := store.startStateDiff(ctx)
	require.ErrorContains(t, "state-diff exponents changed", err)
}

func TestStartStateDiff_MissingOffsetSnapshot(t *testing.T) {
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()
	setDefaultStateDiffExponents()

	store := setupDB(t)
	require.NoError(t, store.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}
		offsetBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(offsetBytes, 0)
		if err := bucket.Put(offsetKey, offsetBytes); err != nil {
			return err
		}
		encoded, err := encodeStateDiffExponents(flags.Get().StateDiffExponents)
		if err != nil {
			return err
		}
		return bucket.Put(exponentsKey, encoded)
	}))

	ctx := t.Context()
	err := store.startStateDiff(ctx)
	require.ErrorContains(t, "missing offset snapshot", err)
}

func TestStartStateDiff_ValidateOnStartup(t *testing.T) {
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()
	setDefaultStateDiffExponents()

	globalFlags := flags.GlobalFlags{
		StateDiffExponents:         flags.Get().StateDiffExponents,
		StateDiffValidateOnStartup: true,
	}
	flags.Init(&globalFlags)

	store := setupDB(t)
	require.NoError(t, store.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}
		st, _ := createState(t, 0, version.Phase0)
		stateBytes, err := st.MarshalSSZ()
		if err != nil {
			return err
		}
		enc, err := addKey(st.Version(), stateBytes)
		if err != nil {
			return err
		}
		offsetBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(offsetBytes, 0)
		if err := bucket.Put(offsetKey, offsetBytes); err != nil {
			return err
		}
		encoded, err := encodeStateDiffExponents(flags.Get().StateDiffExponents)
		if err != nil {
			return err
		}
		if err := bucket.Put(exponentsKey, encoded); err != nil {
			return err
		}
		key := makeKeyForStateDiffTree(0, 0)
		return bucket.Put(key, enc)
	}))

	err := store.startStateDiff(t.Context())
	require.NoError(t, err)
}

func Test_setupBlockStorageType(t *testing.T) {
	ctx := t.Context()
	t.Run("fresh database with feature enabled to store full blocks should store full blocks", func(t *testing.T) {
		resetFn := features.InitWithReset(&features.Flags{
			SaveFullExecutionPayloads: true,
		})
		defer resetFn()
		store := setupDB(t)

		blk := util.NewBeaconBlockBellatrix()
		blk.Block.Body.ExecutionPayload.BlockNumber = 1
		wrappedBlock, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		root, err := wrappedBlock.Block().HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, store.SaveBlock(ctx, wrappedBlock))
		require.NoError(t, store.SaveStateSummary(ctx, &ethpb.StateSummary{Root: root[:]}))
		require.NoError(t, store.SaveHeadBlockRoot(ctx, root))
		retrievedBlk, err := store.Block(ctx, root)
		require.NoError(t, err)
		require.Equal(t, false, retrievedBlk.IsBlinded())
		require.DeepEqual(t, wrappedBlock, retrievedBlk)
	})
	t.Run("fresh database with default settings should store blinded", func(t *testing.T) {
		resetFn := features.InitWithReset(&features.Flags{
			SaveFullExecutionPayloads: false,
		})
		defer resetFn()
		store := setupDB(t)

		blk := util.NewBeaconBlockBellatrix()
		blk.Block.Body.ExecutionPayload.BlockNumber = 1
		wrappedBlock, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		root, err := wrappedBlock.Block().HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, store.SaveBlock(ctx, wrappedBlock))
		require.NoError(t, store.SaveStateSummary(ctx, &ethpb.StateSummary{Root: root[:]}))
		require.NoError(t, store.SaveHeadBlockRoot(ctx, root))
		retrievedBlk, err := store.Block(ctx, root)
		require.NoError(t, err)
		require.Equal(t, true, retrievedBlk.IsBlinded())

		wantedBlk, err := wrappedBlock.ToBlinded()
		require.NoError(t, err)
		require.DeepEqual(t, wantedBlk, retrievedBlk)
	})
	t.Run("existing database with blinded blocks but no key in metadata bucket should continue storing blinded blocks", func(t *testing.T) {
		store := setupDB(t)
		require.NoError(t, store.db.Update(func(tx *bolt.Tx) error {
			return tx.Bucket(chainMetadataBucket).Put(saveBlindedBeaconBlocksKey, []byte{1})
		}))

		blk := util.NewBlindedBeaconBlockBellatrix()
		blk.Block.Body.ExecutionPayloadHeader.BlockNumber = 1
		wrappedBlock, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		root, err := wrappedBlock.Block().HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, store.SaveBlock(ctx, wrappedBlock))
		require.NoError(t, store.SaveStateSummary(ctx, &ethpb.StateSummary{Root: root[:]}))
		require.NoError(t, store.SaveHeadBlockRoot(ctx, root))
		retrievedBlk, err := store.Block(ctx, root)
		require.NoError(t, err)
		require.Equal(t, true, retrievedBlk.IsBlinded())
		require.DeepEqual(t, wrappedBlock, retrievedBlk)

		// We then delete the key from the bucket.
		require.NoError(t, store.db.Update(func(tx *bolt.Tx) error {
			return tx.Bucket(chainMetadataBucket).Delete(saveBlindedBeaconBlocksKey)
		}))

		// Not a fresh database, has blinded blocks already and should continue being that way.
		err = store.setupBlockStorageType(ctx)
		require.NoError(t, err)

		var shouldSaveBlinded bool
		require.NoError(t, store.db.Update(func(tx *bolt.Tx) error {
			bkt := tx.Bucket(chainMetadataBucket)
			shouldSaveBlinded = len(bkt.Get(saveBlindedBeaconBlocksKey)) > 0
			return nil
		}))

		// Should have set the chain metadata bucket to save blinded
		require.Equal(t, true, shouldSaveBlinded)

		blkFull := util.NewBeaconBlockBellatrix()
		blkFull.Block.Body.ExecutionPayload.BlockNumber = 2
		wrappedBlock, err = blocks.NewSignedBeaconBlock(blkFull)
		require.NoError(t, err)
		root, err = wrappedBlock.Block().HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, store.SaveBlock(ctx, wrappedBlock))
		wrappedBlinded, err := wrappedBlock.ToBlinded()
		require.NoError(t, err)

		retrievedBlk, err = store.Block(ctx, root)
		require.NoError(t, err)
		require.Equal(t, true, retrievedBlk.IsBlinded())

		// Compare retrieved value by root, and marshaled bytes.
		mSrc, err := wrappedBlinded.MarshalSSZ()
		require.NoError(t, err)
		mTgt, err := retrievedBlk.MarshalSSZ()
		require.NoError(t, err)
		require.Equal(t, true, bytes.Equal(mSrc, mTgt))

		rSrc, err := wrappedBlinded.Block().HashTreeRoot()
		require.NoError(t, err)
		rTgt, err := retrievedBlk.Block().HashTreeRoot()
		require.NoError(t, err)
		require.Equal(t, rSrc, rTgt)
	})
	t.Run("existing database with full blocks type should continue storing full blocks", func(t *testing.T) {
		store := setupDB(t)
		require.NoError(t, store.db.Update(func(tx *bolt.Tx) error {
			return tx.Bucket(chainMetadataBucket).Delete(saveBlindedBeaconBlocksKey)
		}))

		blk := util.NewBeaconBlockBellatrix()
		blk.Block.Body.ExecutionPayload.BlockNumber = 1
		wrappedBlock, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		root, err := wrappedBlock.Block().HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, store.SaveBlock(ctx, wrappedBlock))
		require.NoError(t, store.SaveStateSummary(ctx, &ethpb.StateSummary{Root: root[:]}))
		require.NoError(t, store.SaveHeadBlockRoot(ctx, root))
		retrievedBlk, err := store.Block(ctx, root)
		require.NoError(t, err)
		require.Equal(t, false, retrievedBlk.IsBlinded())
		require.DeepEqual(t, wrappedBlock, retrievedBlk)

		// Not a fresh database, has full blocks already and should continue being that way.
		err = store.setupBlockStorageType(ctx)
		require.NoError(t, err)

		blk = util.NewBeaconBlockBellatrix()
		blk.Block.Body.ExecutionPayload.BlockNumber = 2
		wrappedBlock, err = blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		root, err = wrappedBlock.Block().HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, store.SaveBlock(ctx, wrappedBlock))

		retrievedBlk, err = store.Block(ctx, root)
		require.NoError(t, err)
		require.Equal(t, false, retrievedBlk.IsBlinded())

		// Compare retrieved value by root, and marshaled bytes.
		mSrc, err := wrappedBlock.MarshalSSZ()
		require.NoError(t, err)
		mTgt, err := retrievedBlk.MarshalSSZ()
		require.NoError(t, err)
		require.Equal(t, true, bytes.Equal(mSrc, mTgt))

		rTgt, err := retrievedBlk.Block().HashTreeRoot()
		require.NoError(t, err)
		require.Equal(t, root, rTgt)
	})
	t.Run("existing database with blinded blocks type should error if user enables full blocks feature flag", func(t *testing.T) {
		store := setupDB(t)

		blk := util.NewBeaconBlockBellatrix()
		blk.Block.Body.ExecutionPayload.BlockNumber = 1
		wrappedBlock, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		root, err := wrappedBlock.Block().HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, store.SaveBlock(ctx, wrappedBlock))
		require.NoError(t, store.SaveStateSummary(ctx, &ethpb.StateSummary{Root: root[:]}))
		require.NoError(t, store.SaveHeadBlockRoot(ctx, root))
		retrievedBlk, err := store.Block(ctx, root)
		require.NoError(t, err)
		require.Equal(t, true, retrievedBlk.IsBlinded())
		wantedBlk, err := wrappedBlock.ToBlinded()
		require.NoError(t, err)
		require.DeepEqual(t, wantedBlk, retrievedBlk)

		// Trying to enable full blocks with a database that is already storing blinded blocks should error.
		resetFn := features.InitWithReset(&features.Flags{
			SaveFullExecutionPayloads: true,
		})
		defer resetFn()
		err = store.setupBlockStorageType(ctx)
		errMsg := "cannot use the %s flag with this existing database, as it has already been initialized"
		require.ErrorContains(t, fmt.Sprintf(errMsg, features.SaveFullExecutionPayloads.Name), err)
	})
}
