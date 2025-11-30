package endtoend_kurtosis

import (
	"testing"
)

const (
	MINIMAL_ENCLAVE_NAME = "e2e-minimal-enclave"
)

func TestEndToEnd_Minimal(t *testing.T) {
	ctx := t.Context()

	LoadPrysmDockerImages(t)
	_ = CreateKurtosisEnclave(t, ctx, MINIMAL_ENCLAVE_NAME)
}
