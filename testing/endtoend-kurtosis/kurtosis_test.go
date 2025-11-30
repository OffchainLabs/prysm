package endtoend_kurtosis

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/kurtosis-tech/kurtosis/api/golang/core/lib/enclaves"
	"github.com/kurtosis-tech/kurtosis/api/golang/engine/lib/kurtosis_context"
)

func CreateKurtosisEnclave(t *testing.T, ctx context.Context, enclaveName string) *enclaves.EnclaveContext {
	kurtosisCtx, err := kurtosis_context.NewKurtosisContextFromLocalEngine()
	require.NoError(t, err, "Failed to create Kurtosis context from local engine")

	enclaveCtx, err := kurtosisCtx.CreateEnclave(ctx, enclaveName)
	require.NoError(t, err, "Failed to create Kurtosis enclave: %s", enclaveName)

	return enclaveCtx
}
