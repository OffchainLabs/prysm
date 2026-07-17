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
	require.Equal(t, blobTransactionsWithSidecars, blobTransactionModeAtEpoch(cfg.FuluForkEpoch-1, cfg))
	require.Equal(t, blobTransactionsWithCellProofs, blobTransactionModeAtEpoch(cfg.FuluForkEpoch, cfg))
}

func TestUseDedicatedBlobV0Account(t *testing.T) {
	cfg := &params.BeaconChainConfig{
		FuluForkEpoch:  16,
		FarFutureEpoch: math.MaxUint64,
	}

	require.Equal(t, true, useDedicatedBlobV0Account(blobTransactionsWithSidecars, cfg))
	require.Equal(t, false, useDedicatedBlobV0Account(blobTransactionsWithCellProofs, cfg))

	cfg.FuluForkEpoch = cfg.FarFutureEpoch
	require.Equal(t, false, useDedicatedBlobV0Account(blobTransactionsWithSidecars, cfg))
}

func TestNeedsDedicatedBlobV0Account(t *testing.T) {
	cfg := &params.BeaconChainConfig{
		FuluForkEpoch:  16,
		FarFutureEpoch: math.MaxUint64,
	}

	require.Equal(t, true, needsDedicatedBlobV0Account(cfg))

	cfg.FuluForkEpoch = cfg.FarFutureEpoch
	require.Equal(t, false, needsDedicatedBlobV0Account(cfg))
}
