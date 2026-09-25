package state

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

type readOnlyDecoupledFields interface {
	JustifiedCheckpointDecoupled() (*ethpb.CheckpointDecoupled, error)
	FinalizedCheckpointDecoupled() (*ethpb.CheckpointDecoupled, error)
	AvailableCommitteeWindow() ([]*ethpb.AvailableCommittee, error)
	JustifiedHeight() (primitives.Height, error)
	FinalizedHeight() (primitives.Height, error)
	CurrentHeight() (primitives.Height, error)
	CurrentHeightNonjustifiable() (bool, error)
	CurrentHeightTarget() (*ethpb.CheckpointDecoupled, error)
	TargetParticipation() (bitfield.Bitlist, error)
	Progress() (bitfield.Bitlist, error)
	FinalityParticipation() (bitfield.Bitlist, error)
	BuilderPendingPaymentsDecoupled() ([]*ethpb.BuilderPendingPaymentDecoupled, error)
}

type writeOnlyDecoupledFields interface {
	SetJustifiedCheckpointDecoupled(*ethpb.CheckpointDecoupled) error
	SetFinalizedCheckpointDecoupled(*ethpb.CheckpointDecoupled) error
	SetAvailableCommitteeWindow([]*ethpb.AvailableCommittee) error
	SetJustifiedHeight(primitives.Height) error
	SetFinalizedHeight(primitives.Height) error
	SetCurrentHeight(primitives.Height) error
	SetCurrentHeightNonjustifiable(bool) error
	SetCurrentHeightTarget(*ethpb.CheckpointDecoupled) error
	SetTargetParticipation(bitfield.Bitlist) error
	SetProgress(bitfield.Bitlist) error
	SetFinalityParticipation(bitfield.Bitlist) error
	SetBuilderPendingPaymentsDecoupled([]*ethpb.BuilderPendingPaymentDecoupled) error
	SetBuilderPendingPaymentDecoupled(primitives.Slot, *ethpb.BuilderPendingPaymentDecoupled) error
	AppendDecoupledValidatorBits() error
}
