package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/runtime/logging"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
)

// errGloasBlockNotSeen is the sentinel error returned by validateDataColumnGloas when
// the sidecar's block has not yet been seen. The caller detects this with errors.Is
// to queue the sidecar for deferred re-validation once the block arrives.
var errGloasBlockNotSeen = errors.New("gloas data column block not yet seen")

func (s *Service) validateDataColumnGloas(
	ctx context.Context,
	msg *pubsub.Message,
	roDataColumn blocks.RODataColumn,
	dataColumnSidecarSubTopic string,
) (blocks.VerifiedRODataColumn, error) {
	// data_column_sidecar_{subnet_id}
	// [Modified in Gloas:EIP7732]
	//
	// [IGNORE] A valid block for the sidecar's slot has been seen (via gossip or non-gossip sources).
	// If not yet seen, a client MUST queue the sidecar for deferred validation and possible processing once
	// the block is received or retrieved.
	if s.cfg.chain == nil || !s.cfg.chain.HasBlock(ctx, roDataColumn.BlockRoot()) {
		return blocks.VerifiedRODataColumn{}, ignoreValidation(errGloasBlockNotSeen)
	}

	block, err := s.cfg.beaconDB.Block(ctx, roDataColumn.BlockRoot())
	if err != nil {
		return blocks.VerifiedRODataColumn{}, ignoreValidation(err)
	}
	verifier := verification.NewGloasDataColumnVerifier(roDataColumn, block.Block(), verification.GossipDataColumnSidecarRequirementsGloas)
	verifier.SatisfyRequirement(verification.RequireBlockSeenGloas)

	// [REJECT] The sidecar's slot matches the slot of the block with root beacon_block_root.
	if err := verifier.VerifyDataColumnSidecarSlotMatchesBlockGloas(); err != nil {
		return blocks.VerifiedRODataColumn{}, errors.Wrap(err, "gloas data column validation")
	}

	// [REJECT] The sidecar is valid as verified by verify_data_column_sidecar(sidecar, bid.blob_kzg_commitments).
	if err := verifier.VerifyDataColumnSidecarGloas(); err != nil {
		return blocks.VerifiedRODataColumn{}, errors.Wrap(err, "gloas data column validation")
	}

	// [REJECT] The sidecar is for the correct subnet -- i.e.
	// compute_subnet_for_data_column_sidecar(sidecar.index) == subnet_id.
	if err := verifier.CorrectSubnet(dataColumnSidecarSubTopic, []string{*msg.Topic}); err != nil {
		return blocks.VerifiedRODataColumn{}, errors.Wrap(err, "gloas data column validation")
	}

	// [REJECT] The sidecar's column data is valid as verified by
	// verify_data_column_sidecar_kzg_proofs(sidecar, bid.blob_kzg_commitments).
	if err := verifier.VerifyDataColumnSidecarKzgProofsGloas(); err != nil {
		return blocks.VerifiedRODataColumn{}, errors.Wrap(err, "gloas data column validation")
	}

	// [IGNORE] The sidecar is the first sidecar for the tuple
	// (sidecar.beacon_block_root, sidecar.index) with valid kzg proof.
	//
	// Note: If the sidecar fails deferred validation, its forwarding peers MUST be downscored
	// retroactively. If validation succeeds, the client MUST re-broadcast the sidecar.
	if s.hasSeenDataColumnRootIndex(roDataColumn.BlockRoot(), roDataColumn.Index()) {
		return blocks.VerifiedRODataColumn{}, ignoreValidation(errors.New("data column sidecar already seen for block root"))
	}
	verifier.SatisfyRequirement(verification.RequireNotSeenGloas)

	verifiedRODataColumn, err := verifier.VerifiedRODataColumn()
	if err != nil {
		log.WithError(err).WithFields(logging.DataColumnFields(roDataColumn)).Error("Failed to get verified gloas data columns")
		return blocks.VerifiedRODataColumn{}, ignoreValidation(err)
	}

	s.setSeenDataColumnRootIndex(verifiedRODataColumn.BlockRoot(), verifiedRODataColumn.Index(), verifiedRODataColumn.Slot())
	return verifiedRODataColumn, nil
}

func (s *Service) hasSeenDataColumnRootIndex(root [fieldparams.RootLength]byte, index uint64) bool {
	key := computeRootIndexCacheKey(root, index)
	_, seen := s.seenDataColumnCache.Get(key)
	return seen
}

func (s *Service) setSeenDataColumnRootIndex(root [fieldparams.RootLength]byte, index uint64, slot primitives.Slot) {
	key := computeRootIndexCacheKey(root, index)
	s.seenDataColumnCache.Add(slot, key, true)
}

func computeRootIndexCacheKey(root [fieldparams.RootLength]byte, index uint64) string {
	key := make([]byte, 0, fieldparams.RootLength+32)
	key = append(key, root[:]...)
	key = append(key, bytesutil.Bytes32(index)...)
	return string(key)
}
