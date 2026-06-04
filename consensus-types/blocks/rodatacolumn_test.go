package blocks

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestNewRODataColumnWithAndWithoutRoot(t *testing.T) {
	cases := []struct {
		name   string
		dcFunc func(t *testing.T) *ethpb.DataColumnSidecar
		err    error
		root   []byte
	}{
		{
			name: "nil signed data column",
			dcFunc: func(t *testing.T) *ethpb.DataColumnSidecar {
				return nil
			},
			err:  errNilDataColumn,
			root: bytesutil.PadTo([]byte("sup"), fieldparams.RootLength),
		},
		{
			name: "nil signed block header",
			dcFunc: func(t *testing.T) *ethpb.DataColumnSidecar {
				return &ethpb.DataColumnSidecar{
					SignedBlockHeader: nil,
				}
			},
			err:  errNilBlockHeader,
			root: bytesutil.PadTo([]byte("sup"), fieldparams.RootLength),
		},
		{
			name: "nil inner header",
			dcFunc: func(t *testing.T) *ethpb.DataColumnSidecar {
				return &ethpb.DataColumnSidecar{
					SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
						Header: nil,
					},
				}
			},
			err:  errNilBlockHeader,
			root: bytesutil.PadTo([]byte("sup"), fieldparams.RootLength),
		},
		{
			name: "nil signature",
			dcFunc: func(t *testing.T) *ethpb.DataColumnSidecar {
				return &ethpb.DataColumnSidecar{
					SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
						Header: &ethpb.BeaconBlockHeader{
							ParentRoot: make([]byte, fieldparams.RootLength),
							StateRoot:  make([]byte, fieldparams.RootLength),
							BodyRoot:   make([]byte, fieldparams.RootLength),
						},
						Signature: nil,
					},
				}
			},
			err:  errMissingBlockSignature,
			root: bytesutil.PadTo([]byte("sup"), fieldparams.RootLength),
		},
		{
			name: "nominal",
			dcFunc: func(t *testing.T) *ethpb.DataColumnSidecar {
				return &ethpb.DataColumnSidecar{
					SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
						Header: &ethpb.BeaconBlockHeader{
							ParentRoot: make([]byte, fieldparams.RootLength),
							StateRoot:  make([]byte, fieldparams.RootLength),
							BodyRoot:   make([]byte, fieldparams.RootLength),
						},
						Signature: make([]byte, fieldparams.BLSSignatureLength),
					},
				}
			},
			root: bytesutil.PadTo([]byte("sup"), fieldparams.RootLength),
		},
	}
	for _, c := range cases {
		t.Run(c.name+" NewRODataColumn", func(t *testing.T) {
			dataColumnSidecar := c.dcFunc(t)
			roDataColumnSidecar, err := NewRODataColumn(dataColumnSidecar)

			if c.err != nil {
				require.ErrorIs(t, err, c.err)
				return
			}

			require.NoError(t, err)
			hr, err := dataColumnSidecar.SignedBlockHeader.Header.HashTreeRoot()
			require.NoError(t, err)
			require.Equal(t, hr, roDataColumnSidecar.BlockRoot())
		})

		if len(c.root) == 0 {
			continue
		}

		t.Run(c.name+" NewRODataColumnWithRoot", func(t *testing.T) {
			b := c.dcFunc(t)

			// We want the same validation when specifying a root.
			bl, err := NewRODataColumnWithRoot(b, bytesutil.ToBytes32(c.root))
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
				return
			}

			assert.Equal(t, bytesutil.ToBytes32(c.root), bl.BlockRoot())
		})
	}
}

func TestDataColumn_BlockRoot(t *testing.T) {
	root := [fieldparams.RootLength]byte{1}
	dataColumn := &RODataColumn{root: root}
	assert.Equal(t, root, dataColumn.BlockRoot())
}

func TestDataColumn_Slot(t *testing.T) {
	slot := primitives.Slot(1)

	dataColumn := &RODataColumn{
		fulu: &ethpb.DataColumnSidecar{
			SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
				Header: &ethpb.BeaconBlockHeader{
					Slot: slot,
				},
			},
		},
	}

	assert.Equal(t, slot, dataColumn.Slot())
}

func TestDataColumn_ParentRoot(t *testing.T) {
	root := [fieldparams.RootLength]byte{1}
	dataColumn := &RODataColumn{
		fulu: &ethpb.DataColumnSidecar{
			SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
				Header: &ethpb.BeaconBlockHeader{
					ParentRoot: root[:],
				},
			},
		},
	}

	parentRoot, err := dataColumn.ParentRoot()
	assert.NoError(t, err)
	assert.Equal(t, root, parentRoot)
}

func TestDataColumn_ProposerIndex(t *testing.T) {
	proposerIndex := primitives.ValidatorIndex(1)
	dataColumn := &RODataColumn{
		fulu: &ethpb.DataColumnSidecar{
			SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
				Header: &ethpb.BeaconBlockHeader{
					ProposerIndex: proposerIndex,
				},
			},
		},
	}

	pi, err := dataColumn.ProposerIndex()
	assert.NoError(t, err)
	assert.Equal(t, proposerIndex, pi)
}

func TestRODataColumnsToCellProofBundles(t *testing.T) {
	sidecars := []RODataColumn{
		{fulu: &ethpb.DataColumnSidecar{
			Index:          1,
			Column:         sizedSlices(2, 2048, 1),
			KzgCommitments: sizedSlices(2, 48, 10),
			KzgProofs:      sizedSlices(2, 48, 20),
		}},
		{fulu: &ethpb.DataColumnSidecar{
			Index:          2,
			Column:         sizedSlices(3, 2048, 30),
			KzgCommitments: sizedSlices(3, 48, 40),
			KzgProofs:      sizedSlices(3, 48, 50),
		}},
	}

	got, err := RODataColumnsToCellProofBundles(sidecars)
	require.NoError(t, err)
	require.Equal(t, 5, len(got))

	// First bundle pairs the first sidecar's first cell/commitment/proof.
	require.Equal(t, uint64(1), got[0].ColumnIndex)
	require.DeepEqual(t, sidecars[0].Column()[0], got[0].Cell)
	require.DeepEqual(t, sidecars[0].fulu.KzgCommitments[0], got[0].Commitment)
	require.DeepEqual(t, sidecars[0].KzgProofs()[0], got[0].Proof)

	// Bundles for the second sidecar carry its own column index.
	require.Equal(t, uint64(2), got[2].ColumnIndex)
}

func TestRODataColumnsToCellProofBundlesLengthMismatch(t *testing.T) {
	cases := []struct {
		name string
		dc   *ethpb.DataColumnSidecar
	}{
		{
			name: "fewer commitments than cells",
			dc: &ethpb.DataColumnSidecar{
				Column:         sizedSlices(3, 2048, 1),
				KzgCommitments: sizedSlices(2, 48, 10),
				KzgProofs:      sizedSlices(3, 48, 20),
			},
		},
		{
			name: "fewer proofs than cells",
			dc: &ethpb.DataColumnSidecar{
				Column:         sizedSlices(3, 2048, 1),
				KzgCommitments: sizedSlices(3, 48, 10),
				KzgProofs:      sizedSlices(2, 48, 20),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// A mismatched sidecar must surface an error rather than silently
			// returning a partial/empty result.
			got, err := RODataColumnsToCellProofBundles([]RODataColumn{{fulu: tc.dc}})
			require.NotNil(t, err)
			require.Equal(t, 0, len(got))
		})
	}
}
