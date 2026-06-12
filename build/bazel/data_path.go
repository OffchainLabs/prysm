// Copyright 2020 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package bazel

import (
	"path"
	"path/filepath"
	"testing"
)

// TestDataPath returns a path to an asset in the testdata directory. It knows
// to access accesses the right path when executing under bazel.
//
// For example, if there is a file testdata/a.txt, you can get a path to that
// file using TestDataPath(t, "a.txt").
//
// It uses testing.TB directly (rather than the testing/require helper) so this
// package stays non-testonly and can be imported by non-test code (e.g.
// testing/benchmark, which feeds the benchmark-files-gen binary).
func TestDataPath(t testing.TB, relative ...string) string {
	t.Helper()
	relative = append([]string{"testdata"}, relative...)
	// dev notifies the library that the test is running in a subdirectory of the
	// workspace with the environment variable below.
	if BuiltWithBazel() {
		runfiles, err := RunfilesPath()
		if err != nil {
			t.Fatalf("get runfiles path: %v", err)
		}
		return path.Join(runfiles, RelativeTestTargetPath(), path.Join(relative...))
	}

	// Otherwise we're in the package directory and can just return a relative path.
	ret := path.Join(relative...)
	ret, err := filepath.Abs(ret)
	if err != nil {
		t.Fatalf("get absolute path: %v", err)
	}
	return ret
}
