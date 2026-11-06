package kv

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	bolt "go.etcd.io/bbolt"
	"go.opentelemetry.io/otel"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/protobuf/proto"
)

func TestStore_LastValidatedCheckpoint_CanSaveRetrieve(t *testing.T) {
	db := setupDB(t)
	ctx := t.Context()
	root := bytesutil.ToBytes32([]byte{'A'})
	cp := &ethpb.Checkpoint{
		Epoch: 10,
		Root:  root[:],
	}
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(1))
	require.NoError(t, db.SaveState(ctx, st, root))
	require.NoError(t, db.SaveLastValidatedCheckpoint(ctx, cp))

	retrieved, err := db.LastValidatedCheckpoint(ctx)
	require.NoError(t, err)
	assert.Equal(t, true, proto.Equal(cp, retrieved), "Wanted %v, received %v", cp, retrieved)
}

func TestStore_LastValidatedCheckpoint_Recover(t *testing.T) {
	db := setupDB(t)
	ctx := t.Context()
	blk := util.HydrateSignedBeaconBlock(&ethpb.SignedBeaconBlock{})
	r, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	cp := &ethpb.Checkpoint{
		Epoch: 2,
		Root:  r[:],
	}
	wb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, wb))
	require.NoError(t, db.SaveLastValidatedCheckpoint(ctx, cp))
	retrieved, err := db.LastValidatedCheckpoint(ctx)
	require.NoError(t, err)
	assert.Equal(t, true, proto.Equal(cp, retrieved), "Wanted %v, received %v", cp, retrieved)
}

func BenchmarkStore_SaveLastValidatedCheckpoint(b *testing.B) {
	db := setupDB(b)
	ctx := b.Context()
	root := bytesutil.ToBytes32([]byte{'A'})
	cp := &ethpb.Checkpoint{
		Epoch: 10,
		Root:  root[:],
	}
	st, err := util.NewBeaconState()
	require.NoError(b, err)
	require.NoError(b, st.SetSlot(1))
	require.NoError(b, db.SaveState(ctx, st, root))
	db.stateSummaryCache.clear()

	for b.Loop() {
		require.NoError(b, db.SaveLastValidatedCheckpoint(ctx, cp))
	}
}

func TestStore_LastValidatedCheckpoint_DefaultIsFinalized(t *testing.T) {
	db := setupDB(t)
	ctx := t.Context()

	genesis := bytesutil.ToBytes32([]byte{'G', 'E', 'N', 'E', 'S', 'I', 'S'})
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesis))

	blk := util.NewBeaconBlock()
	blk.Block.ParentRoot = genesis[:]
	blk.Block.Slot = 40

	root, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)

	cp := &ethpb.Checkpoint{
		Epoch: 5,
		Root:  root[:],
	}

	// a valid chain is required to save finalized checkpoint.
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, wsb))
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(1))
	// a state is required to save checkpoint
	require.NoError(t, db.SaveState(ctx, st, root))

	require.NoError(t, db.SaveFinalizedCheckpoint(ctx, cp))

	retrieved, err := db.LastValidatedCheckpoint(ctx)
	require.NoError(t, err)
	assert.Equal(t, true, proto.Equal(cp, retrieved), "Wanted %v, received %v", cp, retrieved)
}

type spanHookTracerProvider struct {
	noop.TracerProvider
	onStart func(name string)
}

func (p spanHookTracerProvider) Tracer(string, ...oteltrace.TracerOption) oteltrace.Tracer {
	return spanHookTracer{onStart: p.onStart}
}

type spanHookTracer struct {
	noop.Tracer
	onStart func(name string)
}

func (t spanHookTracer) Start(ctx context.Context, name string, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	t.onStart(name)
	return noop.Tracer{}.Start(ctx, name, opts...)
}

func TestStore_LastValidatedCheckpoint_NoDeadlockWithConcurrentRemap(t *testing.T) {
	ctx := t.Context()
	// Default mmap size so a small write remaps; NewKVStore maps 512MiB.
	boltDB, err := bolt.Open(StoreDatafilePath(t.TempDir()), 0o600, &bolt.Options{Timeout: time.Second})
	require.NoError(t, err)
	db := &Store{db: boltDB}

	root := bytesutil.ToBytes32([]byte{'A'})
	cp := &ethpb.Checkpoint{Epoch: 5, Root: root[:]}
	require.NoError(t, boltDB.Update(func(tx *bolt.Tx) error {
		bkt, err := tx.CreateBucket(checkpointBucket)
		if err != nil {
			return err
		}
		enc, err := encode(ctx, cp)
		if err != nil {
			return err
		}
		return bkt.Put(finalizedCheckpointKey, enc)
	}))

	var fired atomic.Bool
	writerDone := make(chan error, 1)
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(spanHookTracerProvider{onStart: func(name string) {
		if name == "BeaconDB.LastValidatedCheckpoint" || !fired.CompareAndSwap(false, true) {
			return
		}
		// Inside the read tx: start a write that must remap the DB.
		go func() {
			writerDone <- boltDB.Update(func(tx *bolt.Tx) error {
				b, err := tx.CreateBucketIfNotExists([]byte("remap"))
				if err != nil {
					return err
				}
				return b.Put([]byte("grow"), make([]byte, 4*1024*1024))
			})
		}()
		time.Sleep(500 * time.Millisecond)
	}})
	trace.TracingEnabled = true
	defer func() {
		trace.TracingEnabled = false
		otel.SetTracerProvider(prevTP)
	}()

	type result struct {
		cp  *ethpb.Checkpoint
		err error
	}
	done := make(chan result, 1)
	go func() {
		c, err := db.LastValidatedCheckpoint(ctx)
		done <- result{c, err}
	}()

	select {
	case res := <-done:
		require.NoError(t, res.err)
		assert.Equal(t, true, proto.Equal(cp, res.cp), "Wanted %v, received %v", cp, res.cp)
	case <-time.After(5 * time.Second):
		t.Fatal("LastValidatedCheckpoint deadlocked with a concurrent mmap-growing write")
	}
	require.Equal(t, true, fired.Load(), "hook did not fire inside the read tx")
	require.NoError(t, <-writerDone)
	require.NoError(t, boltDB.Close())
}
