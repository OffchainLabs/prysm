package eth_test

import (
	"os"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// TestPartialDataColumnsProtoGoPackageVersion verifies that the go_package
// option in partial_data_columns.proto uses v7, matching the rest of the
// codebase. A mismatch (e.g. v6) would cause the next codegen run to produce
// code with the wrong import path.
func TestPartialDataColumnsProtoGoPackageVersion(t *testing.T) {
	content, err := os.ReadFile("partial_data_columns.proto")
	require.NoError(t, err, "failed to read proto file")

	want := `go_package = "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1;eth"`
	require.StringContains(t, want, string(content), "partial_data_columns.proto has wrong go_package")
}
