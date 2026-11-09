package util

import (
	"testing"

	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// TransactionList builds a ProgressiveTransactionList from raw transactions
// for test fixtures, failing the test if the transactions exceed the payload
// limits.
func TransactionList(t testing.TB, txs ...[]byte) *enginev1.ProgressiveTransactionList {
	t.Helper()
	l, err := enginev1.NewProgressiveTransactionList(txs)
	require.NoError(t, err)
	return l
}
