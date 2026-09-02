package remote_web3signer

import (
	"bytes"
	"fmt"
	"maps"
	"slices"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
)

// pubkey is a BLS public key in its raw form.
type pubkey = [fieldparams.BLSPubkeyLength]byte

// sortedKeys returns the keys of set in ascending byte order.
func sortedKeys(set map[pubkey]struct{}) []pubkey {
	return slices.SortedFunc(maps.Keys(set), func(a, b pubkey) int {
		return bytes.Compare(a[:], b[:])
	})
}

// decodePublicKeys decodes and dedups hex-encoded BLS public keys.
func decodePublicKeys(raw []string) ([]pubkey, error) {
	var (
		seen = make(map[pubkey]struct{}, len(raw))
		keys = make([]pubkey, 0, len(raw))
	)

	for _, k := range raw {
		b, err := bytesutil.DecodeHex48(k)
		if err != nil {
			return nil, fmt.Errorf("decode public key %s: %w", k, err)
		}
		if _, ok := seen[b]; ok {
			continue
		}
		seen[b] = struct{}{}
		keys = append(keys, b)
	}
	return keys, nil
}
