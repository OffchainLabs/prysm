package blocks

import (
	"testing"

	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestROBlobNilChecks(t *testing.T) {
	cases := []struct {
		name  string
		bfunc func(t *testing.T) *ethpb.BlobSidecar
		err   error
		root  []byte
	}{
		{
			name: "nil signed blob",
			bfunc: func(t *testing.T) *ethpb.BlobSidecar {
				return nil
			},
			err:  errNilBlob,
			root: bytesutil.PadTo([]byte("sup"), 32),
		},
		{
			name: "nil signed block header",
			bfunc: func(t *testing.T) *ethpb.BlobSidecar {
				return &ethpb.BlobSidecar{
					SignedBlockHeader: nil,
				}
			},
			err:  errNilBlockHeader,
			root: bytesutil.PadTo([]byte("sup"), 32),
		},
		{
			name: "nil inner header",
			bfunc: func(t *testing.T) *ethpb.BlobSidecar {
				return &ethpb.BlobSidecar{
					SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
						Header: nil,
					},
				}
			},
			err:  errNilBlockHeader,
			root: bytesutil.PadTo([]byte("sup"), 32),
		},
		{
			name: "nil signature",
			bfunc: func(t *testing.T) *ethpb.BlobSidecar {
				return &ethpb.BlobSidecar{
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
			root: bytesutil.PadTo([]byte("sup"), 32),
		},
	}
	for _, c := range cases {
		t.Run(c.name+" NewROBlob", func(t *testing.T) {
			b := c.bfunc(t)
			bl, err := NewROBlob(b)
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
			} else {
				require.NoError(t, err)
				hr, err := b.SignedBlockHeader.HashTreeRoot()
				require.NoError(t, err)
				require.Equal(t, hr, bl.BlockRoot())
			}
		})
		if len(c.root) == 0 {
			continue
		}
		t.Run(c.name+" NewROBlobWithRoot", func(t *testing.T) {
			b := c.bfunc(t)
			// We want the same validation when specifying a root.
			bl, err := NewROBlobWithRoot(b, bytesutil.ToBytes32(c.root))
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
			} else {
				assert.Equal(t, bytesutil.ToBytes32(c.root), bl.BlockRoot())
			}
		})
	}
}

func TestBlockRoot(t *testing.T) {
	root := [32]byte{1}
	blob := &ROBlob{
		root: root,
	}
	assert.Equal(t, root, blob.BlockRoot())
}

func TestSlot(t *testing.T) {
	slot := primitives.Slot(1)
	blob := &ROBlob{
		BlobSidecar: &ethpb.BlobSidecar{
			SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
				Header: &ethpb.BeaconBlockHeader{
					Slot: slot,
				},
			},
		},
	}
	assert.Equal(t, slot, blob.Slot())
}

func TestParentRoot(t *testing.T) {
	root := [32]byte{1}
	blob := &ROBlob{
		BlobSidecar: &ethpb.BlobSidecar{
			SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
				Header: &ethpb.BeaconBlockHeader{
					ParentRoot: root[:],
				},
			},
		},
	}
	assert.Equal(t, root, blob.ParentRoot())
}

func TestBodyRoot(t *testing.T) {
	root := [32]byte{1}
	blob := &ROBlob{
		BlobSidecar: &ethpb.BlobSidecar{
			SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
				Header: &ethpb.BeaconBlockHeader{
					BodyRoot: root[:],
				},
			},
		},
	}
	assert.Equal(t, root, blob.BodyRoot())
}

func TestProposeIndex(t *testing.T) {
	index := primitives.ValidatorIndex(1)
	blob := &ROBlob{
		BlobSidecar: &ethpb.BlobSidecar{
			SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
				Header: &ethpb.BeaconBlockHeader{
					ProposerIndex: index,
				},
			},
		},
	}
	assert.Equal(t, index, blob.ProposerIndex())
}
