package endtoend_kurtosis

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

const (
	ETHEREUM_PACKAGE = "github.com/ethpandaops/ethereum-package"

	MINIMAL_ENCLAVE_NAME = "e2e-minimal-enclave"

	DEFAULT_NETWORK_CONFIG_YAML_PATH = "testing/endtoend-kurtosis/network-config/default.yaml"
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

	err = kurtosisWrapper.RunPackageWithNetworkConfig(
		MINIMAL_ENCLAVE_NAME,
		ETHEREUM_PACKAGE,
		DEFAULT_NETWORK_CONFIG_YAML_PATH,
	)
	require.NoError(t, err, "Failed to run ethereum package")

	// Temp: Keep test running for 20 minutes to allow manual inspection of Dora/Assertoor UI
	doraCtx, err := kurtosisWrapper.enclaves[MINIMAL_ENCLAVE_NAME].GetServiceContext("dora")
	require.NoError(t, err, "Failed to get dora service context")

	doraHTTPPort := doraCtx.GetPublicPorts()["http"].GetNumber()
	t.Logf("Visit Dora UI at http://localhost:%d", doraHTTPPort)

	assertoorCtx, err := kurtosisWrapper.enclaves[MINIMAL_ENCLAVE_NAME].GetServiceContext("assertoor")
	require.NoError(t, err, "Failed to get assertoor service context")

	assertoorHTTPPort := assertoorCtx.GetPublicPorts()["http"].GetNumber()
	t.Logf("Visit Assertoor UI at http://localhost:%d", assertoorHTTPPort)

	time.Sleep(20 * time.Minute)
}
