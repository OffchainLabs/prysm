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
	"github.com/pkg/errors"
)

type requestAuthKey struct {
	pk   pubkey
	slot primitives.Slot
	data string
}

// builderConfigForSlot resolves the builder config for pk's proposal at slot from one
// settings snapshot, signing any request auth not already cached by the warm push.
func (v *validator) builderConfigForSlot(ctx context.Context, pk pubkey, slot primitives.Slot) *ethpb.BuilderConfig {
	ps := v.ProposerSettings()
	if ps == nil {
		return nil
	}
	bc := ps.EffectiveBuilderConfig(pk)
	if bc == nil {
		return nil
	}
	cfg := &ethpb.BuilderConfig{
		MinBid:             primitives.Gwei(uint64OrDefault(uint64Ptr(bc.MinBid), 0)),
		BuilderBoostFactor: uint64OrDefault(uint64Ptr(bc.BuilderBoostFactor), uint64(proposer.NeutralBuilderBoostFactor)),
	}
	targets := builderTargets(bc)
	if len(targets) == 0 {
		return cfg
	}
	km, err := v.Keymanager()
	if err != nil {
		log.WithError(err).Warn("Could not get keymanager for builder request auths")
		return cfg
	}
	for _, t := range targets {
		signed, err := v.signRequestAuthCached(ctx, km, pk, t.authData, slot)
		if err != nil {
			log.WithError(err).Warn("Failed to sign builder request auth")
			continue
		}
		cfg.Builders = append(cfg.Builders, &ethpb.BuilderEntry{
			Url:                 t.url,
			Auth:                signed,
			BuilderPubkeys:      t.pubkeys,
			MaxExecutionPayment: primitives.Gwei(t.maxPayment),
			MinBid:              primitives.Gwei(uint64OrDefault(t.minBid, 0)),
			BuilderBoostFactor:  uint64OrDefault(t.boostFactor, uint64(proposer.NeutralBuilderBoostFactor)),
		})
	}
	return cfg
}

func uint64OrDefault(v *uint64, def uint64) uint64 {
	if v == nil {
		return def
	}
	return *v
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

func (v *validator) signRequestAuthCached(ctx context.Context, km keymanager.IKeymanager, pk pubkey, authData []byte, slot primitives.Slot) (*ethpb.SignedRequestAuth, error) {
	key := requestAuthKey{pk: pk, slot: slot, data: string(authData)}
	v.signedRequestAuthsLock.Lock()
	signed, ok := v.signedRequestAuths[key]
	v.signedRequestAuthsLock.Unlock()
	if ok {
		return signed, nil
	}
	signed, err := v.signRequestAuth(ctx, km, pk, &ethpb.RequestAuth{Data: authData, Slot: slot})
	if err != nil {
		return nil, err
	}
	v.signedRequestAuthsLock.Lock()
	if v.signedRequestAuths == nil {
		v.signedRequestAuths = make(map[requestAuthKey]*ethpb.SignedRequestAuth)
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
	auth *ethpb.RequestAuth,
) (*ethpb.SignedRequestAuth, error) {
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

	return &ethpb.SignedRequestAuth{
		Message:   auth,
		Signature: sig.Marshal(),
	}, nil
}
