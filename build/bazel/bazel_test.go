package bazel_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/build/bazel"
)

func TestBuildWithBazel(t *testing.T) {
	if !bazel.BuiltWithBazel() {
		t.Error("not built with Bazel")
	}
}
