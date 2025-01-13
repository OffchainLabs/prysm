package helpers

import (
	"context"
	"strconv"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	chainmock "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	dbtest "github.com/prysmaticlabs/prysm/v5/beacon-chain/db/testing"
	doublylinkedtree "github.com/prysmaticlabs/prysm/v5/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/testutil"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func TestIsOptimistic(t *testing.T) {
	ctx := context.Background()

	t.Run("head optimistic", func(t *testing.T) {
		cs := &chainmock.ChainService{Optimistic: true}
		o, err := IsOptimistic(ctx, []byte("head"), cs, nil, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, true, o)
	})
	t.Run("head not optimistic", func(t *testing.T) {
		cs := &chainmock.ChainService{Optimistic: false}
		o, err := IsOptimistic(ctx, []byte("head"), cs, nil, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, false, o)
	})
	t.Run("genesis", func(t *testing.T) {
		o, err := IsOptimistic(ctx, []byte("genesis"), nil, nil, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, false, o)
	})
	t.Run("finalized", func(t *testing.T) {
		t.Run("finalized checkpoint is optimistic", func(t *testing.T) {
			st, err := util.NewBeaconState()
			require.NoError(t, err)
			cs := &chainmock.ChainService{Optimistic: true, FinalizedCheckPoint: &eth.Checkpoint{}, OptimisticRoots: map[[32]byte]bool{{}: true}}
			mf := &testutil.MockStater{BeaconState: st}
			o, err := IsOptimistic(ctx, []byte("finalized"), cs, mf, cs, nil)
			require.NoError(t, err)
			assert.Equal(t, true, o)
		})
		t.Run("finalized checkpoint is not optimistic", func(t *testing.T) {
			st, err := util.NewBeaconState()
			require.NoError(t, err)
			cs := &chainmock.ChainService{Optimistic: true, FinalizedCheckPoint: &eth.Checkpoint{}}
			mf := &testutil.MockStater{BeaconState: st}
			o, err := IsOptimistic(ctx, []byte("finalized"), cs, mf, cs, nil)
			require.NoError(t, err)
			assert.Equal(t, false, o)
		})
	})
	t.Run("justified", func(t *testing.T) {
		t.Run("justified checkpoint is optimistic", func(t *testing.T) {
			st, err := util.NewBeaconState()
			require.NoError(t, err)
			cs := &chainmock.ChainService{Optimistic: true, CurrentJustifiedCheckPoint: &eth.Checkpoint{}, OptimisticRoots: map[[32]byte]bool{{}: true}}
			mf := &testutil.MockStater{BeaconState: st}
			o, err := IsOptimistic(ctx, []byte("justified"), cs, mf, cs, nil)
			require.NoError(t, err)
			assert.Equal(t, true, o)
		})
		t.Run("justified checkpoint is not optimistic", func(t *testing.T) {
			st, err := util.NewBeaconState()
			require.NoError(t, err)
			cs := &chainmock.ChainService{Optimistic: true, CurrentJustifiedCheckPoint: &eth.Checkpoint{}}
			mf := &testutil.MockStater{BeaconState: st}
			o, err := IsOptimistic(ctx, []byte("justified"), cs, mf, cs, nil)
			require.NoError(t, err)
			assert.Equal(t, false, o)
		})
	})
	t.Run("root", func(t *testing.T) {
		t.Run("is head and head is optimistic", func(t *testing.T) {
			st, err := util.NewBeaconState()
			require.NoError(t, err)
			cs := &chainmock.ChainService{Optimistic: true}
			mf := &testutil.MockStater{BeaconState: st}
			o, err := IsOptimistic(ctx, bytesutil.PadTo([]byte("root"), 32), cs, mf, cs, nil)
			require.NoError(t, err)
			assert.Equal(t, true, o)
		})
		t.Run("is head and head is not optimistic", func(t *testing.T) {
			st, err := util.NewBeaconState()
			require.NoError(t, err)
			cs := &chainmock.ChainService{Optimistic: false}
			mf := &testutil.MockStater{BeaconState: st}
			o, err := IsOptimistic(ctx, bytesutil.PadTo([]byte("root"), 32), cs, mf, cs, nil)
			require.NoError(t, err)
			assert.Equal(t, false, o)
		})
		t.Run("root is optimistic", func(t *testing.T) {
			b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
			require.NoError(t, err)
			b.SetStateRoot(bytesutil.PadTo([]byte("root"), 32))
			db := dbtest.SetupDB(t)
			require.NoError(t, db.SaveBlock(ctx, b))
			fetcherSt, err := util.NewBeaconState()
			require.NoError(t, err)
			chainSt, err := util.NewBeaconState()
			require.NoError(t, err)
			require.NoError(t, chainSt.SetSlot(fieldparams.SlotsPerEpoch))
			bRoot, err := b.Block().HashTreeRoot()
			require.NoError(t, err)
			cs := &chainmock.ChainService{State: chainSt, OptimisticRoots: map[[32]byte]bool{bRoot: true}}
			mf := &testutil.MockStater{BeaconState: fetcherSt}
			o, err := IsOptimistic(ctx, bytesutil.PadTo([]byte("root"), 32), cs, mf, cs, db)
			require.NoError(t, err)
			assert.Equal(t, true, o)
		})
		t.Run("root is not optimistic", func(t *testing.T) {
			b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
			require.NoError(t, err)
			b.SetStateRoot(bytesutil.PadTo([]byte("root"), 32))
			db := dbtest.SetupDB(t)
			require.NoError(t, db.SaveBlock(ctx, b))
			fetcherSt, err := util.NewBeaconState()
			require.NoError(t, err)
			chainSt, err := util.NewBeaconState()
			require.NoError(t, err)
			require.NoError(t, chainSt.SetSlot(fieldparams.SlotsPerEpoch))
			cs := &chainmock.ChainService{State: chainSt}
			mf := &testutil.MockStater{BeaconState: fetcherSt}
			o, err := IsOptimistic(ctx, bytesutil.PadTo([]byte("root"), 32), cs, mf, cs, db)
			require.NoError(t, err)
			assert.Equal(t, false, o)
		})
		t.Run("no canonical blocks", func(t *testing.T) {
			b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
			require.NoError(t, err)
			db := dbtest.SetupDB(t)
			require.NoError(t, db.SaveBlock(ctx, b))
			fetcherSt, err := util.NewBeaconState()
			require.NoError(t, err)
			chainSt, err := util.NewBeaconState()
			require.NoError(t, err)
			require.NoError(t, chainSt.SetSlot(fieldparams.SlotsPerEpoch))
			cs := &chainmock.ChainService{Optimistic: false, State: chainSt, CanonicalRoots: map[[32]byte]bool{}}
			mf := &testutil.MockStater{BeaconState: fetcherSt}
			o, err := IsOptimistic(ctx, bytesutil.PadTo([]byte("root"), 32), nil, mf, cs, db)
			require.NoError(t, err)
			assert.Equal(t, true, o)
		})
	})
	t.Run("hex", func(t *testing.T) {
		t.Run("is head and head is optimistic", func(t *testing.T) {
			st, err := util.NewBeaconState()
			require.NoError(t, err)
			cs := &chainmock.ChainService{Optimistic: true}
			mf := &testutil.MockStater{BeaconState: st}
			o, err := IsOptimistic(ctx, []byte(hexutil.Encode(bytesutil.PadTo([]byte("root"), 32))), cs, mf, cs, nil)
			require.NoError(t, err)
			assert.Equal(t, true, o)
		})
		t.Run("is head and head is not optimistic", func(t *testing.T) {
			st, err := util.NewBeaconState()
			require.NoError(t, err)
			cs := &chainmock.ChainService{Optimistic: false}
			mf := &testutil.MockStater{BeaconState: st}
			o, err := IsOptimistic(ctx, []byte(hexutil.Encode(bytesutil.PadTo([]byte("root"), 32))), cs, mf, cs, nil)
			require.NoError(t, err)
			assert.Equal(t, false, o)
		})
		t.Run("root is optimistic", func(t *testing.T) {
			b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
			require.NoError(t, err)
			b.SetStateRoot(bytesutil.PadTo([]byte("root"), 32))
			db := dbtest.SetupDB(t)
			require.NoError(t, db.SaveBlock(ctx, b))
			fetcherSt, err := util.NewBeaconState()
			require.NoError(t, err)
			chainSt, err := util.NewBeaconState()
			require.NoError(t, err)
			require.NoError(t, chainSt.SetSlot(fieldparams.SlotsPerEpoch))
			bRoot, err := b.Block().HashTreeRoot()
			require.NoError(t, err)
			cs := &chainmock.ChainService{State: chainSt, OptimisticRoots: map[[32]byte]bool{bRoot: true}}
			mf := &testutil.MockStater{BeaconState: fetcherSt}
			o, err := IsOptimistic(ctx, []byte(hexutil.Encode(bytesutil.PadTo([]byte("root"), 32))), cs, mf, cs, db)
			require.NoError(t, err)
			assert.Equal(t, true, o)
		})
		t.Run("root is not optimistic", func(t *testing.T) {
			b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
			require.NoError(t, err)
			b.SetStateRoot(bytesutil.PadTo([]byte("root"), 32))
			db := dbtest.SetupDB(t)
			require.NoError(t, db.SaveBlock(ctx, b))
			fetcherSt, err := util.NewBeaconState()
			require.NoError(t, err)
			chainSt, err := util.NewBeaconState()
			require.NoError(t, err)
			require.NoError(t, chainSt.SetSlot(fieldparams.SlotsPerEpoch))
			cs := &chainmock.ChainService{State: chainSt}
			mf := &testutil.MockStater{BeaconState: fetcherSt}
			o, err := IsOptimistic(ctx, []byte(hexutil.Encode(bytesutil.PadTo([]byte("root"), 32))), cs, mf, cs, db)
			require.NoError(t, err)
			assert.Equal(t, false, o)
		})
		t.Run("no canonical blocks", func(t *testing.T) {
			b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
			require.NoError(t, err)
			db := dbtest.SetupDB(t)
			require.NoError(t, db.SaveBlock(ctx, b))
			fetcherSt, err := util.NewBeaconState()
			require.NoError(t, err)
			chainSt, err := util.NewBeaconState()
			require.NoError(t, err)
			require.NoError(t, chainSt.SetSlot(fieldparams.SlotsPerEpoch))
			cs := &chainmock.ChainService{Optimistic: false, State: chainSt, CanonicalRoots: map[[32]byte]bool{}}
			mf := &testutil.MockStater{BeaconState: fetcherSt}
			o, err := IsOptimistic(ctx, []byte(hexutil.Encode(bytesutil.PadTo([]byte("root"), 32))), nil, mf, cs, db)
			require.NoError(t, err)
			assert.Equal(t, true, o)
		})
	})
	t.Run("slot", func(t *testing.T) {
		t.Run("head is not optimistic", func(t *testing.T) {
			cs := &chainmock.ChainService{Optimistic: false}
			o, err := IsOptimistic(ctx, []byte("0"), cs, nil, nil, nil)
			require.NoError(t, err)
			assert.Equal(t, false, o)
		})
		t.Run("is before validated slot when head is optimistic", func(t *testing.T) {
			db := dbtest.SetupDB(t)
			require.NoError(t, db.SaveStateSummary(ctx, &eth.StateSummary{Slot: fieldparams.SlotsPerEpoch, Root: []byte("root")}))
			require.NoError(t, db.SaveLastValidatedCheckpoint(ctx, &eth.Checkpoint{Epoch: 1, Root: []byte("root")}))
			cs := &chainmock.ChainService{Optimistic: true, FinalizedCheckPoint: &eth.Checkpoint{Epoch: 1}}
			o, err := IsOptimistic(ctx, []byte("0"), cs, nil, cs, db)
			require.NoError(t, err)
			assert.Equal(t, false, o)
		})
		t.Run("is equal to validated slot when head is optimistic", func(t *testing.T) {
			db := dbtest.SetupDB(t)
			require.NoError(t, db.SaveStateSummary(ctx, &eth.StateSummary{Slot: fieldparams.SlotsPerEpoch, Root: []byte("root")}))
			require.NoError(t, db.SaveLastValidatedCheckpoint(ctx, &eth.Checkpoint{Epoch: 1, Root: []byte("root")}))
			cs := &chainmock.ChainService{Optimistic: true, FinalizedCheckPoint: &eth.Checkpoint{Epoch: 1}}
			o, err := IsOptimistic(ctx, []byte("32"), cs, nil, cs, db)
			require.NoError(t, err)
			assert.Equal(t, false, o)
		})
		t.Run("is after validated slot and validated slot is before finalized slot", func(t *testing.T) {
			db := dbtest.SetupDB(t)
			require.NoError(t, db.SaveStateSummary(ctx, &eth.StateSummary{Slot: fieldparams.SlotsPerEpoch, Root: []byte("root")}))
			require.NoError(t, db.SaveLastValidatedCheckpoint(ctx, &eth.Checkpoint{Epoch: 1, Root: []byte("root")}))
			cs := &chainmock.ChainService{Optimistic: true, FinalizedCheckPoint: &eth.Checkpoint{Epoch: 2}}
			o, err := IsOptimistic(ctx, []byte("33"), cs, nil, cs, db)
			require.NoError(t, err)
			assert.Equal(t, true, o)
		})
		t.Run("is head", func(t *testing.T) {
			db := dbtest.SetupDB(t)
			require.NoError(t, db.SaveStateSummary(ctx, &eth.StateSummary{Slot: fieldparams.SlotsPerEpoch, Root: []byte("root")}))
			require.NoError(t, db.SaveLastValidatedCheckpoint(ctx, &eth.Checkpoint{Epoch: 1, Root: []byte("root")}))
			fetcherSt, err := util.NewBeaconState()
			require.NoError(t, err)
			chainSt, err := util.NewBeaconState()
			require.NoError(t, err)
			require.NoError(t, chainSt.SetSlot(fieldparams.SlotsPerEpoch*2))
			cs := &chainmock.ChainService{Optimistic: true, State: chainSt, FinalizedCheckPoint: &eth.Checkpoint{Epoch: 0}}
			mf := &testutil.MockStater{BeaconState: fetcherSt}
			o, err := IsOptimistic(ctx, []byte(strconv.Itoa(fieldparams.SlotsPerEpoch*2)), cs, mf, cs, db)
			require.NoError(t, err)
			assert.Equal(t, true, o)
		})
		t.Run("ancestor is optimistic", func(t *testing.T) {
			db := dbtest.SetupDB(t)
			require.NoError(t, db.SaveStateSummary(ctx, &eth.StateSummary{Slot: fieldparams.SlotsPerEpoch, Root: []byte("root")}))
			require.NoError(t, db.SaveLastValidatedCheckpoint(ctx, &eth.Checkpoint{Epoch: 1, Root: []byte("root")}))
			r := bytesutil.ToBytes32([]byte("root"))
			fcs := doublylinkedtree.New()
			finalizedCheckpt := &eth.Checkpoint{Epoch: 0}
			st, root, err := prepareForkchoiceState(fieldparams.SlotsPerEpoch*2, r, [32]byte{}, [32]byte{}, finalizedCheckpt, finalizedCheckpt)
			require.NoError(t, err)
			require.NoError(t, fcs.InsertNode(ctx, st, root))
			headRoot := [32]byte{'r'}
			st, root, err = prepareForkchoiceState(fieldparams.SlotsPerEpoch*2+1, headRoot, r, [32]byte{}, finalizedCheckpt, finalizedCheckpt)
			require.NoError(t, err)
			require.NoError(t, fcs.InsertNode(ctx, st, root))
			cs := &chainmock.ChainService{Root: headRoot[:], Optimistic: true, ForkChoiceStore: fcs, OptimisticRoots: map[[32]byte]bool{r: true}, FinalizedCheckPoint: finalizedCheckpt}
			mf := &testutil.MockStater{BeaconState: st}
			o, err := IsOptimistic(ctx, []byte(strconv.Itoa(fieldparams.SlotsPerEpoch*2)), cs, mf, cs, db)
			require.NoError(t, err)
			assert.Equal(t, true, o)
		})
		t.Run("ancestor is not optimistic", func(t *testing.T) {
			db := dbtest.SetupDB(t)
			require.NoError(t, db.SaveStateSummary(ctx, &eth.StateSummary{Slot: fieldparams.SlotsPerEpoch, Root: []byte("root")}))
			require.NoError(t, db.SaveLastValidatedCheckpoint(ctx, &eth.Checkpoint{Epoch: 1, Root: []byte("root")}))
			r := bytesutil.ToBytes32([]byte("root"))
			fcs := doublylinkedtree.New()
			finalizedCheckpt := &eth.Checkpoint{Epoch: 0}
			st, root, err := prepareForkchoiceState(fieldparams.SlotsPerEpoch*2, r, [32]byte{}, [32]byte{}, finalizedCheckpt, finalizedCheckpt)
			require.NoError(t, err)
			require.NoError(t, fcs.InsertNode(ctx, st, root))
			headRoot := [32]byte{'r'}
			st, root, err = prepareForkchoiceState(fieldparams.SlotsPerEpoch*2+1, headRoot, r, [32]byte{}, finalizedCheckpt, finalizedCheckpt)
			require.NoError(t, err)
			require.NoError(t, fcs.InsertNode(ctx, st, root))
			cs := &chainmock.ChainService{Root: headRoot[:], Optimistic: true, ForkChoiceStore: fcs, OptimisticRoots: map[[32]byte]bool{r: false}, FinalizedCheckPoint: finalizedCheckpt}
			mf := &testutil.MockStater{BeaconState: st}
			o, err := IsOptimistic(ctx, []byte(strconv.Itoa(fieldparams.SlotsPerEpoch*2)), cs, mf, cs, db)
			require.NoError(t, err)
			assert.Equal(t, false, o)
		})
	})
}

// prepareForkchoiceState prepares a beacon state with the given data to mock
// insert into forkchoice
func prepareForkchoiceState(
	slot primitives.Slot,
	blockRoot [32]byte,
	parentRoot [32]byte,
	payloadHash [32]byte,
	justified *eth.Checkpoint,
	finalized *eth.Checkpoint,
) (state.BeaconState, blocks.ROBlock, error) {
	blockHeader := &eth.BeaconBlockHeader{
		ParentRoot: parentRoot[:],
	}

	executionHeader := &enginev1.ExecutionPayloadHeader{
		BlockHash: payloadHash[:],
	}

	base := &eth.BeaconStateBellatrix{
		Slot:                         slot,
		RandaoMixes:                  make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
		BlockRoots:                   make([][]byte, 1),
		CurrentJustifiedCheckpoint:   justified,
		FinalizedCheckpoint:          finalized,
		LatestExecutionPayloadHeader: executionHeader,
		LatestBlockHeader:            blockHeader,
	}

	base.BlockRoots[0] = append(base.BlockRoots[0], blockRoot[:]...)
	st, err := state_native.InitializeFromProtoBellatrix(base)
	if err != nil {
		return nil, blocks.ROBlock{}, err
	}
	blk := &eth.SignedBeaconBlockBellatrix{
		Block: &eth.BeaconBlockBellatrix{
			Slot:       slot,
			ParentRoot: parentRoot[:],
			Body: &eth.BeaconBlockBodyBellatrix{
				ExecutionPayload: &enginev1.ExecutionPayload{
					BlockHash: payloadHash[:],
				},
			},
		},
	}
	signed, err := blocks.NewSignedBeaconBlock(blk)
	if err != nil {
		return nil, blocks.ROBlock{}, err
	}
	roblock, err := blocks.NewROBlockWithRoot(signed, blockRoot)
	return st, roblock, err
}
