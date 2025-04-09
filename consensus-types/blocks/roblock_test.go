package blocks

import (
	"sort"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestROBlockSorting(t *testing.T) {
	one := bytesutil.ToBytes32(bytesutil.PadTo([]byte{1}, 32))
	two := bytesutil.ToBytes32(bytesutil.PadTo([]byte{2}, 32))
	cases := []struct {
		name   string
		ros    []ROBlock
		sorted []ROBlock
	}{
		{
			name:   "1 item",
			ros:    []ROBlock{testROBlock(t, 1, [32]byte{})},
			sorted: []ROBlock{testROBlock(t, 1, [32]byte{})},
		},
		{
			name:   "2 items, sorted",
			ros:    []ROBlock{testROBlock(t, 1, [32]byte{}), testROBlock(t, 2, [32]byte{})},
			sorted: []ROBlock{testROBlock(t, 1, [32]byte{}), testROBlock(t, 2, [32]byte{})},
		},
		{
			name:   "2 items, reversed",
			ros:    []ROBlock{testROBlock(t, 2, [32]byte{}), testROBlock(t, 1, [32]byte{})},
			sorted: []ROBlock{testROBlock(t, 1, [32]byte{}), testROBlock(t, 2, [32]byte{})},
		},
		{
			name: "3 items, reversed, with tie breaker",
			ros: []ROBlock{
				testROBlock(t, 2, two),
				testROBlock(t, 2, one),
				testROBlock(t, 1, [32]byte{}),
			},
			sorted: []ROBlock{
				testROBlock(t, 1, [32]byte{}),
				testROBlock(t, 2, one),
				testROBlock(t, 2, two),
			},
		},
		{
			name: "5 items, reversed, with double root tie",
			ros: []ROBlock{
				testROBlock(t, 0, one),
				testROBlock(t, 2, two),
				testROBlock(t, 2, one),
				testROBlock(t, 2, two),
				testROBlock(t, 2, one),
				testROBlock(t, 1, [32]byte{}),
			},
			sorted: []ROBlock{
				testROBlock(t, 0, one),
				testROBlock(t, 1, [32]byte{}),
				testROBlock(t, 2, one),
				testROBlock(t, 2, one),
				testROBlock(t, 2, two),
				testROBlock(t, 2, two),
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sort.Sort(ROBlockSlice(c.ros))
			for i := 0; i < len(c.sorted); i++ {
				require.Equal(t, c.sorted[i].Block().Slot(), c.ros[i].Block().Slot())
				require.Equal(t, c.sorted[i].Root(), c.ros[i].Root())
			}
		})
	}
}

func testROBlock(t *testing.T, slot primitives.Slot, root [32]byte) ROBlock {
	b, err := NewSignedBeaconBlock(&eth.SignedBeaconBlock{Block: &eth.BeaconBlock{
		Body: &eth.BeaconBlockBody{},
		Slot: slot,
	}})
	require.NoError(t, err)
	return ROBlock{
		ReadOnlySignedBeaconBlock: b,
		root:                      root,
	}
}

func TestROBlockNilChecks(t *testing.T) {
	cases := []struct {
		name  string
		bfunc func(t *testing.T) interfaces.SignedBeaconBlock
		err   error
		root  []byte
	}{
		{
			name: "happy path",
			bfunc: func(t *testing.T) interfaces.SignedBeaconBlock {
				b, err := NewSignedBeaconBlock(hydrateSignedBeaconBlock())
				require.NoError(t, err)
				return b
			},
		},
		{
			name: "happy path - with root",
			bfunc: func(t *testing.T) interfaces.SignedBeaconBlock {
				b, err := NewSignedBeaconBlock(hydrateSignedBeaconBlock())
				require.NoError(t, err)
				return b
			},
			root: bytesutil.PadTo([]byte("sup"), 32),
		},
		{
			name: "nil signed block",
			bfunc: func(t *testing.T) interfaces.SignedBeaconBlock {
				return nil
			},
			err: ErrNilSignedBeaconBlock,
		},
		{
			name: "nil signed block - with root",
			bfunc: func(t *testing.T) interfaces.SignedBeaconBlock {
				return nil
			},
			err:  ErrNilSignedBeaconBlock,
			root: bytesutil.PadTo([]byte("sup"), 32),
		},
		{
			name: "nil inner block",
			bfunc: func(t *testing.T) interfaces.SignedBeaconBlock {
				return &SignedBeaconBlock{
					version:   version.Deneb,
					block:     nil,
					signature: bytesutil.ToBytes96(nil),
				}
			},
			err: ErrNilSignedBeaconBlock,
		},
		{
			name: "nil inner block",
			bfunc: func(t *testing.T) interfaces.SignedBeaconBlock {
				return &SignedBeaconBlock{
					version:   version.Deneb,
					block:     nil,
					signature: bytesutil.ToBytes96(nil),
				}
			},
			err: ErrNilSignedBeaconBlock,
		},
		{
			name: "nil block body",
			bfunc: func(t *testing.T) interfaces.SignedBeaconBlock {
				bb := &BeaconBlock{
					version:       version.Deneb,
					slot:          0,
					proposerIndex: 0,
					parentRoot:    bytesutil.ToBytes32(nil),
					stateRoot:     bytesutil.ToBytes32(nil),
					body:          nil,
				}
				return &SignedBeaconBlock{
					version:   version.Deneb,
					block:     bb,
					signature: bytesutil.ToBytes96(nil),
				}
			},
			err: ErrNilSignedBeaconBlock,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := c.bfunc(t)
			var err error
			if len(c.root) == 0 {
				_, err = NewROBlock(b)
			} else {
				_, err = NewROBlockWithRoot(b, bytesutil.ToBytes32(c.root))
			}
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
				return
			} else {
				require.NoError(t, err)
			}
		})
	}
}
