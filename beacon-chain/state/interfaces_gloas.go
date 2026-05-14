package state

import (
	"context"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

type writeOnlyGloasFields interface {
	// Bids.
	SetExecutionPayloadBid(h interfaces.ROExecutionPayloadBid) error
	SetPTCWindow([]*ethpb.PTCs) error
	RotatePTCWindow([]*ethpb.PTCs) error

	// Builder pending payments / withdrawals.
	SetBuilderPendingPayment(index primitives.Slot, payment *ethpb.BuilderPendingPayment) error
	ClearBuilderPendingPayment(index primitives.Slot) error
	QueueBuilderPaymentForSlot(parentSlot primitives.Slot) error
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
	AddBuildersFromDeposits(deposits []BuilderDeposit) error
	UpdatePendingPaymentWeight(att ethpb.Att, indices []uint64, participatedFlags map[uint8]bool) error
	UpdateBuilderAtIndex(index primitives.BuilderIndex, builder *ethpb.Builder) error

	// Withdrawals.
	SetPayloadExpectedWithdrawals(withdrawals []*enginev1.Withdrawal) error
	DecreaseWithdrawalBalances(withdrawals []*enginev1.Withdrawal) error
	DequeueBuilderPendingWithdrawals(num uint64) error
	SetNextWithdrawalBuilderIndex(idx primitives.BuilderIndex) error
	OnboardBuildersFromPendingDeposits(ctx context.Context) error
}

type readOnlyGloasFields interface {
	// Bids.
	LatestExecutionPayloadBid() (interfaces.ROExecutionPayloadBid, error)
	PTCWindow() ([]*ethpb.PTCs, error)
	PayloadCommitteeReadOnly(slot primitives.Slot) ([]primitives.ValidatorIndex, error)

	// Builder pending payments / withdrawals.
	BuilderPendingPayments() ([]*ethpb.BuilderPendingPayment, error)
	WithdrawalsMatchPayloadExpected(withdrawals []*enginev1.Withdrawal) (bool, error)
	BuilderPendingWithdrawals() ([]*ethpb.BuilderPendingWithdrawal, error)

	// Misc.
	LatestBlockHash() ([32]byte, error)

	// Builders.
	Builder(index primitives.BuilderIndex) (*ethpb.Builder, error)
	Builders() ([]*ethpb.Builder, error)
	BuilderPubkey(primitives.BuilderIndex) ([48]byte, error)
	BuilderIndexByPubkey(pubkey [fieldparams.BLSPubkeyLength]byte) (primitives.BuilderIndex, bool)
	IsActiveBuilder(primitives.BuilderIndex) (bool, error)
	CanBuilderCoverBid(primitives.BuilderIndex, primitives.Gwei) (bool, error)
	BuilderPendingBalanceToWithdraw(primitives.BuilderIndex) (uint64, error)
	IsAttestationSameSlot(blockRoot [32]byte, slot primitives.Slot) (bool, error)
	BuilderPendingPayment(index uint64) (*ethpb.BuilderPendingPayment, error)
	ExecutionPayloadAvailability(slot primitives.Slot) (uint64, error)
	ExecutionPayloadAvailabilityVector() ([]byte, error)
	NextWithdrawalBuilderIndex() (primitives.BuilderIndex, error)

	// Withdrawals
	LatestBlockHashMatchesBidBlockHash() (bool, error)
	ExpectedWithdrawalsGloas() (ExpectedWithdrawalsGloasResult, error)
	PayloadExpectedWithdrawals() ([]*enginev1.Withdrawal, error)
	WithdrawalsForPayload() ([]*enginev1.Withdrawal, error)
}

// BuilderDeposit carries the data needed to register a new builder from a
// deposit. Used by AddBuildersFromDeposits to batch-register many builders
// under a single state-lock cycle.
type BuilderDeposit struct {
	Pubkey                [fieldparams.BLSPubkeyLength]byte
	WithdrawalCredentials [fieldparams.RootLength]byte
	Amount                uint64
}

// ExpectedWithdrawalsGloasResult bundles the expected withdrawals and related counters
// for the Gloas fork to avoid positional return mistakes.
type ExpectedWithdrawalsGloasResult struct {
	Withdrawals                      []*enginev1.Withdrawal
	ProcessedBuilderWithdrawalsCount uint64
	ProcessedPartialWithdrawalsCount uint64
	NextWithdrawalBuilderIndex       primitives.BuilderIndex
}
