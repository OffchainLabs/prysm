package sync

import (
	"context"
	"fmt"
	"strings"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

func (s *Service) validateCell(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	const cellSidecarSubTopic = "/cell_sidecar_%d/"

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
		return pubsub.ValidationReject, errInvalidTopic
	}

	// Decode the message.
	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		log.WithError(err).Error("Failed to decode message")
		return pubsub.ValidationReject, err
	}

	// Reject messages that are not of the expected type.
	cellSidecar, ok := m.(*eth.CellSidecar)
	if !ok {
		log.WithField("message", m).Error("Message is not of type *eth.CellSidecar")
		return pubsub.ValidationReject, errWrongMessage
	}

	// todo(healthykim): Do we need verifier?
	// todo(healthykim): Elaborate verifying logic

	// Convert cellSidecar to ROCell type
	roCell, err := blocks.NewROCell(cellSidecar)
	if err != nil {
		log.WithError(err).Error("Failed to create ROCell from cellSidecar")
		return pubsub.ValidationReject, err
	}

	cellSidecars := []blocks.ROCell{roCell}

	// todo(healthykim): Add geth api for step 1
	// Step 2 Correct subnet
	expectedTopic := *msg.Topic

	actualSubnet := peerdas.ComputeSubnetForCellSidecar(cellSidecar.ColumnIndex)
	actualSubTopic := fmt.Sprintf(cellSidecarSubTopic, actualSubnet)

	if !strings.Contains(expectedTopic, actualSubTopic) {
		return pubsub.ValidationReject, errors.New("topic is not of the one expected")
	}

	// Step 3 kzg_cell_proof against kzg_commitment for position column
	if err := peerdas.VerifyCellSidecarKZGProofs(cellSidecars); err != nil {
		return pubsub.ValidationReject, err
	}

	// Step 4 first received
	seen := s.hasSeenCellIndex(cellSidecar.TxHash, cellSidecar.BlobIndex, cellSidecar.ColumnIndex)

	if seen {
		return pubsub.ValidationIgnore, nil
	}

	verifiedCell := blocks.NewVerifiedROCell(roCell)
	msg.ValidatorData = verifiedCell

	return pubsub.ValidationAccept, nil
}

// Returns true if the cell with the same txHash, blob index and column index has been seen before.
func (s *Service) hasSeenCellIndex(txHash []byte, blobIndex uint32, columnIndex uint64) bool {
	b := append(txHash, bytesutil.Bytes32(uint64(blobIndex))...)
	b = append(b, bytesutil.Bytes32(columnIndex)...)
	_, seen := s.seenCellCache.Get(string(b))
	return seen
}

// Sets the cell with the same txHash, blob index and column index as seen.
func (s *Service) setSeenCellIndex(txHash []byte, blobIndex uint32, columnIndex uint64) {
	b := append(txHash, bytesutil.Bytes32(uint64(blobIndex))...)
	b = append(b, bytesutil.Bytes32(columnIndex)...)
	s.seenDataColumnCache.Add(string(b), true)
}
