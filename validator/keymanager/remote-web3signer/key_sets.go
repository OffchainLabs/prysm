package remote_web3signer

import (
	"maps"
	"sync"

	"github.com/OffchainLabs/prysm/v7/cmd/validator/flags"
)

type (
	// keySource identifies the channel a public key came from.
	// Every key has exactly one owner and a source may only replace its own set.
	keySource uint8
)

const (
	sourceFlag keySource = iota // command line; fixed at startup
	sourceURL                   // public-keys URL; replaced whole on every poll
	sourceFile                  // key file; written by the operator and the keymanager API
	numKeySources
)

func (s keySource) String() string {
	switch s {
	case sourceFlag:
		return "the --" + flags.Web3SignerPublicValidatorKeysFlag.Name + " flag"
	case sourceURL:
		return "the remote signer public keys URL"
	case sourceFile:
		return "the --" + flags.Web3SignerKeyFileFlag.Name + " file"
	default:
		return "an unknown source"
	}
}

// keySets holds one key set per source. The validating set is their union.
type keySets struct {
	mu    sync.RWMutex
	sets  [numKeySources]map[pubkey]struct{}
	union map[pubkey]struct{}
}

// replace swaps src's set for keys and reports whether the union changed, along with
// the new union.
func (k *keySets) replace(src keySource, keys []pubkey) (bool, []pubkey) {
	k.mu.Lock()
	defer k.mu.Unlock()

	// Build a new set for src, and replace the old one.
	set := make(map[pubkey]struct{}, len(keys))
	for _, key := range keys {
		set[key] = struct{}{}
	}
	k.sets[src] = set

	// Rebuild the union of all sets.
	union := make(map[pubkey]struct{}, len(k.union))
	for _, s := range k.sets {
		maps.Copy(union, s)
	}

	if maps.Equal(union, k.union) {
		return false, nil
	}

	k.union = union
	return true, sortedKeys(union)
}

// get returns a copy of src's set. The copy is safe for the caller to mutate.
func (k *keySets) get(src keySource) map[pubkey]struct{} {
	k.mu.RLock()
	defer k.mu.RUnlock()

	oldSet := k.sets[src]
	set := make(map[pubkey]struct{}, len(oldSet))
	maps.Copy(set, oldSet)

	return set
}

// all returns a copy of the union of every source's set.
// Returned keys are sorted for deterministic output.
func (k *keySets) all() []pubkey {
	k.mu.RLock()
	defer k.mu.RUnlock()

	return sortedKeys(k.union)
}

// owner returns the source owning key. A key present in several sets is owned by the
// first source in declaration order
// - 1. flag, then 2. URL, then 3. file.
func (k *keySets) owner(key pubkey) (keySource, bool) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	for src, set := range k.sets {
		if _, ok := set[key]; ok {
			return keySource(src), true
		}
	}

	return 0, false
}
