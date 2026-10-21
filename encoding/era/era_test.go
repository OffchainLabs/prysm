package era_test

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/era"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/golang/snappy"
)

// Record types from the format doc.
var (
	typeVersion   = [2]byte{0x65, 0x32}
	typeBlock     = [2]byte{0x01, 0x00}
	typeState     = [2]byte{0x02, 0x00}
	typeSlotIndex = [2]byte{0x69, 0x32}
)

// A mainnet era in Deneb.
const testEra = 1100

func appendRecord(b []byte, typ [2]byte, data []byte) []byte {
	b = append(b, typ[:]...)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(data)))
	b = append(b, 0, 0)
	return append(b, data...)
}

func compress(t *testing.T, data []byte) []byte {
	var buf bytes.Buffer
	w := snappy.NewBufferedWriter(&buf)
	_, err := w.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

// appendSlotIndex appends a SlotIndex for the given file offsets. Like Nimbus, it writes an empty
// slot (offset 0) as -indexStart.
func appendSlotIndex(b []byte, start primitives.Slot, offsets []int) []byte {
	indexStart := len(b)
	data := binary.LittleEndian.AppendUint64(nil, uint64(start))
	for _, off := range offsets {
		data = binary.LittleEndian.AppendUint64(data, uint64(int64(off-indexStart)))
	}
	data = binary.LittleEndian.AppendUint64(data, uint64(len(offsets)))
	return appendRecord(b, typeSlotIndex, data)
}

// eraFile builds an era file with blks and st. blks must be in the era that ends at st's slot.
func eraFile(t *testing.T, st state.BeaconState, blks []interfaces.ReadOnlySignedBeaconBlock) []byte {
	b := appendRecord(nil, typeVersion, nil)

	slotsPerHistoricalRoot := params.BeaconConfig().SlotsPerHistoricalRoot
	start := st.Slot() - min(st.Slot(), slotsPerHistoricalRoot)
	offsets := make([]int, st.Slot()-start)
	for _, blk := range blks {
		enc, err := blk.MarshalSSZ()
		require.NoError(t, err)
		offsets[blk.Block().Slot()-start] = len(b)
		b = appendRecord(b, typeBlock, compress(t, enc))
	}

	stateOffset := len(b)
	enc, err := st.MarshalSSZ()
	require.NoError(t, err)
	b = appendRecord(b, typeState, compress(t, enc))

	if st.Slot() > 0 {
		b = appendSlotIndex(b, start, offsets)
	}
	return appendSlotIndex(b, st.Slot(), []int{stateOffset})
}

func denebState(t *testing.T, slot primitives.Slot) state.BeaconState {
	st, err := util.NewBeaconStateDeneb()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(slot))
	cfg := params.BeaconConfig()
	require.NoError(t, st.SetFork(&ethpb.Fork{
		PreviousVersion: cfg.CapellaForkVersion,
		CurrentVersion:  cfg.DenebForkVersion,
		Epoch:           cfg.DenebForkEpoch,
	}))
	return st
}

func denebBlock(t *testing.T, slot primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
	b := util.NewBeaconBlockDeneb()
	b.Block.Slot = slot
	blk, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	return blk
}

// testEraFile returns an era file with blocks after an empty first slot, in the middle, and at the last slot.
func testEraFile(t *testing.T) ([]byte, primitives.Slot, []interfaces.ReadOnlySignedBeaconBlock) {
	stateSlot := primitives.Slot(testEra) * params.BeaconConfig().SlotsPerHistoricalRoot
	start := stateSlot - params.BeaconConfig().SlotsPerHistoricalRoot
	blks := []interfaces.ReadOnlySignedBeaconBlock{
		denebBlock(t, start+1),
		denebBlock(t, start+100),
		denebBlock(t, stateSlot-1),
	}
	return eraFile(t, denebState(t, stateSlot), blks), stateSlot, blks
}

func TestReader(t *testing.T) {
	file, stateSlot, blks := testEraFile(t)
	path := filepath.Join(t.TempDir(), "mainnet-01100-00000000.era")
	require.NoError(t, os.WriteFile(path, file, 0600))

	r, err := era.Open(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, r.Close()) }()
	require.Equal(t, stateSlot, r.StateSlot())

	_, err = r.State()
	require.NoError(t, err)

	for _, want := range blks {
		got, err := r.Block(want.Block().Slot())
		require.NoError(t, err)
		wantRoot, err := want.Block().HashTreeRoot()
		require.NoError(t, err)
		gotRoot, err := got.Block().HashTreeRoot()
		require.NoError(t, err)
		require.Equal(t, wantRoot, gotRoot)
	}

	start := stateSlot - params.BeaconConfig().SlotsPerHistoricalRoot
	empty, err := r.Block(start)
	require.NoError(t, err)
	require.Equal(t, true, empty == nil)

	_, err = r.Block(start - 1)
	require.ErrorContains(t, "outside the blocks", err)
	_, err = r.Block(stateSlot)
	require.ErrorContains(t, "outside the blocks", err)
}

func TestReader_Genesis(t *testing.T) {
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	file := eraFile(t, st, nil)

	r, err := era.NewReader(bytes.NewReader(file), int64(len(file)))
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(0), r.StateSlot())
	got, err := r.State()
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(0), got.Slot())
	_, err = r.Block(0)
	require.ErrorContains(t, "outside the blocks", err)
}

func TestNewReader_Rejects(t *testing.T) {
	file, _, _ := testEraFile(t)
	stateIndexStart := len(file) - 32

	tests := []struct {
		name   string
		modify func(b []byte) []byte
		want   string
	}{
		{"offset after the index", func(b []byte) []byte {
			binary.LittleEndian.PutUint64(b[stateIndexStart+16:], 8)
			return b
		}, "out of range"},
		{"two era groups", func(b []byte) []byte { return append(append([]byte{}, b...), b...) }, "exactly one era group"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := tt.modify(append([]byte{}, file...))
			_, err := era.NewReader(bytes.NewReader(b), int64(len(b)))
			require.ErrorContains(t, tt.want, err)
		})
	}
}

// The spec writes an empty slot as 0. TestReader covers the Nimbus form.
func TestReader_ZeroOffsetIsEmpty(t *testing.T) {
	file, stateSlot, blks := testEraFile(t)
	slotsPerHistoricalRoot := params.BeaconConfig().SlotsPerHistoricalRoot
	blockIndexStart := len(file) - 32 - 8*int(slotsPerHistoricalRoot) - 24
	slot := blks[1].Block().Slot()
	binary.LittleEndian.PutUint64(file[blockIndexStart+16+8*int(slot-(stateSlot-slotsPerHistoricalRoot)):], 0)

	r, err := era.NewReader(bytes.NewReader(file), int64(len(file)))
	require.NoError(t, err)
	blk, err := r.Block(slot)
	require.NoError(t, err)
	require.Equal(t, true, blk == nil)
}
