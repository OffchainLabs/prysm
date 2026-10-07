package stategen

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/kv"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

type envelopeCountingHistory struct {
	*mockHistory
	envelopeCalls int
}

func (h *envelopeCountingHistory) ExecutionPayloadEnvelope(_ context.Context, _ [32]byte) (*ethpb.SignedBlindedExecutionPayloadEnvelope, error) {
	h.envelopeCalls++
	return nil, nil
}

func headerFromBlock(b interfaces.ReadOnlySignedBeaconBlock) (*ethpb.BeaconBlockHeader, error) {
	bodyRoot, err := b.Block().Body().HashTreeRoot()
	if err != nil {
		return nil, err
	}
	stateRoot := b.Block().StateRoot()
	parentRoot := b.Block().ParentRoot()
	return &ethpb.BeaconBlockHeader{
		Slot:          b.Block().Slot(),
		StateRoot:     stateRoot[:],
		ProposerIndex: b.Block().ProposerIndex(),
		BodyRoot:      bodyRoot[:],
		ParentRoot:    parentRoot[:],
	}, nil
}

func TestReplayBlocks_ZeroDiff(t *testing.T) {
	logHook := logTest.NewGlobal()
	ctx := t.Context()
	specs := []mockHistorySpec{{slot: 0}}
	hist := newMockHistory(t, specs, 0)
	ch := NewCanonicalHistory(hist, hist, hist)
	_, err := ch.ReplayerForSlot(0).ReplayBlocks(ctx)
	require.NoError(t, err)
	require.LogsDoNotContain(t, logHook, "Replaying canonical blocks from most recent state")
}

func TestReplayBlocks_MissingHistoricalStateDiff(t *testing.T) {
	oldFlags := *flags.Get()
	t.Cleanup(func() { flags.Init(&oldFlags) })
	setStateDiffExponents() // Full snapshots at 64-slot intervals, diffs every 32 slots.
	beaconDB := testDB.SetupDB(t)
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	reset := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	t.Cleanup(reset)

	hist := newMockHistory(t, []mockHistorySpec{
		{slot: 32, canonicalBlock: true},
		{slot: 64, canonicalBlock: true},
		{slot: 96, canonicalBlock: true},
	}, 97)
	ctx := t.Context()
	for _, slot := range []primitives.Slot{0, 32, 64, 96} {
		root := hist.slotMap[slot]
		require.NoError(t, beaconDB.SaveBlock(ctx, hist.blocks[root]))
		if slot == 32 {
			continue // Simulate a hole in historical diffs, with newer history intact.
		}
		st := hist.hiddenStates[root]
		if slot == 0 {
			st = hist.states[root]
		}
		require.NoError(t, beaconDB.SaveState(ctx, st, root))
	}

	_, err := beaconDB.StateOrError(ctx, hist.slotMap[32])
	require.ErrorIs(t, err, db.ErrNotFoundState)
	require.ErrorContains(t, "state diff not found", err)

	ch := NewCanonicalHistory(beaconDB, hist, hist)
	for _, slot := range []primitives.Slot{32, 96} {
		got, err := ch.ReplayerForSlot(slot).ReplayBlocks(ctx)
		require.NoError(t, err)
		wantRoot, err := hist.hiddenStates[hist.slotMap[slot]].HashTreeRoot(ctx)
		require.NoError(t, err)
		gotRoot, err := got.HashTreeRoot(ctx)
		require.NoError(t, err)
		require.Equal(t, wantRoot, gotRoot)
	}
}

func TestReplayBlocks(t *testing.T) {
	ctx := t.Context()
	var zero, one, two, three, four, five primitives.Slot = 50, 51, 150, 151, 152, 200
	specs := []mockHistorySpec{
		{slot: zero},
		{slot: one, savedState: true},
		{slot: two},
		{slot: three},
		{slot: four},
		{slot: five, canonicalBlock: true},
	}

	hist := newMockHistory(t, specs, five+1)
	ch := NewCanonicalHistory(hist, hist, hist)
	st, err := ch.ReplayerForSlot(five).ReplayBlocks(ctx)
	require.NoError(t, err)
	expected := hist.hiddenStates[hist.slotMap[five]]
	expectedHTR, err := expected.HashTreeRoot(ctx)
	require.NoError(t, err)
	actualHTR, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)
	expectedLBH := expected.LatestBlockHeader()
	actualLBH := st.LatestBlockHeader()
	require.Equal(t, expectedLBH.Slot, actualLBH.Slot)
	require.Equal(t, bytesutil.ToBytes32(expectedLBH.ParentRoot), bytesutil.ToBytes32(actualLBH.ParentRoot))
	require.Equal(t, bytesutil.ToBytes32(expectedLBH.StateRoot), bytesutil.ToBytes32(actualLBH.StateRoot))
	require.Equal(t, expectedLBH.ProposerIndex, actualLBH.ProposerIndex)
	require.Equal(t, bytesutil.ToBytes32(expectedLBH.BodyRoot), bytesutil.ToBytes32(actualLBH.BodyRoot))
	require.Equal(t, expectedHTR, actualHTR)

	st, err = ch.ReplayerForSlot(one).ReplayBlocks(ctx)
	require.NoError(t, err)
	expected = hist.states[hist.slotMap[one]]

	// no canonical blocks in between, so latest block process_block_header will be for genesis
	expectedLBH, err = headerFromBlock(hist.blocks[hist.slotMap[0]])
	require.NoError(t, err)
	actualLBH = st.LatestBlockHeader()
	require.Equal(t, expectedLBH.Slot, actualLBH.Slot)
	require.Equal(t, bytesutil.ToBytes32(expectedLBH.ParentRoot), bytesutil.ToBytes32(actualLBH.ParentRoot))
	require.Equal(t, bytesutil.ToBytes32(expectedLBH.StateRoot), bytesutil.ToBytes32(actualLBH.StateRoot))
	require.Equal(t, expectedLBH.ProposerIndex, actualLBH.ProposerIndex)
	require.Equal(t, bytesutil.ToBytes32(expectedLBH.BodyRoot), bytesutil.ToBytes32(actualLBH.BodyRoot))

	require.Equal(t, expected.Slot(), st.Slot())
	// NOTE: HTR is not compared, because process_block is not called for non-canonical blocks,
	// so there are multiple differences compared to the "db" state that applies all blocks
}

func TestReplayerBlocks_SkipsExecutionPayloadEnvelopeLookup_PreGloas(t *testing.T) {
	ctx := t.Context()
	specs := []mockHistorySpec{
		{slot: 1, canonicalBlock: true},
	}

	base := newMockHistory(t, specs, 2)
	hist := &envelopeCountingHistory{mockHistory: base}
	ch := NewCanonicalHistory(hist, hist, hist)
	_, err := ch.ReplayerForSlot(1).ReplayBlocks(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, hist.envelopeCalls)
}

func TestReplayToSlot(t *testing.T) {
	ctx := t.Context()
	var zero, one, two, three, four, five primitives.Slot = 50, 51, 150, 151, 152, 200
	specs := []mockHistorySpec{
		{slot: zero},
		{slot: one, savedState: true},
		{slot: two},
		{slot: three},
		{slot: four},
		{slot: five, canonicalBlock: true},
	}

	// first case tests that ReplayToSlot is equivalent to ReplayBlocks
	hist := newMockHistory(t, specs, five+1)
	ch := NewCanonicalHistory(hist, hist, hist)

	st, err := ch.ReplayerForSlot(five).ReplayToSlot(ctx, five)
	require.NoError(t, err)
	expected := hist.hiddenStates[hist.slotMap[five]]
	expectedHTR, err := expected.HashTreeRoot(ctx)
	require.NoError(t, err)
	actualHTR, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)
	expectedLBH := expected.LatestBlockHeader()
	actualLBH := st.LatestBlockHeader()
	require.Equal(t, expectedLBH.Slot, actualLBH.Slot)
	require.Equal(t, bytesutil.ToBytes32(expectedLBH.ParentRoot), bytesutil.ToBytes32(actualLBH.ParentRoot))
	require.Equal(t, bytesutil.ToBytes32(expectedLBH.StateRoot), bytesutil.ToBytes32(actualLBH.StateRoot))
	require.Equal(t, expectedLBH.ProposerIndex, actualLBH.ProposerIndex)
	require.Equal(t, bytesutil.ToBytes32(expectedLBH.BodyRoot), bytesutil.ToBytes32(actualLBH.BodyRoot))
	require.Equal(t, expectedHTR, actualHTR)

	st, err = ch.ReplayerForSlot(five).ReplayToSlot(ctx, five+100)
	require.NoError(t, err)
	require.Equal(t, five+100, st.Slot())
	expectedLBH, err = headerFromBlock(hist.blocks[hist.slotMap[five]])
	require.NoError(t, err)
	actualLBH = st.LatestBlockHeader()
	require.Equal(t, expectedLBH.Slot, actualLBH.Slot)
	require.Equal(t, bytesutil.ToBytes32(expectedLBH.ParentRoot), bytesutil.ToBytes32(actualLBH.ParentRoot))
	require.Equal(t, bytesutil.ToBytes32(expectedLBH.StateRoot), bytesutil.ToBytes32(actualLBH.StateRoot))
	require.Equal(t, expectedLBH.ProposerIndex, actualLBH.ProposerIndex)
	require.Equal(t, bytesutil.ToBytes32(expectedLBH.BodyRoot), bytesutil.ToBytes32(actualLBH.BodyRoot))
}
