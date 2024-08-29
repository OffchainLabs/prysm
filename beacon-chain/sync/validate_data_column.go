package sync

import (
	"context"
	"fmt"
	"strings"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/rand"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/logging"
	prysmTime "github.com/prysmaticlabs/prysm/v5/time"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
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

	ds, err := blocks.NewRODataColumn(dspb)
	if err != nil {
		return pubsub.ValidationReject, errors.Wrap(err, "roDataColumn conversion failure")
	}

	verifier := s.newColumnVerifier(ds, verification.GossipColumnSidecarRequirements)

	if err := verifier.DataColumnIndexInBounds(); err != nil {
		return pubsub.ValidationReject, err
	}

	// [REJECT] The sidecar is for the correct subnet -- i.e. compute_subnet_for_data_column_sidecar(sidecar.index) == subnet_id.
	want := fmt.Sprintf("data_column_sidecar_%d", computeSubnetForColumnSidecar(ds.ColumnIndex))
	if !strings.Contains(*msg.Topic, want) {
		log.Debug("Column Sidecar index does not match topic")
		return pubsub.ValidationReject, fmt.Errorf("wrong topic name: %s", *msg.Topic)
	}

	if err := verifier.NotFromFutureSlot(); err != nil {
		return pubsub.ValidationIgnore, err
	}

	// [IGNORE] The sidecar is the first sidecar for the tuple (block_header.slot, block_header.proposer_index, sidecar.index) with valid header signature, sidecar inclusion proof, and kzg proof.
	if s.hasSeenDataColumnIndex(ds.Slot(), ds.ProposerIndex(), ds.DataColumnSidecar.ColumnIndex) {
		return pubsub.ValidationIgnore, nil
	}

	if err := verifier.SlotAboveFinalized(); err != nil {
		return pubsub.ValidationIgnore, err
	}
	if err := verifier.SidecarParentSeen(s.hasBadBlock); err != nil {
		go func() {
			if err := s.sendBatchRootRequest(context.Background(), [][32]byte{ds.ParentRoot()}, rand.NewGenerator()); err != nil {
				log.WithError(err).WithFields(logging.DataColumnFields(ds)).Debug("Failed to send batch root request")
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
	startTime, err := slots.ToTime(uint64(s.cfg.chain.GenesisTime().Unix()), ds.SignedBlockHeader.Header.Slot)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	verifiedRODataColumn, err := verifier.VerifiedRODataColumn()
	if err != nil {
		return pubsub.ValidationReject, err
	}

	msg.ValidatorData = verifiedRODataColumn

	fields := logging.DataColumnFields(ds)
	sinceSlotStartTime := receivedTime.Sub(startTime)
	validationTime := s.cfg.clock.Now().Sub(receivedTime)
	fields["sinceSlotStartTime"] = sinceSlotStartTime
	fields["validationTime"] = validationTime

	log.WithFields(fields).Debug("Accepted data column sidecar gossip")
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
