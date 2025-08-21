package verification

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition"
	forkchoicetypes "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	lruwrpr "github.com/OffchainLabs/prysm/v6/cache/lru"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	lru "github.com/hashicorp/golang-lru"
	"github.com/sirupsen/logrus"
)

const (
	defaultSignatureCacheSize      = 256
	defaultInclusionProofCacheSize = 2
)

// validatorAtIndexer defines the method needed to retrieve a validator by its index.
// This interface is satisfied by state.BeaconState, but can also be satisfied by a cache.
type validatorAtIndexer interface {
	ValidatorAtIndex(idx primitives.ValidatorIndex) (*ethpb.Validator, error)
}

// signatureCache represents a type that can perform signature verification and cache the result so that it
// can be used when the same signature is seen in multiple places, like a SignedBeaconBlockHeader
// found in multiple BlobSidecars.
type signatureCache interface {
	// VerifySignature perform signature verification and caches the result.
	VerifySignature(sig signatureData, v validatorAtIndexer) (err error)
	// SignatureVerified accesses the result of a previous signature verification.
	SignatureVerified(sig signatureData) (bool, error)
}

// signatureData represents the set of parameters that together uniquely identify a signature observed on
// a beacon block. This is used as the key for the signature cache.
type signatureData struct {
	Root      [32]byte
	Parent    [32]byte
	Signature [96]byte
	Proposer  primitives.ValidatorIndex
	Slot      primitives.Slot
}

func (d signatureData) logFields() logrus.Fields {
	return logrus.Fields{
		"root":       fmt.Sprintf("%#x", d.Root),
		"parentRoot": fmt.Sprintf("%#x", d.Parent),
		"signature":  fmt.Sprintf("%#x", d.Signature),
		"proposer":   d.Proposer,
		"slot":       d.Slot,
	}
}

func newSigCache(vr []byte, size int, gf forkLookup) *sigCache {
	if gf == nil {
		gf = params.Fork
	}
	return &sigCache{Cache: lruwrpr.New(size), valRoot: vr, getFork: gf}
}

type sigCache struct {
	*lru.Cache
	valRoot []byte
	getFork forkLookup
}

type inclusionProofCache struct {
	*lru.Cache
}

func newInclusionProofCache(size int) *inclusionProofCache {
	return &inclusionProofCache{Cache: lruwrpr.New(size)}
}

// VerifySignature verifies the given signature data against the key obtained via validatorAtIndexer.
func (c *sigCache) VerifySignature(sig signatureData, v validatorAtIndexer) (err error) {
	defer func() {
		if err == nil {
			c.Add(sig, true)
		} else {
			log.WithError(err).WithFields(sig.logFields()).Debug("Caching failed signature verification result")
			c.Add(sig, false)
		}
	}()
	e := slots.ToEpoch(sig.Slot)
	fork, err := c.getFork(e)
	if err != nil {
		return err
	}
	domain, err := signing.Domain(fork, e, params.BeaconConfig().DomainBeaconProposer, c.valRoot)
	if err != nil {
		return err
	}
	pv, err := v.ValidatorAtIndex(sig.Proposer)
	if err != nil {
		return err
	}
	pb, err := bls.PublicKeyFromBytes(pv.PublicKey)
	if err != nil {
		return err
	}
	s, err := bls.SignatureFromBytes(sig.Signature[:])
	if err != nil {
		return err
	}
	sr, err := signing.ComputeSigningRootForRoot(sig.Root, domain)
	if err != nil {
		return err
	}
	if !s.Verify(pb, sr[:]) {
		return signing.ErrSigFailedToVerify
	}

	return nil
}

// SignatureVerified checks the signature cache for the given key, and returns a boolean value of true
// if it has been seen before, and an error value indicating whether the signature verification succeeded.
// ie only a result of (true, nil) means a previous signature check passed.
func (c *sigCache) SignatureVerified(sig signatureData) (bool, error) {
	val, seen := c.Get(sig)
	if !seen {
		return false, nil
	}
	verified, ok := val.(bool)
	if !ok {
		log.WithFields(sig.logFields()).Debug("Ignoring invalid value found in signature cache")
		// This shouldn't happen, and if it does, the caller should treat it as a cache miss and run verification
		// again to correctly populate the cache key.
		return false, nil
	}
	if verified {
		return true, nil
	}
	return true, signing.ErrSigFailedToVerify
}

// proposerCache represents a type that can compute the proposer for a given slot + parent root,
// and cache the result so that it can be reused when the same verification needs to be performed
// across multiple values.
type proposerCache interface {
	ComputeProposer(ctx context.Context, root [32]byte, slot primitives.Slot, pst state.BeaconState) (primitives.ValidatorIndex, error)
	Proposer(c *forkchoicetypes.Checkpoint, slot primitives.Slot) (primitives.ValidatorIndex, bool)
}

func newPropCache() *propCache {
	return &propCache{}
}

type propCache struct {
}

// ComputeProposer takes the state for the given parent root and slot and computes the proposer index, updating the
// proposer index cache when successful.
func (*propCache) ComputeProposer(ctx context.Context, parent [32]byte, slot primitives.Slot, pst state.BeaconState) (primitives.ValidatorIndex, error) {
	pst, err := transition.ProcessSlotsUsingNextSlotCache(ctx, pst, parent[:], slot)
	if err != nil {
		return 0, err
	}
	idx, err := helpers.BeaconProposerIndex(ctx, pst)
	if err != nil {
		return 0, err
	}
	return idx, nil
}

// Proposer returns the validator index if it is found in the cache, along with a boolean indicating
// whether the value was present, similar to accessing an lru or go map.
func (*propCache) Proposer(c *forkchoicetypes.Checkpoint, slot primitives.Slot) (primitives.ValidatorIndex, bool) {
	id, err := helpers.ProposerIndexAtSlotFromCheckpoint(c, slot)
	if err != nil {
		return 0, false
	}
	return id, true
}
