package backfill

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

var (
	errInvalidBlocks        = errors.New("block validation failure")
	errInvalidBatchChain    = errors.Wrap(errInvalidBlocks, "parent_root of block does not match the previous block's root")
	errProposerIndexTooHigh = errors.Wrap(errInvalidBlocks, "proposer index not present in origin state")
	errUnknownDomain        = errors.Wrap(errInvalidBlocks, "runtime error looking up signing domain for fork")
	errBatchSignatureFailed = errors.Wrap(errInvalidBlocks, "failed to verify block signature in batch")
	errInvalidSignatureData = errors.Wrap(errInvalidBlocks, "could not verify signatures in block batch due to invalid signature data")

	errEmptyVerificationSet = errors.New("no blocks to verify in batch")
)

// verifiedROBlocks represents a slice of blocks that have passed signature verification.
type verifiedROBlocks []blocks.ROBlock

func (v verifiedROBlocks) blobIdents(needed func() das.CurrentNeeds) ([]blobSummary, error) {
	if len(v) == 0 {
		return nil, nil
	}

	needs := needed()
	bs := make([]blobSummary, 0)
	for i := range v {
		slot := v[i].Block().Slot()
		if !needs.Blob.At(slot) {
			continue
		}
		c, err := v[i].Block().Body().BlobKzgCommitments()
		if err != nil {
			return nil, errors.Wrapf(err, "unexpected error checking commitments for block root %#x", v[i].Root())
		}
		if len(c) == 0 {
			continue
		}
		for ci := range c {
			bs = append(bs, blobSummary{
				blockRoot: v[i].Root(), signature: v[i].Signature(),
				index: uint64(ci), commitment: bytesutil.ToBytes48(c[ci])})
		}
	}
	return bs, nil
}

type verifier struct {
	keys   [][fieldparams.BLSPubkeyLength]byte
	maxVal primitives.ValidatorIndex
	domain *domainCache
}

func (vr verifier) verify(blks []blocks.ROBlock) (verifiedROBlocks, error) {
	if len(blks) == 0 {
		// Returning an error here simplifies handling in the caller.
		// errEmptyVerificationSet should not cause the peer to be downscored.
		return nil, errEmptyVerificationSet
	}
	sigSet := bls.NewSet()
	for i := range blks {
		if i > 0 && blks[i-1].Root() != blks[i].Block().ParentRoot() {
			p, b := blks[i-1], blks[i]
			return nil, errors.Wrapf(errInvalidBatchChain,
				"slot %d parent_root=%#x, slot %d root=%#x",
				b.Block().Slot(), b.Block().ParentRoot(),
				p.Block().Slot(), p.Root())
		}
		set, err := vr.blockSignatureBatch(blks[i])
		if err != nil {
			return nil, errors.Wrap(err, "block signature batch")
		}
		sigSet.Join(set)
	}
	v, err := sigSet.Verify()
	if err != nil {
		// The blst wrapper does not give us checkable errors, so we "reverse wrap"
		// the error string to make it checkable for shouldDownscore.
		return nil, errors.Wrap(errInvalidSignatureData, err.Error())
	}
	if !v {
		return nil, errBatchSignatureFailed
	}
	return blks, nil
}

func (vr verifier) blockSignatureBatch(b blocks.ROBlock) (*bls.SignatureBatch, error) {
	pidx := b.Block().ProposerIndex()
	if pidx > vr.maxVal {
		return nil, errProposerIndexTooHigh
	}
	dom, err := vr.domain.forEpoch(slots.ToEpoch(b.Block().Slot()))
	if err != nil {
		return nil, err
	}
	sig := b.Signature()
	pk := vr.keys[pidx][:]
	root := b.Root()
	rootF := func() ([32]byte, error) { return root, nil }
	return signing.BlockSignatureBatch(pk, sig[:], dom, rootF)
}

func newBackfillVerifier(vr []byte, keys [][fieldparams.BLSPubkeyLength]byte) (*verifier, error) {
	dc, err := newDomainCache(vr, params.BeaconConfig().DomainBeaconProposer)
	if err != nil {
		return nil, err
	}
	v := &verifier{
		keys:   keys,
		domain: dc,
	}
	v.maxVal = primitives.ValidatorIndex(len(v.keys) - 1)
	return v, nil
}

// domainCache provides a fast signing domain lookup by epoch.
type domainCache struct {
	forkDomains map[[4]byte][]byte
	dType       [bls.DomainByteLength]byte
}

func newDomainCache(vRoot []byte, dType [bls.DomainByteLength]byte) (*domainCache, error) {
	dc := &domainCache{
		forkDomains: make(map[[4]byte][]byte),
		dType:       dType,
	}
	for _, entry := range params.SortedForkSchedule() {
		d, err := signing.ComputeDomain(dc.dType, entry.ForkVersion[:], vRoot)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to pre-compute signing domain for fork version=%#x", entry.ForkVersion)
		}
		dc.forkDomains[entry.ForkVersion] = d
	}
	return dc, nil
}

func (dc *domainCache) forEpoch(e primitives.Epoch) ([]byte, error) {
	fork, err := params.Fork(e)
	if err != nil {
		return nil, err
	}
	d, ok := dc.forkDomains[[4]byte(fork.CurrentVersion)]
	if !ok {
		return nil, errors.Wrapf(errUnknownDomain, "fork version=%#x, epoch=%d", fork, e)
	}
	return d, nil
}
