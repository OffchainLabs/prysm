// Package errcheck implements an static analysis analyzer to ensure that errors are handled in go
// code. This analyzer was adapted from https://github.com/kisielk/errcheck (MIT License).
package errcheck

import (
	analyzer "github.com/kisielk/errcheck/errcheck"
)

var Analyzer = analyzer.Analyzer
