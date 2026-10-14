package state

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// Spec: σ. Returned values are shared with state copies, replace them through the setters instead of mutating.
type readOnlyDecoupledFields interface {
	Height() (primitives.Height, error)
	Entry() (*ethpb.CheckpointDecoupled, error)
	TargetParticipation() (bitfield.Bitlist, error)
	Progress() (bitfield.Bitlist, error)
	JustifiedHeight() (primitives.Height, error)
	Justified() (*ethpb.CheckpointDecoupled, error)
	FinalizedHeight() (primitives.Height, error)
	Finalized() (*ethpb.CheckpointDecoupled, error)
	PendingFinality() ([]*ethpb.PendingFinality, error)
}

type writeOnlyDecoupledFields interface {
	SetHeight(primitives.Height) error
	SetEntry(*ethpb.CheckpointDecoupled) error
	SetTargetParticipation(bitfield.Bitlist) error
	SetProgress(bitfield.Bitlist) error
	SetJustifiedHeight(primitives.Height) error
	SetJustified(*ethpb.CheckpointDecoupled) error
	SetFinalizedHeight(primitives.Height) error
	SetFinalized(*ethpb.CheckpointDecoupled) error
	SetPendingFinality([]*ethpb.PendingFinality) error
}
