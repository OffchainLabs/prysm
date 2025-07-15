// Package logcapitalization implements a static analyzer to ensure all log messages
// start with a capitalized letter for consistent log formatting.
package logcapitalization

import (
	"errors"
	"go/ast"
	"go/token"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Doc explaining the tool.
const Doc = "Tool to enforce that all log messages start with a capitalized letter"

var errLogNotCapitalized = errors.New("log message should start with a capitalized letter for consistent formatting")

// Analyzer runs static analysis.
var Analyzer = &analysis.Analyzer{
	Name:     "logcapitalization",
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
		(*ast.File)(nil),
	}

	// Track imports that might be used for logging
	hasLogImport := false
	logPackageAliases := make(map[string]bool)

	// Common logging functions that output messages
	logFunctions := []string{
		// logrus
		"Info", "Infof", "InfoWithFields",
		"Debug", "Debugf", "DebugWithFields",
		"Warn", "Warnf", "WarnWithFields",
		"Error", "ErrorWithFields",
		"Fatal", "Fatalf", "FatalWithFields",
		"Panic", "Panicf", "PanicWithFields",
		"Print", "Printf", "Println",
		"Log", "Logf",
		// standard log
		"Print", "Printf", "Println",
		"Fatal", "Fatalf", "Fatalln",
		"Panic", "Panicf", "Panicln",
		// fmt excluded - often used for user prompts, not logging
	}

	inspection.Preorder(nodeFilter, func(node ast.Node) {
		switch stmt := node.(type) {
		case *ast.File:
			// Reset per file
			hasLogImport = false
			logPackageAliases = make(map[string]bool)

			// Check imports for logging packages
			for _, imp := range stmt.Imports {
				if imp.Path != nil {
					path := strings.Trim(imp.Path.Value, "\"")
					if isLoggingPackage(path) {
						hasLogImport = true

						// Track package alias
						if imp.Name != nil {
							logPackageAliases[imp.Name.Name] = true
						} else {
							// Default package name from path
							parts := strings.Split(path, "/")
							if len(parts) > 0 {
								logPackageAliases[parts[len(parts)-1]] = true
							}
						}
					}
				}
			}

		case *ast.CallExpr:
			if !hasLogImport {
				return
			}

			// Check if this is a logging function call
			if !isLoggingCall(stmt, logFunctions, logPackageAliases) {
				return
			}

			// Check the first argument (message) for capitalization
			if len(stmt.Args) > 0 {
				firstArg := stmt.Args[0]

				// Check if it's a format function (like Printf, Infof)
				if isFormatFunction(stmt) {
					checkFormatStringCapitalization(firstArg, pass, node)
				} else {
					checkMessageCapitalization(firstArg, pass, node)
				}
			}
		}
	})

	return nil, nil
}

// isLoggingPackage checks if the import path is a logging package
func isLoggingPackage(path string) bool {
	loggingPaths := []string{
		"github.com/sirupsen/logrus",
		"log",
		"github.com/rs/zerolog",
		"go.uber.org/zap",
		"github.com/golang/glog",
		"k8s.io/klog",
	}

	for _, logPath := range loggingPaths {
		if strings.Contains(path, logPath) {
			return true
		}
	}
	return false
}

// isLoggingCall checks if the call expression is a logging function
func isLoggingCall(call *ast.CallExpr, logFunctions []string, aliases map[string]bool) bool {
	var functionName string
	var packageName string

	switch fun := call.Fun.(type) {
	case *ast.Ident:
		// Direct function call
		functionName = fun.Name
	case *ast.SelectorExpr:
		// Package.Function call
		functionName = fun.Sel.Name
		if ident, ok := fun.X.(*ast.Ident); ok {
			packageName = ident.Name
		}
	default:
		return false
	}

	// Check if it's a logging function
	for _, logFunc := range logFunctions {
		if functionName == logFunc {
			// If no package specified, could be a logging call
			if packageName == "" {
				return true
			}
			// Check if package is a known logging package alias
			if aliases[packageName] {
				return true
			}
			// Check for common logging package names
			if isCommonLogPackage(packageName) {
				return true
			}
		}
	}

	return false
}

// isCommonLogPackage checks for common logging package names
func isCommonLogPackage(pkg string) bool {
	common := []string{"log", "logrus", "zerolog", "zap", "glog", "klog"}
	for _, c := range common {
		if pkg == c {
			return true
		}
	}
	return false
}

// isFormatFunction checks if this is a format function (ending with 'f')
func isFormatFunction(call *ast.CallExpr) bool {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return strings.HasSuffix(fun.Name, "f")
	case *ast.SelectorExpr:
		return strings.HasSuffix(fun.Sel.Name, "f")
	}
	return false
}

// checkFormatStringCapitalization checks if format strings start with capital letter
func checkFormatStringCapitalization(expr ast.Expr, pass *analysis.Pass, node ast.Node) {
	if basicLit, ok := expr.(*ast.BasicLit); ok && basicLit.Kind == token.STRING {
		if len(basicLit.Value) >= 3 { // At least quotes + one character
			unquoted, err := strconv.Unquote(basicLit.Value)
			if err != nil {
				return
			}

			if !isCapitalized(unquoted) {
				pass.Reportf(expr.Pos(),
					"%s: format string should start with a capital letter (found: %q)",
					errLogNotCapitalized.Error(),
					getFirstWord(unquoted))
			}
		}
	}
}

// checkMessageCapitalization checks if message strings start with capital letter
func checkMessageCapitalization(expr ast.Expr, pass *analysis.Pass, node ast.Node) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING && len(e.Value) >= 3 {
			unquoted, err := strconv.Unquote(e.Value)
			if err != nil {
				return
			}

			if !isCapitalized(unquoted) {
				pass.Reportf(expr.Pos(),
					"%s: log message should start with a capital letter (found: %q)",
					errLogNotCapitalized.Error(),
					getFirstWord(unquoted))
			}
		}
	case *ast.BinaryExpr:
		// For string concatenation, check the first part
		if e.Op == token.ADD {
			checkMessageCapitalization(e.X, pass, node)
		}
	}
}

// isCapitalized checks if a string starts with a capital letter
func isCapitalized(s string) bool {
	if len(s) == 0 {
		return true // Empty strings are OK
	}

	// Skip leading whitespace
	trimmed := strings.TrimLeft(s, " \t\n\r")
	if len(trimmed) == 0 {
		return true // Only whitespace is OK
	}

	// Get the first character
	firstRune := []rune(trimmed)[0]

	// Check for special cases that are acceptable
	if isAcceptableStart(firstRune, trimmed) {
		return true
	}

	// Must be uppercase letter
	return unicode.IsUpper(firstRune)
}

// isAcceptableStart checks for acceptable ways to start log messages
func isAcceptableStart(firstRune rune, s string) bool {
	// Numbers are OK
	if unicode.IsDigit(firstRune) {
		return true
	}

	// Special characters that are OK to start with
	acceptableChars := []rune{'%', '$', '/', '\\', '[', '(', '{', '"', '\'', '`', '-'}
	for _, char := range acceptableChars {
		if firstRune == char {
			return true
		}
	}

	// URLs/paths are OK
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "file://") {
		return true
	}

	// Command line flags are OK (--flag, -flag)
	if strings.HasPrefix(s, "--") || (strings.HasPrefix(s, "-") && len(s) > 1 && unicode.IsLetter([]rune(s)[1])) {
		return true
	}

	// Configuration keys or technical terms in lowercase are sometimes OK
	if strings.Contains(s, "=") || strings.Contains(s, ":") {
		// Looks like a key=value or key: value format
		return true
	}

	// Technical keywords that are acceptable in lowercase
	technicalKeywords := []string{"gRPC"}

	// Check if the string starts with any technical keyword
	lowerS := strings.ToLower(s)
	for _, keyword := range technicalKeywords {
		if strings.HasPrefix(lowerS, strings.ToLower(keyword)) {
			return true
		}
	}

	return false
}

// getFirstWord extracts the first few characters for error reporting
func getFirstWord(s string) string {
	trimmed := strings.TrimLeft(s, " \t\n\r")
	if len(trimmed) == 0 {
		return s
	}

	words := strings.Fields(trimmed)
	if len(words) > 0 {
		if len(words[0]) > 20 {
			return words[0][:20] + "..."
		}
		return words[0]
	}

	// Fallback to first 20 characters
	if len(trimmed) > 20 {
		return trimmed[:20] + "..."
	}
	return trimmed
}
