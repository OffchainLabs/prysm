package blockchain

import (
	"bytes"
	"context"
	"sort"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	statefeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/state"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/blstoexec"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestSaveHead_Same(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)

	r := [32]byte{'A'}
	service.head = &head{root: r}
	b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
	require.NoError(t, err)
	st, _ := util.DeterministicGenesisState(t, 1)
	require.NoError(t, service.saveHead(t.Context(), r, b, st, false))
	assert.Equal(t, primitives.Slot(0), service.headSlot(), "Head did not stay the same")
	assert.Equal(t, r, service.headRoot(), "Head did not stay the same")
}

func TestSaveHead_Different(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)

	oldBlock := util.SaveBlock(t, t.Context(), service.cfg.BeaconDB, util.NewBeaconBlock())
	oldRoot, err := oldBlock.Block().HashTreeRoot()
	require.NoError(t, err)
	ojc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	state, blkRoot, err := prepareForkchoiceState(ctx, oldBlock.Block().Slot(), oldRoot, oldBlock.Block().ParentRoot(), [32]byte{}, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
	service.head = &head{
		root:  oldRoot,
		block: oldBlock,
	}

	newHeadSignedBlock := util.NewBeaconBlock()
	newHeadSignedBlock.Block.Slot = 1
	newHeadBlock := newHeadSignedBlock.Block

	wsb := util.SaveBlock(t, t.Context(), service.cfg.BeaconDB, newHeadSignedBlock)
	newRoot, err := newHeadBlock.HashTreeRoot()
	require.NoError(t, err)
	state, blkRoot, err = prepareForkchoiceState(ctx, slots.PrevSlot(wsb.Block().Slot()), wsb.Block().ParentRoot(), service.cfg.ForkChoiceStore.CachedHeadRoot(), [32]byte{}, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))

	state, blkRoot, err = prepareForkchoiceState(ctx, wsb.Block().Slot(), newRoot, wsb.Block().ParentRoot(), [32]byte{}, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
	headState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, headState.SetSlot(1))
	require.NoError(t, service.cfg.BeaconDB.SaveStateSummary(t.Context(), &ethpb.StateSummary{Slot: 1, Root: newRoot[:]}))
	require.NoError(t, service.cfg.BeaconDB.SaveState(t.Context(), headState, newRoot))
	require.NoError(t, service.saveHead(t.Context(), newRoot, wsb, headState, false))

	assert.Equal(t, primitives.Slot(1), service.HeadSlot(), "Head did not change")

	cachedRoot, err := service.HeadRoot(t.Context())
	require.NoError(t, err)
	assert.DeepEqual(t, cachedRoot, newRoot[:], "Head did not change")
	headBlock, err := service.headBlock()
	require.NoError(t, err)
	pb, err := headBlock.Proto()
	require.NoError(t, err)
	assert.DeepEqual(t, newHeadSignedBlock, pb, "Head did not change")
	assert.DeepSSZEqual(t, headState.ToProto(), service.headState(ctx).ToProto(), "Head did not change")
}

func TestSaveHead_Different_Reorg(t *testing.T) {
	ctx := t.Context()
	hook := logTest.NewGlobal()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)

	oldBlock := util.SaveBlock(t, t.Context(), service.cfg.BeaconDB, util.NewBeaconBlock())
	oldRoot, err := oldBlock.Block().HashTreeRoot()
	require.NoError(t, err)
	ojc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	state, blkRoot, err := prepareForkchoiceState(ctx, oldBlock.Block().Slot(), oldRoot, oldBlock.Block().ParentRoot(), [32]byte{}, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
	service.head = &head{
		root:  oldRoot,
		block: oldBlock,
	}

	reorgChainParent := [32]byte{'B'}
	state, blkRoot, err = prepareForkchoiceState(ctx, 0, reorgChainParent, oldRoot, oldBlock.Block().ParentRoot(), ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))

	newHeadSignedBlock := util.NewBeaconBlock()
	newHeadSignedBlock.Block.Slot = 1
	newHeadSignedBlock.Block.ParentRoot = reorgChainParent[:]
	newHeadBlock := newHeadSignedBlock.Block

	wsb := util.SaveBlock(t, t.Context(), service.cfg.BeaconDB, newHeadSignedBlock)
	newRoot, err := newHeadBlock.HashTreeRoot()
	require.NoError(t, err)
	state, blkRoot, err = prepareForkchoiceState(ctx, wsb.Block().Slot(), newRoot, wsb.Block().ParentRoot(), [32]byte{}, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
	headState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, headState.SetSlot(1))
	require.NoError(t, service.cfg.BeaconDB.SaveStateSummary(t.Context(), &ethpb.StateSummary{Slot: 1, Root: newRoot[:]}))
	require.NoError(t, service.cfg.BeaconDB.SaveState(t.Context(), headState, newRoot))
	require.NoError(t, service.saveHead(t.Context(), newRoot, wsb, headState, false))

	assert.Equal(t, primitives.Slot(1), service.HeadSlot(), "Head did not change")

	cachedRoot, err := service.HeadRoot(t.Context())
	require.NoError(t, err)
	if !bytes.Equal(cachedRoot, newRoot[:]) {
		t.Error("Head did not change")
	}
	headBlock, err := service.headBlock()
	require.NoError(t, err)
	pb, err := headBlock.Proto()
	require.NoError(t, err)
	assert.DeepEqual(t, newHeadSignedBlock, pb, "Head did not change")
	assert.DeepSSZEqual(t, headState.ToProto(), service.headState(ctx).ToProto(), "Head did not change")
	require.LogsContain(t, hook, "Chain reorg occurred")
	require.LogsContain(t, hook, "distance=1")
	require.LogsContain(t, hook, "depth=1")
}

func Test_notifyNewHeadEvent(t *testing.T) {
	t.Run("genesis_state_root", func(t *testing.T) {
		srv := testServiceWithDB(t)
		srv.SetGenesisTime(time.Now())
		notifier := srv.cfg.StateNotifier.(*mock.MockStateNotifier)
		srv.originBlockRoot = [32]byte{1}
		st, blk, err := prepareForkchoiceState(t.Context(), 0, [32]byte{}, [32]byte{}, [32]byte{}, &ethpb.Checkpoint{}, &ethpb.Checkpoint{})
		require.NoError(t, err)
		require.NoError(t, srv.cfg.ForkChoiceStore.InsertNode(t.Context(), st, blk))
		newHeadStateRoot := [32]byte{2}
		newHeadRoot := [32]byte{3}
		st, blk, err = prepareForkchoiceState(t.Context(), 1, newHeadRoot, [32]byte{}, [32]byte{}, &ethpb.Checkpoint{}, &ethpb.Checkpoint{})
		require.NoError(t, err)
		require.NoError(t, srv.cfg.ForkChoiceStore.InsertNode(t.Context(), st, blk))
		require.NoError(t, srv.notifyNewHeadEvent(t.Context(), 1, newHeadStateRoot, newHeadRoot))
		require.Eventually(t, func() bool {
			return len(notifier.ReceivedEvents()) == 1
		}, 5*time.Second, 50*time.Millisecond, "Expected exactly 1 state notification")
		events := notifier.ReceivedEvents()

		eventHead, ok := events[0].Data.(*statefeed.HeadData)
		require.Equal(t, true, ok)
		wanted := &statefeed.HeadData{
			Slot:                      1,
			Block:                     newHeadRoot,
			State:                     newHeadStateRoot,
			EpochTransition:           false,
			PreviousDutyDependentRoot: srv.originBlockRoot,
			CurrentDutyDependentRoot:  srv.originBlockRoot,
		}
		require.DeepEqual(t, wanted, eventHead)
	})

	t.Run("zero head slot", func(t *testing.T) {
		srv := testServiceWithDB(t)
		srv.SetGenesisTime(time.Now())
		notifier := srv.cfg.StateNotifier.(*mock.MockStateNotifier)
		srv.originBlockRoot = [32]byte{1}
		st, blk, err := prepareForkchoiceState(t.Context(), 0, [32]byte{}, [32]byte{}, [32]byte{}, &ethpb.Checkpoint{}, &ethpb.Checkpoint{})
		require.NoError(t, err)
		require.NoError(t, srv.cfg.ForkChoiceStore.InsertNode(t.Context(), st, blk))
		newHeadStateRoot := [32]byte{2}
		newHeadRoot := [32]byte{3}
		require.NoError(t, srv.notifyNewHeadEvent(t.Context(), 0, newHeadStateRoot, newHeadRoot))
		require.Eventually(t, func() bool {
			return len(notifier.ReceivedEvents()) == 1
		}, 5*time.Second, 50*time.Millisecond, "Expected exactly 1 state notification")
		events := notifier.ReceivedEvents()

		eventHead, ok := events[0].Data.(*statefeed.HeadData)
		require.Equal(t, true, ok)
		wanted := &statefeed.HeadData{
			Slot:                      0,
			Block:                     newHeadRoot,
			State:                     newHeadStateRoot,
			EpochTransition:           true,
			PreviousDutyDependentRoot: srv.originBlockRoot,
			CurrentDutyDependentRoot:  srv.originBlockRoot,
		}
		require.DeepEqual(t, wanted, eventHead)
	})

	t.Run("non_genesis_values", func(t *testing.T) {
		bState, _ := util.DeterministicGenesisState(t, 10)
		genesisRoot := [32]byte{1}
		srv := testServiceWithDB(t)
		srv.SetGenesisTime(time.Now())
		srv.originBlockRoot = genesisRoot
		notifier := srv.cfg.StateNotifier.(*mock.MockStateNotifier)
		st, blk, err := prepareForkchoiceState(t.Context(), 0, srv.originBlockRoot, [32]byte{}, [32]byte{}, &ethpb.Checkpoint{}, &ethpb.Checkpoint{})
		require.NoError(t, err)
		require.NoError(t, srv.cfg.ForkChoiceStore.InsertNode(t.Context(), st, blk))
		epoch1Start, err := slots.EpochStart(1)
		require.NoError(t, err)
		epoch2Start, err := slots.EpochStart(1)
		require.NoError(t, err)
		require.NoError(t, bState.SetSlot(epoch1Start))

		newHeadStateRoot := [32]byte{2}
		newHeadRoot := [32]byte{3}
		st, blk, err = prepareForkchoiceState(t.Context(), 0, newHeadRoot, srv.originBlockRoot, [32]byte{}, &ethpb.Checkpoint{}, &ethpb.Checkpoint{})
		require.NoError(t, err)
		require.NoError(t, srv.cfg.ForkChoiceStore.InsertNode(t.Context(), st, blk))
		err = srv.notifyNewHeadEvent(t.Context(), epoch2Start, newHeadStateRoot, newHeadRoot)
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			return len(notifier.ReceivedEvents()) == 1
		}, 5*time.Second, 50*time.Millisecond, "Expected exactly 1 state notification")
		events := notifier.ReceivedEvents()

		eventHead, ok := events[0].Data.(*statefeed.HeadData)
		require.Equal(t, true, ok)
		wanted := &statefeed.HeadData{
			Slot:                      epoch2Start,
			Block:                     newHeadRoot,
			State:                     newHeadStateRoot,
			EpochTransition:           true,
			PreviousDutyDependentRoot: srv.originBlockRoot,
			CurrentDutyDependentRoot:  srv.originBlockRoot,
		}
		require.DeepEqual(t, wanted, eventHead)
	})
	t.Run("epoch transition", func(t *testing.T) {
		srv := testServiceWithDB(t)
		srv.SetGenesisTime(time.Now())
		notifier := srv.cfg.StateNotifier.(*mock.MockStateNotifier)
		srv.originBlockRoot = [32]byte{1}
		st, blk, err := prepareForkchoiceState(t.Context(), 0, srv.originBlockRoot, [32]byte{}, [32]byte{}, &ethpb.Checkpoint{}, &ethpb.Checkpoint{})
		require.NoError(t, err)
		require.NoError(t, srv.cfg.ForkChoiceStore.InsertNode(t.Context(), st, blk))
		newHeadStateRoot := [32]byte{2}
		newHeadRoot := [32]byte{3}
		st, blk, err = prepareForkchoiceState(t.Context(), 32, newHeadRoot, srv.originBlockRoot, [32]byte{}, &ethpb.Checkpoint{}, &ethpb.Checkpoint{})
		require.NoError(t, err)
		require.NoError(t, srv.cfg.ForkChoiceStore.InsertNode(t.Context(), st, blk))
		newHeadSlot := params.BeaconConfig().SlotsPerEpoch
		require.NoError(t, srv.notifyNewHeadEvent(t.Context(), newHeadSlot, newHeadStateRoot, newHeadRoot))
		require.Eventually(t, func() bool {
			return len(notifier.ReceivedEvents()) == 1
		}, 5*time.Second, 50*time.Millisecond, "Expected exactly 1 state notification")
		events := notifier.ReceivedEvents()

		eventHead, ok := events[0].Data.(*statefeed.HeadData)
		require.Equal(t, true, ok)
		wanted := &statefeed.HeadData{
			Slot:                      newHeadSlot,
			Block:                     newHeadRoot,
			State:                     newHeadStateRoot,
			EpochTransition:           true,
			PreviousDutyDependentRoot: srv.originBlockRoot,
			CurrentDutyDependentRoot:  srv.originBlockRoot,
		}
		require.DeepEqual(t, wanted, eventHead)
	})
	t.Run("genesis dependent root uses origin", func(t *testing.T) {
		srv := testServiceWithDB(t)
		srv.SetGenesisTime(time.Now())
		notifier := srv.cfg.StateNotifier.(*mock.MockStateNotifier)
		srv.originBlockRoot = [32]byte{0xab}
		st, blk, err := prepareForkchoiceState(t.Context(), 0, srv.originBlockRoot, [32]byte{}, [32]byte{}, &ethpb.Checkpoint{}, &ethpb.Checkpoint{})
		require.NoError(t, err)
		require.NoError(t, srv.cfg.ForkChoiceStore.InsertNode(t.Context(), st, blk))
		newHeadRoot := [32]byte{3}
		st, blk, err = prepareForkchoiceState(t.Context(), 32, newHeadRoot, srv.originBlockRoot, [32]byte{}, &ethpb.Checkpoint{}, &ethpb.Checkpoint{})
		require.NoError(t, err)
		require.NoError(t, srv.cfg.ForkChoiceStore.InsertNode(t.Context(), st, blk))
		newHeadSlot := params.BeaconConfig().SlotsPerEpoch
		require.NoError(t, srv.notifyNewHeadEvent(t.Context(), newHeadSlot, [32]byte{2}, newHeadRoot))
		require.Eventually(t, func() bool {
			return len(notifier.ReceivedEvents()) == 1
		}, 5*time.Second, 50*time.Millisecond, "Expected exactly 1 state notification")
		events := notifier.ReceivedEvents()

		eventHead, ok := events[0].Data.(*statefeed.HeadData)
		require.Equal(t, true, ok)
		// Epoch zero uses the genesis root.
		assert.DeepEqual(t, srv.originBlockRoot, eventHead.PreviousDutyDependentRoot)
		assert.DeepEqual(t, srv.originBlockRoot, eventHead.CurrentDutyDependentRoot)
	})
}

func Test_notifyNewHeadV2Event(t *testing.T) {
	setupHeadV2Service := func(t *testing.T, headSlot primitives.Slot) (*Service, chan *feed.Event, [32]byte, [32]byte) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		cfg.InitializeForkSchedule()
		params.OverrideBeaconConfig(cfg)

		srv := testServiceWithDB(t)
		srv.SetGenesisTime(time.Now())
		srv.originBlockRoot = [32]byte{1}
		st, blk, err := prepareForkchoiceState(t.Context(), 0, srv.originBlockRoot, [32]byte{}, [32]byte{}, &ethpb.Checkpoint{}, &ethpb.Checkpoint{})
		require.NoError(t, err)
		require.NoError(t, srv.cfg.ForkChoiceStore.InsertNode(t.Context(), st, blk))
		newHeadStateRoot := [32]byte{2}
		newHeadRoot := [32]byte{3}
		st, blk, err = prepareForkchoiceState(t.Context(), headSlot, newHeadRoot, srv.originBlockRoot, [32]byte{}, &ethpb.Checkpoint{}, &ethpb.Checkpoint{})
		require.NoError(t, err)
		require.NoError(t, srv.cfg.ForkChoiceStore.InsertNode(t.Context(), st, blk))
		require.NoError(t, srv.cfg.ForkChoiceStore.SetOptimisticToValid(t.Context(), newHeadRoot))
		events := make(chan *feed.Event, 10)
		srv.cfg.StateNotifier.StateFeed().Subscribe(events)
		return srv, events, newHeadStateRoot, newHeadRoot
	}

	// requireSingleHeadV2 is a helper function for draining the feed
	// and asserting that exactly one head_v2 event is present, returning its data for further checks.
	requireSingleHeadV2 := func(t *testing.T, events chan *feed.Event) *statefeed.HeadV2Data {
		var headV2 *statefeed.HeadV2Data
		var count int
		for {
			select {
			case e := <-events:
				if e.Type == statefeed.NewHeadV2 {
					count++
					d, ok := e.Data.(*statefeed.HeadV2Data)
					require.Equal(t, true, ok)
					headV2 = d
				}
				continue
			default:
			}
			break
		}
		require.Equal(t, 1, count)
		require.NotNil(t, headV2)
		return headV2
	}

	t.Run("zero head slot", func(t *testing.T) {
		srv, events, newHeadStateRoot, newHeadRoot := setupHeadV2Service(t, 0)
		require.NoError(t, srv.notifyNewHeadV2Event(t.Context(), 0, newHeadStateRoot, newHeadRoot, version.Gloas, true))
		wantedV2 := &statefeed.HeadV2Data{
			Slot:                      0,
			Block:                     newHeadRoot,
			State:                     newHeadStateRoot,
			EpochTransition:           true,
			CurrentEpochDependentRoot: srv.originBlockRoot,
			NextEpochDependentRoot:    srv.originBlockRoot,
			PayloadStatus:             statefeed.PayloadStatusFull,
			Version:                   version.Gloas,
		}
		require.DeepEqual(t, wantedV2, requireSingleHeadV2(t, events))
	})

	t.Run("dependent roots fall back to genesis block root on underflow", func(t *testing.T) {
		srv, events, newHeadStateRoot, newHeadRoot := setupHeadV2Service(t, 1)
		require.NoError(t, srv.notifyNewHeadV2Event(t.Context(), 1, newHeadStateRoot, newHeadRoot, version.Gloas, true))
		wantedV2 := &statefeed.HeadV2Data{
			Slot:                      1,
			Block:                     newHeadRoot,
			State:                     newHeadStateRoot,
			EpochTransition:           false,
			CurrentEpochDependentRoot: srv.originBlockRoot,
			NextEpochDependentRoot:    srv.originBlockRoot,
			PayloadStatus:             statefeed.PayloadStatusFull,
			Version:                   version.Gloas,
		}
		require.DeepEqual(t, wantedV2, requireSingleHeadV2(t, events))
	})

	t.Run("pre-gloas always reports a full payload status", func(t *testing.T) {
		srv, events, newHeadStateRoot, newHeadRoot := setupHeadV2Service(t, 1)
		require.NoError(t, srv.notifyNewHeadV2Event(t.Context(), 1, newHeadStateRoot, newHeadRoot, version.Deneb, true))
		wantedV2 := &statefeed.HeadV2Data{
			Slot:                      1,
			Block:                     newHeadRoot,
			State:                     newHeadStateRoot,
			EpochTransition:           false,
			CurrentEpochDependentRoot: srv.originBlockRoot,
			NextEpochDependentRoot:    srv.originBlockRoot,
			PayloadStatus:             statefeed.PayloadStatusFull,
			Version:                   version.Deneb,
		}
		require.DeepEqual(t, wantedV2, requireSingleHeadV2(t, events))
	})

	t.Run("gloas head with delivered payload reports full", func(t *testing.T) {
		srv, events, newHeadStateRoot, newHeadRoot := setupHeadV2Service(t, 1)
		require.NoError(t, srv.notifyNewHeadV2Event(t.Context(), 1, newHeadStateRoot, newHeadRoot, version.Gloas, true))
		require.Equal(t, "full", requireSingleHeadV2(t, events).PayloadStatus.String())
	})

	t.Run("optimistic status is scoped to the event root", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.BellatrixForkEpoch = 0
		cfg.InitializeForkSchedule()
		params.OverrideBeaconConfig(cfg)

		srv, events, newHeadStateRoot, newHeadRoot := setupHeadV2Service(t, 1)
		srv.head = &head{
			root:       [32]byte{0x99},
			slot:       1,
			optimistic: true,
		}
		require.NoError(t, srv.notifyNewHeadV2Event(t.Context(), 1, newHeadStateRoot, newHeadRoot, version.Gloas, true))
		require.Equal(t, false, requireSingleHeadV2(t, events).ExecutionOptimistic)
	})

	t.Run("epoch transition is reported when the head crosses an epoch boundary", func(t *testing.T) {
		newHeadSlot := params.BeaconConfig().SlotsPerEpoch
		srv, events, newHeadStateRoot, newHeadRoot := setupHeadV2Service(t, newHeadSlot)
		require.NoError(t, srv.notifyNewHeadV2Event(t.Context(), newHeadSlot, newHeadStateRoot, newHeadRoot, version.Gloas, true))
		require.Equal(t, true, requireSingleHeadV2(t, events).EpochTransition)
	})

	t.Run("same root and status is announced once", func(t *testing.T) {
		srv, events, newHeadStateRoot, newHeadRoot := setupHeadV2Service(t, 1)
		require.NoError(t, srv.notifyNewHeadV2Event(t.Context(), 1, newHeadStateRoot, newHeadRoot, version.Gloas, true))
		require.NoError(t, srv.notifyNewHeadV2Event(t.Context(), 1, newHeadStateRoot, newHeadRoot, version.Gloas, true))
		requireSingleHeadV2(t, events)
		require.NoError(t, srv.notifyNewHeadV2Event(t.Context(), 1, newHeadStateRoot, newHeadRoot, version.Gloas, false))
		require.Equal(t, "empty", requireSingleHeadV2(t, events).PayloadStatus.String())
	})
}

func TestRetrieveHead_ReadOnly(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)

	oldBlock := util.SaveBlock(t, t.Context(), service.cfg.BeaconDB, util.NewBeaconBlock())
	oldRoot, err := oldBlock.Block().HashTreeRoot()
	require.NoError(t, err)
	service.head = &head{
		root:  oldRoot,
		block: oldBlock,
	}

	newHeadSignedBlock := util.NewBeaconBlock()
	newHeadSignedBlock.Block.Slot = 1
	newHeadBlock := newHeadSignedBlock.Block
	ojc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}

	wsb := util.SaveBlock(t, t.Context(), service.cfg.BeaconDB, newHeadSignedBlock)
	newRoot, err := newHeadBlock.HashTreeRoot()
	require.NoError(t, err)
	state, blkRoot, err := prepareForkchoiceState(ctx, slots.PrevSlot(wsb.Block().Slot()), wsb.Block().ParentRoot(), service.cfg.ForkChoiceStore.CachedHeadRoot(), [32]byte{}, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))

	state, blkRoot, err = prepareForkchoiceState(ctx, wsb.Block().Slot(), newRoot, wsb.Block().ParentRoot(), [32]byte{}, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
	headState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, headState.SetSlot(1))
	require.NoError(t, service.cfg.BeaconDB.SaveStateSummary(t.Context(), &ethpb.StateSummary{Slot: 1, Root: newRoot[:]}))
	require.NoError(t, service.cfg.BeaconDB.SaveState(t.Context(), headState, newRoot))
	require.NoError(t, service.saveHead(t.Context(), newRoot, wsb, headState, false))

	rOnlyState, err := service.HeadStateReadOnly(ctx)
	require.NoError(t, err)

	assert.Equal(t, rOnlyState, service.head.state, "Head is not the same object")
}

func TestSaveOrphanedAtts(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)
	service.genesisTime = time.Now().Add(time.Duration(-10*int64(1)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second)

	// Chain setup
	// 0 -- 1 -- 2 -- 3
	//  \-4
	st, keys := util.DeterministicGenesisState(t, 64)
	blkG, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), 0)
	assert.NoError(t, err)

	util.SaveBlock(t, ctx, service.cfg.BeaconDB, blkG)
	rG, err := blkG.Block.HashTreeRoot()
	require.NoError(t, err)

	blk1, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), 1)
	assert.NoError(t, err)
	blk1.Block.ParentRoot = rG[:]
	r1, err := blk1.Block.HashTreeRoot()
	require.NoError(t, err)

	blk2, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), 2)
	assert.NoError(t, err)
	blk2.Block.ParentRoot = r1[:]
	r2, err := blk2.Block.HashTreeRoot()
	require.NoError(t, err)

	blk3, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), 3)
	assert.NoError(t, err)
	blk3.Block.ParentRoot = r2[:]
	r3, err := blk3.Block.HashTreeRoot()
	require.NoError(t, err)

	blk4 := util.NewBeaconBlock()
	blk4.Block.Slot = 4
	blk4.Block.ParentRoot = rG[:]
	r4, err := blk4.Block.HashTreeRoot()
	require.NoError(t, err)
	ojc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}

	for _, blk := range []*ethpb.SignedBeaconBlock{blkG, blk1, blk2, blk3, blk4} {
		r, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		state, blkRoot, err := prepareForkchoiceState(ctx, blk.Block.Slot, r, bytesutil.ToBytes32(blk.Block.ParentRoot), [32]byte{}, ojc, ofc)
		require.NoError(t, err)
		require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
		util.SaveBlock(t, ctx, beaconDB, blk)
	}

	require.NoError(t, service.saveOrphanedOperations(ctx, r3, r4))
	require.Equal(t, 3, service.cfg.AttPool.AggregatedAttestationCount())
	wantAtts := []ethpb.Att{
		blk3.Block.Body.Attestations[0],
		blk2.Block.Body.Attestations[0],
		blk1.Block.Body.Attestations[0],
	}
	atts := service.cfg.AttPool.AggregatedAttestations()
	sort.Slice(atts, func(i, j int) bool {
		return atts[i].GetData().Slot > atts[j].GetData().Slot
	})
	require.DeepEqual(t, wantAtts, atts)
}

func TestSaveOrphanedAttsElectra(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)
	service.genesisTime = time.Now().Add(time.Duration(-10*int64(1)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second)

	// Chain setup
	// 0 -- 1 -- 2 -- 3
	//  \-4
	st, keys := util.DeterministicGenesisStateElectra(t, 64)
	blkG, err := util.GenerateFullBlockElectra(st, keys, util.DefaultBlockGenConfig(), 1)
	assert.NoError(t, err)

	util.SaveBlock(t, ctx, service.cfg.BeaconDB, blkG)
	rG, err := blkG.Block.HashTreeRoot()
	require.NoError(t, err)

	blk1, err := util.GenerateFullBlockElectra(st, keys, util.DefaultBlockGenConfig(), 2)
	assert.NoError(t, err)
	blk1.Block.ParentRoot = rG[:]
	r1, err := blk1.Block.HashTreeRoot()
	require.NoError(t, err)

	blk2, err := util.GenerateFullBlockElectra(st, keys, util.DefaultBlockGenConfig(), 3)
	assert.NoError(t, err)
	blk2.Block.ParentRoot = r1[:]
	r2, err := blk2.Block.HashTreeRoot()
	require.NoError(t, err)

	blk3, err := util.GenerateFullBlockElectra(st, keys, util.DefaultBlockGenConfig(), 4)
	assert.NoError(t, err)
	blk3.Block.ParentRoot = r2[:]
	r3, err := blk3.Block.HashTreeRoot()
	require.NoError(t, err)

	blk4 := util.NewBeaconBlockElectra()
	blk4.Block.Slot = 4
	blk4.Block.ParentRoot = rG[:]
	r4, err := blk4.Block.HashTreeRoot()
	require.NoError(t, err)
	ojc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}

	for _, blk := range []*ethpb.SignedBeaconBlockElectra{blkG, blk1, blk2, blk3, blk4} {
		r, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		state, blkRoot, err := prepareForkchoiceState(ctx, blk.Block.Slot, r, bytesutil.ToBytes32(blk.Block.ParentRoot), [32]byte{}, ojc, ofc)
		require.NoError(t, err)
		require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
		util.SaveBlock(t, ctx, beaconDB, blk)
	}

	require.NoError(t, service.saveOrphanedOperations(ctx, r3, r4))
	require.Equal(t, 3, len(service.cfg.AttPool.BlockAttestations()))
	wantAtts := []ethpb.Att{
		blk3.Block.Body.Attestations[0],
		blk2.Block.Body.Attestations[0],
		blk1.Block.Body.Attestations[0],
	}
	atts := service.cfg.AttPool.BlockAttestations()
	sort.Slice(atts, func(i, j int) bool {
		return atts[i].GetData().Slot > atts[j].GetData().Slot
	})
	require.DeepEqual(t, wantAtts, atts)
}

func TestSaveOrphanedOps(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.ShardCommitteePeriod = 0
	params.OverrideBeaconConfig(config)

	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)
	service.SetGenesisTime(time.Now().Add(time.Duration(-10*int64(1)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second))

	// Chain setup
	// 0 -- 1 -- 2 -- 3
	//  \-4
	st, keys := util.DeterministicGenesisState(t, 64)
	service.head = &head{state: st}
	blkG, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), 0)
	require.NoError(t, err)

	util.SaveBlock(t, ctx, service.cfg.BeaconDB, blkG)
	rG, err := blkG.Block.HashTreeRoot()
	require.NoError(t, err)

	blk1, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	blk1.Block.ParentRoot = rG[:]
	r1, err := blk1.Block.HashTreeRoot()
	require.NoError(t, err)

	blk2, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), 2)
	require.NoError(t, err)
	blk2.Block.ParentRoot = r1[:]
	r2, err := blk2.Block.HashTreeRoot()
	require.NoError(t, err)

	blkConfig := util.DefaultBlockGenConfig()
	blkConfig.NumBLSChanges = 5
	blkConfig.NumProposerSlashings = 1
	blkConfig.NumAttesterSlashings = 1
	blkConfig.NumVoluntaryExits = 1
	blk3, err := util.GenerateFullBlock(st, keys, blkConfig, 3)
	require.NoError(t, err)
	blk3.Block.ParentRoot = r2[:]
	r3, err := blk3.Block.HashTreeRoot()
	require.NoError(t, err)

	blk4 := util.NewBeaconBlock()
	blk4.Block.Slot = 4
	blk4.Block.ParentRoot = rG[:]
	r4, err := blk4.Block.HashTreeRoot()
	require.NoError(t, err)
	ojc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}

	for _, blk := range []*ethpb.SignedBeaconBlock{blkG, blk1, blk2, blk3, blk4} {
		r, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		state, blkRoot, err := prepareForkchoiceState(ctx, blk.Block.Slot, r, bytesutil.ToBytes32(blk.Block.ParentRoot), [32]byte{}, ojc, ofc)
		require.NoError(t, err)
		require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
		util.SaveBlock(t, ctx, beaconDB, blk)
	}

	require.NoError(t, service.saveOrphanedOperations(ctx, r3, r4))
	require.Equal(t, 3, service.cfg.AttPool.AggregatedAttestationCount())
	wantAtts := []ethpb.Att{
		blk3.Block.Body.Attestations[0],
		blk2.Block.Body.Attestations[0],
		blk1.Block.Body.Attestations[0],
	}
	atts := service.cfg.AttPool.AggregatedAttestations()
	sort.Slice(atts, func(i, j int) bool {
		return atts[i].GetData().Slot > atts[j].GetData().Slot
	})
	require.DeepEqual(t, wantAtts, atts)
	require.Equal(t, 1, len(service.cfg.SlashingPool.PendingProposerSlashings(ctx, st, false)))
	require.Equal(t, 1, len(service.cfg.SlashingPool.PendingAttesterSlashings(ctx, st, false)))
	exits, err := service.cfg.ExitPool.PendingExits()
	require.NoError(t, err)
	require.Equal(t, 1, len(exits))
}

func TestSaveOrphanedAtts_CanFilter(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)
	service.cfg.BLSToExecPool = blstoexec.NewPool()
	service.genesisTime = time.Now().Add(time.Duration(-1*int64(params.BeaconConfig().SlotsPerEpoch+2)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second)

	// Chain setup
	// 0 -- 1 -- 2
	//  \-4
	st, keys := util.DeterministicGenesisStateCapella(t, 64)
	blkConfig := util.DefaultBlockGenConfig()
	blkConfig.NumBLSChanges = 5
	blkG, err := util.GenerateFullBlockCapella(st, keys, blkConfig, 1)
	assert.NoError(t, err)
	util.SaveBlock(t, ctx, service.cfg.BeaconDB, blkG)
	rG, err := blkG.Block.HashTreeRoot()
	require.NoError(t, err)

	blkConfig.NumBLSChanges = 10
	blk1, err := util.GenerateFullBlockCapella(st, keys, blkConfig, 2)
	assert.NoError(t, err)
	blk1.Block.ParentRoot = rG[:]
	r1, err := blk1.Block.HashTreeRoot()
	require.NoError(t, err)

	blkConfig.NumBLSChanges = 15
	blk2, err := util.GenerateFullBlockCapella(st, keys, blkConfig, 3)
	assert.NoError(t, err)
	blk2.Block.ParentRoot = r1[:]
	r2, err := blk2.Block.HashTreeRoot()
	require.NoError(t, err)

	blk4 := util.NewBeaconBlockCapella()
	blkConfig.NumBLSChanges = 0
	blk4.Block.Slot = 4
	blk4.Block.ParentRoot = rG[:]
	r4, err := blk4.Block.HashTreeRoot()
	require.NoError(t, err)
	ojc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}

	for _, blk := range []*ethpb.SignedBeaconBlockCapella{blkG, blk1, blk2, blk4} {
		r, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		state, blkRoot, err := prepareForkchoiceState(ctx, blk.Block.Slot, r, bytesutil.ToBytes32(blk.Block.ParentRoot), [32]byte{}, ojc, ofc)
		require.NoError(t, err)
		require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
		util.SaveBlock(t, ctx, beaconDB, blk)
	}

	require.NoError(t, service.saveOrphanedOperations(ctx, r2, r4))
	require.Equal(t, 1, service.cfg.AttPool.AggregatedAttestationCount())
	pending, err := service.cfg.BLSToExecPool.PendingBLSToExecChanges()
	require.NoError(t, err)
	require.Equal(t, 15, len(pending))
}

func TestSaveOrphanedAtts_DoublyLinkedTrie(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)
	service.genesisTime = time.Now().Add(time.Duration(-10*int64(1)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second)

	// Chain setup
	// 0 -- 1 -- 2 -- 3
	//  \-4
	st, keys := util.DeterministicGenesisState(t, 64)
	blkG, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), 0)
	assert.NoError(t, err)
	util.SaveBlock(t, ctx, service.cfg.BeaconDB, blkG)
	rG, err := blkG.Block.HashTreeRoot()
	require.NoError(t, err)

	blk1, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), 1)
	assert.NoError(t, err)
	blk1.Block.ParentRoot = rG[:]
	r1, err := blk1.Block.HashTreeRoot()
	require.NoError(t, err)

	blk2, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), 2)
	assert.NoError(t, err)
	blk2.Block.ParentRoot = r1[:]
	r2, err := blk2.Block.HashTreeRoot()
	require.NoError(t, err)

	blk3, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), 3)
	assert.NoError(t, err)
	blk3.Block.ParentRoot = r2[:]
	r3, err := blk3.Block.HashTreeRoot()
	require.NoError(t, err)

	blk4 := util.NewBeaconBlock()
	blk4.Block.Slot = 4
	blk4.Block.ParentRoot = rG[:]
	r4, err := blk4.Block.HashTreeRoot()
	require.NoError(t, err)

	ojc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	for _, blk := range []*ethpb.SignedBeaconBlock{blkG, blk1, blk2, blk3, blk4} {
		r, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		state, blkRoot, err := prepareForkchoiceState(ctx, blk.Block.Slot, r, bytesutil.ToBytes32(blk.Block.ParentRoot), [32]byte{}, ojc, ofc)
		require.NoError(t, err)
		require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
		util.SaveBlock(t, ctx, beaconDB, blk)
	}

	require.NoError(t, service.saveOrphanedOperations(ctx, r3, r4))
	require.Equal(t, 3, service.cfg.AttPool.AggregatedAttestationCount())
	wantAtts := []ethpb.Att{
		blk3.Block.Body.Attestations[0],
		blk2.Block.Body.Attestations[0],
		blk1.Block.Body.Attestations[0],
	}
	atts := service.cfg.AttPool.AggregatedAttestations()
	sort.Slice(atts, func(i, j int) bool {
		return atts[i].GetData().Slot > atts[j].GetData().Slot
	})
	require.DeepEqual(t, wantAtts, atts)
}

func TestSaveOrphanedAtts_CanFilter_DoublyLinkedTrie(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)
	service.genesisTime = time.Now().Add(time.Duration(-1*int64(params.BeaconConfig().SlotsPerEpoch+2)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second)

	// Chain setup
	// 0 -- 1 -- 2
	//  \-4
	st, keys := util.DeterministicGenesisState(t, 64)
	blkG, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), 0)
	assert.NoError(t, err)
	util.SaveBlock(t, ctx, service.cfg.BeaconDB, blkG)
	rG, err := blkG.Block.HashTreeRoot()
	require.NoError(t, err)

	blk1, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), 1)
	assert.NoError(t, err)
	blk1.Block.ParentRoot = rG[:]
	r1, err := blk1.Block.HashTreeRoot()
	require.NoError(t, err)

	blk2, err := util.GenerateFullBlock(st, keys, util.DefaultBlockGenConfig(), 2)
	assert.NoError(t, err)
	blk2.Block.ParentRoot = r1[:]
	r2, err := blk2.Block.HashTreeRoot()
	require.NoError(t, err)

	blk4 := util.NewBeaconBlock()
	blk4.Block.Slot = 4
	blk4.Block.ParentRoot = rG[:]
	r4, err := blk4.Block.HashTreeRoot()
	require.NoError(t, err)

	ojc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	for _, blk := range []*ethpb.SignedBeaconBlock{blkG, blk1, blk2, blk4} {
		r, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		state, blkRoot, err := prepareForkchoiceState(ctx, blk.Block.Slot, r, bytesutil.ToBytes32(blk.Block.ParentRoot), [32]byte{}, ojc, ofc)
		require.NoError(t, err)
		require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
		util.SaveBlock(t, ctx, beaconDB, blk)
	}

	require.NoError(t, service.saveOrphanedOperations(ctx, r2, r4))
	require.Equal(t, 0, service.cfg.AttPool.AggregatedAttestationCount())
}

func TestUpdateHead_noSavedChanges(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, beaconDB, fcs := tr.ctx, tr.db, tr.fcs

	ojp := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	st, blkRoot, err := prepareForkchoiceState(ctx, 0, [32]byte{}, [32]byte{}, [32]byte{}, ojp, ojp)
	require.NoError(t, err)
	require.NoError(t, fcs.InsertNode(ctx, st, blkRoot))

	bellatrixBlk := util.SaveBlock(t, ctx, beaconDB, util.NewBeaconBlockBellatrix())
	bellatrixBlkRoot, err := bellatrixBlk.Block().HashTreeRoot()
	require.NoError(t, err)
	fcp := &ethpb.Checkpoint{
		Root:  bellatrixBlkRoot[:],
		Epoch: 0,
	}
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, bellatrixBlkRoot))

	bellatrixState, _ := util.DeterministicGenesisStateBellatrix(t, 2)
	require.NoError(t, beaconDB.SaveState(ctx, bellatrixState, bellatrixBlkRoot))
	service.cfg.StateGen.SaveFinalizedState(bellatrixBlkRoot, bellatrixState)

	headRoot := service.headRoot()
	require.Equal(t, [32]byte{}, headRoot)

	st, blkRoot, err = prepareForkchoiceState(ctx, 0, bellatrixBlkRoot, [32]byte{}, [32]byte{}, fcp, fcp)
	require.NoError(t, err)
	require.NoError(t, fcs.InsertNode(ctx, st, blkRoot))
	fcs.SetBalancesByRooter(func(context.Context, [32]byte) ([]uint64, error) { return []uint64{1, 2}, nil })
	require.NoError(t, fcs.UpdateJustifiedCheckpoint(ctx, &forkchoicetypes.Checkpoint{}))
	newRoot, err := service.cfg.ForkChoiceStore.Head(ctx)
	require.NoError(t, err)
	require.NotEqual(t, headRoot, newRoot)
	require.Equal(t, headRoot, service.headRoot())
}

func Test_notifyHeadEvents_CheckpointHistory(t *testing.T) {
	spe := params.BeaconConfig().SlotsPerEpoch
	history := params.BeaconConfig().SlotsPerHistoricalRoot
	anchor, parent, older, headRoot := [32]byte{'a'}, [32]byte{'p'}, [32]byte{'o'}, [32]byte{'h'}
	tests := []struct {
		name       string
		anchorSlot primitives.Slot
		headSlot   primitives.Slot
		stateSlot  primitives.Slot
		wantPrev   [32]byte
		wantCurr   [32]byte
	}{
		{name: "anchor epoch", anchorSlot: 2 * spe, headSlot: 2*spe + 1, stateSlot: 2*spe + 1, wantPrev: older, wantCurr: parent},
		{name: "following epoch", anchorSlot: 2 * spe, headSlot: 3 * spe, stateSlot: 3 * spe, wantPrev: parent, wantCurr: anchor},
		{name: "skipped checkpoint boundary", anchorSlot: 2*spe - 1, headSlot: 2*spe + 1, stateSlot: 2*spe + 1, wantPrev: older, wantCurr: anchor},
		{name: "historical ring wrapped", anchorSlot: history + 2*spe, headSlot: history + 2*spe + 1, stateSlot: history + 2*spe + 1, wantPrev: older, wantCurr: parent},
		{name: "newer head state", anchorSlot: 2 * spe, headSlot: 2*spe + 1, stateSlot: 3*spe + 1, wantPrev: older, wantCurr: parent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			srv := testServiceWithDB(t)
			srv.SetGenesisTime(time.Now())
			srv.originBlockRoot = anchor
			st, blk, err := prepareForkchoiceState(ctx, tt.anchorSlot, anchor, parent, [32]byte{}, &ethpb.Checkpoint{}, &ethpb.Checkpoint{})
			require.NoError(t, err)
			require.NoError(t, srv.cfg.ForkChoiceStore.InsertNode(ctx, st, blk))
			st, blk, err = prepareForkchoiceState(ctx, tt.headSlot, headRoot, anchor, [32]byte{}, &ethpb.Checkpoint{}, &ethpb.Checkpoint{})
			require.NoError(t, err)
			require.NoError(t, srv.cfg.ForkChoiceStore.InsertNode(ctx, st, blk))
			currentHeadRoot := headRoot
			if tt.stateSlot > tt.headSlot {
				currentHeadRoot = [32]byte{'n'}
				st, blk, err = prepareForkchoiceState(ctx, tt.stateSlot, currentHeadRoot, headRoot, [32]byte{}, &ethpb.Checkpoint{}, &ethpb.Checkpoint{})
				require.NoError(t, err)
				require.NoError(t, srv.cfg.ForkChoiceStore.InsertNode(ctx, st, blk))
			}
			selectedRoot, err := srv.cfg.ForkChoiceStore.Head(ctx)
			require.NoError(t, err)
			require.Equal(t, currentHeadRoot, selectedRoot)

			headState, err := util.NewBeaconState()
			require.NoError(t, err)
			require.NoError(t, headState.SetSlot(tt.stateSlot))
			currentStart, err := slots.EpochStart(slots.ToEpoch(tt.headSlot))
			require.NoError(t, err)
			require.NoError(t, headState.UpdateBlockRootAtIndex(uint64((currentStart-spe-1)%history), tt.wantPrev))
			require.NoError(t, headState.UpdateBlockRootAtIndex(uint64((currentStart-1)%history), tt.wantCurr))
			srv.head = &head{root: currentHeadRoot, slot: tt.stateSlot, state: headState}

			events := make(chan *feed.Event, 3)
			sub := srv.cfg.StateNotifier.StateFeed().Subscribe(events)
			defer sub.Unsubscribe()
			headStateRoot := [32]byte{'s'}
			require.NoError(t, srv.notifyNewHeadEvent(ctx, tt.headSlot, headStateRoot, headRoot))
			event := <-events
			require.Equal(t, feed.EventType(statefeed.NewHead), event.Type)
			legacy, ok := event.Data.(*statefeed.HeadData)
			require.Equal(t, true, ok)
			require.Equal(t, tt.wantPrev, legacy.PreviousDutyDependentRoot)
			require.Equal(t, tt.wantCurr, legacy.CurrentDutyDependentRoot)

			for _, full := range []bool{false, true} {
				require.NoError(t, srv.notifyNewHeadV2Event(ctx, tt.headSlot, headStateRoot, headRoot, version.Gloas, full))
				event = <-events
				require.Equal(t, feed.EventType(statefeed.NewHeadV2), event.Type)
				v2, ok := event.Data.(*statefeed.HeadV2Data)
				require.Equal(t, true, ok)
				require.Equal(t, tt.wantPrev, v2.CurrentEpochDependentRoot)
				require.Equal(t, tt.wantCurr, v2.NextEpochDependentRoot)
				wantStatus := statefeed.PayloadStatusEmpty
				if full {
					wantStatus = statefeed.PayloadStatusFull
				}
				require.Equal(t, wantStatus, v2.PayloadStatus)
			}
		})
	}
}

func Test_headEventDependentRoots(t *testing.T) {
	spe := params.BeaconConfig().SlotsPerEpoch
	t.Run("forkchoice roots need no head state", func(t *testing.T) {
		ctx := t.Context()
		srv := testServiceWithDB(t)
		root := [32]byte{'r'}
		st, blk, err := prepareForkchoiceState(ctx, spe-1, root, [32]byte{'p'}, [32]byte{}, &ethpb.Checkpoint{}, &ethpb.Checkpoint{})
		require.NoError(t, err)
		require.NoError(t, srv.cfg.ForkChoiceStore.InsertNode(ctx, st, blk))
		srv.cfg.StateGen = nil
		prev, curr, err := srv.headEventDependentRoots(ctx, 2*spe)
		require.NoError(t, err)
		require.Equal(t, root, prev)
		require.Equal(t, root, curr)
	})
	t.Run("genesis needs no forkchoice history or head state", func(t *testing.T) {
		srv := testServiceWithDB(t)
		srv.originBlockRoot = [32]byte{'g'}
		srv.cfg.StateGen = nil
		for _, headSlot := range []primitives.Slot{0, spe - 1} {
			prev, curr, err := srv.headEventDependentRoots(t.Context(), headSlot)
			require.NoError(t, err)
			require.Equal(t, srv.originBlockRoot, prev)
			require.Equal(t, srv.originBlockRoot, curr)
		}
	})
	t.Run("missing history propagates state bounds error", func(t *testing.T) {
		ctx := t.Context()
		srv := testServiceWithDB(t)
		anchor := [32]byte{'a'}
		st, blk, err := prepareForkchoiceState(ctx, 2*spe, anchor, [32]byte{'p'}, [32]byte{}, &ethpb.Checkpoint{}, &ethpb.Checkpoint{})
		require.NoError(t, err)
		require.NoError(t, srv.cfg.ForkChoiceStore.InsertNode(ctx, st, blk))
		headState, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, headState.SetSlot(params.BeaconConfig().SlotsPerHistoricalRoot+2*spe))
		srv.head = &head{root: anchor, state: headState}
		_, _, err = srv.headEventDependentRoots(ctx, 2*spe+1)
		require.ErrorContains(t, "out of bounds", err)
	})
}
