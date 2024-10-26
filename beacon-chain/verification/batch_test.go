package verification

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/stretchr/testify/require"
)

func TestBatchVerifier(t *testing.T) {
	ctx := context.Background()
	mockCV := func(err error) roblobCommitmentVerifier {
		return func(...blocks.ROBlob) error {
			return err
		}
	}
	var invCmtErr = errors.New("mock invalid commitment")
	type vbcbt func() (blocks.VerifiedROBlob, error)
	vbcb := func(bl blocks.ROBlob, err error) vbcbt {
		return func() (blocks.VerifiedROBlob, error) {
			return blocks.VerifiedROBlob{ROBlob: bl}, err
		}
	}
	cases := []struct {
		name   string
		nv     func() NewBlobVerifier
		cv     roblobCommitmentVerifier
		bandb  func(t *testing.T, n int) (blocks.ROBlock, []blocks.ROBlob)
		err    error
		nblobs int
		reqs   []Requirement
	}{
		{
			name: "no blobs",
			bandb: func(t *testing.T, nb int) (blocks.ROBlock, []blocks.ROBlob) {
				return util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, 0, nb)
			},
			nv: func() NewBlobVerifier {
				return func(bl blocks.ROBlob, reqs []Requirement) BlobVerifier {
					return &MockBlobVerifier{CbVerifiedROBlob: vbcb(bl, nil)}
				}
			},
			nblobs: 0,
		},
		{
			name: "happy path",
			nv: func() NewBlobVerifier {
				return func(bl blocks.ROBlob, reqs []Requirement) BlobVerifier {
					return &MockBlobVerifier{CbVerifiedROBlob: vbcb(bl, nil)}
				}
			},
			bandb: func(t *testing.T, nb int) (blocks.ROBlock, []blocks.ROBlob) {
				return util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, 0, nb)
			},
			nblobs: 3,
		},
		{
			name: "partial batch",
			nv: func() NewBlobVerifier {
				return func(bl blocks.ROBlob, reqs []Requirement) BlobVerifier {
					return &MockBlobVerifier{CbVerifiedROBlob: vbcb(bl, nil)}
				}
			},
			bandb: func(t *testing.T, nb int) (blocks.ROBlock, []blocks.ROBlob) {
				// Add extra blobs to the block that we won't return
				blk, blbs := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, 0, nb+3)
				return blk, blbs[0:3]
			},
			nblobs: 3,
		},
		{
			name: "invalid commitment",
			nv: func() NewBlobVerifier {
				return func(bl blocks.ROBlob, reqs []Requirement) BlobVerifier {
					return &MockBlobVerifier{CbVerifiedROBlob: func() (blocks.VerifiedROBlob, error) {
						t.Fatal("Batch verifier should stop before this point")
						return blocks.VerifiedROBlob{}, nil
					}}
				}
			},
			cv: mockCV(invCmtErr),
			bandb: func(t *testing.T, nb int) (blocks.ROBlock, []blocks.ROBlob) {
				return util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, 0, nb)
			},
			err:    invCmtErr,
			nblobs: 1,
		},
		{
			name: "signature mismatch",
			nv: func() NewBlobVerifier {
				return func(bl blocks.ROBlob, reqs []Requirement) BlobVerifier {
					return &MockBlobVerifier{CbVerifiedROBlob: func() (blocks.VerifiedROBlob, error) {
						t.Fatal("Batch verifier should stop before this point")
						return blocks.VerifiedROBlob{}, nil
					}}
				}
			},
			bandb: func(t *testing.T, nb int) (blocks.ROBlock, []blocks.ROBlob) {
				blk, blbs := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, 0, nb)
				blbs[0].SignedBlockHeader.Signature = []byte("wrong")
				return blk, blbs
			},
			err:    ErrBatchSignatureMismatch,
			nblobs: 2,
		},
		{
			name: "root mismatch",
			nv: func() NewBlobVerifier {
				return func(bl blocks.ROBlob, reqs []Requirement) BlobVerifier {
					return &MockBlobVerifier{CbVerifiedROBlob: func() (blocks.VerifiedROBlob, error) {
						t.Fatal("Batch verifier should stop before this point")
						return blocks.VerifiedROBlob{}, nil
					}}
				}
			},
			bandb: func(t *testing.T, nb int) (blocks.ROBlock, []blocks.ROBlob) {
				blk, blbs := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, 0, nb)
				wr, err := blocks.NewROBlobWithRoot(blbs[0].BlobSidecar, bytesutil.ToBytes32([]byte("wrong")))
				require.NoError(t, err)
				blbs[0] = wr
				return blk, blbs
			},
			err:    ErrBatchBlockRootMismatch,
			nblobs: 1,
		},
		{
			name: "idx oob",
			nv: func() NewBlobVerifier {
				return func(bl blocks.ROBlob, reqs []Requirement) BlobVerifier {
					return &MockBlobVerifier{
						ErrBlobIndexInBounds: ErrBlobIndexInvalid,
						CbVerifiedROBlob: func() (blocks.VerifiedROBlob, error) {
							t.Fatal("Batch verifier should stop before this point")
							return blocks.VerifiedROBlob{}, nil
						}}
				}
			},
			bandb: func(t *testing.T, nb int) (blocks.ROBlock, []blocks.ROBlob) {
				return util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, 0, nb)
			},
			nblobs: 1,
			err:    ErrBlobIndexInvalid,
		},
		{
			name: "inclusion proof invalid",
			nv: func() NewBlobVerifier {
				return func(bl blocks.ROBlob, reqs []Requirement) BlobVerifier {
					return &MockBlobVerifier{
						ErrSidecarInclusionProven: ErrSidecarInclusionProofInvalid,
						CbVerifiedROBlob: func() (blocks.VerifiedROBlob, error) {
							t.Fatal("Batch verifier should stop before this point")
							return blocks.VerifiedROBlob{}, nil
						}}
				}
			},
			bandb: func(t *testing.T, nb int) (blocks.ROBlock, []blocks.ROBlob) {
				return util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, 0, nb)
			},
			nblobs: 1,
			err:    ErrSidecarInclusionProofInvalid,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			blk, blbs := c.bandb(t, c.nblobs)
			reqs := c.reqs
			if reqs == nil {
				reqs = InitsyncSidecarRequirements
			}
			bbv := NewBlobBatchVerifier(c.nv(), reqs)
			if c.cv == nil {
				bbv.verifyKzg = mockCV(nil)
			} else {
				bbv.verifyKzg = c.cv
			}
			vb, err := bbv.VerifiedROBlobs(ctx, blk, blbs)
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.nblobs, len(vb))
		})
	}
}
