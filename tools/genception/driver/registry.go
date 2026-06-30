package driver

import (
	"strings"
)

const stdlibPrefix = "@@io_bazel_rules_go//stdlib:"

// Query prefixes defined by the go/packages driver protocol. A `file=` query asks for
// the package enclosing a source file; a `pattern=` query escapes its argument so it is
// passed through to the build tool verbatim. Anything else is a bare package pattern.
const (
	queryFilePrefix    = "file="
	queryPatternPrefix = "pattern="
)

type registry struct {
	env       environment
	packages  map[string]*flatPackage
	stdlib    map[string]string
	files     map[string]string
	resolve   pathResolverFunc
	tagFilter tagFilterFunc
}

func newRegistry(env environment) *registry {
	return &registry{
		env:       env,
		packages:  map[string]*flatPackage{},
		stdlib:    map[string]string{},
		files:     map[string]string{},
		resolve:   newPathResolver(env),
		tagFilter: goTagFilter(env),
	}
}

// --- ingest: take raw flatPackages from the JSON inventory into the registry ---

func (r *registry) add(pkgs ...*flatPackage) *registry {
	for _, pkg := range pkgs {
		rewritePackage(pkg)
		r.update(pkg)

		if pkg.isStdlib() {
			r.stdlib[pkg.PkgPath] = pkg.ID
		}
	}
	return r
}

// update merges the contents of 2 packages together in the instance where they have the same package path.
// This can happen when the gopackages aspect traverses to a child label and generates separate json files transitive targets.
// For example, in //proto/prysm/v1alpha1 we see both `:go_default_library` and `:go_proto` from `//proto/engine/v1`.
// Without the merge, `:go_proto` can overwrite `:go_default_library`, leaving sources files out of the final graph.
// When the incoming package's source set is a superset of the existing one, it is the more complete view of the
// package, so we keep it wholesale (GoFiles/CompiledGoFiles/Imports all come from the same target and must stay
// consistent). Otherwise the existing entry is at least as complete, so we keep it.
func (r *registry) update(pkg *flatPackage) {
	existing, ok := r.packages[pkg.PkgPath]
	if !ok || isSuperset(pkg.GoFiles, existing.GoFiles) {
		r.packages[pkg.PkgPath] = pkg
	}
}

func rewritePackage(pkg *flatPackage) {
	pkg.ID = pkg.PkgPath
	for k, v := range pkg.Imports {
		// rewrite package ID mapping to be the same as the path
		pkg.Imports[k] = canonicalizeID(k, v, pkg)
	}
}

func canonicalizeID(path, id string, pkg *flatPackage) string {
	if strings.HasPrefix(id, stdlibPrefix) {
		return id[len(stdlibPrefix):]
	}
	if pkg.isStdlib() {
		return id
	}
	return path
}

// isSuperset reports whether b is contained in a as an ordered subsequence (i.e. a
// is a superset of b). It relies on both slices being in the same sorted order, which
// holds for the GoFiles lists Bazel emits in the package JSON.
func isSuperset(a, b []string) bool {
	if len(a) < len(b) {
		return false
	}
	bi := 0
	for i := range a {
		if a[i] == b[bi] {
			bi++
			if bi == len(b) {
				return true
			}
		}
	}
	return false
}

// --- resolve: post-process the ingested packages into a complete, queryable graph ---

// resolvePackages performs post-processing on the json package data to add context that will
// be needed to give complete information to the package data requester outside the bazel environment.
//   - adds stdlib imports to packages. This is required because stdlib packages are not part of the
//     JSON file exports as bazel is unaware of them.
//   - We need to process all the bazel file paths to replace symbolic path names with
//     fully qualified paths. This is done by the path resolver func.
func (r *registry) resolvePackages() error {
	for _, pkg := range r.packages {
		pkg.resolvePaths(r.resolve, r.tagFilter)
		if err := pkg.resolveStdlib(r.stdlibPkgID); err != nil {
			return err
		}
		// extract test sources and imports into their own flatPackage
		testFp := pkg.deriveTestPackage()
		if testFp != nil {
			r.packages[testFp.ID] = testFp
		}
	}

	// Build the file->package index once paths are absolute and test packages exist,
	// so file= queries can be resolved to the package that owns the file.
	r.indexFiles()

	return nil
}

// indexFiles maps each resolved source file path to the ID of the package that owns it.
// Each file belongs to exactly one package (internal test files stay with the base
// package, external test files move to the derived _xtest package), so there is no
// ambiguity to resolve here.
func (r *registry) indexFiles() {
	for _, pkg := range r.packages {
		for _, f := range pkg.GoFiles {
			r.files[f] = pkg.ID
		}
	}
}

func (r *registry) stdlibPkgID(importPath string) string {
	return r.stdlib[importPath]
}

// --- query: resolve driver queries to a package and its transitive dependencies ---

// resolveQueryID maps a single driver query to a known package ID. It understands the
// file= and pattern= query forms from the go/packages driver protocol; any other query
// is treated as a bare package pattern, which after rewritePackage is just the import
// path. The boolean is false when the query matches no known package.
func (r *registry) resolveQueryID(query string) (string, bool) {
	switch {
	case strings.HasPrefix(query, queryFilePrefix):
		id, ok := r.files[query[len(queryFilePrefix):]]
		return id, ok
	case strings.HasPrefix(query, queryPatternPrefix):
		query = query[len(queryPatternPrefix):]
	}
	_, ok := r.packages[query]
	return query, ok
}

func (r *registry) query(queries []string) ([]string, []*flatPackage) {
	walkedPackages := map[string]*flatPackage{}
	retRoots := make([]string, 0, len(queries))
	seenRoot := make(map[string]bool, len(queries))
	for _, q := range queries {
		id, ok := r.resolveQueryID(q)
		if !ok {
			log.WithField("query", q).Warn("driver query did not match any known package")
			continue
		}
		if !seenRoot[id] {
			seenRoot[id] = true
			retRoots = append(retRoots, id)
		}
		r.walk(walkedPackages, id)
	}

	retPkgs := make([]*flatPackage, 0, len(walkedPackages))
	for _, pkg := range walkedPackages {
		retPkgs = append(retPkgs, pkg)
	}

	return retRoots, retPkgs
}

func (r *registry) walk(acc map[string]*flatPackage, root string) {
	pkg := r.packages[root]

	if pkg == nil {
		log.WithField("root", root).Error("package ID not found")
		return
	}

	acc[pkg.ID] = pkg
	for _, pkgID := range pkg.Imports {
		if _, ok := acc[pkgID]; !ok {
			r.walk(acc, pkgID)
		}
	}
}
