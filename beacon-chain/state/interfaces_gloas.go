package state

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

type writeOnlyGloasFields interface {
	SetExecutionPayloadBid(h interfaces.ROExecutionPayloadBid) error
	SetBuilderPendingPayment(index primitives.Slot, payment *ethpb.BuilderPendingPayment) error
	ClearBuilderPendingPayment(index primitives.Slot) error
	RotateBuilderPendingPayments() error
	AppendBuilderPendingWithdrawals([]*ethpb.BuilderPendingWithdrawal) error
	UpdateExecutionPayloadAvailabilityAtIndex(idx uint64, val byte) error

	SetPayloadExpectedWithdrawals(withdrawals []*enginev1.Withdrawal) error
	DequeueBuilderPendingWithdrawals(num uint64) error
	SetNextWithdrawalBuilderIndex(idx primitives.BuilderIndex) error
	DecreaseBuilderBalance(builderIndex primitives.BuilderIndex, amount uint64) error
}

type readOnlyGloasFields interface {
	BuilderPubkey(primitives.BuilderIndex) ([48]byte, error)
	IsActiveBuilder(primitives.BuilderIndex) (bool, error)
	CanBuilderCoverBid(primitives.BuilderIndex, primitives.Gwei) (bool, error)
	LatestBlockHash() ([32]byte, error)
	BuilderPendingPayments() ([]*ethpb.BuilderPendingPayment, error)

	IsParentBlockFull() (bool, error)
	ExpectedWithdrawalsGloas() ([]*enginev1.Withdrawal, uint64, uint64, primitives.BuilderIndex, error)
}
