package slasher

import (
	"context"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/blocks"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// Verifies attester slashings, logs them, and submits them to the slashing operations pool
// in the beacon node if they pass validation.
func (s *Service) processAttesterSlashings(
	ctx context.Context, slashings map[[fieldparams.RootLength]byte]ethpb.AttSlashing,
) (map[[fieldparams.RootLength]byte]ethpb.AttSlashing, error) {
	processedSlashings := map[[fieldparams.RootLength]byte]ethpb.AttSlashing{}

	// If no slashings, return early.
	if len(slashings) == 0 {
		return processedSlashings, nil
	}

	// Get the head state.
	beaconState, err := s.serviceCfg.HeadStateFetcher.HeadState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get head state")
	}

	for root, slashing := range slashings {
		// Verify the signature of the first attestation.
		if err := s.verifyAttSignature(ctx, slashing.FirstAttestation()); err != nil {
			log.WithError(err).WithField("a", slashing.FirstAttestation()).Warn(
				"Invalid signature for attestation in detected slashing offense",
			)

			continue
		}

		// Verify the signature of the second attestation.
		if err := s.verifyAttSignature(ctx, slashing.SecondAttestation()); err != nil {
			log.WithError(err).WithField("b", slashing.SecondAttestation()).Warn(
				"Invalid signature for attestation in detected slashing offense",
			)

			continue
		}

		// Log the slashing event and insert into the beacon node's operations pool.
		logAttesterSlashing(slashing)
		if err := s.serviceCfg.SlashingPoolInserter.InsertAttesterSlashing(ctx, beaconState, slashing); err != nil {
			log.WithError(err).Error("Could not insert attester slashing into operations pool")
		}

		processedSlashings[root] = slashing
	}

	return processedSlashings, nil
}

// Verifies proposer slashings, logs them, and submits them to the slashing operations pool
// in the beacon node if they pass validation.
func (s *Service) processProposerSlashings(ctx context.Context, slashings []*ethpb.ProposerSlashing) error {
	// If no slashings, return early.
	if len(slashings) == 0 {
		return nil
	}

	// Get the head state.
	beaconState, err := s.serviceCfg.HeadStateFetcher.HeadState(ctx)
	if err != nil {
		return err
	}

	for _, slashing := range slashings {
		// Verify the signature of the first block.
		if err := s.verifyBlockSignature(ctx, slashing.Header_1); err != nil {
			log.WithError(err).WithField("a", slashing.Header_1).Warn(
				"Invalid signature for block header in detected slashing offense",
			)

			continue
		}

		// Verify the signature of the second block.
		if err := s.verifyBlockSignature(ctx, slashing.Header_2); err != nil {
			log.WithError(err).WithField("b", slashing.Header_2).Warn(
				"Invalid signature for block header in detected slashing offense",
			)

			continue
		}

		// Log the slashing event and insert into the beacon node's operations pool.
		logProposerSlashing(slashing)
		if err := s.serviceCfg.SlashingPoolInserter.InsertProposerSlashing(ctx, beaconState, slashing); err != nil {
			log.WithError(err).Error("Could not insert proposer slashing into operations pool")
		}
	}

	return nil
}

func (s *Service) verifyBlockSignature(ctx context.Context, header *ethpb.SignedBeaconBlockHeader) error {
	parentState, err := s.serviceCfg.StateGen.StateByRoot(ctx, bytesutil.ToBytes32(header.Header.ParentRoot))
	if err != nil {
		return err
	}
	return blocks.VerifyBlockHeaderSignature(parentState, header)
}

func (s *Service) verifyAttSignature(ctx context.Context, att ethpb.IndexedAtt) error {
	preState, err := s.serviceCfg.AttestationStateFetcher.AttestationTargetState(ctx, att.GetData().Target)
	if err != nil {
		return err
	}
	return blocks.VerifyIndexedAttestation(ctx, preState, att)
}
