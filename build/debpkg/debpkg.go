// Package debpkg builds the Prysm .deb packages with nfpm.
package debpkg

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type pkg struct {
	name   string // deb package name, e.g. "prysm-beacon-chain"
	binary string // cross-built binary base name in $DIST, e.g. "beacon-chain"
	config string // path to the nfpm.yaml (repo-root-relative)
}

var allPackages = []pkg{
	{name: "prysm-beacon-chain", binary: "beacon-chain", config: "beacon-chain/package/nfpm.yaml"},
	{name: "prysm-validator", binary: "validator", config: "validator/package/nfpm.yaml"},
}

var allArches = []string{"amd64", "arm64"}

type Config struct {
	Go       string   // GO
	Dist     string   // DIST
	Tag      string   // GIT_TAG (e.g. "v7.0.0"); names the dist binaries
	Version  string   // Tag with a leading "v" stripped (the deb version; version_schema: none)
	Packages []pkg    // packages to build (DEB_BINARIES filter, else all)
	Arches   []string // linux arches to build (DEB_ARCHES, else all)
}

// ConfigFromEnv builds a Config from the environment the Makefile exports.
func ConfigFromEnv() Config {
	tag := gitTag()
	return Config{
		Go:       env("GO", "go"),
		Dist:     env("DIST", "dist"),
		Tag:      tag,
		Version:  strings.TrimPrefix(tag, "v"),
		Packages: selectedPackages(),
		Arches:   selectedArches(),
	}
}

func selectedPackages() []pkg {
	want, ok := os.LookupEnv("DEB_BINARIES")
	if !ok {
		return allPackages
	}

	set := set(want)
	var out []pkg
	for _, p := range allPackages {
		if set[p.binary] {
			out = append(out, p)
		}
	}

	return out
}

func selectedArches() []string {
	spec, ok := os.LookupEnv("DEB_ARCHES")
	if !ok {
		return allArches
	}

	return strings.Fields(spec)
}

// Build produces every (package × arch) .deb from the binaries already in $DIST.
func (c Config) Build() error {
	if len(c.Packages) == 0 || len(c.Arches) == 0 {
		fmt.Println("deb: nothing to package (no deb-able binary / linux arch selected)")
		return nil
	}

	if err := os.MkdirAll(c.Dist, 0o755); err != nil {
		return fmt.Errorf("create dist dir: %w", err)
	}

	total := len(c.Packages) * len(c.Arches)
	n := 0
	for _, p := range c.Packages {
		for _, arch := range c.Arches {
			n++

			// `make dist` names linux binaries "<bin>-<tag>-linux-<arch>" (no extension).
			bin := filepath.Join(c.Dist, fmt.Sprintf("%s-%s-linux-%s", p.binary, c.Tag, arch))
			if _, err := os.Stat(bin); err != nil {
				return fmt.Errorf("missing %s - run `make dist %s platform=linux/%s` (or `make dist`) first", bin, p.binary, arch)
			}

			out := filepath.Join(c.Dist, fmt.Sprintf("%s_%s_%s.deb", p.name, c.Version, arch))
			fmt.Printf("[%d/%d] → linux/%s  %s\n", n, total, arch, p.name)
			if err := c.run(p, arch, bin, out); err != nil {
				return fmt.Errorf("package %s/%s: %w", p.name, arch, err)
			}
		}
	}

	fmt.Printf("✅ deb: built %d/%d package(s) → %s/\n", n, total, c.Dist)
	return nil
}

func (c Config) run(p pkg, arch, bin, out string) error {
	cmd := exec.Command(c.Go, "tool", "nfpm", "package",
		"--config", p.config,
		"--packager", "deb",
		"--target", out,
	)

	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(),
		"ARCH="+arch,
		"VERSION="+c.Version,
		"BIN_PATH="+bin,
	)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nfpm: %w", err)
	}

	return nil
}

func set(s string) map[string]bool {
	out := map[string]bool{}
	for f := range strings.FieldsSeq(s) {
		out[f] = true
	}

	return out
}

func gitTag() string {
	if tag := os.Getenv("GIT_TAG"); tag != "" {
		return tag
	}

	out, err := exec.Command("git", "describe", "--tags", "--abbrev=0").Output()
	if err != nil {
		return "Unknown"
	}

	return strings.TrimSpace(string(out))
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}
