package omitzero

import "golang.org/x/tools/go/analysis/passes/modernize"

var Analyzer = modernize.OmitZeroAnalyzer
