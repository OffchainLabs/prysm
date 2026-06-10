// Package crossbuild cross-compiles the distributed Prysm binaries for every
// run-target (Phase 4 of the Bazel->Go-toolchain migration), choosing a C
// toolchain per-OS:
//
//	linux   -> install-zig.sh        (hermetic zig cc; triple pins glibc 2.31)
//	darwin  -> install-osxcross.sh   (osxcross o64/oa64-clang; needs osxcross on PATH for ld64)
//	windows -> install-mingw.sh      (mingw-w64; herumi's prebuilt lib needs libstdc++)
//
// It is driven by `make cross-build` (via the build/cross command) and reused
// in-process by the docker image build (build/docker). All paths are relative to
// the repository root, which is the working directory both callers run from.
package crossbuild

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Config holds the run parameters. ConfigFromEnv populates it from the same
// environment variables the Makefile exports, with defaults that also keep the
// build runnable standalone.
type Config struct {
	Go           string   // GO
	Dist         string   // DIST
	Tag          string   // GIT_TAG (falls back to `git describe`, then "Unknown")
	Binaries     []string // CROSS_BINARIES
	Targets      []Target // CROSS_TARGETS
	Arm64CFlags  string   // CGO_CFLAGS_LINUX_ARM64
	BLSTPortable string   // BLST_PORTABLE
	Ldflags      string   // LDFLAGS
	Tagflag      string   // TAGFLAG (e.g. "-tags=develop", or empty)
	PGOBeacon    string   // PGO_beacon_chain (e.g. "-pgo=...", or empty)
	Mode         string   // BUILD_MODE (dev|release) — display only, for the progress line
}

// Target is one "<goos>/<goarch>/<c-target-triple>" entry from CROSS_TARGETS.
type Target struct {
	OS, Arch, Triple string
}

// Build cross-compiles every (binary × target) artifact into Config.Dist.
func (c Config) Build() error {
	// Host guard: darwin (osxcross) and windows (mingw-w64) targets can only be built
	// from a Linux host. linux targets use zig, which is host-agnostic, so they build
	// from any host (incl. macOS) — this is what lets `make docker-build` run on a Mac.
	for _, t := range c.Targets {
		if (t.OS == "darwin" || t.OS == "windows") && runtime.GOOS != "linux" {
			return fmt.Errorf("'%s' targets require a Linux x86_64 host (osxcross/mingw-w64). "+
				"Current host: %s/%s. Linux targets build from any host", t.OS, runtime.GOOS, runtime.GOARCH)
		}
	}

	// Provision zig once up front (every run uses it for the linux targets).
	zig, err := provision("install-zig.sh")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.Dist, 0o755); err != nil {
		return err
	}

	total := c.artifactCount()
	n := 0
	// Cache provisioned toolchain paths so darwin/windows aren't re-provisioned per target.
	osxcross := ""

	for _, t := range c.Targets {
		var cc, cxx, extra, pathPrefix string
		switch t.OS {
		case "linux":
			cc = zig + " cc -target " + t.Triple
			cxx = zig + " c++ -target " + t.Triple
			if t.Arch == "arm64" {
				extra = c.Arm64CFlags
			}
		case "darwin":
			if osxcross == "" {
				if osxcross, err = provision("install-osxcross.sh"); err != nil {
					return err
				}
			}
			pathPrefix = osxcross + string(os.PathListSeparator)
			if t.Arch == "amd64" {
				cc, cxx = filepath.Join(osxcross, "o64-clang"), filepath.Join(osxcross, "o64-clang++")
			} else {
				cc, cxx = filepath.Join(osxcross, "oa64-clang"), filepath.Join(osxcross, "oa64-clang++")
			}
		case "windows":
			if _, err := provision("install-mingw.sh"); err != nil {
				return err
			}
			cc, cxx = "x86_64-w64-mingw32-gcc", "x86_64-w64-mingw32-g++"
		default:
			return fmt.Errorf("unknown target OS %q", t.OS)
		}

		ext := ""
		if t.OS == "windows" {
			ext = ".exe"
		}

		for _, bin := range c.Binaries {
			pgo := ""
			if bin == "beacon-chain" {
				pgo = c.PGOBeacon
			}
			out := filepath.Join(c.Dist, fmt.Sprintf("%s-%s-%s-%s%s", bin, c.Tag, t.OS, t.Arch, ext))
			n++
			b := builder{cfg: c, t: t, cc: cc, cxx: cxx, pathPrefix: pathPrefix}
			fmt.Printf("[%d/%d] → %s/%s  %s  (%s - portable)\n", n, total, t.OS, t.Arch, bin, c.Mode)
			if err := b.compile(out, "./cmd/"+bin, c.BLSTPortable+" "+extra, pgo); err != nil {
				return err
			}
			// The amd64 beacon-chain also ships a -modern- (ADX) artifact built without
			// the portable flag.
			if bin == "beacon-chain" && t.Arch == "amd64" {
				modern := filepath.Join(c.Dist, fmt.Sprintf("beacon-chain-%s-modern-%s-%s%s", c.Tag, t.OS, t.Arch, ext))
				n++
				fmt.Printf("[%d/%d] → %s/%s  beacon-chain  (%s - modern)\n", n, total, t.OS, t.Arch, c.Mode)
				if err := b.compile(modern, "./cmd/beacon-chain", extra, pgo); err != nil {
					return err
				}
			}
		}
	}

	fmt.Printf("✅ cross: built %d/%d artifact(s) → %s/\n", n, total, c.Dist)
	return nil
}

// builder carries the per-target compiler settings for one or more `go build` calls.
type builder struct {
	cfg        Config
	t          Target
	cc, cxx    string
	pathPrefix string
}

// compile runs `go build` for one binary with this target's CGO toolchain.
func (b builder) compile(out, pkg, cgoCFlags, pgo string) error {
	args := []string{"build"}
	if b.cfg.Tagflag != "" {
		args = append(args, b.cfg.Tagflag)
	}
	args = append(args, "-trimpath")
	if pgo != "" {
		args = append(args, pgo)
	}
	// Strip flags (-s -w) are not forced here: the caller controls them via LDFLAGS
	// (the Makefile adds -s -w only for mode=release), so a dev cross/image build keeps
	// symbols and links faster.
	args = append(args, "-ldflags", b.cfg.Ldflags, "-o", out, pkg)

	cmd := exec.Command(b.cfg.Go, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(),
		"GOOS="+b.t.OS,
		"GOARCH="+b.t.Arch,
		"CGO_ENABLED=1",
		"CC="+b.cc,
		"CXX="+b.cxx,
		"CGO_CFLAGS="+cgoCFlags,
	)
	if b.pathPrefix != "" {
		cmd.Env = append(cmd.Env, "PATH="+b.pathPrefix+os.Getenv("PATH"))
	}
	return cmd.Run()
}

// artifactCount mirrors the [n/m] denominator: one build per (binary × target),
// plus one extra for the amd64 beacon-chain "modern" artifact.
func (c Config) artifactCount() int {
	hasBeacon := false
	for _, b := range c.Binaries {
		if b == "beacon-chain" {
			hasBeacon = true
		}
	}
	m := 0
	for _, t := range c.Targets {
		m += len(c.Binaries)
		if t.Arch == "amd64" && hasBeacon {
			m++
		}
	}
	return m
}

// provision runs a sibling install-*.sh script (idempotent) and returns the trimmed
// path it prints on stdout; the script's own logging goes to stderr.
func provision(script string) (string, error) {
	cmd := exec.Command(filepath.Join("tools", "cross-toolchain", script))
	cmd.Stderr = os.Stderr
	path, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w", script, err)
	}
	return strings.TrimSpace(string(path)), nil
}

// ConfigFromEnv builds a Config from the environment the Makefile exports.
func ConfigFromEnv() Config {
	cfg := Config{
		Go:           env("GO", "go"),
		Dist:         env("DIST", "dist"),
		Tag:          GitTag(),
		Binaries:     strings.Fields(env("CROSS_BINARIES", "beacon-chain validator client-stats prysmctl")),
		Arm64CFlags:  os.Getenv("CGO_CFLAGS_LINUX_ARM64"),
		BLSTPortable: env("BLST_PORTABLE", "-D__BLST_PORTABLE__"),
		Ldflags:      os.Getenv("LDFLAGS"),
		Tagflag:      os.Getenv("TAGFLAG"),
		PGOBeacon:    os.Getenv("PGO_beacon_chain"),
		Mode:         env("BUILD_MODE", "dev"),
	}
	const defaultTargets = "linux/amd64/x86_64-linux-gnu.2.31 linux/arm64/aarch64-linux-gnu.2.31 " +
		"darwin/amd64/x86_64-macos darwin/arm64/aarch64-macos windows/amd64/x86_64-windows-gnu"
	cfg.Targets = ParseTargets(env("CROSS_TARGETS", defaultTargets))
	return cfg
}

// ParseTargets splits a space-separated "<os>/<arch>/<triple>" list, skipping
// (with a warning) any malformed entry.
func ParseTargets(spec string) []Target {
	var targets []Target
	for field := range strings.FieldsSeq(spec) {
		parts := strings.SplitN(field, "/", 3)
		if len(parts) != 3 {
			fmt.Fprintf(os.Stderr, "cross: skipping malformed target %q\n", field)
			continue
		}
		targets = append(targets, Target{OS: parts[0], Arch: parts[1], Triple: parts[2]})
	}
	return targets
}

// GitTag resolves GIT_TAG, falling back to `git describe --tags --abbrev=0`, then "Unknown".
func GitTag() string {
	if t := os.Getenv("GIT_TAG"); t != "" {
		return t
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
