package driver

import (
	"slices"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestIsSuperset(t *testing.T) {
	cases := []struct {
		a        []string
		b        []string
		expected bool
	}{
		{[]string{"a", "b", "c", "d"}, []string{"a", "b"}, true},
		{[]string{"a", "b", "c", "d"}, []string{"a", "b", "c", "d"}, true},
		{[]string{"a", "b", "c", "d"}, []string{"a", "b", "c", "d", "e"}, false},
		{[]string{"a", "b", "c", "d"}, []string{"a", "b", "c"}, true},
		{[]string{}, []string{"a"}, false},
	}
	for _, c := range cases {
		t.Run(strings.Join(c.a, "_")+"__"+strings.Join(c.b, "_"), func(t *testing.T) {
			if isSuperset(c.a, c.b) != c.expected {
				t.Errorf("isSuperset(%v, %v) != %v", c.a, c.b, c.expected)
			}
		})
	}
}

// TestRegistryMergeKeepsSuperset guards against the regression where two bazel targets
// mapping to the same Go package path (e.g. :go_proto and :go_default_library for
// //proto/prysm/v1alpha1) would clobber each other in the registry, dropping source files.
// Whichever target carries the more complete source set must win, regardless of add order.
func TestRegistryMergeKeepsSuperset(t *testing.T) {
	const pkgPath = "github.com/example/p"
	full := []string{"a.go", "b.go", "c.go"}
	subset := []string{"a.go", "c.go"}

	mkPkg := func(files []string) *flatPackage {
		return &flatPackage{
			PkgPath:         pkgPath,
			Name:            "p",
			GoFiles:         slices.Clone(files),
			CompiledGoFiles: slices.Clone(files),
		}
	}

	cases := []struct {
		name   string
		first  []string
		second []string
	}{
		{"superset added first", full, subset},
		{"subset added first", subset, full},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRegistry(environment{})
			r.add(mkPkg(c.first))
			r.add(mkPkg(c.second))

			got, ok := r.packages[pkgPath]
			require.Equal(t, true, ok)
			// The complete source set wins, and CompiledGoFiles stays consistent with it.
			require.DeepEqual(t, full, got.GoFiles)
			require.DeepEqual(t, full, got.CompiledGoFiles)
		})
	}
}

func testRegistry() *registry {
	r := newRegistry(environment{})
	r.packages["example.com/p"] = &flatPackage{ID: "example.com/p", PkgPath: "example.com/p"}
	r.files["/abs/p/a.go"] = "example.com/p"
	return r
}

func TestResolveQueryID(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
		ok    bool
	}{
		{"bare hit", "example.com/p", "example.com/p", true},
		{"bare miss", "example.com/q", "example.com/q", false},
		{"pattern hit", "pattern=example.com/p", "example.com/p", true},
		{"pattern miss", "pattern=example.com/q", "example.com/q", false},
		{"file hit", "file=/abs/p/a.go", "example.com/p", true},
		{"file miss", "file=/abs/p/missing.go", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, ok := testRegistry().resolveQueryID(c.query)
			require.Equal(t, c.ok, ok)
			require.Equal(t, c.want, id)
		})
	}
}

// TestQueryRootsResolved checks that Roots holds resolved package IDs (a file= query
// reports the owning package, not the raw "file=" string) and that queries matching no
// package are dropped from Roots rather than emitted as dangling roots.
func TestQueryRootsResolved(t *testing.T) {
	roots, pkgs := testRegistry().query([]string{"file=/abs/p/a.go", "example.com/missing"})
	require.DeepEqual(t, []string{"example.com/p"}, roots)
	require.Equal(t, 1, len(pkgs))
	require.Equal(t, "example.com/p", pkgs[0].ID)
}
