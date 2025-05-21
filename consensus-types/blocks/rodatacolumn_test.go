package blocks

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
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
		DataColumnSidecar: &ethpb.DataColumnSidecar{
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
		DataColumnSidecar: &ethpb.DataColumnSidecar{
			SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
				Header: &ethpb.BeaconBlockHeader{
					ParentRoot: root[:],
				},
			},
		},
	}

	assert.Equal(t, root, dataColumn.ParentRoot())
}

func TestDataColumn_ProposerIndex(t *testing.T) {
	proposerIndex := primitives.ValidatorIndex(1)
	dataColumn := &RODataColumn{
		DataColumnSidecar: &ethpb.DataColumnSidecar{
			SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
				Header: &ethpb.BeaconBlockHeader{
					ProposerIndex: proposerIndex,
				},
			},
		},
	}

	assert.Equal(t, proposerIndex, dataColumn.ProposerIndex())
}
