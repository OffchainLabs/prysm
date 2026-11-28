// Package pqkeys derives the post-quantum keys of a leanXMSS validator from its BLS secret key.
//
// The spec does not say how the keys are derived. Here, they are derived as the ethPandaOps genesis tool
// (https://github.com/ethpandaops/eth-beacon-genesis/tree/pq) and Lighthouse derive them, from the validator's
// BLS secret key `sk`. A validator client holding the usual BLS keystore then gets back the keys of the genesis state:
//   - the XMSS private key is `leanxmss.NewKeyFromSeed(sha256("leanxmss-seed" || sk))`,
//   - the VRF tree is the full tree of package vrftree. Its leaves below 2^20 are derived from `sk`, and the branch
//     joining them to the VRF root comes from the VRF state the genesis tool publishes.
package pqkeys

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	leanxmss "github.com/nalepae/go-leanxmss"
	"github.com/sirupsen/logrus"
)

// xmssSeedDomain separates the XMSS seed from the other secrets derived from the BLS secret key.
var xmssSeedDomain = []byte("leanxmss-seed")

// Keys are the post-quantum keys of a validator, except its VRF tree.
// The VRF tree needs the VRF root of the validator in the state: the registry builds it once the root is known.
type Keys struct {
	XMSS *leanxmss.PrivateKey // Signs the messages of the validator.

	cache *xmssDiskCache // Tree cache of XMSS on disk. Nil if there is none.
}

// PersistXMSS writes the trees of the XMSS key to its tree cache on disk, if it has one and they changed since they
// were last written or read, so that a restart loads them instead of building them.
func (k *Keys) PersistXMSS() error {
	if k.cache == nil {
		return nil
	}

	if err := k.cache.write(k.XMSS); err != nil {
		return fmt.Errorf("write the XMSS tree cache %s: %w", k.cache.path, err)
	}

	return nil
}

// Derive derives the post-quantum keys of the validator whose BLS secret key is `secretKey`.
// It takes a few CPU seconds, spread over all CPUs: it builds the top tree of the XMSS key.
func Derive(ctx context.Context, secretKey bls.SecretKey) (*Keys, error) {
	if secretKey == nil {
		return nil, errors.New("nil secret key")
	}

	// Building the XMSS key cannot be cancelled. Do not start it if the context is already done.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	keys := &Keys{
		XMSS: leanxmss.NewKeyFromSeed(xmssSeed(secretKey.Marshal())),
	}

	return keys, nil
}

// DeriveAll derives the post-quantum keys of the validators whose BLS secret keys are `secretKeys`,
// indexed by BLS public key. It derives the keys one validator after the other: Derive already uses all CPUs.
// It uses no tree cache: see XMSSLoader.DeriveAll.
func DeriveAll(ctx context.Context, secretKeys []bls.SecretKey) (map[[fieldparams.BLSPubkeyLength]byte]*Keys, error) {
	keysByPubkey := make(map[[fieldparams.BLSPubkeyLength]byte]*Keys, len(secretKeys))
	start := time.Now()

	for i, secretKey := range secretKeys {
		if secretKey == nil {
			return nil, fmt.Errorf("nil secret key at index %d", i)
		}

		pubkey := [fieldparams.BLSPubkeyLength]byte(secretKey.PublicKey().Marshal())
		if _, ok := keysByPubkey[pubkey]; ok {
			continue
		}

		keys, err := Derive(ctx, secretKey)
		if err != nil {
			return nil, fmt.Errorf("derive keys of %#x: %w", pubkey, err)
		}

		keysByPubkey[pubkey] = keys

		log.WithFields(logrus.Fields{
			"derived": len(keysByPubkey),
			"total":   len(secretKeys),
			"elapsed": time.Since(start),
		}).Debug("Derived post-quantum keys")
	}

	return keysByPubkey, nil
}

// xmssSeed returns the seed of the XMSS key of the validator whose serialized BLS secret key is `secretKey`.
func xmssSeed(secretKey []byte) [32]byte {
	input := make([]byte, 0, len(xmssSeedDomain)+len(secretKey))
	input = append(input, xmssSeedDomain...)
	input = append(input, secretKey...)

	return sha256.Sum256(input)
}

// DeriveAll derives the post-quantum keys of the validators whose BLS secret keys are `secretKeys`, indexed by BLS
// public key, as the package-level DeriveAll does, but loads the trees of the XMSS keys from the tree caches when
// they have valid ones: well under a millisecond per key, instead of a few CPU seconds.
//
// It reads the tree caches on disk, then the published tree cache for the keys not on disk, each for
// `xmssCacheFetchWorkers` keys at once. It then builds the keys without valid trees one after the other: building a
// key already uses all CPUs.
func (l *XMSSLoader) DeriveAll(ctx context.Context, secretKeys []bls.SecretKey) (map[[fieldparams.BLSPubkeyLength]byte]*Keys, error) {
	type pending struct {
		pubkey blsPubkey
		seed   [32]byte
		disk   *xmssDiskCache
		key    *leanxmss.PrivateKey
		from   string // Where the trees were read from. Empty if the key is to build.
	}

	all := make([]*pending, 0, len(secretKeys))
	seen := make(map[blsPubkey]bool, len(secretKeys))
	for i, secretKey := range secretKeys {
		if secretKey == nil {
			return nil, fmt.Errorf("nil secret key at index %d", i)
		}

		pubkey := blsPubkey(secretKey.PublicKey().Marshal())
		if seen[pubkey] {
			continue
		}

		seen[pubkey] = true
		seed := xmssSeed(secretKey.Marshal())
		all = append(all, &pending{pubkey: pubkey, seed: seed, disk: l.newDiskCache(pubkey, seed)})
	}

	start := time.Now()

	// First the tree caches on disk.
	forEachParallel(all, func(p *pending) {
		if p.key = l.loadFromDisk(p.pubkey, p.seed, p.disk); p.key != nil {
			p.from = "disk"
		}
	})

	// Then the published tree cache, for the keys not on disk.
	missing := make([]*pending, 0, len(all))
	for _, p := range all {
		if p.key == nil {
			missing = append(missing, p)
		}
	}

	l.sourceDown.Store(false)
	if len(missing) > 0 && l.usesPublished() {
		if published, err := l.publishedKeys(ctx); err != nil {
			log.WithError(err).Warn("Could not use the published XMSS tree cache, building the keys")
		} else {
			forEachParallel(missing, func(p *pending) {
				if p.key = l.loadPublished(ctx, p.pubkey, p.seed, published); p.key != nil {
					p.from = "published"
				}
			})
		}
	}

	counts := make(map[string]int)
	keysByPubkey := make(map[blsPubkey]*Keys, len(all))
	for _, p := range all {
		if p.key == nil {
			// Building the XMSS key cannot be cancelled. Do not start it if the context is already done.
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("build the XMSS key of %#x: %w", p.pubkey, err)
			}

			p.key, p.from = leanxmss.NewKeyFromSeed(p.seed), "seed"
		}

		counts[p.from]++
		keysByPubkey[p.pubkey] = &Keys{XMSS: p.key, cache: p.disk}

		log.WithFields(logrus.Fields{
			"pubkey":  fmt.Sprintf("%#x", p.pubkey),
			"from":    p.from,
			"derived": len(keysByPubkey),
			"total":   len(all),
			"elapsed": time.Since(start),
		}).Debug("Derived post-quantum keys")
	}

	log.WithFields(logrus.Fields{
		"fromDiskCache":      counts["disk"],
		"fromPublishedCache": counts["published"],
		"builtFromSeed":      counts["seed"],
		"elapsed":            time.Since(start),
	}).Info("Loaded the XMSS keys")

	return keysByPubkey, nil
}

// forEachParallel calls `f` on each item of `items`, on `xmssCacheFetchWorkers` items at once, and returns when all
// calls are done.
func forEachParallel[T any](items []T, f func(T)) {
	jobs := make(chan T)
	var wg sync.WaitGroup
	for range min(xmssCacheFetchWorkers, len(items)) {
		wg.Go(func() {
			for item := range jobs {
				f(item)
			}
		})
	}

	for _, item := range items {
		jobs <- item
	}

	close(jobs)
	wg.Wait()
}
