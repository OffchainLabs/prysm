// Package era reads era files: the blocks of one era and the state at its end, stored in the
// e2store format. See https://github.com/status-im/nimbus-eth2/blob/stable/docs/e2store.md.
package era

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz/detect"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
)

// Record types used by era files.
var (
	typeVersion                     = [2]byte{0x65, 0x32}
	typeCompressedSignedBeaconBlock = [2]byte{0x01, 0x00}
	typeCompressedBeaconState       = [2]byte{0x02, 0x00}
	typeSlotIndex                   = [2]byte{0x69, 0x32}
)

// headerSize is the size of a record header: type (2 bytes), length (4), reserved (2).
const headerSize = 8

// Reader reads one era file. It is safe for concurrent use when its io.ReaderAt is.
type Reader struct {
	r           io.ReaderAt
	size        int64
	closer      io.Closer
	stateSlot   primitives.Slot
	stateOffset int64
	// blockOffsets[i] is the file offset of the block at slot stateSlot-len(blockOffsets)+i, or 0
	// when that slot is empty. It is nil for the genesis era, which has no blocks.
	blockOffsets []int64
}

// Open opens the era file at path.
func Open(path string) (_ *Reader, err error) {
	f, err := os.Open(path) // #nosec G304
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = f.Close()
		}
	}()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	r, err := NewReader(f, info.Size())
	if err != nil {
		return nil, err
	}
	r.closer = f
	return r, nil
}

// NewReader reads the slot indices of an era file of size bytes. The file must hold exactly one
// era group, as published era files do.
func NewReader(r io.ReaderAt, size int64) (*Reader, error) {
	typ, length, err := readHeader(r, size, 0)
	if err != nil {
		return nil, err
	}
	if typ != typeVersion || length != 0 {
		return nil, errors.New("era file does not start with a version record")
	}

	slotsPerHistoricalRoot := params.BeaconConfig().SlotsPerHistoricalRoot
	stateSlot, stateIndexStart, stateOffsets, err := readSlotIndex(r, size, size, 1)
	if err != nil {
		return nil, errors.Wrap(err, "could not read state index")
	}
	if stateSlot%slotsPerHistoricalRoot != 0 {
		return nil, fmt.Errorf("state slot %d is not the end of an era", stateSlot)
	}
	if stateOffsets[0] == 0 {
		return nil, errors.New("state index has no state")
	}
	er := &Reader{r: r, size: size, stateSlot: stateSlot, stateOffset: stateOffsets[0]}

	// The genesis era has a state but no blocks, so it has no block index.
	first := er.stateOffset
	if stateSlot > 0 {
		count := int64(slotsPerHistoricalRoot) // lint:ignore uintcast -- a small config constant.
		blockSlot, _, blockOffsets, err := readSlotIndex(r, size, stateIndexStart, count)
		if err != nil {
			return nil, errors.Wrap(err, "could not read block index")
		}
		if blockSlot+slotsPerHistoricalRoot != stateSlot {
			return nil, fmt.Errorf("block index starts at slot %d, want %d", blockSlot, stateSlot-slotsPerHistoricalRoot)
		}
		er.blockOffsets = blockOffsets
		for _, off := range blockOffsets {
			if off != 0 {
				first = off
				break
			}
		}
	}
	// In a one-group file, the record after the version record is the first block, or the
	// state if the era has no blocks.
	if first != headerSize {
		return nil, errors.New("era file must hold exactly one era group")
	}
	return er, nil
}

// StateSlot returns the slot of the state, which is the first slot after the era's blocks.
func (r *Reader) StateSlot() primitives.Slot {
	return r.stateSlot
}

// State returns the state at StateSlot, with the block at that slot applied if there is one.
func (r *Reader) State() (state.BeaconState, error) {
	enc, err := r.readCompressed(r.stateOffset, typeCompressedBeaconState)
	if err != nil {
		return nil, errors.Wrap(err, "could not read state")
	}
	unmarshaler, err := detect.FromState(enc)
	if err != nil {
		return nil, errors.Wrap(err, "could not detect state fork")
	}
	st, err := unmarshaler.UnmarshalBeaconState(enc)
	if err != nil {
		return nil, err
	}
	if st.Slot() != r.stateSlot {
		return nil, fmt.Errorf("state has slot %d, index says %d", st.Slot(), r.stateSlot)
	}
	return st, nil
}

// Block returns the block at slot, or nil when the slot is empty. slot must be in
// [StateSlot-SLOTS_PER_HISTORICAL_ROOT, StateSlot).
func (r *Reader) Block(slot primitives.Slot) (interfaces.ReadOnlySignedBeaconBlock, error) {
	start := r.stateSlot - primitives.Slot(len(r.blockOffsets))
	if slot < start || slot >= r.stateSlot {
		return nil, fmt.Errorf("slot %d is outside the blocks of this era file [%d, %d)", slot, start, r.stateSlot)
	}
	off := r.blockOffsets[slot-start]
	if off == 0 {
		return nil, nil
	}
	enc, err := r.readCompressed(off, typeCompressedSignedBeaconBlock)
	if err != nil {
		return nil, errors.Wrapf(err, "could not read block at slot %d", slot)
	}
	unmarshaler, err := detect.FromBlock(enc)
	if err != nil {
		return nil, errors.Wrapf(err, "could not detect fork of block at slot %d", slot)
	}
	blk, err := unmarshaler.UnmarshalBeaconBlock(enc)
	if err != nil {
		return nil, errors.Wrapf(err, "could not unmarshal block at slot %d", slot)
	}
	if blk.Block().Slot() != slot {
		return nil, fmt.Errorf("block has slot %d, index says %d", blk.Block().Slot(), slot)
	}
	return blk, nil
}

// Close closes the file opened by Open. It does nothing for a Reader made by NewReader.
func (r *Reader) Close() error {
	if r.closer == nil {
		return nil
	}
	return r.closer.Close()
}

// readCompressed returns the uncompressed data of the record of type want at off.
func (r *Reader) readCompressed(off int64, want [2]byte) ([]byte, error) {
	typ, length, err := readHeader(r.r, r.size, off)
	if err != nil {
		return nil, err
	}
	if typ != want {
		return nil, fmt.Errorf("record at offset %d has type %#x, want %#x", off, typ, want)
	}
	return io.ReadAll(snappy.NewReader(io.NewSectionReader(r.r, off+headerSize, length)))
}

// readHeader reads the record header at off and checks that the record fits in the file.
func readHeader(r io.ReaderAt, size, off int64) ([2]byte, int64, error) {
	if off < 0 || off > size-headerSize {
		return [2]byte{}, 0, fmt.Errorf("record header at offset %d is outside the file", off)
	}
	var h [headerSize]byte
	if _, err := io.ReadFull(io.NewSectionReader(r, off, headerSize), h[:]); err != nil {
		return [2]byte{}, 0, err
	}
	if h[6] != 0 || h[7] != 0 {
		return [2]byte{}, 0, fmt.Errorf("record at offset %d has a non-zero reserved field", off)
	}
	length := int64(binary.LittleEndian.Uint32(h[2:6])) // lint:ignore uintcast -- a uint32 always fits in an int64.
	if length > size-off-headerSize {
		return [2]byte{}, 0, fmt.Errorf("record at offset %d runs past the end of the file", off)
	}
	return [2]byte{h[0], h[1]}, length, nil
}

// readSlotIndex reads the slot index of count slots that ends at end. It returns the starting
// slot, the offset where the index starts, and one file offset per slot, 0 for an empty slot.
//
// A slot index is: header | starting slot | one offset per slot | count, each number 8 bytes
// little-endian. Offsets are relative to the start of the index.
func readSlotIndex(r io.ReaderAt, size, end, count int64) (primitives.Slot, int64, []int64, error) {
	length := 8 * (count + 2) // starting slot and count take 8 bytes each
	start := end - headerSize - length
	if start < 0 {
		return 0, 0, nil, fmt.Errorf("file is too short for a slot index of %d slots", count)
	}
	typ, gotLength, err := readHeader(r, size, start)
	if err != nil {
		return 0, 0, nil, err
	}
	if typ != typeSlotIndex || gotLength != length {
		return 0, 0, nil, fmt.Errorf("no slot index of %d slots at offset %d", count, start)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(io.NewSectionReader(r, start+headerSize, length), data); err != nil {
		return 0, 0, nil, err
	}
	if got := binary.LittleEndian.Uint64(data[length-8:]); got != uint64(count) {
		return 0, 0, nil, fmt.Errorf("slot index has %d slots, want %d", got, count)
	}

	offsets := make([]int64, count)
	for i := range offsets {
		rel := int64(binary.LittleEndian.Uint64(data[8+8*i:])) // lint:ignore uintcast -- slot index entries are signed.
		if rel == 0 || rel == -start {
			continue
		}
		// Data comes after the version record and before the index.
		if rel > -headerSize || rel < headerSize-start {
			return 0, 0, nil, fmt.Errorf("slot index entry %d has offset %d out of range", i, rel)
		}
		offsets[i] = start + rel
	}
	return primitives.Slot(binary.LittleEndian.Uint64(data[:8])), start, offsets, nil
}
