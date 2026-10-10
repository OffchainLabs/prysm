package enginev1

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/OffchainLabs/methodical-ssz/ssz"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/pkg/errors"
)

// ProgressiveTransactionList keeps the SSZ ProgressiveList[Transaction] of an
// execution payload exactly as it is encoded on the wire, split in two:
// Offsets is the 4-byte little-endian offset table (each entry relative to
// the start of the list encoding, so the first entry is always 4*n) and Data
// is the concatenated transaction bytes. MarshalSSZ concatenates the two and
// UnmarshalSSZ validates and copies them; nothing is ever decoded into
// []uint32 or [][]byte unless a caller asks for it.
//
// Values are immutable once constructed. To build or modify a list, use
// TransactionListBuilder, which holds offsets as []uint32 relative to the data
// and performs the conversion to the wire form once, in Build. Offsets and
// Data are exported only because protobuf requires it; construct lists with
// NewProgressiveTransactionList, UnmarshalSSZ or a builder, and read them
// through Len, Get and Slice. A list built by writing the fields directly is
// unsupported: Get returns nil where the table does not describe the data and
// the SSZ methods return errors, but nothing else is guaranteed.

const txOffsetSize = 4

var (
	// ErrTransactionIndexOutOfRange is returned by TransactionListBuilder.Set for an index outside [0, Len()).
	ErrTransactionIndexOutOfRange = errors.New("transaction index out of range")
	// ErrTransactionListCorrupt is returned when the offset table does not describe the data.
	ErrTransactionListCorrupt = errors.New("transaction list offsets are inconsistent with data")
	// ErrTransactionListTooLarge is returned when the encoded list would not fit 4-byte SSZ offsets.
	ErrTransactionListTooLarge = errors.New("transaction list encoding exceeds the 4-byte offset range")
)

// NewProgressiveTransactionList builds a list from a slice of transactions in
// one pass and one allocation. The result never aliases txs.
func NewProgressiveTransactionList(txs [][]byte) (*ProgressiveTransactionList, error) {
	if len(txs) > fieldparams.MaxTxsPerPayloadLength {
		return nil, fmt.Errorf("%d transactions exceeds the maximum of %d: %w", len(txs), fieldparams.MaxTxsPerPayloadLength, ssz.ErrListTooBig)
	}
	tableLen := uint64(len(txs)) * txOffsetSize
	total := tableLen
	for i, tx := range txs {
		if len(tx) > fieldparams.MaxBytesPerTxLength {
			return nil, fmt.Errorf("transaction %d is %d bytes, exceeding the maximum of %d: %w", i, len(tx), fieldparams.MaxBytesPerTxLength, ssz.ErrBytesLength)
		}
		total += uint64(len(tx))
	}
	if total > math.MaxUint32 {
		return nil, ErrTransactionListTooLarge
	}
	b := make([]byte, total)
	offsets := b[:tableLen:tableLen]
	data := b[tableLen:]
	pos := tableLen
	for i, tx := range txs {
		binary.LittleEndian.PutUint32(offsets[i*txOffsetSize:], uint32(pos))
		pos += uint64(copy(data[pos-tableLen:], tx))
	}
	return &ProgressiveTransactionList{Offsets: offsets, Data: data}, nil
}

// fields returns the receiver's slices, treating a nil receiver as an empty list.
func (t *ProgressiveTransactionList) fields() (offsets, data []byte) {
	if t == nil {
		return nil, nil
	}
	return t.Offsets, t.Data
}

// Len returns the number of transactions.
func (t *ProgressiveTransactionList) Len() int {
	offsets, _ := t.fields()
	return len(offsets) / txOffsetSize
}

// Get returns transaction i as a read-only, capacity-limited view into the
// list's data. It returns nil for an index outside [0, Len()) or when the
// offset table does not describe the data; it never panics. A transaction
// that is legitimately empty is returned as a non-nil empty slice.
func (t *ProgressiveTransactionList) Get(i int) []byte {
	offsets, data := t.fields()
	start, end, ok := txBounds(offsets, data, i)
	if !ok {
		return nil
	}
	if start == end {
		return []byte{}
	}
	return data[start:end:end]
}

// Slice returns every transaction as a read-only view. It allocates the outer
// slice only; no transaction bytes are copied.
func (t *ProgressiveTransactionList) Slice() [][]byte {
	n := t.Len()
	out := make([][]byte, n)
	for i := range out {
		out[i] = t.Get(i)
	}
	return out
}

// Copy returns a deep copy. A nil receiver copies to nil.
func (t *ProgressiveTransactionList) Copy() *ProgressiveTransactionList {
	if t == nil {
		return nil
	}
	return &ProgressiveTransactionList{
		Offsets: bytesutil.SafeCopyBytes(t.Offsets),
		Data:    bytesutil.SafeCopyBytes(t.Data),
	}
}

// SizeSSZ returns the SSZ-encoded size: the offset table plus the data.
func (t *ProgressiveTransactionList) SizeSSZ() int {
	offsets, data := t.fields()
	return len(offsets) + len(data)
}

// MarshalSSZ encodes the list as an SSZ ProgressiveList[Transaction].
func (t *ProgressiveTransactionList) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, t.SizeSSZ())
	return t.MarshalSSZTo(buf[:0])
}

// MarshalSSZTo appends the SSZ encoding to dst: the offset table followed by
// the data, both verbatim.
func (t *ProgressiveTransactionList) MarshalSSZTo(dst []byte) ([]byte, error) {
	offsets, data := t.fields()
	if err := txValidate(offsets, data); err != nil {
		return nil, err
	}
	dst = append(dst, offsets...)
	return append(dst, data...), nil
}

// UnmarshalSSZ decodes an SSZ ProgressiveList[Transaction], validating the
// offset table exactly as the generated code does (first offset non-zero,
// 4-aligned and within the buffer; offsets non-decreasing and within the
// buffer; element count and sizes within the payload limits), then copies the
// encoding in one allocation.
func (t *ProgressiveTransactionList) UnmarshalSSZ(buf []byte) error {
	if t == nil {
		return errors.New("cannot unmarshal into a nil ProgressiveTransactionList")
	}
	offsets, data, err := txUnmarshal(buf)
	if err != nil {
		return err
	}
	t.Offsets = offsets
	t.Data = data
	return nil
}

// HashTreeRoot computes the progressive merkle root of the list.
func (t *ProgressiveTransactionList) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := t.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

// HashTreeRootWith merkleizes the list as ProgressiveList[ProgressiveByteList],
// exactly as methodical generates for a progressive list of progressive byte
// lists. Offsets only locate each element within the data; they never enter
// the hash.
func (t *ProgressiveTransactionList) HashTreeRootWith(hh *ssz.Hasher) error {
	offsets, data := t.fields()
	if err := txValidate(offsets, data); err != nil {
		return err
	}
	n := len(offsets) / txOffsetSize
	listIndx := hh.Index()
	for i := 0; i < n; i++ {
		start, end, _ := txBounds(offsets, data, i)
		tx := data[start:end]
		subIndx := hh.Index()
		hh.AppendBytes32(tx)
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(tx)))
	}
	hh.MerkleizeProgressiveWithMixin(listIndx, uint64(n))
	return nil
}

// TransactionListBuilder assembles a ProgressiveTransactionList
// incrementally. It keeps offsets as []uint32 relative to the start of its
// data, so Append is amortized constant time and never touches existing
// entries; Build converts to the wire form once. A builder may be seeded from
// an existing list, which is copied and left unchanged. Build may be called
// more than once; each result is independent of the builder and of the other
// results.
type TransactionListBuilder struct {
	offsets []uint32
	data    []byte
}

// NewTransactionListBuilder returns a builder holding a copy of base, or an
// empty builder when base is nil.
func NewTransactionListBuilder(base *ProgressiveTransactionList) (*TransactionListBuilder, error) {
	offsets, data := base.fields()
	if err := txValidate(offsets, data); err != nil {
		return nil, err
	}
	n := len(offsets) / txOffsetSize
	b := &TransactionListBuilder{
		offsets: make([]uint32, n),
		data:    append([]byte{}, data...),
	}
	table := uint32(len(offsets))
	for i := range b.offsets {
		b.offsets[i] = binary.LittleEndian.Uint32(offsets[i*txOffsetSize:]) - table
	}
	return b, nil
}

// Len returns the number of transactions currently in the builder.
func (b *TransactionListBuilder) Len() int {
	return len(b.offsets)
}

// Append adds txs to the end of the list. The bytes are copied.
func (b *TransactionListBuilder) Append(txs ...[]byte) error {
	if len(b.offsets)+len(txs) > fieldparams.MaxTxsPerPayloadLength {
		return fmt.Errorf("%d transactions exceeds the maximum of %d: %w", len(b.offsets)+len(txs), fieldparams.MaxTxsPerPayloadLength, ssz.ErrListTooBig)
	}
	total := b.encodedSize() + uint64(len(txs))*txOffsetSize
	for i, tx := range txs {
		if len(tx) > fieldparams.MaxBytesPerTxLength {
			return fmt.Errorf("transaction %d is %d bytes, exceeding the maximum of %d: %w", i, len(tx), fieldparams.MaxBytesPerTxLength, ssz.ErrBytesLength)
		}
		total += uint64(len(tx))
	}
	if total > math.MaxUint32 {
		return ErrTransactionListTooLarge
	}
	for _, tx := range txs {
		b.offsets = append(b.offsets, uint32(len(b.data)))
		b.data = append(b.data, tx...)
	}
	return nil
}

// Set replaces transaction i with tx, shifting the elements after it.
func (b *TransactionListBuilder) Set(i int, tx []byte) error {
	n := len(b.offsets)
	if i < 0 || i >= n {
		return errors.Wrapf(ErrTransactionIndexOutOfRange, "index %d, length %d", i, n)
	}
	if len(tx) > fieldparams.MaxBytesPerTxLength {
		return fmt.Errorf("transaction is %d bytes, exceeding the maximum of %d: %w", len(tx), fieldparams.MaxBytesPerTxLength, ssz.ErrBytesLength)
	}
	start := uint64(b.offsets[i])
	end := uint64(len(b.data))
	if i+1 < n {
		end = uint64(b.offsets[i+1])
	}
	oldLen := end - start
	newLen := uint64(len(tx))
	if b.encodedSize()-oldLen+newLen > math.MaxUint32 {
		return ErrTransactionListTooLarge
	}
	tail := append([]byte{}, b.data[end:]...)
	b.data = append(append(b.data[:start], tx...), tail...)
	for j := i + 1; j < n; j++ {
		b.offsets[j] = uint32(uint64(b.offsets[j]) - oldLen + newLen)
	}
	return nil
}

// Build converts the builder's contents to the wire form in one pass and one
// allocation. The builder remains usable afterwards and the result does not
// alias it.
func (b *TransactionListBuilder) Build() (*ProgressiveTransactionList, error) {
	n := len(b.offsets)
	if n > fieldparams.MaxTxsPerPayloadLength {
		return nil, fmt.Errorf("%d transactions exceeds the maximum of %d: %w", n, fieldparams.MaxTxsPerPayloadLength, ssz.ErrListTooBig)
	}
	total := b.encodedSize()
	if total > math.MaxUint32 {
		return nil, ErrTransactionListTooLarge
	}
	tableLen := uint64(n) * txOffsetSize
	out := make([]byte, total)
	for i, o := range b.offsets {
		binary.LittleEndian.PutUint32(out[uint64(i)*txOffsetSize:], o+uint32(tableLen))
	}
	copy(out[tableLen:], b.data)
	return &ProgressiveTransactionList{Offsets: out[:tableLen:tableLen], Data: out[tableLen:]}, nil
}

// encodedSize is the SSZ size the builder's contents would occupy.
func (b *TransactionListBuilder) encodedSize() uint64 {
	return uint64(len(b.offsets))*txOffsetSize + uint64(len(b.data))
}

// txBounds returns the [start, end) range of element i within data, or ok=false
// when i is out of range or the table does not describe the data. Wire offsets
// are relative to the start of the encoding, so the table length is subtracted.
// The bounds are returned as uint64 so callers slice with them directly.
func txBounds(offsets, data []byte, i int) (start, end uint64, ok bool) {
	n := len(offsets) / txOffsetSize
	if i < 0 || i >= n {
		return 0, 0, false
	}
	table := uint64(len(offsets))
	total := table + uint64(len(data))
	start = uint64(binary.LittleEndian.Uint32(offsets[i*txOffsetSize:]))
	end = total
	if i+1 < n {
		end = uint64(binary.LittleEndian.Uint32(offsets[(i+1)*txOffsetSize:]))
	}
	if start < table || start > end || end > total {
		return 0, 0, false
	}
	return start - table, end - table, true
}

// txValidate checks that a wire offset table describes data: 4-aligned, a
// first entry equal to the table length, non-decreasing entries within the
// encoding, and every element and the element count within the payload
// limits. It also rejects a list whose encoding would overflow 4-byte
// offsets.
func txValidate(offsets, data []byte) error {
	if len(offsets)%txOffsetSize != 0 {
		return errors.Wrapf(ErrTransactionListCorrupt, "offset table length %d is not a multiple of %d", len(offsets), txOffsetSize)
	}
	n := len(offsets) / txOffsetSize
	if n == 0 {
		if len(data) != 0 {
			return errors.Wrap(ErrTransactionListCorrupt, "data present without offsets")
		}
		return nil
	}
	if n > fieldparams.MaxTxsPerPayloadLength {
		return fmt.Errorf("%d transactions exceeds the maximum of %d: %w", n, fieldparams.MaxTxsPerPayloadLength, ssz.ErrListTooBig)
	}
	table := uint64(len(offsets))
	total := table + uint64(len(data))
	if total > math.MaxUint32 {
		return ErrTransactionListTooLarge
	}
	prev := uint64(binary.LittleEndian.Uint32(offsets))
	if prev != table {
		return errors.Wrapf(ErrTransactionListCorrupt, "first offset is %d, want the table length %d", prev, table)
	}
	for i := 1; i <= n; i++ {
		cur := total
		if i < n {
			cur = uint64(binary.LittleEndian.Uint32(offsets[i*txOffsetSize:]))
		}
		if cur < prev {
			return errors.Wrapf(ErrTransactionListCorrupt, "offset %d is less than previous offset %d", cur, prev)
		}
		if cur > total {
			return errors.Wrapf(ErrTransactionListCorrupt, "offset %d points past the end of the encoding (%d bytes)", cur, total)
		}
		if cur-prev > fieldparams.MaxBytesPerTxLength {
			return fmt.Errorf("transaction %d is %d bytes, exceeding the maximum of %d: %w", i-1, cur-prev, fieldparams.MaxBytesPerTxLength, ssz.ErrBytesLength)
		}
		prev = cur
	}
	return nil
}

// txUnmarshal validates an SSZ ProgressiveList[Transaction] encoding and
// returns the offset table and data, backed by a single fresh copy of the
// input with the table capacity-limited.
func txUnmarshal(buf []byte) (offsets, data []byte, err error) {
	if len(buf) == 0 {
		return nil, nil, nil
	}
	if len(buf) < txOffsetSize {
		return nil, nil, errors.New("list bytes too short to contain an offset when decoding transactions")
	}
	size := uint64(len(buf))
	first := uint64(binary.LittleEndian.Uint32(buf))
	if first == 0 {
		return nil, nil, errors.New("encountered invalid offset of 0 when decoding transactions")
	}
	if first%txOffsetSize != 0 {
		return nil, nil, fmt.Errorf("misaligned list bytes: when decoding transactions, end-of-list offset is %d, which is not a multiple of %d (offset size)", first, txOffsetSize)
	}
	if first > size {
		return nil, nil, fmt.Errorf("offset %d points past the end of buffer when decoding transactions", first)
	}
	n := first / txOffsetSize
	if n > fieldparams.MaxTxsPerPayloadLength {
		return nil, nil, fmt.Errorf("ssz-max exceeded: transactions has %d elements, ssz-max is %d: %w", n, fieldparams.MaxTxsPerPayloadLength, ssz.ErrListTooBig)
	}
	prev := first
	for i := uint64(1); i <= n; i++ {
		cur := size
		if i < n {
			cur = uint64(binary.LittleEndian.Uint32(buf[i*txOffsetSize:]))
			if cur > size {
				return nil, nil, fmt.Errorf("offset %d points past the end of buffer when decoding transactions", cur)
			}
		}
		if cur < prev {
			return nil, nil, fmt.Errorf("offset %d is less than start offset %d when decoding transactions", cur, prev)
		}
		if cur-prev > fieldparams.MaxBytesPerTxLength {
			return nil, nil, fmt.Errorf("transaction %d is %d bytes, exceeding the maximum of %d: %w", i-1, cur-prev, fieldparams.MaxBytesPerTxLength, ssz.ErrBytesLength)
		}
		prev = cur
	}
	b := make([]byte, len(buf))
	copy(b, buf)
	return b[:first:first], b[first:], nil
}
