// Package debpkg builds the Prysm Debian packages with nfpm (Phase 6 of the
// Bazel->Go-toolchain migration), replacing the rules_pkg pkg_deb/pkg_tar rules that
// lived in beacon-chain/package/BUILD.bazel and validator/package/BUILD.bazel.
//
// It mirrors the build/docker <-> build/crossbuild split: the linux portable binaries
// the packages embed are cross-compiled in-process via the crossbuild package (so
// `make deb` is turnkey on any host — the linux targets use zig), then one .deb per
// (package x architecture) is produced by `go tool nfpm` against the per-package
// nfpm.yaml. Unlike Bazel (which shipped amd64 only) we build both amd64 and arm64,
// matching the Phase 5 multi-arch docker images. All paths are relative to the
// repository root, which is the working directory the Makefile runs from.
package debpkg

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/OffchainLabs/prysm/v7/build/crossbuild"
)

// pkg is one Debian package: its Debian name, the binary it bundles (which is also the
// cmd/<bin> output name and the dist artifact prefix), and the directory holding its
// nfpm.yaml + static assets (config, systemd unit, maintainer scripts).
type pkg struct {
	name string // Debian package name, e.g. "prysm-beacon-chain"
	bin  string // binary / cmd name, e.g. "beacon-chain"
	dir  string // asset directory, e.g. "beacon-chain/package"
}

// packages is the fixed set Bazel shipped (no prysmctl / client-stats deb).
var packages = []pkg{
	{name: "prysm-beacon-chain", bin: "beacon-chain", dir: "beacon-chain/package"},
	{name: "prysm-validator", bin: "validator", dir: "validator/package"},
}

// Config holds the run parameters, populated by ConfigFromEnv from the same
// environment the Makefile exports for the cross/docker helpers.
type Config struct {
	Go      string              // GO
	Dist    string              // DIST
	Tag     string              // GIT_TAG (binary suffix; falls back to `git describe`)
	Arches  []string            // DEB_ARCHES (e.g. ["amd64", "arm64"])
	Targets []crossbuild.Target // the linux/<arch>/<triple> targets to cross-build, filtered to Arches
}

// Build cross-compiles the linux portable binaries, then packages each into a .deb.
func (c Config) Build() error {
	if len(c.Targets) == 0 {
		return fmt.Errorf("no linux targets to build (check DEB_ARCHES / CROSS_TARGETS_LINUX)")
	}

	// --- 1. cross-build the binaries the packages embed (in-process, like build/docker) ---
	cb := crossbuild.ConfigFromEnv()
	cb.Binaries = binNames()
	cb.Targets = c.Targets
	fmt.Fprintf(os.Stderr, "deb: building binaries %v for %d linux target(s)\n", cb.Binaries, len(cb.Targets))
	if err := cb.Build(); err != nil {
		return err
	}

	// --- 2. package each (package x arch) with nfpm ----------------------------------------
	version := strings.TrimPrefix(c.Tag, "v") // matches runtime/version_file ("| tr -d v")
	total := len(packages) * len(c.Arches)
	n := 0
	for _, p := range packages {
		for _, arch := range c.Arches {
			n++
			binSrc := filepath.Join(c.Dist, fmt.Sprintf("%s-%s-linux-%s", p.bin, c.Tag, arch))
			if _, err := os.Stat(binSrc); err != nil {
				return fmt.Errorf("expected binary %s not found (cross-build did not produce it): %w", binSrc, err)
			}
			out := filepath.Join(c.Dist, fmt.Sprintf("%s_%s_%s.deb", p.name, version, arch))
			fmt.Printf("[%d/%d] → linux/%s  %s\n", n, total, arch, p.name)
			if err := c.runNFPM(p, arch, version, binSrc, out); err != nil {
				return err
			}
		}
	}

	fmt.Printf("✅ deb: built %d package(s) → %s/\n", n, c.Dist)
	return nil
}

// runNFPM invokes `go tool nfpm` for one package/arch, feeding the per-build values
// (arch, version, binary path) through the env vars the nfpm.yaml expands.
func (c Config) runNFPM(p pkg, arch, version, binSrc, out string) error {
	cmd := exec.Command(c.Go, "tool", "nfpm", "package",
		"--config", filepath.Join(p.dir, "nfpm.yaml"),
		"--packager", "deb",
		"--target", out,
	)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(),
		"PRYSM_ARCH="+arch,
		"PRYSM_VERSION="+version,
		"PRYSM_BIN_SRC="+binSrc,
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nfpm %s/%s: %w", p.name, arch, err)
	}
	return nil
}

// binNames returns the distinct binaries to cross-build for the packages.
func binNames() []string {
	bins := make([]string, 0, len(packages))
	for _, p := range packages {
		bins = append(bins, p.bin)
	}
	return bins
}

// ConfigFromEnv builds a Config from the environment the Makefile exports.
func ConfigFromEnv() Config {
	arches := strings.Fields(env("DEB_ARCHES", "amd64 arm64"))
	linux := crossbuild.ParseTargets(env("CROSS_TARGETS_LINUX",
		"linux/amd64/x86_64-linux-gnu.2.31 linux/arm64/aarch64-linux-gnu.2.31"))
	return Config{
		Go:      env("GO", "go"),
		Dist:    env("DIST", "dist"),
		Tag:     crossbuild.GitTag(),
		Arches:  arches,
		Targets: filterTargets(linux, arches),
	}
}

// filterTargets keeps only the linux targets whose arch is in arches, preserving the
// arches order so the [n/m] progress and the produced set stay predictable.
func filterTargets(targets []crossbuild.Target, arches []string) []crossbuild.Target {
	byArch := make(map[string]crossbuild.Target, len(targets))
	for _, t := range targets {
		byArch[t.Arch] = t
	}
	var out []crossbuild.Target
	for _, a := range arches {
		if t, ok := byArch[a]; ok {
			out = append(out, t)
		}
	}
	return out
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
