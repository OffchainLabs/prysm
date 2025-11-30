package endtoend_kurtosis

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

const (
	MINIMAL_ENCLAVE_NAME = "e2e-minimal-enclave"
)

func TestEndToEnd_Minimal(t *testing.T) {
	ctx := t.Context()

	LoadPrysmDockerImages(t)

	kurtosisWrapper, err := NewKurtosisWrapper(t, ctx)
	require.NoError(t, err, "Failed to create Kurtosis wrapper")

	err = kurtosisWrapper.CreateEnclave(MINIMAL_ENCLAVE_NAME)
	require.NoError(t, err, "Failed to create Kurtosis enclave")

	t.Cleanup(func() {
		if err := kurtosisWrapper.DestroyEnclave(MINIMAL_ENCLAVE_NAME); err != nil {
			t.Logf("Failed to cleanup enclave: %v", err)
		}
	})
}
