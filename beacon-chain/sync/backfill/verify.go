package backfill

import (
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/network/forks"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

var errInvalidBatchChain = errors.New("parent_root of block does not match the previous block's root")
var errProposerIndexTooHigh = errors.New("proposer index not present in origin state")
var errUnknownDomain = errors.New("runtime error looking up signing domain for fork")

// verifiedROBlocks represents a slice of blocks that have passed signature verification.
type verifiedROBlocks []blocks.ROBlock

func (v verifiedROBlocks) blobIdents(retentionStart primitives.Slot) ([]blobSummary, error) {
	// early return if the newest block is outside the retention window
	if len(v) > 0 && v[len(v)-1].Block().Slot() < retentionStart {
		return nil, nil
	}
	bs := make([]blobSummary, 0)
	for i := range v {
		if v[i].Block().Slot() < retentionStart {
			continue
		}
		if v[i].Block().Version() < version.Deneb {
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

// TODO: rewrite this to use ROBlock.
func (vr verifier) verify(blks []interfaces.ReadOnlySignedBeaconBlock) (verifiedROBlocks, error) {
	var err error
	result := make([]blocks.ROBlock, len(blks))
	sigSet := bls.NewSet()
	for i := range blks {
		result[i], err = blocks.NewROBlock(blks[i])
		if err != nil {
			return nil, err
		}
		if i > 0 && result[i-1].Root() != result[i].Block().ParentRoot() {
			p, b := result[i-1], result[i]
			return nil, errors.Wrapf(errInvalidBatchChain,
				"slot %d parent_root=%#x, slot %d root=%#x",
				b.Block().Slot(), b.Block().ParentRoot(),
				p.Block().Slot(), p.Root())
		}
		set, err := vr.blockSignatureBatch(result[i])
		if err != nil {
			return nil, err
		}
		sigSet.Join(set)
	}
	v, err := sigSet.Verify()
	if err != nil {
		return nil, errors.Wrap(err, "block signature verification error")
	}
	if !v {
		return nil, errors.New("batch block signature verification failed")
	}
	return result, nil
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
	dc, err := newDomainCache(vr, params.BeaconConfig().DomainBeaconProposer,
		forks.NewOrderedSchedule(params.BeaconConfig()))
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
	fsched      forks.OrderedSchedule
	forkDomains map[[4]byte][]byte
	dType       [bls.DomainByteLength]byte
}

func newDomainCache(vRoot []byte, dType [bls.DomainByteLength]byte, fsched forks.OrderedSchedule) (*domainCache, error) {
	dc := &domainCache{
		fsched:      fsched,
		forkDomains: make(map[[4]byte][]byte),
		dType:       dType,
	}
	for _, entry := range fsched {
		d, err := signing.ComputeDomain(dc.dType, entry.Version[:], vRoot)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to pre-compute signing domain for fork version=%#x", entry.Version)
		}
		dc.forkDomains[entry.Version] = d
	}
	return dc, nil
}

func (dc *domainCache) forEpoch(e primitives.Epoch) ([]byte, error) {
	fork, err := dc.fsched.VersionForEpoch(e)
	if err != nil {
		return nil, err
	}
	d, ok := dc.forkDomains[fork]
	if !ok {
		return nil, errors.Wrapf(errUnknownDomain, "fork version=%#x, epoch=%d", fork, e)
	}
	return d, nil
}
