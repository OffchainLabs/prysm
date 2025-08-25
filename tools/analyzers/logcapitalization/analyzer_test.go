package logcapitalization_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/OffchainLabs/prysm/v6/build/bazel"
	"github.com/OffchainLabs/prysm/v6/tools/analyzers/logcapitalization"
)

func init() {
	if bazel.BuiltWithBazel() {
		bazel.SetGoEnv()
	}
}

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.RunWithSuggestedFixes(t, testdata, logcapitalization.Analyzer, "a")
}
