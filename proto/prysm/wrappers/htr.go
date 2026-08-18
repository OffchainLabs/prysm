// Package wrappers contains hash-tree-root helpers for concrete consensus
// proto types. These live here, rather than in encoding/ssz, so that the
// low-level encoding/ssz package stays free of dependencies on the generated
// proto packages (which depend on encoding/ssz, transitively, through
// consensus-types/primitives).
package wrappers

import (
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// Transaction is a single execution-layer transaction, used to compute the
// hash-tree-root of the Transactions field of an ExecutionPayload.
type Transaction []byte

func (t Transaction) HashTreeRoot() ([32]byte, error) {
	return ssz.ByteSliceRoot(t, fieldparams.MaxBytesPerTxLength)
}

// ProgressiveTransaction is a single execution-layer transaction whose
// hash-tree-root is computed as a progressive byte list (EIP-7916).
type ProgressiveTransaction []byte

func (t ProgressiveTransaction) HashTreeRoot() ([32]byte, error) {
	return ssz.ByteSliceRootProgressive(t)
}

// TransactionsRoot computes the HTR for the Transactions' property of the ExecutionPayload
func TransactionsRoot(txs [][]byte) ([32]byte, error) {
	transactions := make([]Transaction, len(txs))
	for i, tx := range txs {
		transactions[i] = Transaction(tx)
	}
	return ssz.SliceRoot(transactions, fieldparams.MaxTxsPerPayloadLength)
}

// TransactionsRootProgressive computes the progressive HTR for the
// Transactions property of the ExecutionPayload.
func TransactionsRootProgressive(txs [][]byte) ([32]byte, error) {
	transactions := make([]ProgressiveTransaction, len(txs))
	for i, tx := range txs {
		transactions[i] = ProgressiveTransaction(tx)
	}
	return ssz.SliceRootProgressive(transactions)
}

// ForkRoot computes the HashTreeRoot Merkleization of Fork
func ForkRoot(fork *ethpb.Fork) ([32]byte, error) {
	if fork == nil {
		fieldRoots := make([][32]byte, 3)
		return ssz.BitwiseMerkleize(fieldRoots, uint64(len(fieldRoots)), uint64(len(fieldRoots)))
	}
	return fork.HashTreeRoot()
}

// CheckpointRoot computes the HashTreeRoot Merkleization of Checkpoint
func CheckpointRoot(checkpoint *ethpb.Checkpoint) ([32]byte, error) {
	if checkpoint == nil {
		fieldRoots := make([][32]byte, 2)
		return ssz.BitwiseMerkleize(fieldRoots, uint64(len(fieldRoots)), uint64(len(fieldRoots)))
	}
	return checkpoint.HashTreeRoot()
}

// WithdrawalRoot computes the HTR of a single withdrawal.
func WithdrawalRoot(w *enginev1.Withdrawal) ([32]byte, error) {
	if w == nil {
		fieldRoots := make([][32]byte, 4)
		return ssz.BitwiseMerkleize(fieldRoots, uint64(len(fieldRoots)), uint64(len(fieldRoots)))
	}
	return w.HashTreeRoot()
}

// WithdrawalSliceRoot computes the HTR of a slice of withdrawals.
// The limit parameter is used as input to the bitwise merkleization algorithm.
func WithdrawalSliceRoot(withdrawals []*enginev1.Withdrawal, limit uint64) ([32]byte, error) {
	return ssz.SliceRoot(withdrawals, limit)
}

// WithdrawalSliceRootProgressive computes the progressive HTR of a slice of
// withdrawals.
func WithdrawalSliceRootProgressive(withdrawals []*enginev1.Withdrawal) ([32]byte, error) {
	return ssz.SliceRootProgressive(withdrawals)
}

// DepositRequestsSliceRoot computes the HTR of a slice of deposit requests.
// The limit parameter is used as input to the bitwise merkleization algorithm.
func DepositRequestsSliceRoot(depositRequests []*enginev1.DepositRequest, limit uint64) ([32]byte, error) {
	return ssz.SliceRoot(depositRequests, limit)
}

// WithdrawalRequestsSliceRoot computes the HTR of a slice of withdrawal requests from the EL.
// The limit parameter is used as input to the bitwise merkleization algorithm.
func WithdrawalRequestsSliceRoot(withdrawalRequests []*enginev1.WithdrawalRequest, limit uint64) ([32]byte, error) {
	return ssz.SliceRoot(withdrawalRequests, limit)
}
