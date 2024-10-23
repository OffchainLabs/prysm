package sync

import (
	"context"
	"fmt"
	"strings"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
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

// https://github.com/ethereum/consensus-specs/blob/dev/specs/_features/eip7594/p2p-interface.md#the-gossip-domain-gossipsub
func (s *Service) validateDataColumn(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
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
			"columnIndex": roDataColumn.ColumnIndex,
			"blockRoot":   fmt.Sprintf("%#x", roDataColumn.BlockRoot()),
		}).Warning("Voluntary ignore data column sidecar gossip")

		return pubsub.ValidationIgnore, err
	}

	roDataColumns := []blocks.RODataColumn{roDataColumn}

	verifier := s.newColumnsVerifier(roDataColumns, verification.GossipColumnSidecarRequirements)

	if err := verifier.DataColumnsIndexInBounds(); err != nil {
		return pubsub.ValidationReject, err
	}

	// [REJECT] The sidecar is for the correct subnet -- i.e. compute_subnet_for_data_column_sidecar(sidecar.index) == subnet_id.
	want := fmt.Sprintf("data_column_sidecar_%d", computeSubnetForColumnSidecar(roDataColumn.ColumnIndex))
	if !strings.Contains(*msg.Topic, want) {
		log.Debug("Column Sidecar index does not match topic")
		return pubsub.ValidationReject, fmt.Errorf("wrong topic name: %s", *msg.Topic)
	}

	if err := verifier.NotFromFutureSlot(); err != nil {
		return pubsub.ValidationIgnore, err
	}

	// [IGNORE] The sidecar is the first sidecar for the tuple (block_header.slot, block_header.proposer_index, sidecar.index) with valid header signature, sidecar inclusion proof, and kzg proof.
	if s.hasSeenDataColumnIndex(roDataColumn.Slot(), roDataColumn.ProposerIndex(), roDataColumn.DataColumnSidecar.ColumnIndex) {
		return pubsub.ValidationIgnore, nil
	}

	if err := verifier.SlotAboveFinalized(); err != nil {
		return pubsub.ValidationIgnore, err
	}
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
	if err := verifier.SidecarParentValid(s.hasBadBlock); err != nil {
		return pubsub.ValidationReject, err
	}

	if err := verifier.SidecarParentSlotLower(); err != nil {
		return pubsub.ValidationReject, err
	}

	if err := verifier.SidecarDescendsFromFinalized(); err != nil {
		return pubsub.ValidationReject, err
	}

	if err := verifier.SidecarInclusionProven(); err != nil {
		return pubsub.ValidationReject, err
	}

	if err := verifier.SidecarKzgProofVerified(); err != nil {
		return pubsub.ValidationReject, err
	}
	if err := verifier.ValidProposerSignature(ctx); err != nil {
		return pubsub.ValidationReject, err
	}
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
		return pubsub.ValidationReject, err
	}

	verifiedRODataColumnsCount := len(verifiedRODataColumns)

	if verifiedRODataColumnsCount != 1 {
		// This should never happen.
		log.WithField("verifiedRODataColumnsCount", verifiedRODataColumnsCount).Error("Verified data columns count is not 1")
		return pubsub.ValidationIgnore, errors.New("Wrong number of verified data columns")
	}

	msg.ValidatorData = verifiedRODataColumns[0]

	sinceSlotStartTime := receivedTime.Sub(startTime)
	validationTime := s.cfg.clock.Now().Sub(receivedTime)

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
	s.seenDataColumnLock.RLock()
	defer s.seenDataColumnLock.RUnlock()
	b := append(bytesutil.Bytes32(uint64(slot)), bytesutil.Bytes32(uint64(proposerIndex))...)
	b = append(b, bytesutil.Bytes32(index)...)
	_, seen := s.seenDataColumnCache.Get(string(b))
	return seen
}

// Sets the data column with the same slot, proposer index, and data column index as seen.
func (s *Service) setSeenDataColumnIndex(slot primitives.Slot, proposerIndex primitives.ValidatorIndex, index uint64) {
	s.seenDataColumnLock.Lock()
	defer s.seenDataColumnLock.Unlock()

	b := append(bytesutil.Bytes32(uint64(slot)), bytesutil.Bytes32(uint64(proposerIndex))...)
	b = append(b, bytesutil.Bytes32(index)...)
	s.seenDataColumnCache.Add(string(b), true)
}

func computeSubnetForColumnSidecar(colIdx uint64) uint64 {
	return colIdx % params.BeaconConfig().DataColumnSidecarSubnetCount
}
