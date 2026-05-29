package waitgroup

import (
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/modernize"
)

var Analyzer = waitGroupAnalyzer()

func waitGroupAnalyzer() *analysis.Analyzer {
	for _, analyzer := range modernize.Suite {
		if analyzer.Name == "waitgroup" || analyzer.Name == "waitgroupgo" {
			return analyzer
		}
	}
	panic("modernize waitgroup analyzer not found")
}
