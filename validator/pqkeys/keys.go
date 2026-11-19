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
