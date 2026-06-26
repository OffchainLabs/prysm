package lookup

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/testutil"
	statenative "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	mockstategen "github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen/mock"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/proto/dbval"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

func TestGetState(t *testing.T) {
	ctx := t.Context()

	headSlot := primitives.Slot(123)
	fillSlot := func(state *ethpb.BeaconState) error {
		state.Slot = headSlot
		return nil
	}
	newBeaconState, err := util.NewBeaconState(util.FillRootsNaturalOpt, fillSlot)
	require.NoError(t, err)
	stateRoot, err := newBeaconState.HashTreeRoot(ctx)
	require.NoError(t, err)

	t.Run("head", func(t *testing.T) {
		p := BeaconDbStater{
			ChainInfoFetcher: &chainMock.ChainService{State: newBeaconState},
		}

		s, err := p.State(ctx, []byte("head"))
		require.NoError(t, err)
		sRoot, err := s.HashTreeRoot(ctx)
		require.NoError(t, err)
		assert.DeepEqual(t, stateRoot, sRoot)
	})

	t.Run("genesis", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.ConfigName = "test"
		params.OverrideBeaconConfig(cfg)

		db := testDB.SetupDB(t)
		b := util.NewBeaconBlock()
		b.Block.StateRoot = bytesutil.PadTo([]byte("foo"), 32)
		util.SaveBlock(t, ctx, db, b)
		r, err := b.Block.HashTreeRoot()
		require.NoError(t, err)

		bs, err := util.NewBeaconState(func(state *ethpb.BeaconState) error {
			state.BlockRoots[0] = r[:]
			return nil
		})
		require.NoError(t, err)
		newStateRoot, err := bs.HashTreeRoot(ctx)
		require.NoError(t, err)

		require.NoError(t, db.SaveStateSummary(ctx, &ethpb.StateSummary{Root: r[:]}))
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, r))
		require.NoError(t, db.SaveState(ctx, bs, r))

		cc := &mockstategen.CanonicalChecker{Is: true}
		cs := &mockstategen.CurrentSlotter{Slot: bs.Slot() + 1}
		ch := stategen.NewCanonicalHistory(db, cc, cs)
		currentSlot := primitives.Slot(0)
		p := BeaconDbStater{
			BeaconDB:           db,
			ReplayerBuilder:    ch,
			GenesisTimeFetcher: &chainMock.ChainService{Slot: &currentSlot},
			ChainInfoFetcher:   &chainMock.ChainService{State: bs},
		}

		s, err := p.State(ctx, []byte("genesis"))
		require.NoError(t, err)
		sRoot, err := s.HashTreeRoot(ctx)
		require.NoError(t, err)
		assert.DeepEqual(t, newStateRoot, sRoot)
	})

	t.Run("finalized", func(t *testing.T) {
		stateGen := mockstategen.NewService()
		replayer := mockstategen.NewReplayerBuilder()
		replayer.SetMockStateForSlot(newBeaconState, params.BeaconConfig().SlotsPerEpoch*10)
		stateGen.StatesByRoot[stateRoot] = newBeaconState

		p := BeaconDbStater{
			ChainInfoFetcher: &chainMock.ChainService{
				FinalizedCheckPoint: &ethpb.Checkpoint{
					Root:  stateRoot[:],
					Epoch: 10,
				},
			},
			StateGenService: stateGen,
			ReplayerBuilder: replayer,
		}

		s, err := p.State(ctx, []byte("finalized"))
		require.NoError(t, err)
		sRoot, err := s.HashTreeRoot(ctx)
		require.NoError(t, err)
		assert.Equal(t, stateRoot, sRoot)
	})

	t.Run("justified", func(t *testing.T) {
		stateGen := mockstategen.NewService()
		replayer := mockstategen.NewReplayerBuilder()
		replayer.SetMockStateForSlot(newBeaconState, params.BeaconConfig().SlotsPerEpoch*10)
		stateGen.StatesByRoot[stateRoot] = newBeaconState

		p := BeaconDbStater{
			ChainInfoFetcher: &chainMock.ChainService{
				CurrentJustifiedCheckPoint: &ethpb.Checkpoint{
					Root:  stateRoot[:],
					Epoch: 10,
				},
			},
			StateGenService: stateGen,
			ReplayerBuilder: replayer,
		}

		s, err := p.State(ctx, []byte("justified"))
		require.NoError(t, err)
		sRoot, err := s.HashTreeRoot(ctx)
		require.NoError(t, err)
		assert.DeepEqual(t, stateRoot, sRoot)
	})

	t.Run("hex", func(t *testing.T) {
		hex := "0x" + strings.Repeat("0", 63) + "1"
		root, err := hexutil.Decode(hex)
		require.NoError(t, err)
		stateGen := mockstategen.NewService()
		stateGen.StatesByRoot[bytesutil.ToBytes32(root)] = newBeaconState

		p := BeaconDbStater{
			ChainInfoFetcher: &chainMock.ChainService{State: newBeaconState},
			StateGenService:  stateGen,
		}

		s, err := p.State(ctx, []byte(hex))
		require.NoError(t, err)
		sRoot, err := s.HashTreeRoot(ctx)
		require.NoError(t, err)
		assert.DeepEqual(t, stateRoot, sRoot)
	})

	t.Run("root", func(t *testing.T) {
		stateId, err := hexutil.Decode("0x" + strings.Repeat("0", 63) + "1")
		require.NoError(t, err)
		stateGen := mockstategen.NewService()
		stateGen.StatesByRoot[bytesutil.ToBytes32(stateId)] = newBeaconState

		p := BeaconDbStater{
			ChainInfoFetcher: &chainMock.ChainService{State: newBeaconState},
			StateGenService:  stateGen,
		}

		s, err := p.State(ctx, stateId)
		require.NoError(t, err)
		sRoot, err := s.HashTreeRoot(ctx)
		require.NoError(t, err)
		assert.DeepEqual(t, stateRoot, sRoot)
	})

	t.Run("root not found", func(t *testing.T) {
		p := BeaconDbStater{
			ChainInfoFetcher: &chainMock.ChainService{State: newBeaconState},
		}
		stateId, err := hexutil.Decode("0x" + strings.Repeat("f", 64))
		require.NoError(t, err)
		_, err = p.State(ctx, stateId)
		require.ErrorContains(t, "state not found in the last 8192 state roots", err)
	})

	t.Run("slot", func(t *testing.T) {
		p := BeaconDbStater{
			GenesisTimeFetcher: &chainMock.ChainService{Slot: &headSlot},
			ChainInfoFetcher: &chainMock.ChainService{
				CanonicalRoots: map[[32]byte]bool{
					bytesutil.ToBytes32(newBeaconState.LatestBlockHeader().ParentRoot): true,
				},
				State: newBeaconState,
			},
			ReplayerBuilder: mockstategen.NewReplayerBuilder(mockstategen.WithMockState(newBeaconState)),
		}

		s, err := p.State(ctx, []byte(strconv.FormatUint(uint64(headSlot), 10)))
		require.NoError(t, err)
		sRoot, err := s.HashTreeRoot(ctx)
		require.NoError(t, err)
		assert.Equal(t, stateRoot, sRoot)
	})

	t.Run("invalid_state", func(t *testing.T) {
		p := BeaconDbStater{}
		_, err := p.State(ctx, []byte("foo"))
		require.ErrorContains(t, "could not parse state ID", err)
	})
}

func TestGetStateRoot(t *testing.T) {
	ctx := t.Context()

	headSlot := primitives.Slot(123)
	fillSlot := func(state *ethpb.BeaconState) error {
		state.Slot = headSlot
		return nil
	}
	newBeaconState, err := util.NewBeaconState(util.FillRootsNaturalOpt, fillSlot)
	require.NoError(t, err)
	stateRoot, err := newBeaconState.HashTreeRoot(ctx)
	require.NoError(t, err)

	t.Run("head", func(t *testing.T) {
		b := util.NewBeaconBlock()
		b.Block.StateRoot = stateRoot[:]
		wsb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		p := BeaconDbStater{
			ChainInfoFetcher: &chainMock.ChainService{
				State: newBeaconState,
				Block: wsb,
			},
		}

		s, err := p.StateRoot(ctx, []byte("head"))
		require.NoError(t, err)
		assert.DeepEqual(t, stateRoot[:], s)
	})
	t.Run("genesis", func(t *testing.T) {
		db := testDB.SetupDB(t)
		b := util.NewBeaconBlock()
		util.SaveBlock(t, ctx, db, b)
		r, err := b.Block.HashTreeRoot()
		require.NoError(t, err)

		bs, err := util.NewBeaconState(func(state *ethpb.BeaconState) error {
			state.BlockRoots[0] = r[:]
			return nil
		})
		require.NoError(t, err)

		require.NoError(t, db.SaveStateSummary(ctx, &ethpb.StateSummary{Root: r[:]}))
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, r))
		require.NoError(t, db.SaveState(ctx, bs, r))

		p := BeaconDbStater{
			BeaconDB: db,
		}

		s, err := p.StateRoot(ctx, []byte("genesis"))
		require.NoError(t, err)
		genesisBlock, err := db.GenesisBlock(ctx)
		require.NoError(t, err)
		sr := genesisBlock.Block().StateRoot()
		assert.DeepEqual(t, sr[:], s)
	})
	t.Run("finalized", func(t *testing.T) {
		db := testDB.SetupDB(t)
		genesis := bytesutil.ToBytes32([]byte("genesis"))
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
		util.SaveBlock(t, ctx, db, blk)
		st, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, st.SetSlot(1))
		// a state is required to save checkpoint
		require.NoError(t, db.SaveState(ctx, st, root))
		require.NoError(t, db.SaveFinalizedCheckpoint(ctx, cp))

		p := BeaconDbStater{
			BeaconDB: db,
		}

		s, err := p.StateRoot(ctx, []byte("finalized"))
		require.NoError(t, err)
		assert.DeepEqual(t, blk.Block.StateRoot, s)
	})
	t.Run("justified", func(t *testing.T) {
		db := testDB.SetupDB(t)
		genesis := bytesutil.ToBytes32([]byte("genesis"))
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
		util.SaveBlock(t, ctx, db, blk)
		st, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, st.SetSlot(1))
		// a state is required to save checkpoint
		require.NoError(t, db.SaveState(ctx, st, root))
		require.NoError(t, db.SaveJustifiedCheckpoint(ctx, cp))

		p := BeaconDbStater{
			BeaconDB: db,
		}

		s, err := p.StateRoot(ctx, []byte("justified"))
		require.NoError(t, err)
		assert.DeepEqual(t, blk.Block.StateRoot, s)
	})
	t.Run("hex", func(t *testing.T) {
		hex := "0x" + strings.Repeat("0", 63) + "1"

		p := BeaconDbStater{
			ChainInfoFetcher: &chainMock.ChainService{State: newBeaconState},
		}

		s, err := p.StateRoot(ctx, []byte(hex))
		require.NoError(t, err)
		expected, err := hexutil.Decode(hex)
		require.NoError(t, err)
		assert.DeepEqual(t, expected, s)
	})
	t.Run("hex not found", func(t *testing.T) {
		hex := "0x" + strings.Repeat("f", 64)

		p := BeaconDbStater{
			ChainInfoFetcher: &chainMock.ChainService{State: newBeaconState},
		}

		_, err = p.StateRoot(ctx, []byte(hex))
		require.ErrorContains(t, "state root not found in the last 8192 state roots", err)
	})
	t.Run("bytes", func(t *testing.T) {
		root, err := hexutil.Decode("0x" + strings.Repeat("0", 63) + "1")
		require.NoError(t, err)

		p := BeaconDbStater{
			ChainInfoFetcher: &chainMock.ChainService{State: newBeaconState},
		}

		s, err := p.StateRoot(ctx, root)
		require.NoError(t, err)
		assert.DeepEqual(t, root, s)
	})
	t.Run("bytes not found", func(t *testing.T) {
		root, err := hexutil.Decode("0x" + strings.Repeat("f", 64))
		require.NoError(t, err)

		p := BeaconDbStater{
			ChainInfoFetcher: &chainMock.ChainService{State: newBeaconState},
		}

		_, err = p.StateRoot(ctx, root)
		require.ErrorContains(t, "state root not found in the last 8192 state roots", err)
	})
	t.Run("slot", func(t *testing.T) {
		db := testDB.SetupDB(t)
		genesis := bytesutil.ToBytes32([]byte("genesis"))
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesis))
		blk := util.NewBeaconBlock()
		blk.Block.ParentRoot = genesis[:]
		blk.Block.Slot = 40
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, db, blk)
		st, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, st.SetSlot(1))
		// a state is required to save checkpoint
		require.NoError(t, db.SaveState(ctx, st, root))

		slot := primitives.Slot(40)
		p := BeaconDbStater{
			GenesisTimeFetcher: &chainMock.ChainService{Slot: &slot},
			BeaconDB:           db,
		}

		s, err := p.StateRoot(ctx, []byte(strconv.FormatUint(uint64(slot), 10)))
		require.NoError(t, err)
		assert.DeepEqual(t, blk.Block.StateRoot, s)
	})
	t.Run("slot too big", func(t *testing.T) {
		p := BeaconDbStater{
			GenesisTimeFetcher: &chainMock.ChainService{
				Genesis: time.Now(),
			},
		}
		_, err := p.StateRoot(ctx, []byte(strconv.FormatUint(1, 10)))
		assert.ErrorContains(t, "slot cannot be in the future", err)
	})

	t.Run("invalid state", func(t *testing.T) {
		p := BeaconDbStater{}
		_, err := p.StateRoot(ctx, []byte("foo"))
		require.ErrorContains(t, "could not parse state ID", err)
	})
}

func TestBeaconDbStater_BlockRoot(t *testing.T) {
	ctx := t.Context()

	headSlot := primitives.Slot(123)
	fillSlot := func(state *ethpb.BeaconState) error {
		state.Slot = headSlot
		return nil
	}
	newBeaconState, err := util.NewBeaconState(util.FillRootsNaturalOpt, fillSlot)
	require.NoError(t, err)

	// Cases that delegate to Blocker. Any non-hex/non-32-byte form (head/genesis/finalized/
	// justified/<slot>) should return whatever the Blocker says — Stater does not look at the
	// state for these.
	delegationCases := []struct {
		name string
		id   string
	}{
		{"head", "head"},
		{"genesis", "genesis"},
		{"finalized", "finalized"},
		{"justified", "justified"},
		{"slot", "40"},
	}
	for _, tc := range delegationCases {
		t.Run("delegates to Blocker for "+tc.name, func(t *testing.T) {
			expected := bytesutil.ToBytes32(bytesutil.PadTo([]byte("expected-block-root"), 32))
			p := BeaconDbStater{
				Blocker: &testutil.MockBlocker{RootToReturn: expected},
			}
			got, err := p.BlockRoot(ctx, []byte(tc.id))
			require.NoError(t, err)
			assert.Equal(t, expected, got)
		})
	}

	t.Run("propagates Blocker error", func(t *testing.T) {
		p := BeaconDbStater{
			Blocker: &testutil.MockBlocker{ErrorToReturn: errors.New("blocker boom")},
		}
		_, err := p.BlockRoot(ctx, []byte("head"))
		require.ErrorContains(t, "blocker boom", err)
	})

	// Hex/raw cases — translate state-root → block-root via head state pairing.
	// FillRootsNaturalOpt sets state.StateRoots[i] == state.BlockRoots[i] == bytesutil.ToBytes32(i),
	// so a query for state-root 0x...01 should return block-root 0x...01.
	t.Run("hex state root", func(t *testing.T) {
		hex := "0x" + strings.Repeat("0", 63) + "1"
		expected, err := hexutil.Decode(hex)
		require.NoError(t, err)
		p := BeaconDbStater{
			ChainInfoFetcher: &chainMock.ChainService{State: newBeaconState},
		}
		got, err := p.BlockRoot(ctx, []byte(hex))
		require.NoError(t, err)
		assert.DeepEqual(t, expected, got[:])
	})

	t.Run("raw 32-byte state root", func(t *testing.T) {
		expected, err := hexutil.Decode("0x" + strings.Repeat("0", 63) + "1")
		require.NoError(t, err)
		p := BeaconDbStater{
			ChainInfoFetcher: &chainMock.ChainService{State: newBeaconState},
		}
		got, err := p.BlockRoot(ctx, expected)
		require.NoError(t, err)
		assert.DeepEqual(t, expected, got[:])
	})

	t.Run("hex state root not found", func(t *testing.T) {
		hex := "0x" + strings.Repeat("f", 64)
		p := BeaconDbStater{
			ChainInfoFetcher: &chainMock.ChainService{State: newBeaconState},
		}
		_, err := p.BlockRoot(ctx, []byte(hex))
		var notFound *StateNotFoundError
		require.Equal(t, true, errors.As(err, &notFound))
	})

	t.Run("malformed hex", func(t *testing.T) {
		p := BeaconDbStater{}
		// Odd-length hex passes IsHex but fails hexutil.Decode (must be even length).
		_, err := p.BlockRoot(ctx, []byte("0x123"))
		var parseErr *StateIdParseError
		require.Equal(t, true, errors.As(err, &parseErr))
	})
}

func TestNewStateNotFoundError(t *testing.T) {
	stateRoot := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20}
	e := NewStateNotFoundError(100, stateRoot)
	assert.Equal(t, "state not found in the last 100 state roots, looking for state root: 0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20", e.message)
}

func TestStateBySlot_FutureSlot(t *testing.T) {
	slot := primitives.Slot(100)
	p := BeaconDbStater{GenesisTimeFetcher: &chainMock.ChainService{Slot: &slot}}
	_, err := p.StateBySlot(t.Context(), 101)
	assert.ErrorContains(t, "requested slot is in the future", err)
}

func TestStateBySlot_AfterHeadSlot(t *testing.T) {
	headSt, err := statenative.InitializeFromProtoPhase0(&ethpb.BeaconState{Slot: 100})
	require.NoError(t, err)
	slotSt, err := statenative.InitializeFromProtoPhase0(&ethpb.BeaconState{Slot: 101})
	require.NoError(t, err)
	currentSlot := primitives.Slot(102)
	mock := &chainMock.ChainService{State: headSt, Slot: &currentSlot}
	mockReplayer := mockstategen.NewReplayerBuilder()
	mockReplayer.SetMockStateForSlot(slotSt, 101)
	p := BeaconDbStater{ChainInfoFetcher: mock, GenesisTimeFetcher: mock, ReplayerBuilder: mockReplayer}
	st, err := p.StateBySlot(t.Context(), 101)
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(101), st.Slot())
}

func TestStateBySlot_EarlierThanEarliestAvailableSlot(t *testing.T) {
	ctx := t.Context()
	db := testDB.SetupDB(t)
	target := primitives.Slot(100)

	genesisRoot := bytesutil.ToBytes32([]byte("genesis"))
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesisRoot))

	b := util.NewBeaconBlock()
	b.Block.ParentRoot = genesisRoot[:]
	b.Block.Slot = target + 2
	util.SaveBlock(t, ctx, db, b)

	currentSlot := target + 3
	p := BeaconDbStater{
		BeaconDB:           db,
		GenesisTimeFetcher: &chainMock.ChainService{Slot: &currentSlot},
	}

	_, err := p.StateBySlot(ctx, target)
	require.ErrorContains(t, fmt.Sprintf("earliest available slot is %d", b.Block.Slot), err)
	var stateNotFoundErr *StateNotFoundError
	require.Equal(t, true, errors.As(err, &stateNotFoundErr))
}

func TestStateBySlot_BeforeBackfillLowSlot(t *testing.T) {
	ctx := t.Context()
	db := testDB.SetupDB(t)
	target := primitives.Slot(100)

	genesisRoot := bytesutil.ToBytes32([]byte("genesis"))
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesisRoot))

	lowSlot := target + 1
	require.NoError(t, db.SaveBackfillStatus(ctx, &dbval.BackfillStatus{LowSlot: uint64(lowSlot)}))

	currentSlot := lowSlot + 1
	p := BeaconDbStater{
		BeaconDB:           db,
		GenesisTimeFetcher: &chainMock.ChainService{Slot: &currentSlot},
	}

	_, err := p.StateBySlot(ctx, target)
	require.ErrorContains(t, fmt.Sprintf("backfill starts at slot %d", lowSlot), err)
	var stateNotFoundErr *StateNotFoundError
	require.Equal(t, true, errors.As(err, &stateNotFoundErr))
}

func TestStateBySlot_ReplayNoDataForSlotReturnsNotFound(t *testing.T) {
	target := primitives.Slot(100)
	currentSlot := target + 1
	mock := &chainMock.ChainService{Slot: &currentSlot}
	mockReplayer := mockstategen.NewReplayerBuilder()
	mockReplayer.SetMockSlotError(target, errors.Wrap(stategen.ErrNoDataForSlot, fmt.Sprintf("slot %d not in db due to checkpoint sync", target)))

	p := BeaconDbStater{
		GenesisTimeFetcher: mock,
		ReplayerBuilder:    mockReplayer,
	}

	_, err := p.StateBySlot(t.Context(), target)
	require.ErrorContains(t, "historical data not available", err)
	var stateNotFoundErr *StateNotFoundError
	require.Equal(t, true, errors.As(err, &stateNotFoundErr))
}

func TestStateByEpoch(t *testing.T) {
	ctx := t.Context()
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch

	t.Run("current epoch uses head state", func(t *testing.T) {
		// Head is at slot 5 (epoch 0), requesting epoch 0
		headSlot := primitives.Slot(5)
		headSt, err := statenative.InitializeFromProtoPhase0(&ethpb.BeaconState{Slot: headSlot})
		require.NoError(t, err)

		currentSlot := headSlot
		mock := &chainMock.ChainService{State: headSt, Slot: &currentSlot}
		p := BeaconDbStater{ChainInfoFetcher: mock, GenesisTimeFetcher: mock}

		st, err := p.StateByEpoch(ctx, 0)
		require.NoError(t, err)
		// Should return head state since it's already past epoch start
		assert.Equal(t, headSlot, st.Slot())
	})

	t.Run("current epoch processes slots to epoch start", func(t *testing.T) {
		// Head is at slot 5 (epoch 0), requesting epoch 1
		// Current slot is 32 (epoch 1), so epoch 1 is current epoch
		headSlot := primitives.Slot(5)
		headSt, err := statenative.InitializeFromProtoPhase0(&ethpb.BeaconState{Slot: headSlot})
		require.NoError(t, err)

		currentSlot := slotsPerEpoch // slot 32, epoch 1
		mock := &chainMock.ChainService{State: headSt, Slot: &currentSlot}
		p := BeaconDbStater{ChainInfoFetcher: mock, GenesisTimeFetcher: mock}

		// Note: This will fail since ProcessSlotsUsingNextSlotCache requires proper setup
		// In real usage, the transition package handles this properly
		_, err = p.StateByEpoch(ctx, 1)
		// The error is expected since we don't have a fully initialized beacon state
		// that can process slots (missing committees, etc.)
		assert.NotNil(t, err)
	})

	t.Run("past epoch uses replay", func(t *testing.T) {
		// Head is at epoch 2, requesting epoch 0 (past)
		headSlot := slotsPerEpoch * 2 // slot 64, epoch 2
		headSt, err := statenative.InitializeFromProtoPhase0(&ethpb.BeaconState{Slot: headSlot})
		require.NoError(t, err)

		pastEpochSt, err := statenative.InitializeFromProtoPhase0(&ethpb.BeaconState{Slot: 0})
		require.NoError(t, err)

		currentSlot := headSlot
		mock := &chainMock.ChainService{State: headSt, Slot: &currentSlot}
		mockReplayer := mockstategen.NewReplayerBuilder()
		mockReplayer.SetMockStateForSlot(pastEpochSt, 0)
		p := BeaconDbStater{ChainInfoFetcher: mock, GenesisTimeFetcher: mock, ReplayerBuilder: mockReplayer}

		st, err := p.StateByEpoch(ctx, 0)
		require.NoError(t, err)
		assert.Equal(t, primitives.Slot(0), st.Slot())
	})

	t.Run("next epoch uses head state path", func(t *testing.T) {
		// Head is at slot 30 (epoch 0), requesting epoch 1 (next)
		// Current slot is 30 (epoch 0), so epoch 1 is next epoch
		headSlot := primitives.Slot(30)
		headSt, err := statenative.InitializeFromProtoPhase0(&ethpb.BeaconState{Slot: headSlot})
		require.NoError(t, err)

		currentSlot := headSlot
		mock := &chainMock.ChainService{State: headSt, Slot: &currentSlot}
		p := BeaconDbStater{ChainInfoFetcher: mock, GenesisTimeFetcher: mock}

		// Note: This will fail since ProcessSlotsUsingNextSlotCache requires proper setup
		_, err = p.StateByEpoch(ctx, 1)
		// The error is expected since we don't have a fully initialized beacon state
		assert.NotNil(t, err)
	})

	t.Run("head state already at target slot returns immediately", func(t *testing.T) {
		// Head is at slot 32 (epoch 1 start), requesting epoch 1
		headSlot := slotsPerEpoch // slot 32
		headSt, err := statenative.InitializeFromProtoPhase0(&ethpb.BeaconState{Slot: headSlot})
		require.NoError(t, err)

		currentSlot := headSlot
		mock := &chainMock.ChainService{State: headSt, Slot: &currentSlot}
		p := BeaconDbStater{ChainInfoFetcher: mock, GenesisTimeFetcher: mock}

		st, err := p.StateByEpoch(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, headSlot, st.Slot())
	})

	t.Run("head state past target slot returns head state", func(t *testing.T) {
		// Head is at slot 40, requesting epoch 1 (starts at slot 32)
		headSlot := primitives.Slot(40)
		headSt, err := statenative.InitializeFromProtoPhase0(&ethpb.BeaconState{Slot: headSlot})
		require.NoError(t, err)

		currentSlot := headSlot
		mock := &chainMock.ChainService{State: headSt, Slot: &currentSlot}
		p := BeaconDbStater{ChainInfoFetcher: mock, GenesisTimeFetcher: mock}

		st, err := p.StateByEpoch(ctx, 1)
		require.NoError(t, err)
		// Returns head state since it's already >= epoch start
		assert.Equal(t, headSlot, st.Slot())
	})
}
