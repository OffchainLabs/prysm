package state

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

type writeOnlyGloasFields interface {
	SetExecutionPayloadBid(h interfaces.ROExecutionPayloadBid) error
	SetBuilderPendingPayment(index primitives.Slot, payment *ethpb.BuilderPendingPayment) error
	ClearBuilderPendingPayment(index primitives.Slot) error
	RotateBuilderPendingPayments() error
	AppendBuilderPendingWithdrawals([]*ethpb.BuilderPendingWithdrawal) error
	UpdateExecutionPayloadAvailabilityAtIndex(idx uint64, val byte) error
	UpdatePendingPaymentWeight(att ethpb.Att, indices []uint64, participatedFlags map[uint8]bool) error
}

type readOnlyGloasFields interface {
	BuilderPubkey(primitives.BuilderIndex) ([48]byte, error)
	IsActiveBuilder(primitives.BuilderIndex) (bool, error)
	CanBuilderCoverBid(primitives.BuilderIndex, primitives.Gwei) (bool, error)
	LatestBlockHash() ([32]byte, error)
	IsAttestationSameSlot(blockRoot [32]byte, slot primitives.Slot) (bool, error)
	BuilderPendingPayment(index uint64) (*ethpb.BuilderPendingPayment, error)
	BuilderPendingPayments() ([]*ethpb.BuilderPendingPayment, error)
	ExecutionPayloadAvailability(slot primitives.Slot) (uint64, error)
}
