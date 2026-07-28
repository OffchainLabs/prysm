//go:build bazel

package bazel_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/build/bazel"
	inner "github.com/bazelbuild/rules_go/go/tools/bazel"
)

func TestBuildWithBazel(t *testing.T) {
	if !bazel.BuiltWithBazel() {
		t.Error("not built with Bazel")
	}
}

func TestListRunfiles(t *testing.T) {
	want, err := inner.ListRunfiles()
	if err != nil {
		t.Fatalf("inner.ListRunfiles() returned error: %v", err)
	}

	got, err := bazel.ListRunfiles()
	if err != nil {
		t.Fatalf("ListRunfiles() returned error: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("ListRunfiles() returned %d entries, want %d", len(got), len(want))
	}

	for i, w := range want {
		g := got[i]
		if g.Path != w.Path {
			t.Errorf("entry %d: Path = %q, want %q", i, g.Path, w.Path)
		}
		if g.ShortPath != w.ShortPath {
			t.Errorf("entry %d: ShortPath = %q, want %q", i, g.ShortPath, w.ShortPath)
		}
		if g.Workspace != w.Workspace {
			t.Errorf("entry %d: Workspace = %q, want %q", i, g.Workspace, w.Workspace)
		}
	}
}
