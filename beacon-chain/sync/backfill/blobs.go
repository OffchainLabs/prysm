package backfill

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/das"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filesystem"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
)

var (
	errUnexpectedResponseSize    = errors.New("received more blobs than expected for the requested range")
	errUnexpectedCommitment      = errors.New("BlobSidecar commitment does not match block")
	errUnexpectedResponseContent = errors.New("BlobSidecar response does not include expected values in expected order")
	errBatchVerifierMismatch     = errors.New("the list of blocks passed to the availability check does not match what was verified")
)

type blobSummary struct {
	blockRoot  [32]byte
	index      uint64
	commitment [48]byte
	signature  [fieldparams.BLSSignatureLength]byte
}

type blobSyncConfig struct {
	retentionStart primitives.Slot
	nbv            verification.NewBlobVerifier
	store          *filesystem.BlobStorage
}

func newBlobSync(current primitives.Slot, vbs verifiedROBlocks, cfg *blobSyncConfig) (*blobSync, error) {
	expected, err := vbs.blobIdents(cfg.retentionStart)
	if err != nil {
		return nil, err
	}
	bbv := newBlobBatchVerifier(cfg.nbv)
	as := das.NewLazilyPersistentStore(cfg.store, bbv)
	return &blobSync{current: current, expected: expected, bbv: bbv, store: as}, nil
}

type blobVerifierMap map[[32]byte][fieldparams.MaxBlobsPerBlock]verification.BlobVerifier

type blobSync struct {
	store    das.AvailabilityStore
	expected []blobSummary
	next     int
	bbv      *blobBatchVerifier
	current  primitives.Slot
}

func (bs *blobSync) blobsNeeded() int {
	return len(bs.expected) - bs.next
}

func (bs *blobSync) validateNext(rb blocks.ROBlob) error {
	if bs.next >= len(bs.expected) {
		return errUnexpectedResponseSize
	}
	next := bs.expected[bs.next]
	bs.next += 1
	// Get the super cheap verifications out of the way before we init a verifier.
	if next.blockRoot != rb.BlockRoot() {
		return errors.Wrapf(errUnexpectedResponseContent, "next expected root=%#x, saw=%#x", next.blockRoot, rb.BlockRoot())
	}
	if next.index != rb.Index {
		return errors.Wrapf(errUnexpectedResponseContent, "next expected root=%#x, saw=%#x for root=%#x", next.index, rb.Index, next.blockRoot)
	}
	if next.commitment != bytesutil.ToBytes48(rb.KzgCommitment) {
		return errors.Wrapf(errUnexpectedResponseContent, "next expected commitment=%#x, saw=%#x for root=%#x", next.commitment, rb.KzgCommitment, rb.BlockRoot())
	}

	if bytesutil.ToBytes96(rb.SignedBlockHeader.Signature) != next.signature {
		return verification.ErrInvalidProposerSignature
	}
	v := bs.bbv.newVerifier(rb)
	if err := v.BlobIndexInBounds(); err != nil {
		return err
	}
	v.SatisfyRequirement(verification.RequireValidProposerSignature)
	if err := v.SidecarInclusionProven(); err != nil {
		return err
	}
	if err := v.SidecarKzgProofVerified(); err != nil {
		return err
	}
	if err := bs.store.Persist(bs.current, rb); err != nil {
		return err
	}

	return nil
}

func newBlobBatchVerifier(nbv verification.NewBlobVerifier) *blobBatchVerifier {
	return &blobBatchVerifier{newBlobVerifier: nbv, verifiers: make(blobVerifierMap)}
}

type blobBatchVerifier struct {
	newBlobVerifier verification.NewBlobVerifier
	verifiers       blobVerifierMap
}

func (bbv *blobBatchVerifier) newVerifier(rb blocks.ROBlob) verification.BlobVerifier {
	m := bbv.verifiers[rb.BlockRoot()]
	m[rb.Index] = bbv.newBlobVerifier(rb, verification.BackfillBlobSidecarRequirements)
	bbv.verifiers[rb.BlockRoot()] = m
	return m[rb.Index]
}

func (bbv *blobBatchVerifier) VerifiedROBlobs(_ context.Context, blk blocks.ROBlock, _ []blocks.ROBlob) ([]blocks.VerifiedROBlob, error) {
	m, ok := bbv.verifiers[blk.Root()]
	if !ok {
		return nil, errors.Wrapf(verification.ErrMissingVerification, "no record of verifiers for root %#x", blk.Root())
	}
	c, err := blk.Block().Body().BlobKzgCommitments()
	if err != nil {
		return nil, errors.Wrapf(errUnexpectedCommitment, "error reading commitments from block root %#x", blk.Root())
	}
	vbs := make([]blocks.VerifiedROBlob, len(c))
	for i := range c {
		if m[i] == nil {
			return nil, errors.Wrapf(errBatchVerifierMismatch, "do not have verifier for block root %#x idx %d", blk.Root(), i)
		}
		vb, err := m[i].VerifiedROBlob()
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(vb.KzgCommitment, c[i]) {
			return nil, errors.Wrapf(errBatchVerifierMismatch, "commitments do not match, verified=%#x da check=%#x for root %#x", vb.KzgCommitment, c[i], vb.BlockRoot())
		}
		vbs[i] = vb
	}
	return vbs, nil
}

var _ das.BlobBatchVerifier = &blobBatchVerifier{}
