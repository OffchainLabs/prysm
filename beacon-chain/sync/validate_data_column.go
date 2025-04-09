package sync

import (
	"context"
	"fmt"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/rand"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/logging"
	prysmTime "github.com/prysmaticlabs/prysm/v5/time"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
)

// https://github.com/ethereum/consensus-specs/blob/dev/specs/fulu/p2p-interface.md#the-gossip-domain-gossipsub
func (s *Service) validateDataColumn(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	dataColumnSidecarVerificationRequestsCounter.Inc()
	receivedTime := prysmTime.Now()

	// Always accept messages our own messages.
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}

	// Ignore messages during initial sync.
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	// Ignore message with a nil topic.
	if msg.Topic == nil {
		return pubsub.ValidationReject, errInvalidTopic
	}

	// Decode the message.
	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		log.WithError(err).Error("Failed to decode message")
		return pubsub.ValidationReject, err
	}

	// Ignore messages that are not of the expected type.
	dspb, ok := m.(*eth.DataColumnSidecar)
	if !ok {
		log.WithField("message", m).Error("Message is not of type *eth.DataColumnSidecar")
		return pubsub.ValidationReject, errWrongMessage
	}

	roDataColumn, err := blocks.NewRODataColumn(dspb)
	if err != nil {
		return pubsub.ValidationReject, errors.Wrap(err, "roDataColumn conversion failure")
	}

	// Voluntary ignore messages (for debugging purposes).
	dataColumnsIgnoreSlotMultiple := features.Get().DataColumnsIgnoreSlotMultiple
	blockSlot := uint64(roDataColumn.SignedBlockHeader.Header.Slot)

	if dataColumnsIgnoreSlotMultiple != 0 && blockSlot%dataColumnsIgnoreSlotMultiple == 0 {
		log.WithFields(logrus.Fields{
			"slot":        blockSlot,
			"columnIndex": roDataColumn.Index,
			"blockRoot":   fmt.Sprintf("%#x", roDataColumn.BlockRoot()),
		}).Warning("Voluntary ignore data column sidecar gossip")

		return pubsub.ValidationIgnore, err
	}

	// Compute a batch of only one data column sidecar.
	roDataColumns := []blocks.RODataColumn{roDataColumn}

	// Create the verifier.
	verifier := s.newColumnsVerifier(roDataColumns, verification.GossipDataColumnSidecarRequirements)

	// Start the verification process.
	// https://github.com/ethereum/consensus-specs/blob/dev/specs/fulu/p2p-interface.md#the-gossip-domain-gossipsub

	// [REJECT] The sidecar is valid as verified by `verify_data_column_sidecar(sidecar)`.
	if err := verifier.Valid(); err != nil {
		return pubsub.ValidationReject, err
	}

	// [REJECT] The sidecar is for the correct subnet -- i.e. `compute_subnet_for_data_column_sidecar(sidecar.index) == subnet_id`.
	if err := verifier.CorrectSubnet([]string{*msg.Topic}); err != nil {
		return pubsub.ValidationReject, err
	}

	// [IGNORE] The sidecar is not from a future slot (with a `MAXIMUM_GOSSIP_CLOCK_DISPARITY`` allowance
	//  -- i.e. validate that `block_header.slot <= current_slot` (a client MAY queue future sidecars for processing at the appropriate slot).
	if err := verifier.NotFromFutureSlot(); err != nil {
		return pubsub.ValidationIgnore, err
	}

	// [IGNORE] The sidecar is from a slot greater than the latest finalized slot
	// -- i.e. validate that `block_header.slot > compute_start_slot_at_epoch(state.finalized_checkpoint.epoch)`
	if err := verifier.SlotAboveFinalized(); err != nil {
		return pubsub.ValidationIgnore, err
	}

	// [REJECT] The proposer signature of `sidecar.signed_block_header`, is valid with respect to the `block_header.proposer_index` pubkey.
	if err := verifier.ValidProposerSignature(ctx); err != nil {
		return pubsub.ValidationReject, err
	}

	// [IGNORE] The sidecar's block's parent (defined by `block_header.parent_root`) has been seen (via gossip or non-gossip sources
	// (a client MAY queue sidecars for processing once the parent block is retrieved).
	if err := verifier.SidecarParentSeen(s.hasBadBlock); err != nil {
		// If we haven't seen the parent, request it asynchronously.
		go func() {
			customCtx := context.Background()
			parentRoot := roDataColumn.ParentRoot()
			roots := [][fieldparams.RootLength]byte{parentRoot}
			randGenerator := rand.NewGenerator()
			if err := s.sendBatchRootRequest(customCtx, roots, randGenerator); err != nil {
				log.WithError(err).WithFields(logging.DataColumnFields(roDataColumn)).Debug("Failed to send batch root request")
			}
		}()

		return pubsub.ValidationIgnore, err
	}

	// [REJECT] The sidecar's block's parent (defined by `block_header.parent_root`) passes validation.
	if err := verifier.SidecarParentValid(s.hasBadBlock); err != nil {
		return pubsub.ValidationReject, err
	}

	// [REJECT] The sidecar is from a higher slot than the sidecar's block's parent (defined by `block_header.parent_root`).
	if err := verifier.SidecarParentSlotLower(); err != nil {
		return pubsub.ValidationReject, err
	}

	// [REJECT] The current finalized_checkpoint is an ancestor of the sidecar's block
	// -- i.e. `get_checkpoint_block(store, block_header.parent_root, store.finalized_checkpoint.epoch) == store.finalized_checkpoint.root`.
	if err := verifier.SidecarDescendsFromFinalized(); err != nil {
		return pubsub.ValidationReject, err
	}

	// [REJECT] The sidecar's kzg_commitments field inclusion proof is valid as verified by `verify_data_column_sidecar_inclusion_proof(sidecar)`.
	if err := verifier.SidecarInclusionProven(); err != nil {
		return pubsub.ValidationReject, err
	}

	// [REJECT] The sidecar's column data is valid as verified by `verify_data_column_sidecar_kzg_proofs(sidecar)`.
	if err := verifier.SidecarKzgProofVerified(); err != nil {
		return pubsub.ValidationReject, err
	}

	// TODO: Try to fit this requirement into the verifier.
	// [IGNORE] The sidecar is the first sidecar for the tuple `(block_header.slot, block_header.proposer_index, sidecar.index)`
	// with valid header signature, sidecar inclusion proof, and kzg proof.
	if s.hasSeenDataColumnIndex(roDataColumn.Slot(), roDataColumn.ProposerIndex(), roDataColumn.DataColumnSidecar.Index) {
		return pubsub.ValidationIgnore, nil
	}

	// [REJECT] The sidecar is proposed by the expected `proposer_index` for the block's slot in the context of the current shuffling (defined by block_header.parent_root/block_header.slot).
	// If the `proposer_index` cannot immediately be verified against the expected shuffling, the sidecar MAY be queued for later processing while proposers for the block's branch are calculated
	// -- in such a case do not REJECT, instead IGNORE this message.
	if err := verifier.SidecarProposerExpected(ctx); err != nil {
		return pubsub.ValidationReject, err
	}

	// Get the time at slot start.
	startTime, err := slots.ToTime(uint64(s.cfg.chain.GenesisTime().Unix()), roDataColumn.SignedBlockHeader.Header.Slot)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	verifiedRODataColumns, err := verifier.VerifiedRODataColumns()
	if err != nil {
		// This should never happen.
		log.WithError(err).WithFields(logging.DataColumnFields(roDataColumn)).Error("Failed to get verified data columns")
		return pubsub.ValidationIgnore, err
	}

	verifiedRODataColumnsCount := len(verifiedRODataColumns)

	if verifiedRODataColumnsCount != 1 {
		// This should never happen.
		log.WithField("verifiedRODataColumnsCount", verifiedRODataColumnsCount).Error("Verified data columns count is not 1")
		return pubsub.ValidationIgnore, errors.New("Wrong number of verified data columns")
	}

	msg.ValidatorData = verifiedRODataColumns[0]
	dataColumnSidecarVerificationSuccessesCounter.Inc()

	sinceSlotStartTime := receivedTime.Sub(startTime)
	validationTime := s.cfg.clock.Now().Sub(receivedTime)

	dataColumnSidecarVerificationGossipHistogram.Observe(float64(validationTime.Milliseconds()))

	peerGossipScore := s.cfg.p2p.Peers().Scorers().GossipScorer().Score(pid)

	pidString := pid.String()

	log.
		WithFields(logging.DataColumnFields(roDataColumn)).
		WithFields(logrus.Fields{
			"sinceSlotStartTime": sinceSlotStartTime,
			"validationTime":     validationTime,
			"peer":               pidString[len(pidString)-6:],
			"peerGossipScore":    peerGossipScore,
		}).
		Debug("Accepted data column sidecar gossip")

	return pubsub.ValidationAccept, nil
}

// Returns true if the column with the same slot, proposer index, and column index has been seen before.
func (s *Service) hasSeenDataColumnIndex(slot primitives.Slot, proposerIndex primitives.ValidatorIndex, index uint64) bool {
	b := append(bytesutil.Bytes32(uint64(slot)), bytesutil.Bytes32(uint64(proposerIndex))...)
	b = append(b, bytesutil.Bytes32(index)...)
	_, seen := s.seenDataColumnCache.Get(string(b))
	return seen
}

// Sets the data column with the same slot, proposer index, and data column index as seen.
func (s *Service) setSeenDataColumnIndex(slot primitives.Slot, proposerIndex primitives.ValidatorIndex, index uint64) {
	b := append(bytesutil.Bytes32(uint64(slot)), bytesutil.Bytes32(uint64(proposerIndex))...)
	b = append(b, bytesutil.Bytes32(index)...)
	s.seenDataColumnCache.Add(string(b), true)
}
