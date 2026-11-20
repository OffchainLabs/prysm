package pqkeys

import (
	"context"
	"fmt"
	"sync"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/validator/vrftree"
	"github.com/sirupsen/logrus"
)

type (
	blsPubkey  = [fieldparams.BLSPubkeyLength]byte
	xmssPubkey = [fieldparams.XMSSPubkeyLength]byte

	// Registry holds the post-quantum keys of the validators of a validator client, indexed by BLS public key.
	// The validator client keeps identifying its validators by BLS public key. The registry translates them to the
	// XMSS public keys the beacon node knows them by, and back. It is safe for concurrent use.
	//
	// The registry derives the XMSS keys when it loads the BLS secret keys. It builds the VRF tree of a validator once
	// the validator client sees the VRF root of the validator in the state (`SetVRFRoot`), in the background: the tree
	// needs the record of this root in the VRF state.
	Registry struct {
		ctx    context.Context // Bounds the building of the VRF trees.
		source VRFStateSource  // Nil if the validator client has no VRF state.

		mu         sync.RWMutex
		keysByBLS  map[blsPubkey]*Keys
		secretKeys map[blsPubkey]bls.SecretKey
		blsByXMSS  map[xmssPubkey]blsPubkey
		xmssByBLS  map[blsPubkey]xmssPubkey
		vrfByBLS   map[blsPubkey]*vrfState
		preparing  bool // A goroutine builds the pending VRF trees.
		dirty      bool // A VRF root changed since the goroutine took the pending VRF trees.

		deriveKeys func(context.Context, []bls.SecretKey) (map[blsPubkey]*Keys, error)
		newVRFTree func(context.Context, bls.SecretKey, [32]byte, [vrftree.Depth][32]byte) (*vrftree.Tree, error)
	}

	// vrfState is the VRF tree of a validator, for the VRF root of the validator in the state.
	vrfState struct {
		root   [32]byte
		tree   *vrftree.Tree // Nil until built.
		failed bool          // The tree of this root cannot be built: do not try again.
	}
)

// NewRegistry returns an empty registry. It builds the VRF trees from the VRF state of `source`, which may be nil,
// until `ctx` is done.
func NewRegistry(ctx context.Context, source VRFStateSource) *Registry {
	return &Registry{
		ctx:        ctx,
		source:     source,
		keysByBLS:  make(map[blsPubkey]*Keys),
		secretKeys: make(map[blsPubkey]bls.SecretKey),
		blsByXMSS:  make(map[xmssPubkey]blsPubkey),
		xmssByBLS:  make(map[blsPubkey]xmssPubkey),
		vrfByBLS:   make(map[blsPubkey]*vrfState),
		deriveKeys: DeriveAll,
		newVRFTree: vrftree.NewFull,
	}
}

// Load derives the keys of the validators whose BLS secret keys are `secretKeys`, and adds them to the registry.
// It skips the validators already in the registry: deriving keys takes a few CPU seconds per validator.
func (r *Registry) Load(ctx context.Context, secretKeys []bls.SecretKey) error {
	missing := make([]bls.SecretKey, 0, len(secretKeys))
	secretKeyByBLS := make(map[blsPubkey]bls.SecretKey, len(secretKeys))
	for i, secretKey := range secretKeys {
		if secretKey == nil {
			return fmt.Errorf("nil secret key at index %d", i)
		}

		blsKey := blsPubkey(secretKey.PublicKey().Marshal())
		if _, ok := r.Keys(blsKey); !ok {
			missing = append(missing, secretKey)
			secretKeyByBLS[blsKey] = secretKey
		}
	}

	if len(missing) == 0 {
		return nil
	}

	start := time.Now()

	keysByBLS, err := r.deriveKeys(ctx, missing)
	if err != nil {
		return fmt.Errorf("derive keys: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for blsKey, keys := range keysByBLS {
		xmssKey := xmssPubkey(keys.XMSS.PublicKey().Bytes())

		if other, ok := r.blsByXMSS[xmssKey]; ok && other != blsKey {
			return fmt.Errorf("BLS public keys %#x and %#x derive the same XMSS public key %#x", other, blsKey, xmssKey)
		}

		secretKey, ok := secretKeyByBLS[blsKey]
		if !ok {
			return fmt.Errorf("derived keys for unknown BLS public key %#x", blsKey)
		}

		r.keysByBLS[blsKey] = keys
		r.secretKeys[blsKey] = secretKey
		r.blsByXMSS[xmssKey] = blsKey
		r.xmssByBLS[blsKey] = xmssKey
	}

	log.WithFields(logrus.Fields{
		"derived": len(keysByBLS),
		"total":   len(r.keysByBLS),
		"elapsed": time.Since(start),
	}).Info("Derived post-quantum keys")

	return nil
}

// Keys returns the keys of the validator whose BLS public key is `pubkey`.
func (r *Registry) Keys(pubkey blsPubkey) (*Keys, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys, ok := r.keysByBLS[pubkey]
	return keys, ok
}

// Pubkeys returns the BLS public keys of the validators of the registry.
func (r *Registry) Pubkeys() []blsPubkey {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pubkeys := make([]blsPubkey, 0, len(r.keysByBLS))
	for pubkey := range r.keysByBLS {
		pubkeys = append(pubkeys, pubkey)
	}

	return pubkeys
}

// XMSSPubkey returns the XMSS public key of the validator whose BLS public key is `pubkey`.
func (r *Registry) XMSSPubkey(pubkey blsPubkey) (xmssPubkey, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	xmssKey, ok := r.xmssByBLS[pubkey]
	return xmssKey, ok
}

// BLSPubkey returns the BLS public key of the validator whose XMSS public key is `pubkey`.
func (r *Registry) BLSPubkey(pubkey xmssPubkey) (blsPubkey, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	blsKey, ok := r.blsByXMSS[pubkey]
	return blsKey, ok
}

// VRF returns the VRF tree of the validator whose BLS public key is `pubkey`, for the last VRF root set for it.
// It returns false until the tree is built.
func (r *Registry) VRF(pubkey blsPubkey) (*vrftree.Tree, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	state, ok := r.vrfByBLS[pubkey]
	if !ok || state.tree == nil {
		return nil, false
	}

	return state.tree, true
}

// SetVRFRoot sets `root` as the VRF root of the validator whose BLS public key is `pubkey`, as the state gives it.
// If the VRF tree of this root is not built yet, it builds it in the background. It is cheap otherwise.
func (r *Registry) SetVRFRoot(pubkey blsPubkey, root [32]byte) {
	if r.setVRFRoot(pubkey, root) {
		go r.prepareVRF(r.ctx)
	}
}

// setVRFRoot sets `root` as the VRF root of the validator whose BLS public key is `pubkey`.
// It returns true if the caller must start building the pending VRF trees.
func (r *Registry) setVRFRoot(pubkey blsPubkey, root [32]byte) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.keysByBLS[pubkey]; !ok {
		return false
	}

	state, ok := r.vrfByBLS[pubkey]
	if ok && state.root == root && (state.tree != nil || state.failed) {
		return false
	}

	if !ok || state.root != root {
		if ok {
			log.WithFields(logrus.Fields{
				"pubkey":   fmt.Sprintf("%#x", pubkey),
				"previous": fmt.Sprintf("%#x", state.root),
				"vrfRoot":  fmt.Sprintf("%#x", root),
			}).Warn("The VRF root of the validator changed")
		}

		r.vrfByBLS[pubkey] = &vrfState{root: root}
	}

	if r.preparing {
		r.dirty = true
		return false
	}

	r.preparing = true
	return true
}

// prepareVRF builds the pending VRF trees. It stops once a pass over them leaves no root it can build now.
// A tree that failed because the VRF state could not be read is built again at the next `SetVRFRoot` of its validator.
func (r *Registry) prepareVRF(ctx context.Context) {
	for attempted := false; ; attempted = true {
		r.mu.Lock()
		pending := r.pendingVRF()
		if len(pending) == 0 || (attempted && !r.dirty) {
			r.preparing = false
			r.mu.Unlock()
			return
		}

		r.dirty = false
		r.mu.Unlock()

		r.buildVRF(ctx, pending)
	}
}

// pendingVRF returns the VRF roots, by BLS public key, whose tree is still to build. The caller holds the lock.
func (r *Registry) pendingVRF() map[blsPubkey][32]byte {
	pending := make(map[blsPubkey][32]byte)
	for pubkey, state := range r.vrfByBLS {
		if state.tree == nil && !state.failed {
			pending[pubkey] = state.root
		}
	}

	return pending
}

// buildVRF builds the VRF trees of the roots of `pending`, by BLS public key, from their records in the VRF state.
func (r *Registry) buildVRF(ctx context.Context, pending map[blsPubkey][32]byte) {
	if r.source == nil {
		log.WithField("count", len(pending)).Warn(
			"Without a VRF state, the validator client cannot build the VRF trees of its validators: they cannot propose",
		)

		for pubkey, root := range pending {
			r.storeVRF(pubkey, root, nil)
		}

		return
	}

	start := time.Now()

	roots := make(map[[32]byte]bool, len(pending))
	for _, root := range pending {
		roots[root] = true
	}

	records, err := r.source.Records(ctx, roots)
	if err != nil {
		log.WithError(err).Error("Could not read the VRF state")
		return
	}

	built := 0
	for pubkey, root := range pending {
		if ctx.Err() != nil {
			return
		}

		logger := log.WithFields(logrus.Fields{
			"pubkey":  fmt.Sprintf("%#x", pubkey),
			"vrfRoot": fmt.Sprintf("%#x", root),
		})

		record, ok := records[root]
		if !ok {
			logger.Error("The VRF state has no record for the VRF root of the validator: it cannot propose")
			r.storeVRF(pubkey, root, nil)
			continue
		}

		r.mu.RLock()
		secretKey := r.secretKeys[pubkey]
		r.mu.RUnlock()

		tree, err := r.newVRFTree(ctx, secretKey, root, record.Branch)
		if err != nil {
			if ctx.Err() != nil {
				return
			}

			logger.WithError(err).Error("The VRF state does not match the keys of the validator: it cannot propose")
			r.storeVRF(pubkey, root, nil)
			continue
		}

		r.storeVRF(pubkey, root, tree)
		built++
	}

	log.WithFields(logrus.Fields{
		"built":   built,
		"pending": len(pending),
		"elapsed": time.Since(start),
	}).Info("Built VRF trees")
}

// storeVRF stores `tree`, the VRF tree of `root`, for the validator whose BLS public key is `pubkey`.
// A nil `tree` means the tree of this root cannot be built. It does nothing if the VRF root of the validator changed.
func (r *Registry) storeVRF(pubkey blsPubkey, root [32]byte, tree *vrftree.Tree) {
	r.mu.Lock()
	defer r.mu.Unlock()

	state, ok := r.vrfByBLS[pubkey]
	if !ok || state.root != root {
		return
	}

	state.tree = tree
	state.failed = tree == nil
}
