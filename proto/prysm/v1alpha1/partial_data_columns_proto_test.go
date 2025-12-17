package eth_test

import (
	"os"
	"strings"
	"testing"
)

// TestPartialDataColumnsProtoGoPackageVersion verifies that the go_package
// option in partial_data_columns.proto uses v7, matching the rest of the
// codebase. A mismatch (e.g. v6) would cause the next codegen run to produce
// code with the wrong import path.
func TestPartialDataColumnsProtoGoPackageVersion(t *testing.T) {
	content, err := os.ReadFile("partial_data_columns.proto")
	if err != nil {
		t.Fatalf("failed to read proto file: %v", err)
	}

	want := `go_package = "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1;eth"`
	if !strings.Contains(string(content), want) {
		t.Errorf("partial_data_columns.proto has wrong go_package.\nwant line containing: %s\ngot file content:\n%s",
			want, string(content))
	}
}
