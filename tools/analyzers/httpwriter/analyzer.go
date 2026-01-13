package httpwriter

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var Analyzer = &analysis.Analyzer{
	Name: "httpwriter",
	Doc:  "Ensures that httputil functions which make use of the writer are immediately followed by a return statement.",
	Requires: []*analysis.Analyzer{
		inspect.Analyzer,
	},
}

func Run(pass *analysis.Pass) (any, error) {
	ins := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	ins.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fn := n.(*ast.FuncDecl)
		if fn.Body == nil {
			return
		}
		// top-level: there is no next statement after the function body
		checkBlock(pass, fn, fn.Body, nil)
	})

	return nil, nil
}

func init() {
	Analyzer.Run = Run
}

// checkBlock inspects all statements inside block.
// nextAfter is the statement that comes immediately *after* the statement that contains this block,
// at the enclosing lexical level (or nil if none).
// Example: for `if ... { ... } ; return`, when checking the if's body the nextAfter will be the top-level return stmt.
func checkBlock(pass *analysis.Pass, fn *ast.FuncDecl, block *ast.BlockStmt, nextAfter ast.Stmt) {
	stmts := block.List
	for i, stmt := range stmts {
		// compute the next statement after this statement at the same lexical (ancestor) level:
		// if there is a sibling, use it; otherwise propagate the nextAfter we were given.
		var nextForThisStmt ast.Stmt
		if i+1 < len(stmts) {
			nextForThisStmt = stmts[i+1]
		} else {
			nextForThisStmt = nextAfter
		}

		// Recurse into nested blocks BEFORE checking this stmt's own expr,
		// but we must pass nextForThisStmt to nested blocks so nested HandleError
		// will see the correct "next statement after this statement".
		switch s := stmt.(type) {
		case *ast.IfStmt:
			// pass what's next after the whole if-statement down into its bodies
			if s.Init != nil {
				// init is a statement (rare), treat it as contained in s; it should use next being the if's body first stmt,
				// but for our purposes we don't need to inspect s.Init specially beyond nested calls.
				// We'll just check it with nextForThisStmt as well.
				checkBlock(pass, fn, &ast.BlockStmt{List: []ast.Stmt{s.Init}}, nextForThisStmt)
			}
			if s.Body != nil {
				checkBlock(pass, fn, s.Body, nextForThisStmt)
			}
			if s.Else != nil {
				switch els := s.Else.(type) {
				case *ast.BlockStmt:
					checkBlock(pass, fn, els, nextForThisStmt)
				case *ast.IfStmt:
					// else-if: its body will receive the same nextForThisStmt
					checkBlock(pass, fn, els.Body, nextForThisStmt)
				}
			}

		case *ast.ForStmt:
			if s.Init != nil {
				checkBlock(pass, fn, &ast.BlockStmt{List: []ast.Stmt{s.Init}}, nextForThisStmt)
			}
			if s.Body != nil {
				checkBlock(pass, fn, s.Body, nextForThisStmt)
			}
			if s.Post != nil {
				checkBlock(pass, fn, &ast.BlockStmt{List: []ast.Stmt{s.Post}}, nextForThisStmt)
			}

		case *ast.RangeStmt:
			if s.Body != nil {
				checkBlock(pass, fn, s.Body, nextForThisStmt)
			}

		case *ast.BlockStmt:
			// nested block (e.g. anonymous block) — propagate nextForThisStmt
			checkBlock(pass, fn, s, nextForThisStmt)
		}

		// Now check the current statement itself: is it (or does it contain) a direct call to httputil.HandleError?
		// We only consider ExprStmt that are direct CallExpr to httputil.HandleError.
		call, name := findHandleErrorCall(stmt)
		if call == nil {
			continue
		}

		// Determine the actual "next statement after this call" in lexical function order:
		// - If there is a sibling in the same block after this stmt, that's next.
		// - Otherwise, next is nextForThisStmt (propagated from ancestor).
		var nextStmt ast.Stmt
		if i+1 < len(stmts) {
			nextStmt = stmts[i+1]
		} else {
			nextStmt = nextAfter
		}

		// If there is a next statement and it's a return -> OK
		if nextStmt != nil {
			if _, ok := nextStmt.(*ast.ReturnStmt); ok {
				// immediately followed (in lexical order) by a return at some nesting level -> OK
				continue
			}
			// otherwise it's not a return (even if it's an if/for etc) -> violation
			pass.Reportf(stmt.Pos(), "call to httputil.%s must be immediately followed by a return statement", name)
			continue
		}

		// If nextStmt == nil, this call is lexically the last statement in the function.
		// That is allowed only if the function has no result values.
		if fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
			// void function: allowed
			continue
		}

		// Non-void function and it's the last statement → violation
		pass.Reportf(stmt.Pos(), "call to httputil.%s must be immediately followed by a return statement", name)
	}
}

// findHandleErrorCall returns the call expression if stmt is a direct call to httputil.HandleError(...),
// otherwise nil. We only match direct ExprStmt -> CallExpr -> SelectorExpr where selector is httputil.HandleError.
func findHandleErrorCall(stmt ast.Stmt) (*ast.CallExpr, string) {
	es, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return nil, ""
	}
	call, ok := es.X.(*ast.CallExpr)
	if !ok {
		return nil, ""
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, ""
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil, ""
	}
	selectorName := sel.Sel.Name
	if pkgIdent.Name == "httputil" &&
		(selectorName == "HandleError" || selectorName == "WriteError" || selectorName == "WriteJson" || selectorName == "WriteSSZ") {
		return call, selectorName
	}
	return nil, ""
}
