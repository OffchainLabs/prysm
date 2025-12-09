package waitgroup

import "golang.org/x/tools/go/analysis/passes/modernize"

var Analyzer = modernize.WaitGroupAnalyzer
