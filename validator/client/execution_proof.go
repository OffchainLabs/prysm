package client

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/rand"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/pkg/errors"
)

// errNoActiveValidator means this client manages no validator that could act as
// a prover.
var errNoActiveValidator = errors.New("no active validator available to sign")

// SignExecutionProofEnvelope signs an EIP-8025 execution proof envelope on
// behalf of one of the active validators this client manages, and returns the
// signature together with that validator's index.
func (v *validator) SignExecutionProofEnvelope(
	ctx context.Context,
	envelope *ethpb.ExecutionProofEnvelope,
	epoch primitives.Epoch,
) ([]byte, primitives.ValidatorIndex, error) {
	pubKey, index, err := v.randomActiveValidator()
	if err != nil {
		return nil, 0, err
	}

	domain, err := v.domainData(ctx, epoch, params.BeaconConfig().DomainExecutionProof[:])
	if err != nil {
		return nil, 0, fmt.Errorf("could not get execution proof signing domain: %w", err)
	}

	signingRoot, err := signing.ComputeSigningRoot(envelope, domain.SignatureDomain)
	if err != nil {
		return nil, 0, fmt.Errorf("could not compute execution proof signing root: %w", err)
	}

	sig, err := v.km.Sign(ctx, &validatorpb.SignRequest{
		PublicKey:       pubKey[:],
		SigningRoot:     signingRoot[:],
		SignatureDomain: domain.SignatureDomain,
	})

	if err != nil {
		return nil, 0, fmt.Errorf("could not sign execution proof envelope: %w", err)
	}

	return sig.Marshal(), index, nil
}

// randomActiveValidator picks one of the active validators this client manages.
func (v *validator) randomActiveValidator() ([fieldparams.BLSPubkeyLength]byte, primitives.ValidatorIndex, error) {
	v.pubkeyToStatusLock.RLock()
	defer v.pubkeyToStatusLock.RUnlock()

	active := make([][fieldparams.BLSPubkeyLength]byte, 0, len(v.pubkeyToStatus))
	for pubKey, status := range v.pubkeyToStatus {
		if status.status == nil {
			continue
		}
		if status.status.Status == ethpb.ValidatorStatus_ACTIVE || status.status.Status == ethpb.ValidatorStatus_EXITING {
			active = append(active, pubKey)
		}
	}

	if len(active) == 0 {
		return [fieldparams.BLSPubkeyLength]byte{}, 0, errNoActiveValidator
	}

	pubKey := active[rand.NewGenerator().Intn(len(active))]
	return pubKey, v.pubkeyToStatus[pubKey].index, nil
}
