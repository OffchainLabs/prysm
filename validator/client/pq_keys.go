package client

import (
	"context"
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager"
	"github.com/OffchainLabs/prysm/v7/validator/pqkeys"
)

// errPQRequiresLocalKeys is returned in the PQ fork when the keymanager cannot hand out the BLS secret keys.
var errPQRequiresLocalKeys = errors.New("the PQ fork requires a local or derived keymanager: the post-quantum keys are derived from the BLS secret keys")

// newPQKeys returns the registry of the post-quantum keys of the validators if the chain is in the PQ fork, and nil
// otherwise. Only a PQ fork at genesis is supported: the validator client never changes keys.
// The registry reads the VRF state at `vrfStateURL`, which may be empty, until `ctx` is done.
func newPQKeys(ctx context.Context, vrfStateURL string) (*pqkeys.Registry, error) {
	config := params.BeaconConfig()

	switch config.PQForkEpoch {
	case config.FarFutureEpoch:
		return nil, nil
	case 0:
	default:
		return nil, fmt.Errorf("the PQ fork is scheduled at epoch %d: only a PQ fork at genesis is supported", config.PQForkEpoch)
	}

	if vrfStateURL == "" {
		log.Warn("No VRF state: set --vrf-state-url, or the validators cannot propose in the PQ fork")
		return pqkeys.NewRegistry(ctx, nil), nil
	}

	source, err := pqkeys.NewVRFStateSource(vrfStateURL)
	if err != nil {
		return nil, fmt.Errorf("VRF state: %w", err)
	}

	return pqkeys.NewRegistry(ctx, source), nil
}

// loadPQKeys derives the post-quantum keys of the validators of the keymanager that are not derived yet.
// It does nothing outside the PQ fork.
func (v *validator) loadPQKeys(ctx context.Context) error {
	if v.pqKeys == nil {
		return nil
	}

	km, ok := v.km.(keymanager.KeysFetcher)
	if !ok {
		return errPQRequiresLocalKeys
	}

	rawSecretKeys, err := km.FetchValidatingPrivateKeys(ctx)
	if err != nil {
		return fmt.Errorf("fetch the BLS secret keys: %w", err)
	}

	secretKeys := make([]bls.SecretKey, 0, len(rawSecretKeys))
	for _, rawSecretKey := range rawSecretKeys {
		secretKey, err := bls.SecretKeyFromBytes(rawSecretKey[:])
		if err != nil {
			return fmt.Errorf("decode a BLS secret key: %w", err)
		}

		secretKeys = append(secretKeys, secretKey)
	}

	if err := v.pqKeys.Load(ctx, secretKeys); err != nil {
		return fmt.Errorf("load the post-quantum keys: %w", err)
	}

	return nil
}
