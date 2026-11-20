package client

import (
	"context"
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/cmd/validator/flags"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/OffchainLabs/prysm/v7/validator/db/signingprotection"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager"
	"github.com/OffchainLabs/prysm/v7/validator/pqkeys"
	"github.com/OffchainLabs/prysm/v7/validator/xmsssigner"
	leanxmss "github.com/nalepae/go-leanxmss"
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
		log.Warnf("No VRF state: set --%s, or the validators cannot propose in the PQ fork", flags.VRFStateURLFlag.Name)
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

	if err := v.syncPQSigner(ctx); err != nil {
		return fmt.Errorf("sync the XMSS signer: %w", err)
	}

	return nil
}

// syncPQSigner creates the XMSS signer if needed, and makes it able to sign with every XMSS key of the registry.
// The signer is protected by the signing protection database of the chain, next to the validator database.
func (v *validator) syncPQSigner(ctx context.Context) error {
	v.pqSignerLock.Lock()
	defer v.pqSignerLock.Unlock()

	if v.pqSigner == nil {
		genesisValidatorsRoot, err := v.db.GenesisValidatorsRoot(ctx)
		if err != nil {
			return fmt.Errorf("genesis validators root: %w", err)
		}

		if len(genesisValidatorsRoot) != fieldparams.RootLength {
			return fmt.Errorf("genesis validators root %#x is not %d bytes long", genesisValidatorsRoot, fieldparams.RootLength)
		}

		store, err := signingprotection.NewStore(v.db.DatabasePath(), [fieldparams.RootLength]byte(genesisValidatorsRoot))
		if err != nil {
			return fmt.Errorf("signing protection store: %w", err)
		}

		if v.pqSigner, err = xmsssigner.New(store, nil); err != nil {
			return fmt.Errorf("new signer: %w", err)
		}
	}

	known := make(map[[signingprotection.PubkeyLength]byte]bool)
	for _, pubkey := range v.pqSigner.PublicKeys() {
		known[pubkey] = true
	}

	for _, blsKey := range v.pqKeys.Pubkeys() {
		keys, ok := v.pqKeys.Keys(blsKey)
		if !ok {
			continue
		}

		xmssKey, ok := v.pqKeys.XMSSPubkey(blsKey)
		if !ok || known[xmssKey] {
			continue
		}

		if err := v.pqSigner.AddKey(keys.XMSS); err != nil {
			return fmt.Errorf("add the XMSS key of %#x: %w", blsKey, err)
		}

		v.preparePQKey(keys.XMSS)
	}

	return nil
}

// preparePQKey builds the bottom trees of `key` holding the leaves of the attestations of the current epoch and of the
// messages of the current slot. Signing first at a leaf of a bottom tree builds the tree.
// Building it ahead keeps the first duties of the key on time.
func (v *validator) preparePQKey(key *leanxmss.PrivateKey) {
	slot := slots.CurrentSlot(v.GenesisTime())

	attestationLeaf, err := xmsssigner.AttestationLeafIndex(slots.ToEpoch(slot))
	if err != nil {
		log.WithError(err).Debug("Could not compute the attestation leaf to prepare")
		return
	}

	slotLeaf, err := xmsssigner.SlotLeafIndex(slot, signingprotection.Block)
	if err != nil {
		log.WithError(err).Debug("Could not compute the slot leaf to prepare")
		return
	}

	for _, leaf := range []uint32{attestationLeaf, slotLeaf} {
		if err := key.Prepare(leaf); err != nil {
			log.WithError(err).WithField("leaf", leaf).Warn("Could not prepare the XMSS bottom tree")
		}
	}
}

// pqClient returns the PQ part of the validator client. It returns false outside the PQ fork.
func (v *validator) pqClient() (iface.PQValidatorClient, bool) {
	if v.pqKeys == nil {
		return nil, false
	}

	client, ok := v.validatorClient.(iface.PQValidatorClient)
	return client, ok
}

// isAggregatorPQ returns whether the beacon node selected the validator of index `index` as an aggregator at `slot`.
func (v *validator) isAggregatorPQ(slot primitives.Slot, index primitives.ValidatorIndex) bool {
	client, ok := v.pqClient()
	if !ok {
		return false
	}

	return client.IsAggregatorPQ(slot, index)
}
