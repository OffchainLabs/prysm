package eth1

import (
	"math"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestBlobTransactionModeAtEpoch(t *testing.T) {
	cfg := &params.BeaconChainConfig{
		DenebForkEpoch: 12,
		FuluForkEpoch:  math.MaxUint64,
		FarFutureEpoch: math.MaxUint64,
	}

	require.Equal(t, blobTransactionsDisabled, blobTransactionModeAtEpoch(cfg.DenebForkEpoch-1, cfg))
	require.Equal(t, blobTransactionsWithSidecars, blobTransactionModeAtEpoch(cfg.DenebForkEpoch, cfg))

	cfg.FuluForkEpoch = 16
	require.Equal(t, blobTransactionsWithSidecars, blobTransactionModeAtEpoch(cfg.DenebForkEpoch, cfg))
	require.Equal(t, blobTransactionsWithSidecars, blobTransactionModeAtEpoch(cfg.FuluForkEpoch-2, cfg))
	require.Equal(t, blobTransactionsDisabled, blobTransactionModeAtEpoch(cfg.FuluForkEpoch-1, cfg))
	require.Equal(t, blobTransactionsWithCellProofs, blobTransactionModeAtEpoch(cfg.FuluForkEpoch, cfg))
}
