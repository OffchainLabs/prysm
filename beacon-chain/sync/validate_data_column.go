package sync

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/rand"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/logging"
	prysmTime "github.com/OffchainLabs/prysm/v6/time"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"

	"github.com/sirupsen/logrus"
)

// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/p2p-interface.md#the-gossip-domain-gossipsub
func (s *Service) validateDataColumn(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	const dataColumnSidecarSubTopic = "/data_column_sidecar_%d/"

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

	// Reject messages with a nil topic.
	if msg.Topic == nil {
		return pubsub.ValidationReject, p2p.ErrInvalidTopic
	}

	// Decode the message, reject if it fails.
	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		log.WithError(err).Error("Failed to decode message")
		return pubsub.ValidationReject, err
	}

	// Reject messages that are not of the expected type.
	dcsc, ok := m.(*eth.DataColumnSidecar)
	if !ok {
		log.WithField("message", m).Error("Message is not of type *eth.DataColumnSidecar")
		return pubsub.ValidationReject, errWrongMessage
	}

	// Convert to a read-only data column sidecar.
	roDataColumn, err := blocks.NewRODataColumn(dcsc)
	if err != nil {
		return pubsub.ValidationReject, errors.Wrap(err, "roDataColumn conversion failure")
	}

	// Compute a batch of only one data column sidecar.
	roDataColumns := []blocks.RODataColumn{roDataColumn}

	// Create the verifier.
	verifier := s.newColumnsVerifier(roDataColumns, verification.GossipDataColumnSidecarRequirements)

	// Start the verification process.
	// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/p2p-interface.md#data_column_sidecar_subnet_id

	// [REJECT] The sidecar is valid as verified by `verify_data_column_sidecar(sidecar)`.
	if err := verifier.ValidFields(); err != nil {
		return pubsub.ValidationReject, err
	}

	// [REJECT] The sidecar is for the correct subnet -- i.e. `compute_subnet_for_data_column_sidecar(sidecar.index) == subnet_id`.
	if err := verifier.CorrectSubnet(dataColumnSidecarSubTopic, []string{*msg.Topic}); err != nil {
		return pubsub.ValidationReject, err
	}

	// [IGNORE] The sidecar is not from a future slot (with a `MAXIMUM_GOSSIP_CLOCK_DISPARITY` allowance)
	//  -- i.e. validate that `block_header.slot <= current_slot` (a client MAY queue future sidecars for processing at the appropriate slot).
	if err := verifier.NotFromFutureSlot(); err != nil {
		return pubsub.ValidationIgnore, err
	}

	// [IGNORE] The sidecar is from a slot greater than the latest finalized slot
	// -- i.e. validate that `block_header.slot > compute_start_slot_at_epoch(state.finalized_checkpoint.epoch)`
	if err := verifier.SlotAboveFinalized(); err != nil {
		return pubsub.ValidationIgnore, err
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

	// [REJECT] The proposer signature of `sidecar.signed_block_header`, is valid with respect to the `block_header.proposer_index` pubkey.
	//          We do not strictly respect the spec ordering here. This is necessary because signature verification depends on the parent root,
	//          which is only available if the parent block is known.
	if err := verifier.ValidProposerSignature(ctx); err != nil {
		return pubsub.ValidationReject, err
	}

	// [REJECT] The sidecar is from a higher slot than the sidecar's block's parent (defined by `block_header.parent_root`).
	if err := verifier.SidecarParentSlotLower(); err != nil {
		return pubsub.ValidationReject, err
	}

	// [REJECT] The current `finalized_checkpoint` is an ancestor of the sidecar's block
	// -- i.e. `get_checkpoint_block(store, block_header.parent_root, store.finalized_checkpoint.epoch) == store.finalized_checkpoint.root`.
	if err := verifier.SidecarDescendsFromFinalized(); err != nil {
		return pubsub.ValidationReject, err
	}

	// [REJECT] The sidecar's `kzg_commitments` field inclusion proof is valid as verified by `verify_data_column_sidecar_inclusion_proof(sidecar)`.
	if err := verifier.SidecarInclusionProven(); err != nil {
		return pubsub.ValidationReject, err
	}

	// [REJECT] The sidecar's column data is valid as verified by `verify_data_column_sidecar_kzg_proofs(sidecar)`.
	validationResult, err := s.validateWithKzgBatchVerifier(ctx, roDataColumns)
	if validationResult != pubsub.ValidationAccept {
		return validationResult, err
	}
	// Mark KZG verification as satisfied since we did it via batch verifier
	verifier.SatisfyRequirement(verification.RequireSidecarKzgProofVerified)

	// [IGNORE] The sidecar is the first sidecar for the tuple `(block_header.slot, block_header.proposer_index, sidecar.index)`
	// with valid header signature, sidecar inclusion proof, and kzg proof.
	if s.hasSeenDataColumnIndex(roDataColumn.Slot(), roDataColumn.ProposerIndex(), roDataColumn.DataColumnSidecar.Index) {
		return pubsub.ValidationIgnore, nil
	}

	// [REJECT] The sidecar is proposed by the expected `proposer_index` for the block's slot in the context of the current shuffling (defined by `block_header.parent_root`/`block_header.slot`).
	// If the `proposer_index` cannot immediately be verified against the expected shuffling, the sidecar MAY be queued for later processing while proposers for the block's branch are calculated
	// -- in such a case do not REJECT, instead IGNORE this message.
	if err := verifier.SidecarProposerExpected(ctx); err != nil {
		return pubsub.ValidationReject, err
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

	// Get the time at slot start.
	startTime, err := slots.StartTime(s.cfg.clock.GenesisTime(), roDataColumn.SignedBlockHeader.Header.Slot)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	sinceSlotStartTime := receivedTime.Sub(startTime)
	validationTime := s.cfg.clock.Now().Sub(receivedTime)
	dataColumnSidecarVerificationGossipHistogram.Observe(float64(validationTime.Milliseconds()))

	peerGossipScore := s.cfg.p2p.Peers().Scorers().GossipScorer().Score(pid)

	select {
	case s.dataColumnLogCh <- dataColumnLogEntry{
		Slot:            roDataColumn.Slot(),
		ColIdx:          roDataColumn.Index,
		PropIdx:         roDataColumn.ProposerIndex(),
		BlockRoot:       roDataColumn.BlockRoot(),
		ParentRoot:      roDataColumn.ParentRoot(),
		PeerSuffix:      pid.String()[len(pid.String())-6:],
		PeerGossipScore: peerGossipScore,
		validationTime:  validationTime,
		sinceStartTime:  sinceSlotStartTime,
	}:
	default:
		log.WithField("slot", roDataColumn.Slot()).Warn("Failed to send data column log entry")
	}

	if s.cfg.operationNotifier != nil {
		s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
			Type: operation.DataColumnReceived,
			Data: &operation.DataColumnReceivedData{
				Slot:           roDataColumn.Slot(),
				Index:          roDataColumn.Index,
				BlockRoot:      roDataColumn.BlockRoot(),
				KzgCommitments: bytesutil.SafeCopy2dBytes(roDataColumn.KzgCommitments),
			},
		})
	}

	return pubsub.ValidationAccept, nil
}

// Returns true if the column with the same slot, proposer index, and column index has been seen before.
func (s *Service) hasSeenDataColumnIndex(slot primitives.Slot, proposerIndex primitives.ValidatorIndex, index uint64) bool {
	key := computeCacheKey(slot, proposerIndex, index)
	_, seen := s.seenDataColumnCache.Get(key)
	return seen
}

// Sets the data column with the same slot, proposer index, and data column index as seen.
func (s *Service) setSeenDataColumnIndex(slot primitives.Slot, proposerIndex primitives.ValidatorIndex, index uint64) {
	key := computeCacheKey(slot, proposerIndex, index)
	s.seenDataColumnCache.Add(slot, key, true)
}

func computeCacheKey(slot primitives.Slot, proposerIndex primitives.ValidatorIndex, index uint64) string {
	key := make([]byte, 0, 96)

	key = append(key, bytesutil.Bytes32(uint64(slot))...)
	key = append(key, bytesutil.Bytes32(uint64(proposerIndex))...)
	key = append(key, bytesutil.Bytes32(index)...)

	return string(key)
}

type dataColumnLogEntry struct {
	Slot            primitives.Slot
	ColIdx          uint64
	PropIdx         primitives.ValidatorIndex
	BlockRoot       [32]byte
	ParentRoot      [32]byte
	PeerSuffix      string
	PeerGossipScore float64
	validationTime  time.Duration
	sinceStartTime  time.Duration
}

func (s *Service) processDataColumnLogs() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	slotStats := make(map[primitives.Slot][fieldparams.NumberOfColumns]dataColumnLogEntry)

	for {
		select {
		case entry := <-s.dataColumnLogCh:
			cols := slotStats[entry.Slot]
			cols[entry.ColIdx] = entry
			slotStats[entry.Slot] = cols
		case <-ticker.C:
			for slot, columns := range slotStats {
				var (
					colIndices      = make([]uint64, 0, fieldparams.NumberOfColumns)
					peers           = make([]string, 0, fieldparams.NumberOfColumns)
					gossipScores    = make([]float64, 0, fieldparams.NumberOfColumns)
					validationTimes = make([]string, 0, fieldparams.NumberOfColumns)
					sinceStartTimes = make([]string, 0, fieldparams.NumberOfColumns)
				)

				totalReceived := 0
				for _, entry := range columns {
					if entry.PeerSuffix == "" {
						continue
					}
					colIndices = append(colIndices, entry.ColIdx)
					peers = append(peers, entry.PeerSuffix)
					gossipScores = append(gossipScores, roundFloat(entry.PeerGossipScore, 2))
					validationTimes = append(validationTimes, fmt.Sprintf("%.2fms", float64(entry.validationTime.Milliseconds())))
					sinceStartTimes = append(sinceStartTimes, fmt.Sprintf("%.2fms", float64(entry.sinceStartTime.Milliseconds())))
					totalReceived++
				}

				log.WithFields(logrus.Fields{
					"slot":            slot,
					"receivedCount":   totalReceived,
					"columnIndices":   colIndices,
					"peers":           peers,
					"gossipScores":    gossipScores,
					"validationTimes": validationTimes,
					"sinceStartTimes": sinceStartTimes,
				}).Debug("Accepted data column sidecars summary")
			}
			slotStats = make(map[primitives.Slot][fieldparams.NumberOfColumns]dataColumnLogEntry)
		}
	}
}

func roundFloat(f float64, decimals int) float64 {
	mult := math.Pow(10, float64(decimals))
	return math.Round(f*mult) / mult
}
