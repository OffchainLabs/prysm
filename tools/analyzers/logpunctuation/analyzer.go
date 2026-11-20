// Package logpunctuation implements a static analyzer to ensure logrus
// messages do not end with "." or "!".
package logpunctuation

import (
	"errors"
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v7/tools/analyzers/logstyle"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Doc explaining the tool.
const Doc = "Tool to enforce that logrus messages do not end with \".\" or \"!\""

// Analyzer runs static analysis.
var Analyzer = &analysis.Analyzer{
	Name:     "logpunctuation",
	Doc:      Doc,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	inspection, ok := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	if !ok {
		return nil, errors.New("analyzer is not type *inspector.Inspector")
	}

	inspection.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(node ast.Node) {
		call := node.(*ast.CallExpr)
		fn := logstyle.LogrusCallee(pass.TypesInfo, call)
		if fn == nil || len(call.Args) == 0 {
			return
		}
		isFormat, ok := logstyle.MessageMethods[fn.Name()]
		if !ok {
			return
		}
		// The message is the format string, or the last argument of a
		// non-format call (logrus concatenates the arguments).
		msg := call.Args[len(call.Args)-1]
		if isFormat {
			msg = call.Args[0]
		}
		check(pass, msg)
	})

	return nil, nil
}

// check reports a message given as a single string literal, which can be fixed in place.
func check(pass *analysis.Pass, e ast.Expr) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return
	}
	msg, err := strconv.Unquote(lit.Value)
	if err != nil {
		return
	}
	trimmed := strings.TrimRight(msg, ".! \t\n")
	if trimmed == "" || trimmed == strings.TrimRight(msg, " \t\n") {
		return
	}

	fixed := strconv.Quote(trimmed)
	if strings.HasPrefix(lit.Value, "`") {
		fixed = "`" + trimmed + "`"
	}
	pass.Report(analysis.Diagnostic{
		Pos:     lit.Pos(),
		Message: "log message should not end with \".\" or \"!\"",
		SuggestedFixes: []analysis.SuggestedFix{{
			Message:   "Remove the trailing punctuation",
			TextEdits: []analysis.TextEdit{{Pos: lit.Pos(), End: lit.End(), NewText: []byte(fixed)}},
		}},
	})
}
