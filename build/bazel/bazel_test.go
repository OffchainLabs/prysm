package bazel_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/build/bazel"
)

func TestBuiltWithBazel(t *testing.T) {
	if bazel.BuiltWithBazel() {
		t.Error("should not be built with Bazel")
	}
}
