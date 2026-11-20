package logpunctuation_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/OffchainLabs/prysm/v7/build/bazel"
	"github.com/OffchainLabs/prysm/v7/tools/analyzers/logpunctuation"
)

func init() {
	if bazel.BuiltWithBazel() {
		bazel.SetGoEnv()
	}
}

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.RunWithSuggestedFixes(t, testdata, logpunctuation.Analyzer, "a")
}
