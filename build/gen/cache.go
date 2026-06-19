package main

// This file implements a content-based cache that lets `make gen` skip a kind
// (proto / ssz / mocks) when none of the files that can affect its output have
// changed since the last successful run on this checkout.
//
// For each kind we build a manifest: a sorted (repo-relative-path, sha256) list
// over that kind's input AND output files, then a single SHA256 over the list.
// The manifest is compared against the value stored in .gen-cache.json. A match
// means nothing relevant changed -> skip. Outputs are part of the manifest, so a
// manual edit or deletion of a generated file invalidates the cache too.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

const (
	cacheFile    = ".gen-cache.json"
	cacheVersion = 1
	modulePath   = "github.com/OffchainLabs/prysm/v7"
)

// genCache is the on-disk cache: one manifest hash per kind. version lets us
// invalidate the whole cache when the manifest scheme changes.
type genCache struct {
	Version int               `json:"version"`
	Kinds   map[string]string `json:"kinds"`
}

// loadCache reads cacheFile.
func loadCache() genCache {
	cache := genCache{Version: cacheVersion, Kinds: map[string]string{}}

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return cache
	}

	if err := json.Unmarshal(data, &cache); err == nil && cache.Version == cacheVersion && cache.Kinds != nil {
		return cache
	}

	return genCache{Version: cacheVersion, Kinds: map[string]string{}}
}

func storeCache(c genCache) error {
	c.Version = cacheVersion

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	if err := os.WriteFile(cacheFile, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("writeFile: %w", err)
	}

	return nil
}

// manifest returns the content hash of every file that determines kind k's
// output (the common toolchain files plus k's specific inputs and outputs).
func manifest(k kind) (string, error) {
	files, err := kindFiles(k)
	if err != nil {
		return "", fmt.Errorf("kindFiles %s: %w", k, err)
	}

	h := sha256.New()
	for _, f := range dedupeSorted(files) {
		sum, err := fileSum(f)
		if err != nil {
			return "", fmt.Errorf("fileSum %s: %w", f, err)
		}

		if _, err := fmt.Fprintf(h, "%s %s\n", f, sum); err != nil {
			return "", fmt.Errorf("fmt.Fprintf: %w", err)
		}
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func fileSum(path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path comes from the repo-relative gen file sets
	if err != nil {
		if os.IsNotExist(err) {
			return "absent", nil
		}

		return "", fmt.Errorf("readFile %s: %w", path, err)
	}

	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:]), nil
}

func kindFiles(k kind) ([]string, error) {
	common, err := commonFiles()
	if err != nil {
		return nil, fmt.Errorf("commonFiles: %w", err)
	}

	specific, err := specificFiles(k)
	if err != nil {
		return nil, fmt.Errorf("specificFiles %s: %w", k, err)
	}

	return append(common, specific...), nil
}

func specificFiles(k kind) ([]string, error) {
	switch k {
	case kindProto:
		return protoFiles()
	case kindSSZ:
		return sszFiles()
	case kindMocks:
		return mockFiles()
	default:
		return nil, fmt.Errorf("unknown kind %q", k)
	}
}

func commonFiles() ([]string, error) {
	files := []string{"go.mod", "go.sum"}

	entries, err := os.ReadDir("build/gen")
	if err != nil {
		return nil, fmt.Errorf("readDir build/gen: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}

		files = append(files, "build/gen/"+entry.Name())
	}

	return files, nil
}

func protoFiles() ([]string, error) {
	var files []string
	err := filepath.WalkDir("proto", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walkDir proto: %w", err)
		}

		if d.IsDir() {
			return nil
		}

		name := d.Name()
		if strings.HasSuffix(name, ".proto") || name == "BUILD.bazel" || strings.HasSuffix(name, ".pb.go") {
			files = append(files, filepath.ToSlash(path))
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walkDir proto: %w", err)
	}

	files = append(files, sszProtoLibraryBzl)

	return files, nil
}

func sszFiles() ([]string, error) {
	targets, err := loadSSZTargets()
	if err != nil {
		return nil, fmt.Errorf("loadSSZTargets: %w", err)
	}

	bzl, err := buildBazelFiles()
	if err != nil {
		return nil, fmt.Errorf("buildBazelFiles: %w", err)
	}

	files := slices.Clone(bzl)
	for _, target := range targets {
		for _, dir := range append([]string{target.pkg}, target.protoInc...) {
			pbs, err := pbgoFiles(dir)
			if err != nil {
				return nil, fmt.Errorf("pbgoFiles %s: %w", dir, err)
			}

			files = append(files, pbs...)
		}

		out := filepath.ToSlash(filepath.Join(target.pkg, target.out))
		minOut := strings.TrimSuffix(out, ".ssz.go") + ".minimal.ssz.go"
		files = append(files, out, minOut)
	}

	return files, nil
}

func mockFiles() ([]string, error) {
	specs := mockSpecsList()

	var files []string
	for _, m := range specs.reflect {
		goFiles, err := goPkgFiles(pkgDir(m.importPath))
		if err != nil {
			return nil, fmt.Errorf("goPkgFiles %s: %w", m.importPath, err)
		}

		files = append(files, goFiles...)
		files = append(files, m.dest)
	}

	for _, mock := range slices.Concat(specs.beaconAPI, specs.bls) {
		files = append(files, mock.source, mock.dest)
	}

	return files, nil
}

func pbgoFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("readDir %s: %w", dir, err)
	}

	var out []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".pb.go") || strings.HasSuffix(name, ".minimal.pb.go") {
			continue
		}

		out = append(out, filepath.ToSlash(filepath.Join(dir, name)))
	}

	return out, nil
}

func goPkgFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("readDir %s: %w", dir, err)
	}

	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		out = append(out, filepath.ToSlash(filepath.Join(dir, name)))
	}

	return out, nil
}

func pkgDir(importPath string) string {
	return strings.TrimPrefix(importPath, modulePath+"/")
}

func dedupeSorted(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}

		seen[s] = true
		out = append(out, s)
	}

	sort.Strings(out)

	return out
}
