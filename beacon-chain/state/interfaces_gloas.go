package state

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

type writeOnlyGloasFields interface {
	SetExecutionPayloadBid(h interfaces.ROExecutionPayloadBid) error
	SetBuilderPendingPayment(index primitives.Slot, payment *ethpb.BuilderPendingPayment) error
	RotateBuilderPendingPayments() error
	AppendBuilderPendingWithdrawals([]*ethpb.BuilderPendingWithdrawal) error
	UpdateExecutionPayloadAvailabilityAtIndex(idx uint64, val byte) error
}

type readOnlyGloasFields interface {
	BuilderPubkey(primitives.BuilderIndex) ([48]byte, error)
	IsActiveBuilder(primitives.BuilderIndex) (bool, error)
	CanBuilderCoverBid(primitives.BuilderIndex, primitives.Gwei) (bool, error)
	LatestBlockHash() ([32]byte, error)
	BuilderPendingPayments() ([]*ethpb.BuilderPendingPayment, error)
}
