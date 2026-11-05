package helpers

import (
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// ComputeDomainData computes the signature domain for the given epoch and domain type
// from the local fork schedule.
func ComputeDomainData(
	epoch primitives.Epoch,
	domainType [4]byte,
	genesisValidatorsRoot []byte,
	genesisTime time.Time,
) (*ethpb.DomainResponse, error) {
	if len(genesisValidatorsRoot) != fieldparams.RootLength {
		return nil, fmt.Errorf("invalid genesis validators root length: got %d, want %d", len(genesisValidatorsRoot), fieldparams.RootLength)
	}

	cfg := params.BeaconConfig()

	fork, err := params.Fork(epoch)
	if err != nil {
		return nil, fmt.Errorf("failed to get fork version for epoch %d: %w", epoch, err)
	}

	// EIP-7044: once the chain is in Deneb, the beacon node verifies every voluntary exit
	// with the Capella fork version, whatever the exit epoch.
	if domainType == cfg.DomainVoluntaryExit && slots.ToEpoch(slots.CurrentSlot(genesisTime)) >= cfg.DenebForkEpoch {
		fork = &ethpb.Fork{
			PreviousVersion: cfg.CapellaForkVersion,
			CurrentVersion:  cfg.CapellaForkVersion,
			Epoch:           cfg.CapellaForkEpoch,
		}
	}

	signatureDomain, err := signing.Domain(fork, epoch, domainType, genesisValidatorsRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to compute signature domain: %w", err)
	}

	return &ethpb.DomainResponse{SignatureDomain: signatureDomain}, nil
}
