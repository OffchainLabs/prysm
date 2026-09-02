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

type builderRequestAuthKey struct {
	pk   pubkey
	slot primitives.Slot
	data string
}

// builderConfigForSlot never returns nil; absent settings yield a neutral config.
func (v *validator) builderConfigForSlot(ctx context.Context, pk pubkey, slot primitives.Slot) *ethpb.BuilderConfig {
	cfg := &ethpb.BuilderConfig{BuilderBoostFactor: uint64(proposer.NeutralBuilderBoostFactor)}
	ps := v.ProposerSettings()
	if ps == nil {
		return cfg
	}
	bc := ps.EffectiveBuilderConfig(pk)
	if bc == nil {
		return cfg
	}
	cfg.MinBid = primitives.Gwei(uint64OrDefault(uint64Ptr(bc.MinBid), 0))
	cfg.BuilderBoostFactor = uint64OrDefault(uint64Ptr(bc.BuilderBoostFactor), uint64(proposer.NeutralBuilderBoostFactor))
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
		signed, err := v.signBuilderRequestAuthCached(ctx, km, pk, t.authData, slot)
		if err != nil {
			log.WithError(err).Warn("Failed to sign builder request auth")
			continue
		}
		cfg.Builders = append(cfg.Builders, &ethpb.BuilderEntry{
			Url:                 []byte(t.url),
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

func (v *validator) pruneSignedBuilderRequestAuths(slot primitives.Slot) {
	v.builderRequestAuthsLock.Lock()
	defer v.builderRequestAuthsLock.Unlock()
	for k := range v.builderRequestAuths {
		if k.slot < slot {
			delete(v.builderRequestAuths, k)
		}
	}
}

func (v *validator) signBuilderRequestAuthCached(ctx context.Context, km keymanager.IKeymanager, pk pubkey, authData []byte, slot primitives.Slot) (*ethpb.SignedBuilderRequestAuth, error) {
	key := builderRequestAuthKey{pk: pk, slot: slot, data: string(authData)}
	v.builderRequestAuthsLock.Lock()
	signed, ok := v.builderRequestAuths[key]
	v.builderRequestAuthsLock.Unlock()
	if ok {
		return signed, nil
	}
	signed, err := v.signBuilderRequestAuth(ctx, km, pk, &ethpb.BuilderRequestAuth{Data: authData, Slot: slot})
	if err != nil {
		return nil, err
	}
	v.builderRequestAuthsLock.Lock()
	if v.builderRequestAuths == nil {
		v.builderRequestAuths = make(map[builderRequestAuthKey]*ethpb.SignedBuilderRequestAuth)
	}
	v.builderRequestAuths[key] = signed
	v.builderRequestAuthsLock.Unlock()
	return signed, nil
}

// Domain is fork-independent: compute_domain(DOMAIN_BUILDER_REQUEST_AUTH) with genesis fork version and zero genesis validators root.
func (v *validator) signBuilderRequestAuth(
	ctx context.Context,
	km keymanager.IKeymanager,
	pubkey [fieldparams.BLSPubkeyLength]byte,
	auth *ethpb.BuilderRequestAuth,
) (*ethpb.SignedBuilderRequestAuth, error) {
	ctx, span := trace.StartSpan(ctx, "validator.signBuilderRequestAuth")
	defer span.End()

	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainBuilderRequestAuth, params.BeaconConfig().GenesisForkVersion, make([]byte, 32))
	if err != nil {
		return nil, errors.Wrap(err, "could not compute builder request auth domain")
	}

	r, err := signing.ComputeSigningRoot(auth, domain)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute signing root")
	}

	sig, err := km.Sign(ctx, &validatorpb.SignRequest{
		PublicKey:       pubkey[:],
		SigningRoot:     r[:],
		SignatureDomain: domain,
		Object:          &validatorpb.SignRequest_BuilderRequestAuth{BuilderRequestAuth: auth},
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not sign builder request auth")
	}

	return &ethpb.SignedBuilderRequestAuth{
		Message:   auth,
		Signature: sig.Marshal(),
	}, nil
}
