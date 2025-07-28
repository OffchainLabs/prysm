// Package operation contains types for block operation-specific events fired during the runtime of a beacon node.
package operation

import (
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
)

const (
	// UnaggregatedAttReceived is sent after an unaggregated attestation object has been received
	// from the outside world. (eg. in RPC or sync)
	UnaggregatedAttReceived = iota + 1

	// AggregatedAttReceived is sent after an aggregated attestation object has been received
	// from the outside world. (eg. in sync)
	AggregatedAttReceived

	// ExitReceived is sent after an voluntary exit object has been received from the outside world (eg in RPC or sync)
	ExitReceived

	// SyncCommitteeContributionReceived is sent after a sync committee contribution object has been received.
	SyncCommitteeContributionReceived

	// BLSToExecutionChangeReceived is sent after a BLS to execution change object has been received from gossip or rpc.
	BLSToExecutionChangeReceived

	// BlobSidecarReceived is sent after a blob sidecar is received from gossip or rpc.
	BlobSidecarReceived = 6

	// ProposerSlashingReceived is sent after a proposer slashing is received from gossip or rpc
	ProposerSlashingReceived = 7

	// AttesterSlashingReceived is sent after an attester slashing is received from gossip or rpc
	AttesterSlashingReceived = 8

	// SingleAttReceived is sent after a single attestation object is received from gossip or rpc
	SingleAttReceived = 9

	// DataColumnSidecarReceived is sent after a data column sidecar is received from gossip or rpc.
	DataColumnSidecarReceived = 10

	// BlockGossipReceived is sent after a block has been received from gossip or API that passes validation rules.
	BlockGossipReceived = 11

	// DataColumnReceived is sent after a data column has been seen after gossip validation rules.
	DataColumnReceived = 12
)

// UnAggregatedAttReceivedData is the data sent with UnaggregatedAttReceived events.
type UnAggregatedAttReceivedData struct {
	// Attestation is the unaggregated attestation object.
	Attestation ethpb.Att
}

// AggregatedAttReceivedData is the data sent with AggregatedAttReceived events.
type AggregatedAttReceivedData struct {
	// Attestation is the aggregated attestation object.
	Attestation ethpb.AggregateAttAndProof
}

// ExitReceivedData is the data sent with ExitReceived events.
type ExitReceivedData struct {
	// Exit is the voluntary exit object.
	Exit *ethpb.SignedVoluntaryExit
}

// SyncCommitteeContributionReceivedData is the data sent with SyncCommitteeContributionReceived objects.
type SyncCommitteeContributionReceivedData struct {
	// Contribution is the sync committee contribution object.
	Contribution *ethpb.SignedContributionAndProof
}

// BLSToExecutionChangeReceivedData is the data sent with BLSToExecutionChangeReceived events.
type BLSToExecutionChangeReceivedData struct {
	Change *ethpb.SignedBLSToExecutionChange
}

// BlobSidecarReceivedData is the data sent with BlobSidecarReceived events.
type BlobSidecarReceivedData struct {
	Blob *blocks.VerifiedROBlob
}

// ProposerSlashingReceivedData is the data sent with ProposerSlashingReceived events.
type ProposerSlashingReceivedData struct {
	ProposerSlashing *ethpb.ProposerSlashing
}

// AttesterSlashingReceivedData is the data sent with AttesterSlashingReceived events.
type AttesterSlashingReceivedData struct {
	AttesterSlashing ethpb.AttSlashing
}

// SingleAttReceivedData is the data sent with SingleAttReceived events.
type SingleAttReceivedData struct {
	Attestation ethpb.Att
}

// DataColumnSidecarReceivedData is the data sent with DataColumnSidecarReceived events.
type DataColumnSidecarReceivedData struct {
	DataColumn *blocks.VerifiedRODataColumn
}

// BlockGossipReceivedData is the data sent with BlockGossipReceived events.
type BlockGossipReceivedData struct {
	// SignedBlock is the block that was received.
	SignedBlock interfaces.ReadOnlySignedBeaconBlock
}

type DataColumnReceivedData struct {
	Slot           primitives.Slot
	Index          uint64
	BlockRoot      [32]byte
	KzgCommitments [][]byte
}
