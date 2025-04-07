package bazel_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/build/bazel"
)

func TestBuildWithBazel(t *testing.T) {
	if !bazel.BuiltWithBazel() {
		t.Error("not built with Bazel")
	}
}
