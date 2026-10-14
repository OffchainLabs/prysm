package filesystem

import (
	"context"
	"errors"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// TODO(DC): the minimal slashing protection store keeps no Decoupled record, so these duties refuse to sign.
var errDecoupledNotSupported = errors.New("decoupled duties are not supported by the minimal slashing protection database")

func (*Store) FinalityHistory(context.Context, [fieldparams.BLSPubkeyLength]byte) ([]*ethpb.AttestationDataDecoupled, bool, bool, error) {
	return nil, false, false, errDecoupledNotSupported
}

func (*Store) MarkFinalityKey(context.Context, [fieldparams.BLSPubkeyLength]byte) error {
	return errDecoupledNotSupported
}

func (*Store) MarkFinalitySubmitted(context.Context, [fieldparams.BLSPubkeyLength]byte, *ethpb.AttestationDataDecoupled) error {
	return errDecoupledNotSupported
}

func (*Store) SaveFinalityAttestation(context.Context, [fieldparams.BLSPubkeyLength]byte, *ethpb.AttestationDataDecoupled) error {
	return errDecoupledNotSupported
}

func (*Store) ReserveAvailableVote(context.Context, [fieldparams.BLSPubkeyLength]byte, primitives.Slot, [32]byte) error {
	return errDecoupledNotSupported
}
