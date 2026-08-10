package driver

import (
	"go/parser"
	"go/token"
	"maps"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

// flatPackage is the JSON form of a packages.Package, maintaining only the essential
// fields that are necessary to respond to the driver request and encapsulating the
// data transformations for translating between the bazel json package metadata information
// and what the driver expects outside the bazel build context.
type flatPackage struct {
	ID              string
	Name            string            `json:",omitempty"`
	PkgPath         string            `json:",omitempty"`
	Errors          []packages.Error  `json:",omitempty"`
	GoFiles         []string          `json:",omitempty"`
	CompiledGoFiles []string          `json:",omitempty"`
	OtherFiles      []string          `json:",omitempty"`
	ExportFile      string            `json:",omitempty"`
	Imports         map[string]string `json:",omitempty"`
	Standard        bool              `json:",omitempty"`
}

func resolvePathsInPlace(prf pathResolverFunc, paths []string) {
	for i, path := range paths {
		paths[i] = prf(path)
	}
}

func (fp *flatPackage) resolvePaths(prf pathResolverFunc, tagFilter tagFilterFunc) {
	resolvePathsInPlace(prf, fp.CompiledGoFiles)
	resolvePathsInPlace(prf, fp.GoFiles)
	resolvePathsInPlace(prf, fp.OtherFiles)
	fp.ExportFile = prf(fp.ExportFile)

	// filter down to files that would be included based on go build tags
	fp.GoFiles = tagFilter(fp.GoFiles)
	fp.CompiledGoFiles = tagFilter(fp.CompiledGoFiles)
}

func (fp *flatPackage) groupTestFiles(files []string) (testFiles, xTestFiles, nonTestFiles []string, err error) {
	for _, filename := range files {
		if strings.HasSuffix(filename, "_test.go") {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, filename, nil, parser.PackageClauseOnly)
			if err != nil {
				return nil, nil, nil, err
			}
			if f.Name.Name == fp.Name {
				testFiles = append(testFiles, filename)
			} else {
				xTestFiles = append(xTestFiles, filename)
			}
		} else {
			nonTestFiles = append(nonTestFiles, filename)
		}
	}
	return
}

func (fp *flatPackage) deriveTestPackage() *flatPackage {
	internalTests, externalTests, nonTests, err := fp.groupTestFiles(fp.GoFiles)
	if err != nil {
		log.WithError(err).WithField("package", fp.PkgPath).Warn("unable to group test files; skipping test package derivation")
		return nil
	}
	// Internal test files compile into the package itself; recombine them once and
	// share the slice between GoFiles and CompiledGoFiles (they are identical here).
	compiled := slices.Concat(nonTests, internalTests)
	fp.GoFiles = compiled
	fp.CompiledGoFiles = compiled

	if len(externalTests) == 0 {
		return nil
	}

	newImports := make(map[string]string, len(fp.Imports))
	maps.Copy(newImports, fp.Imports)

	newImports[fp.PkgPath] = fp.ID

	// Clone package, only xtgf files
	return &flatPackage{
		ID:              fp.ID + "_xtest",
		Name:            fp.Name + "_test",
		PkgPath:         fp.PkgPath + "_test",
		Imports:         newImports,
		Errors:          fp.Errors,
		GoFiles:         slices.Clone(externalTests),
		CompiledGoFiles: slices.Clone(externalTests),
		OtherFiles:      fp.OtherFiles,
		ExportFile:      fp.ExportFile,
		Standard:        fp.Standard,
	}
}

func (fp *flatPackage) isStdlib() bool {
	return fp.Standard
}

// resolveStdlib reconstructs stdlib imports, which bazel omits from the package JSON, so they can be resolved at query time.
func (fp *flatPackage) resolveStdlib(stdlibId func(string) string) error {
	// Stdlib packages are already complete import wise
	if fp.isStdlib() {
		return nil
	}

	fset := token.NewFileSet()
	for _, file := range fp.CompiledGoFiles {
		f, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		// If the name is not provided, fetch it from the sources
		if fp.Name == "" {
			fp.Name = f.Name.Name
		}

		for _, rawImport := range f.Imports {
			imp, err := strconv.Unquote(rawImport.Path.Value)
			if err != nil {
				continue
			}
			// ignore CGo packages
			if imp == "C" {
				continue
			}
			if _, ok := fp.Imports[imp]; ok {
				continue
			}

			if key := stdlibId(imp); key != "" {
				fp.Imports[imp] = key
			}
		}
	}

	return nil
}
