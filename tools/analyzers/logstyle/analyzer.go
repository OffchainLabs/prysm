// Package logstyle implements a static analyzer for logrus calls: the
// printf-style methods (Infof, ...) are only used with format arguments.
package logstyle

import (
	"errors"
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/types/typeutil"
)

// Doc explaining the tool.
const Doc = "Tool to enforce the non-f form of logrus calls without format arguments"

const logrusPath = "github.com/sirupsen/logrus"

// Analyzer runs static analysis.
var Analyzer = &analysis.Analyzer{
	Name:     "logstyle",
	Doc:      Doc,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// MessageMethods are the logrus methods that take a message. The value is
// true for the printf-style methods, whose first argument is the format.
var MessageMethods = map[string]bool{
	"Trace": false, "Debug": false, "Info": false, "Print": false, "Warn": false,
	"Warning": false, "Error": false, "Fatal": false, "Panic": false,
	"Traceln": false, "Debugln": false, "Infoln": false, "Println": false, "Warnln": false,
	"Warningln": false, "Errorln": false, "Fatalln": false, "Panicln": false,
	"Tracef": true, "Debugf": true, "Infof": true, "Printf": true, "Warnf": true,
	"Warningf": true, "Errorf": true, "Fatalf": true, "Panicf": true,
}

// LogrusCallee returns the logrus function or method that call invokes, or nil.
func LogrusCallee(info *types.Info, call *ast.CallExpr) *types.Func {
	fn, ok := typeutil.Callee(info, call).(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != logrusPath {
		return nil
	}
	return fn
}

func run(pass *analysis.Pass) (any, error) {
	inspection, ok := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	if !ok {
		return nil, errors.New("analyzer is not type *inspector.Inspector")
	}

	inspection.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(node ast.Node) {
		call := node.(*ast.CallExpr)
		fn := LogrusCallee(pass.TypesInfo, call)
		if fn == nil || !MessageMethods[fn.Name()] || len(call.Args) != 1 || call.Ellipsis.IsValid() {
			return
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return
		}
		name := fn.Name()
		plain := strings.TrimSuffix(name, "f")
		pass.Report(analysis.Diagnostic{
			Pos:     call.Pos(),
			Message: name + " has no format arguments: use " + plain,
			SuggestedFixes: []analysis.SuggestedFix{{
				Message:   "Use " + plain,
				TextEdits: []analysis.TextEdit{{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte(plain)}},
			}},
		})
	})

	return nil, nil
}
