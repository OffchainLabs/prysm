package state

import (
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

type writeOnlyGloasFields interface {
	// Bids.
	SetExecutionPayloadBid(h interfaces.ROExecutionPayloadBid) error

	// Builder pending payments / withdrawals.
	SetBuilderPendingPayment(index primitives.Slot, payment *ethpb.BuilderPendingPayment) error
	ClearBuilderPendingPayment(index primitives.Slot) error
	QueueBuilderPayment() error
	RotateBuilderPendingPayments() error
	AppendBuilderPendingWithdrawals([]*ethpb.BuilderPendingWithdrawal) error

	// Execution payload availability.
	UpdateExecutionPayloadAvailabilityAtIndex(idx uint64, val byte) error

	// Misc.
	SetLatestBlockHash(hash [32]byte) error
	SetExecutionPayloadAvailability(index primitives.Slot, available bool) error

	// Builders.
	IncreaseBuilderBalance(index primitives.BuilderIndex, amount uint64) error
	AddBuilderFromDeposit(pubkey [fieldparams.BLSPubkeyLength]byte, withdrawalCredentials [fieldparams.RootLength]byte, amount uint64) error
	UpdatePendingPaymentWeight(att ethpb.Att, indices []uint64, participatedFlags map[uint8]bool) error
	OnboardBuildersFromPendingDeposits() error
}

type readOnlyGloasFields interface {
	// Bids.
	LatestExecutionPayloadBid() (interfaces.ROExecutionPayloadBid, error)

	// Builder pending payments / withdrawals.
	BuilderPendingPayments() ([]*ethpb.BuilderPendingPayment, error)
	WithdrawalsMatchPayloadExpected(withdrawals []*enginev1.Withdrawal) (bool, error)

	// Misc.
	LatestBlockHash() ([32]byte, error)

	// Builders.
	Builder(index primitives.BuilderIndex) (*ethpb.Builder, error)
	BuilderPubkey(primitives.BuilderIndex) ([48]byte, error)
	BuilderIndexByPubkey(pubkey [fieldparams.BLSPubkeyLength]byte) (primitives.BuilderIndex, bool)
	IsActiveBuilder(primitives.BuilderIndex) (bool, error)
	CanBuilderCoverBid(primitives.BuilderIndex, primitives.Gwei) (bool, error)
	IsAttestationSameSlot(blockRoot [32]byte, slot primitives.Slot) (bool, error)
	BuilderPendingPayment(index uint64) (*ethpb.BuilderPendingPayment, error)
	ExecutionPayloadAvailability(slot primitives.Slot) (uint64, error)
}
