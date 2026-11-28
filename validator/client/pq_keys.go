package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"time"

	"github.com/OffchainLabs/prysm/v7/cmd/validator/flags"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/OffchainLabs/prysm/v7/validator/db/signingprotection"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager"
	"github.com/OffchainLabs/prysm/v7/validator/pqkeys"
	"github.com/OffchainLabs/prysm/v7/validator/xmsssigner"
	leanxmss "github.com/nalepae/go-leanxmss"
	"github.com/sirupsen/logrus"
)

// xmssBottomTreeHeight is the height of an XMSS bottom tree: it holds 2^xmssBottomTreeHeight consecutive leaves.
const xmssBottomTreeHeight = 16

// xmssPrepareLookahead is how many epochs ahead the XMSS bottom trees are built: building the tree of every key takes
// far longer than a slot.
const xmssPrepareLookahead = 4

// errPQRequiresLocalKeys is returned in the PQ fork when the keymanager cannot hand out the BLS secret keys.
var errPQRequiresLocalKeys = errors.New("the PQ fork requires a local or derived keymanager: the post-quantum keys are derived from the BLS secret keys")

// newPQKeys returns the registry of the post-quantum keys of the validators if the chain is in the PQ fork, and nil
// otherwise. Only a PQ fork at genesis is supported: the validator client never changes keys.
// The registry reads the VRF state at `vrfStateURL`, which may be empty, until `ctx` is done. It loads the trees of the
// XMSS keys from their tree caches in `dataDir`, then from the published tree cache at `xmssCacheURL`, which may be
// empty, and builds the keys without valid trees. It finds the records of the published tree cache with the XMSS
// public keys of the validator registry, from `chainClient`.
func newPQKeys(ctx context.Context, vrfStateURL, xmssCacheURL, dataDir string, chainClient iface.ChainClient) (*pqkeys.Registry, error) {
	config := params.BeaconConfig()

	switch config.PQForkEpoch {
	case config.FarFutureEpoch:
		return nil, nil
	case 0:
	default:
		return nil, fmt.Errorf("the PQ fork is scheduled at epoch %d: only a PQ fork at genesis is supported", config.PQForkEpoch)
	}

	var xmssSource pqkeys.XMSSCacheSource
	if xmssCacheURL != "" {
		source, err := pqkeys.NewXMSSCacheSource(xmssCacheURL)
		if err != nil {
			log.WithError(err).Warn("Could not use the published XMSS tree cache, building the keys")
		} else {
			xmssSource = source
		}
	}

	cacheDir := ""
	if dataDir != "" {
		cacheDir = filepath.Join(dataDir, pqkeys.XMSSCacheDirName)
	}

	var vrfSource pqkeys.VRFStateSource
	if vrfStateURL == "" {
		log.Warnf("No VRF state: set --%s, or the validators cannot propose in the PQ fork", flags.VRFStateURLFlag.Name)
	}

	if vrfStateURL != "" {
		source, err := pqkeys.NewVRFStateSource(vrfStateURL)
		if err != nil {
			return nil, fmt.Errorf("VRF state: %w", err)
		}

		vrfSource = source
	}

	registry := pqkeys.NewRegistry(ctx, vrfSource)
	registry.UseXMSSLoader(pqkeys.NewXMSSLoader(cacheDir, xmssSource, registryXMSSPubkeys(chainClient)))

	return registry, nil
}

// registryXMSSPubkeys returns the function listing the XMSS public keys of the validator registry from the head state
// of `chainClient`. The registry only grows: the validators of the genesis keep their indices and public keys.
func registryXMSSPubkeys(chainClient iface.ChainClient) pqkeys.XMSSPubkeysFunc {
	return func(ctx context.Context, count int) ([][fieldparams.XMSSPubkeyLength]byte, error) {
		pubkeys := make([][fieldparams.XMSSPubkeyLength]byte, 0, count)

		// The beacon REST API, which the PQ fork requires, returns all of them in one page.
		request := &ethpb.ListValidatorsRequest{PageSize: int32(min(count, math.MaxInt32))} // #nosec G115 -- Capped.

		for len(pubkeys) < count {
			response, err := chainClient.Validators(ctx, request)
			if err != nil {
				return nil, fmt.Errorf("list validators: %w", err)
			}

			for _, container := range response.ValidatorList {
				if len(pubkeys) == count {
					break
				}

				if container.Validator == nil || uint64(container.Index) != uint64(len(pubkeys)) {
					return nil, fmt.Errorf("validator at index %d missing from the list", len(pubkeys))
				}

				pubkey, err := registryXMSSPubkey(container.Validator.PublicKey)
				if err != nil {
					return nil, fmt.Errorf("validator %d: %w", container.Index, err)
				}

				pubkeys = append(pubkeys, pubkey)
			}

			if response.NextPageToken == "" || len(response.ValidatorList) == 0 {
				break
			}

			request.PageToken = response.NextPageToken
		}

		return pubkeys, nil
	}
}

// registryXMSSPubkey returns the XMSS public key of a validator from its public key in the beacon API: the XMSS key
// alone, or the 48 bytes of the registry, 16 zero bytes then the XMSS key.
func registryXMSSPubkey(pubkey []byte) ([fieldparams.XMSSPubkeyLength]byte, error) {
	const prefixLength = fieldparams.BLSPubkeyLength - fieldparams.XMSSPubkeyLength

	switch len(pubkey) {
	case fieldparams.XMSSPubkeyLength:
		return [fieldparams.XMSSPubkeyLength]byte(pubkey), nil
	case fieldparams.BLSPubkeyLength:
		if !bytes.Equal(pubkey[:prefixLength], make([]byte, prefixLength)) {
			return [fieldparams.XMSSPubkeyLength]byte{}, fmt.Errorf("public key %#x is not an XMSS public key", pubkey)
		}

		return [fieldparams.XMSSPubkeyLength]byte(pubkey[prefixLength:]), nil
	default:
		return [fieldparams.XMSSPubkeyLength]byte{}, fmt.Errorf("public key of %d bytes", len(pubkey))
	}
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
		// The trees of the next epochs are built again, for the new key.
		v.pqPrepareLock.Lock()
		v.pqKeyGeneration++
		v.pqPreparedEpoch.Store(0)
		v.pqPrepareLock.Unlock()
		if err := keys.PersistXMSS(); err != nil {
			log.WithError(err).WithField("pubkey", fmt.Sprintf("%#x", blsKey)).Warn("Could not keep the XMSS trees on disk: the next start builds them again")
		}
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

// prepareNextEpochPQKeys builds the XMSS bottom trees that the next `xmssPrepareLookahead` epochs sign in for the first
// time, for every XMSS key, then writes the trees to the tree caches on disk.
// It looks at each epoch once, at any slot, so a restart or a failed duties update does not skip a tree.
// It uses a single goroutine.
func (v *validator) prepareNextEpochPQKeys(ctx context.Context, slot primitives.Slot) {
	if v.pqKeys == nil {
		return
	}

	epoch := slots.ToEpoch(slot)
	target := epoch + xmssPrepareLookahead
	from := max(epoch, primitives.Epoch(v.pqPreparedEpoch.Load()))
	if from >= target || !v.pqPreparing.CompareAndSwap(false, true) {
		return
	}

	leaves, err := xmssLeavesToPrepare(from, target)
	if err != nil {
		v.pqPreparing.Store(false)
		log.WithError(err).Debug("Could not compute the XMSS leaves to prepare")
		return
	}

	if len(leaves) == 0 {
		v.pqPreparedEpoch.Store(uint64(target))
		v.pqPreparing.Store(false)
		return
	}

	v.pqPrepareLock.Lock()
	generation := v.pqKeyGeneration
	v.pqPrepareLock.Unlock()
	go func() {
		defer v.pqPreparing.Store(false)

		start := time.Now()
		log.WithFields(logrus.Fields{
			"keys":   len(v.pqKeys.Pubkeys()),
			"leaves": leaves,
		}).Info("Building the XMSS bottom trees of the next epochs")

		prepared := 0
		for _, blsKey := range v.pqKeys.Pubkeys() {
			if ctx.Err() != nil {
				return
			}

			keys, ok := v.pqKeys.Keys(blsKey)
			if !ok {
				continue
			}

			pubkeyLog := log.WithField("pubkey", fmt.Sprintf("%#x", blsKey))
			for _, leaf := range leaves {
				if err := keys.XMSS.Prepare(leaf); err != nil {
					pubkeyLog.WithError(err).WithField("leaf", leaf).Warn("Could not prepare the XMSS bottom tree")
				}
			}

			if err := keys.PersistXMSS(); err != nil {
				pubkeyLog.WithError(err).Warning("Could not keep the XMSS trees on disk: the next start builds them again")
			}

			prepared++
		}

		// A key added meanwhile may not be in this run.
		v.pqPrepareLock.Lock()
		if v.pqKeyGeneration == generation {
			v.pqPreparedEpoch.Store(uint64(target))
		}
		v.pqPrepareLock.Unlock()

		log.WithFields(logrus.Fields{
			"keys":    prepared,
			"leaves":  leaves,
			"elapsed": time.Since(start),
		}).Info("Built the XMSS bottom trees of the next epochs")
	}()
}

// xmssLeavesToPrepare returns one leaf of each bottom tree that the epochs after `from`, up to `to`, sign in and that
// `from` does not: the trees these epochs sign in for the first time.
func xmssLeavesToPrepare(from, to primitives.Epoch) ([]uint32, error) {
	current, err := xmssEpochLeaves(from)
	if err != nil {
		return nil, fmt.Errorf("leaves of epoch %d: %w", from, err)
	}

	seen := make(map[uint32]bool, len(current))
	for _, leaf := range current {
		seen[xmssBottomTree(leaf)] = true
	}

	var leaves []uint32
	for epoch := from + 1; epoch <= to; epoch++ {
		next, err := xmssEpochLeaves(epoch)
		if err != nil {
			return nil, fmt.Errorf("leaves of epoch %d: %w", epoch, err)
		}

		for _, leaf := range next {
			if tree := xmssBottomTree(leaf); !seen[tree] {
				seen[tree] = true
				leaves = append(leaves, leaf)
			}
		}
	}

	return leaves, nil
}

// xmssEpochLeaves returns the leaf of the attestation targeting `epoch`, then the first and the last slot leaves of
// `epoch`.
func xmssEpochLeaves(epoch primitives.Epoch) ([]uint32, error) {
	attestation, err := xmsssigner.AttestationLeafIndex(epoch)
	if err != nil {
		return nil, fmt.Errorf("attestation leaf: %w", err)
	}

	firstSlot, err := slots.EpochStart(epoch)
	if err != nil {
		return nil, fmt.Errorf("epoch start: %w", err)
	}

	lastSlot, err := slots.EpochEnd(epoch)
	if err != nil {
		return nil, fmt.Errorf("epoch end: %w", err)
	}

	first, err := xmsssigner.SlotLeafIndex(firstSlot, 0)
	if err != nil {
		return nil, fmt.Errorf("first slot leaf: %w", err)
	}

	last, err := xmsssigner.SlotLeafIndex(lastSlot, signingprotection.SlotMessage(xmsssigner.LeavesPerSlot-1))
	if err != nil {
		return nil, fmt.Errorf("last slot leaf: %w", err)
	}

	return []uint32{attestation, first, last}, nil
}

// xmssBottomTree returns the index of the XMSS bottom tree holding `leaf`.
func xmssBottomTree(leaf uint32) uint32 {
	return leaf >> xmssBottomTreeHeight
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
