package client

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

type requestAuthKey struct {
	pk   pubkey
	slot primitives.Slot
	data string
}

type resolvedBuilderEntry struct {
	maxPayment uint64
	minBid     uint64
	boost      uint64
}

// resolveBuilderEntry applies the config-level defaults to one builder entry.
func resolveBuilderEntry(bc *proposer.BuilderConfig, entry proposer.BuilderEntry) resolvedBuilderEntry {
	r := resolvedBuilderEntry{
		maxPayment: uint64(entry.MaxExecutionPayment),
		minBid:     uint64(entry.MinBid),
		boost:      uint64(entry.BuilderBoostFactor),
	}
	if r.maxPayment == 0 {
		r.maxPayment = uint64(bc.MaxExecutionPayment)
	}
	if r.minBid == 0 {
		r.minBid = uint64(bc.MinBid)
	}
	if r.boost == 0 {
		r.boost = uint64(bc.BuilderBoostFactor)
	}
	// A neutral boost keeps builder and local bids on equal footing.
	if r.boost == 0 {
		r.boost = 100
	}
	return r
}

func (v *validator) builderEntriesForSlot(pk pubkey, slot primitives.Slot) []*ethpb.BuilderRequestEntry {
	bc := v.builderConfigForKey(pk)
	if bc == nil || !bc.Enabled || len(bc.Builders) == 0 {
		return nil
	}
	v.signedRequestAuthsLock.Lock()
	defer v.signedRequestAuthsLock.Unlock()
	var entries []*ethpb.BuilderRequestEntry
	for _, entry := range bc.Builders {
		data, err := entry.AuthDataBytes()
		if err != nil {
			log.WithError(err).WithField("builder", entry.URL).Warn("Invalid builder auth data in proposer settings, skipping builder")
			continue
		}
		signed, ok := v.signedRequestAuths[requestAuthKey{pk: pk, slot: slot, data: string(data)}]
		if !ok {
			continue
		}
		r := resolveBuilderEntry(bc, entry)
		e := &ethpb.BuilderRequestEntry{
			Auth:                signed,
			Url:                 entry.URL,
			MaxExecutionPayment: r.maxPayment,
			MinBid:              r.minBid,
			BuilderBoostFactor:  r.boost,
		}
		if entry.Pubkey != "" {
			pub, err := hexutil.Decode(entry.Pubkey)
			if err != nil || len(pub) != fieldparams.BLSPubkeyLength {
				log.WithField("builder", entry.URL).Warn("Invalid builder pubkey in proposer settings, skipping payment trust binding")
			} else {
				e.Pubkey = pub
			}
		}
		entries = append(entries, e)
	}
	return entries
}

func (v *validator) pruneSignedRequestAuths(slot primitives.Slot) {
	v.signedRequestAuthsLock.Lock()
	defer v.signedRequestAuthsLock.Unlock()
	for k := range v.signedRequestAuths {
		if k.slot < slot {
			delete(v.signedRequestAuths, k)
		}
	}
}

func (v *validator) signRequestAuthCached(ctx context.Context, km keymanager.IKeymanager, pk pubkey, data []byte, slot primitives.Slot) (*ethpb.SignedRequestAuthV1, error) {
	key := requestAuthKey{pk: pk, slot: slot, data: string(data)}
	v.signedRequestAuthsLock.Lock()
	signed, ok := v.signedRequestAuths[key]
	v.signedRequestAuthsLock.Unlock()
	if ok {
		return signed, nil
	}
	signed, err := v.signRequestAuth(ctx, km, pk, &ethpb.RequestAuthV1{Data: data, Slot: slot})
	if err != nil {
		return nil, err
	}
	v.signedRequestAuthsLock.Lock()
	if v.signedRequestAuths == nil {
		v.signedRequestAuths = make(map[requestAuthKey]*ethpb.SignedRequestAuthV1)
	}
	v.signedRequestAuths[key] = signed
	v.signedRequestAuthsLock.Unlock()
	return signed, nil
}

// Domain is fork-independent: compute_domain(DOMAIN_REQUEST_AUTH) with genesis fork version and zero genesis validators root.
func (v *validator) signRequestAuth(
	ctx context.Context,
	km keymanager.IKeymanager,
	pubkey [fieldparams.BLSPubkeyLength]byte,
	auth *ethpb.RequestAuthV1,
) (*ethpb.SignedRequestAuthV1, error) {
	ctx, span := trace.StartSpan(ctx, "validator.signRequestAuth")
	defer span.End()

	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainRequestAuth, params.BeaconConfig().GenesisForkVersion, make([]byte, 32))
	if err != nil {
		return nil, errors.Wrap(err, "could not compute request auth domain")
	}

	r, err := signing.ComputeSigningRoot(auth, domain)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute signing root")
	}

	sig, err := km.Sign(ctx, &validatorpb.SignRequest{
		PublicKey:       pubkey[:],
		SigningRoot:     r[:],
		SignatureDomain: domain,
		Object:          &validatorpb.SignRequest_RequestAuth{RequestAuth: auth},
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not sign request auth")
	}

	return &ethpb.SignedRequestAuthV1{
		Message:   auth,
		Signature: sig.Marshal(),
	}, nil
}
