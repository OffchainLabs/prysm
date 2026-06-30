package externaldata

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

const (
	workspaceFile = "WORKSPACE"
	e2eDepsFile   = "testing/endtoend/deps.bzl"
)

// Parsed Bazel sources
var (
	workspaceContent = sync.OnceValue(func() string { return readBazelFile(workspaceFile) })
	e2eDepsContent   = sync.OnceValue(func() string { return readBazelFile(e2eDepsFile) })

	consensusSpecVersion = sync.OnceValue(func() string { return bazelVar(workspaceContent(), workspaceFile, "consensus_spec_version") })
	blsVersion           = sync.OnceValue(func() string { return bazelVar(workspaceContent(), workspaceFile, "bls_test_version") })
	lighthouseVersion    = sync.OnceValue(func() string { return bazelVar(e2eDepsContent(), e2eDepsFile, "lighthouse_version") })
)

// readBazelFile reads a Bazel build file relative to the module root.
// lint:nopanic
func readBazelFile(relPath string) string {
	root, err := moduleRoot()
	if err != nil {
		panic(fmt.Sprintf("externaldata: locating module root to read %s: %v", relPath, err))
	}

	data, err := os.ReadFile(filepath.Join(root, relPath)) // #nosec G304 -- relPath is a fixed package constant (WORKSPACE / deps.bzl) under the module root
	if err != nil {
		panic(fmt.Sprintf("externaldata: reading %s: %v", relPath, err))
	}

	return string(data)
}

// bazelVar extracts a top-level `name = "value"` string assignment.
// lint:nopanic
func bazelVar(content, file, name string) string {
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\s*=\s*"([^"]*)"`)
	m := re.FindStringSubmatch(content)
	if m == nil {
		panic(fmt.Sprintf("externaldata: variable %q not found in %s", name, file))
	}

	return m[1]
}

var (
	hashAttr = regexp.MustCompile(`(?m)^\s*(?:integrity|sha256)\s*=\s*"([^"]*)"`)
	urlAttr  = regexp.MustCompile(`(?m)^\s*urls?\s*=\s*\[?\s*"([^"]*)"`)
)

// archiveField returns a string attribute of the rule whose `name = "<archive>"`.
// It searches forward from the name declaration to the first matching attribute.
// lint:nopanic
func archiveField(content, file, archive string, attr *regexp.Regexp, what string) string {
	nameRe := regexp.MustCompile(`name\s*=\s*"` + regexp.QuoteMeta(archive) + `"`)
	loc := nameRe.FindStringIndex(content)
	if loc == nil {
		panic(fmt.Sprintf("externaldata: archive %q not found in %s", archive, file))
	}

	m := attr.FindStringSubmatch(content[loc[1]:])
	if m == nil {
		panic(fmt.Sprintf("externaldata: %s for archive %q not found in %s", what, archive, file))
	}

	return m[1]
}

// archiveHash returns the integrity/sha256 of the named archive.
func archiveHash(content, file, archive string) string {
	return archiveField(content, file, archive, hashAttr, "hash")
}

// archiveURL returns the (literal) url/urls of the named archive. It must not be
// used for archives whose Bazel url is assembled from a version variable.
func archiveURL(content, file, archive string) string {
	return archiveField(content, file, archive, urlAttr, "url")
}

// specTestHash returns the integrity of a consensus-spec test flavor, read from
// the flavors map of the consensus_spec_tests rule in the WORKSPACE.
// lint:nopanic
func specTestHash(flavor string) string {
	re := regexp.MustCompile(`"` + regexp.QuoteMeta(flavor) + `"\s*:\s*"([^"]*)"`)
	m := re.FindStringSubmatch(workspaceContent())
	if m == nil {
		panic(fmt.Sprintf("externaldata: consensus-spec test flavor %q not found in %s", flavor, workspaceFile))
	}

	return m[1]
}
