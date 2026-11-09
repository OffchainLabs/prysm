package enginev1_test

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"testing"

	"github.com/OffchainLabs/methodical-ssz/ssz"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/wrappers"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// wireEncode is an independent oracle for the SSZ encoding of a list of byte
// lists: absolute 4-byte little-endian offsets followed by the elements.
func wireEncode(txs [][]byte) []byte {
	n := len(txs)
	out := make([]byte, 0, 4*n)
	pos := 4 * n
	for _, tx := range txs {
		out = binary.LittleEndian.AppendUint32(out, uint32(pos))
		pos += len(tx)
	}
	for _, tx := range txs {
		out = append(out, tx...)
	}
	return out
}

func randomTxs(r *rand.Rand, n, maxLen int) [][]byte {
	txs := make([][]byte, n)
	for i := range txs {
		l := r.Intn(maxLen + 1)
		if r.Intn(8) == 0 {
			l = 0
		}
		tx := make([]byte, l)
		r.Read(tx)
		txs[i] = tx
	}
	return txs
}

var txFixtures = map[string][][]byte{
	"empty":       {},
	"single":      {{0x01, 0x02, 0x03}},
	"many":        {{0xaa}, {0xbb, 0xbb}, {0xcc, 0xcc, 0xcc}, {0xdd}},
	"mixed_empty": {{}, {0x01, 0x02}, {}, {0x03}},
	"all_empty":   {{}, {}, {}},
}

func mustList(t *testing.T, txs [][]byte) *enginev1.ProgressiveTransactionList {
	t.Helper()
	l, err := enginev1.NewProgressiveTransactionList(txs)
	require.NoError(t, err)
	return l
}

func mustMarshal(t *testing.T, l *enginev1.ProgressiveTransactionList) []byte {
	t.Helper()
	enc, err := l.MarshalSSZ()
	require.NoError(t, err)
	return enc
}

func TestProgressiveTransactionList_RoundTrip(t *testing.T) {
	for name, txs := range txFixtures {
		t.Run(name, func(t *testing.T) {
			l := mustList(t, txs)
			require.Equal(t, len(txs), l.Len())
			require.Equal(t, 4*len(txs), len(l.Offsets))
			if l.Len() > 0 {
				require.Equal(t, uint32(4*len(txs)), binary.LittleEndian.Uint32(l.Offsets), "stored offsets are the wire offsets")
			}

			enc := mustMarshal(t, l)
			require.DeepEqual(t, wireEncode(txs), enc)
			require.Equal(t, len(enc), l.SizeSSZ())
			require.DeepEqual(t, enc, append(append([]byte{}, l.Offsets...), l.Data...), "marshal is the concatenation of the fields")

			got := &enginev1.ProgressiveTransactionList{}
			require.NoError(t, got.UnmarshalSSZ(enc))
			require.DeepEqual(t, l, got)
			require.DeepEqual(t, txs, got.Slice())
			require.DeepEqual(t, enc, mustMarshal(t, got))
			if got.Len() > 0 {
				require.Equal(t, len(got.Offsets), cap(got.Offsets), "unmarshal must capacity-limit the table")
			}
		})
	}
}

func TestProgressiveTransactionList_Get(t *testing.T) {
	l := mustList(t, txFixtures["mixed_empty"])

	empty := l.Get(0)
	require.NotNil(t, empty, "a legitimately empty element is non-nil")
	require.Equal(t, 0, len(empty))

	tx := l.Get(1)
	require.DeepEqual(t, []byte{0x01, 0x02}, tx)
	require.Equal(t, len(tx), cap(tx), "views are capacity-limited")
	require.DeepEqual(t, []byte{0x03}, l.Get(3))

	require.IsNil(t, l.Get(-1))
	require.IsNil(t, l.Get(4))

	var nilList *enginev1.ProgressiveTransactionList
	require.IsNil(t, nilList.Get(0))
	require.Equal(t, 0, nilList.Len())
}

func TestProgressiveTransactionList_CorruptTable(t *testing.T) {
	expectCorrupt := func(t *testing.T, l *enginev1.ProgressiveTransactionList) {
		t.Helper()
		_, err := l.MarshalSSZ()
		require.ErrorIs(t, err, enginev1.ErrTransactionListCorrupt)
		_, err = l.HashTreeRoot()
		require.ErrorIs(t, err, enginev1.ErrTransactionListCorrupt)
		_, err = enginev1.NewTransactionListBuilder(l)
		require.ErrorIs(t, err, enginev1.ErrTransactionListCorrupt)
	}
	t.Run("offset past data", func(t *testing.T) {
		l := &enginev1.ProgressiveTransactionList{
			Offsets: []byte{8, 0, 0, 0, 20, 0, 0, 0},
			Data:    []byte{1, 2, 3},
		}
		require.IsNil(t, l.Get(0))
		require.IsNil(t, l.Get(1))
		require.DeepEqual(t, [][]byte{nil, nil}, l.Slice())
		expectCorrupt(t, l)
	})
	t.Run("non-monotonic", func(t *testing.T) {
		l := &enginev1.ProgressiveTransactionList{
			Offsets: []byte{12, 0, 0, 0, 14, 0, 0, 0, 13, 0, 0, 0},
			Data:    []byte{1, 2, 3},
		}
		require.IsNil(t, l.Get(1))
		expectCorrupt(t, l)
	})
	t.Run("first offset inside the table", func(t *testing.T) {
		l := &enginev1.ProgressiveTransactionList{Offsets: []byte{0, 0, 0, 0}, Data: []byte{1, 2}}
		require.IsNil(t, l.Get(0))
		expectCorrupt(t, l)
	})
	t.Run("misaligned table", func(t *testing.T) {
		l := &enginev1.ProgressiveTransactionList{Offsets: []byte{0, 0, 0}, Data: nil}
		require.Equal(t, 0, l.Len())
		expectCorrupt(t, l)
	})
	t.Run("data without offsets", func(t *testing.T) {
		l := &enginev1.ProgressiveTransactionList{Data: []byte{1}}
		expectCorrupt(t, l)
	})
}

func TestTransactionListBuilder_FromNil(t *testing.T) {
	b, err := enginev1.NewTransactionListBuilder(nil)
	require.NoError(t, err)
	require.Equal(t, 0, b.Len())

	empty, err := b.Build()
	require.NoError(t, err)
	require.Equal(t, 0, empty.Len())
	require.Equal(t, 0, len(mustMarshal(t, empty)))

	require.NoError(t, b.Append([]byte{1}, []byte{}, []byte{2, 3}))
	require.Equal(t, 3, b.Len())
	built, err := b.Build()
	require.NoError(t, err)
	expected := [][]byte{{1}, {}, {2, 3}}
	require.DeepEqual(t, wireEncode(expected), mustMarshal(t, built))
	require.DeepEqual(t, expected, built.Slice())
	require.Equal(t, len(built.Offsets), cap(built.Offsets), "build must capacity-limit the table")
}

func TestTransactionListBuilder_FromExisting(t *testing.T) {
	base := mustList(t, txFixtures["many"])
	before := mustMarshal(t, base)

	b, err := enginev1.NewTransactionListBuilder(base)
	require.NoError(t, err)
	require.Equal(t, 4, b.Len())
	require.NoError(t, b.Append([]byte{0x11}, []byte{}))
	res, err := b.Build()
	require.NoError(t, err)

	expected := append(append([][]byte{}, txFixtures["many"]...), []byte{0x11}, []byte{})
	require.DeepEqual(t, mustMarshal(t, mustList(t, expected)), mustMarshal(t, res))
	require.DeepEqual(t, expected, res.Slice())

	// The base list is untouched, and the result shares nothing with it.
	require.DeepEqual(t, before, mustMarshal(t, base))
	res.Data[0] ^= 0xff
	require.DeepEqual(t, before, mustMarshal(t, base))

	// Seeding from a list that was unmarshaled works the same way.
	decoded := &enginev1.ProgressiveTransactionList{}
	require.NoError(t, decoded.UnmarshalSSZ(before))
	b2, err := enginev1.NewTransactionListBuilder(decoded)
	require.NoError(t, err)
	require.NoError(t, b2.Append([]byte{0x22, 0x22}))
	grown, err := b2.Build()
	require.NoError(t, err)
	require.DeepEqual(t, before, mustMarshal(t, decoded))
	require.DeepEqual(t, []byte{0x22, 0x22}, grown.Get(4))
}

func TestTransactionListBuilder_BuildIsRepeatable(t *testing.T) {
	b, err := enginev1.NewTransactionListBuilder(nil)
	require.NoError(t, err)
	require.NoError(t, b.Append([]byte{1, 1}))
	first, err := b.Build()
	require.NoError(t, err)
	firstEnc := mustMarshal(t, first)

	require.NoError(t, b.Append([]byte{2}))
	require.NoError(t, b.Set(0, []byte{9, 9, 9}))
	second, err := b.Build()
	require.NoError(t, err)

	// Earlier results are independent of later builder mutations.
	require.DeepEqual(t, firstEnc, mustMarshal(t, first))
	require.DeepEqual(t, [][]byte{{1, 1}}, first.Slice())
	require.DeepEqual(t, [][]byte{{9, 9, 9}, {2}}, second.Slice())
	require.DeepEqual(t, wireEncode([][]byte{{9, 9, 9}, {2}}), mustMarshal(t, second))
}

func TestTransactionListBuilder_Set(t *testing.T) {
	txs := txFixtures["many"]
	cases := []struct {
		name string
		i    int
		tx   []byte
	}{
		{"first longer", 0, []byte{1, 2, 3, 4, 5}},
		{"last shorter", 3, []byte{}},
		{"middle empty", 1, []byte{}},
		{"middle same length", 2, []byte{9, 9, 9}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := enginev1.NewTransactionListBuilder(mustList(t, txs))
			require.NoError(t, err)
			require.NoError(t, b.Set(c.i, c.tx))
			res, err := b.Build()
			require.NoError(t, err)
			expected := append([][]byte{}, txs...)
			expected[c.i] = c.tx
			require.DeepEqual(t, mustMarshal(t, mustList(t, expected)), mustMarshal(t, res))
			require.DeepEqual(t, expected, res.Slice())
		})
	}

	b, err := enginev1.NewTransactionListBuilder(mustList(t, txs))
	require.NoError(t, err)
	require.ErrorIs(t, b.Set(4, []byte{1}), enginev1.ErrTransactionIndexOutOfRange)
	require.ErrorIs(t, b.Set(-1, []byte{1}), enginev1.ErrTransactionIndexOutOfRange)
}

func TestTransactionListBuilder_Limits(t *testing.T) {
	tooMany := make([][]byte, fieldparams.MaxTxsPerPayloadLength+1)
	_, err := enginev1.NewProgressiveTransactionList(tooMany)
	require.ErrorIs(t, err, ssz.ErrListTooBig)

	b, err := enginev1.NewTransactionListBuilder(nil)
	require.NoError(t, err)
	require.ErrorIs(t, b.Append(tooMany...), ssz.ErrListTooBig)
	require.Equal(t, 0, b.Len(), "a rejected append leaves the builder unchanged")

	atMax := make([][]byte, fieldparams.MaxTxsPerPayloadLength)
	l, err := enginev1.NewProgressiveTransactionList(atMax)
	require.NoError(t, err)
	require.Equal(t, fieldparams.MaxTxsPerPayloadLength, l.Len())
}

func TestProgressiveTransactionList_NilReceiver(t *testing.T) {
	var l *enginev1.ProgressiveTransactionList
	require.Equal(t, 0, l.Len())
	require.Equal(t, 0, l.SizeSSZ())
	require.IsNil(t, l.Copy())
	require.Equal(t, 0, len(l.Slice()))

	enc, err := l.MarshalSSZ()
	require.NoError(t, err)
	require.Equal(t, 0, len(enc))
	dst, err := l.MarshalSSZTo([]byte{9})
	require.NoError(t, err)
	require.DeepEqual(t, []byte{9}, dst)

	root, err := l.HashTreeRoot()
	require.NoError(t, err)
	emptyRoot, err := mustList(t, nil).HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, emptyRoot, root)

	require.NotNil(t, l.UnmarshalSSZ([]byte{}))
}

func TestProgressiveTransactionList_UnmarshalMalformed(t *testing.T) {
	overMax := make([]byte, 4*(fieldparams.MaxTxsPerPayloadLength+1))
	binary.LittleEndian.PutUint32(overMax, uint32(len(overMax)))

	cases := map[string][]byte{
		"too short":         {1, 2, 3},
		"zero first offset": {0, 0, 0, 0},
		"misaligned":        {6, 0, 0, 0, 0, 0},
		"first past end":    {8, 0, 0, 0},
		"non-monotonic":     {8, 0, 0, 0, 7, 0, 0, 0},
		"offset past end":   {8, 0, 0, 0, 20, 0, 0, 0, 1},
		"count over max":    overMax,
	}
	for name, buf := range cases {
		t.Run(name, func(t *testing.T) {
			err := (&enginev1.ProgressiveTransactionList{}).UnmarshalSSZ(buf)
			require.NotNil(t, err)
			if name == "count over max" {
				require.ErrorIs(t, err, ssz.ErrListTooBig)
			}
		})
	}
}

func TestProgressiveTransactionList_ZeroLengthElements(t *testing.T) {
	// Byte-exact encodings observed from the generated fastssz and methodical
	// code for the same inputs.
	mixed := mustList(t, txFixtures["mixed_empty"])
	require.DeepEqual(t, []byte{
		0x10, 0, 0, 0, 0x10, 0, 0, 0, 0x12, 0, 0, 0, 0x12, 0, 0, 0,
		0x01, 0x02, 0x03,
	}, mustMarshal(t, mixed))

	allEmpty := mustList(t, txFixtures["all_empty"])
	require.DeepEqual(t, []byte{0x0c, 0, 0, 0, 0x0c, 0, 0, 0, 0x0c, 0, 0, 0}, mustMarshal(t, allEmpty))

	got := &enginev1.ProgressiveTransactionList{}
	require.NoError(t, got.UnmarshalSSZ(mustMarshal(t, allEmpty)))
	require.Equal(t, 3, got.Len())
	for i := 0; i < 3; i++ {
		require.NotNil(t, got.Get(i))
		require.Equal(t, 0, len(got.Get(i)))
	}
}

func TestProgressiveTransactionList_HashTreeRootDifferential(t *testing.T) {
	check := func(t *testing.T, txs [][]byte) {
		l := mustList(t, txs)
		root, err := l.HashTreeRoot()
		require.NoError(t, err)
		want, err := wrappers.TransactionsRootProgressive(txs)
		require.NoError(t, err)
		require.Equal(t, want, root)

		decoded := &enginev1.ProgressiveTransactionList{}
		require.NoError(t, decoded.UnmarshalSSZ(mustMarshal(t, l)))
		again, err := decoded.HashTreeRoot()
		require.NoError(t, err)
		require.Equal(t, want, again)

		b, err := enginev1.NewTransactionListBuilder(nil)
		require.NoError(t, err)
		require.NoError(t, b.Append(txs...))
		built, err := b.Build()
		require.NoError(t, err)
		viaBuilder, err := built.HashTreeRoot()
		require.NoError(t, err)
		require.Equal(t, want, viaBuilder)
	}
	for name, txs := range txFixtures {
		t.Run(name, func(t *testing.T) { check(t, txs) })
	}
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 64; i++ {
		txs := randomTxs(r, r.Intn(24), 300)
		check(t, txs)
		require.DeepEqual(t, wireEncode(txs), mustMarshal(t, mustList(t, txs)))
	}
}

func TestProgressiveTransactionList_Copy(t *testing.T) {
	l := mustList(t, txFixtures["many"])
	c := l.Copy()
	require.DeepEqual(t, l, c)
	c.Data[0] ^= 0xff
	require.DeepEqual(t, txFixtures["many"], l.Slice())

	var nilList *enginev1.ProgressiveTransactionList
	require.IsNil(t, nilList.Copy())
}

func FuzzProgressiveTransactionList_Unmarshal(f *testing.F) {
	for _, txs := range txFixtures {
		f.Add(wireEncode(txs))
	}
	f.Add([]byte{0, 0, 0, 0})
	f.Add([]byte{8, 0, 0, 0, 7, 0, 0, 0})
	f.Add([]byte{8, 0, 0, 0, 20, 0, 0, 0, 1})
	f.Fuzz(func(t *testing.T, buf []byte) {
		l := &enginev1.ProgressiveTransactionList{}
		if err := l.UnmarshalSSZ(buf); err != nil {
			return
		}
		re, err := l.MarshalSSZ()
		require.NoError(t, err)
		require.Equal(t, true, bytes.Equal(re, buf), "re-marshal must reproduce the input")
		n := 0
		if len(buf) > 0 {
			n = int(binary.LittleEndian.Uint32(buf)) / 4
		}
		require.Equal(t, n, l.Len())
		_, err = l.HashTreeRoot()
		require.NoError(t, err)
		var joined []byte
		for i := 0; i < l.Len(); i++ {
			tx := l.Get(i)
			require.NotNil(t, tx)
			joined = append(joined, tx...)
		}
		require.Equal(t, true, bytes.Equal(joined, buf[4*n:]), "elements must tile the data")

		// A builder seeded from any valid list rebuilds it byte for byte.
		b, err := enginev1.NewTransactionListBuilder(l)
		require.NoError(t, err)
		rebuilt, err := b.Build()
		require.NoError(t, err)
		require.Equal(t, true, bytes.Equal(re, mustMarshal(t, rebuilt)))
	})
}
