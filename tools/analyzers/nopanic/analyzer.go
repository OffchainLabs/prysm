package nopanic

import (
	"errors"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const Doc = "Tool to discourage the use of panic(), except in init functions"

var errNoPanic = errors.New("panic() should not be used, except in rare situations or init functions")

// Analyzer runs static analysis.
var Analyzer = &analysis.Analyzer{
	Name:     "nopanic",
	Doc:      Doc,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspection, ok := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	if !ok {
		return nil, errors.New("analyzer is not type *inspector.Inspector")
	}

	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
	}

	inspection.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.CallExpr:
			if isPanic(stmt) && !hasExclusion(pass, stack) {
				pass.Report(analysis.Diagnostic{
					Pos:     n.Pos(),
					End:     n.End(),
					Message: errNoPanic.Error(),
				})
				return false
			}
		}

		return true
	})

	return nil, nil
}

// isPanic returns true if the method name is exactly "panic", case insensitive.
func isPanic(call *ast.CallExpr) bool {
	i, ok := call.Fun.(*ast.Ident)
	return ok && strings.ToLower(i.Name) == "panic"
}

// hasExclusion looks at the ast stack and if any node in the stack has the magic words "lint:nopanic"
// then this node is considered excluded. This allows exclusions to be placed at the function or package level.
// This method also excludes init functions.
func hasExclusion(pass *analysis.Pass, stack []ast.Node) bool {
	if len(stack) < 2 {
		return false
	}
	// The first value in the stack is always the file, then the second value would be a package level function.
	// Init functions are always package level.
	if fd, ok := stack[1].(*ast.FuncDecl); ok && fd.Name.Name == "init" {
		return true
	}

	// Build a comment map and scan the comments of this node stack.
	cm := ast.NewCommentMap(pass.Fset, stack[0], stack[0].(*ast.File).Comments)
	for _, n := range stack {
		for _, cmt := range cm[n] {
			for _, l := range cmt.List {
				if strings.Contains(l.Text, "lint:nopanic") {
					return true
				}
			}
		}
	}

	return false
}
