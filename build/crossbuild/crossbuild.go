package crossbuild

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

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

// OS is a target operating system (the GOOS value).
type OS string

const (
	Linux   OS = "linux"
	Darwin  OS = "darwin"
	Windows OS = "windows"
)

// Arch is a target architecture (the GOARCH value).
type Arch string

const (
	AMD64 Arch = "amd64"
	ARM64 Arch = "arm64"
)

// beaconChain is the one binary that gets PGO and an extra amd64 "modern" (ADX) artifact.
const beaconChain = "beacon-chain"

type Target struct {
	OS     OS
	Arch   Arch
	Triple string
}

// Build cross-compiles every (binary × target) artifact.
func (c Config) Build() error {
	for _, t := range c.Targets {
		if (t.OS == Darwin || t.OS == Windows) && runtime.GOOS != string(Linux) {
			return fmt.Errorf("'%s' targets require a Linux x86_64 host (osxcross/mingw-w64). Current host: %s/%s.", t.OS, runtime.GOOS, runtime.GOARCH)
		}
	}

	zig, err := provision("install-zig.sh")
	if err != nil {
		return fmt.Errorf("provision zig: %w", err)
	}

	if err := os.MkdirAll(c.Dist, 0o755); err != nil {
		return fmt.Errorf("create dist dir: %w", err)
	}

	total := c.artifactCount()
	n := 0
	osxcross := ""

	for _, t := range c.Targets {
		cc, cxx, extra, pathPrefix, err := c.toolchain(t, zig, &osxcross)
		if err != nil {
			return fmt.Errorf("toolchain: %w", err)
		}

		ext := ""
		if t.OS == Windows {
			ext = ".exe"
		}

		for _, bin := range c.Binaries {
			pgo := ""
			if bin == beaconChain {
				pgo = c.PGOBeacon
			}

			out := filepath.Join(c.Dist, fmt.Sprintf("%s-%s-%s-%s%s", bin, c.Tag, t.OS, t.Arch, ext))
			n++

			b := builder{cfg: c, t: t, cc: cc, cxx: cxx, pathPrefix: pathPrefix}
			fmt.Printf("[%d/%d] → %s/%s  %s  (%s - portable)\n", n, total, t.OS, t.Arch, bin, c.Mode)
			if err := b.compile(out, "./cmd/"+bin, c.BLSTPortable+" "+extra, pgo); err != nil {
				return fmt.Errorf("compile: %w", err)
			}

			if bin == beaconChain && t.Arch == AMD64 {
				modern := filepath.Join(c.Dist, fmt.Sprintf("%s-%s-modern-%s-%s%s", beaconChain, c.Tag, t.OS, t.Arch, ext))
				n++
				fmt.Printf("[%d/%d] → %s/%s  %s  (%s - modern)\n", n, total, t.OS, t.Arch, beaconChain, c.Mode)
				if err := b.compile(modern, "./cmd/"+beaconChain, extra, pgo); err != nil {
					return err
				}
			}
		}
	}

	fmt.Printf("✅ cross: built %d/%d artifact(s) → %s/\n", n, total, c.Dist)
	return nil
}

// ConfigFromEnv builds a Config from the environment the Makefile exports.
func ConfigFromEnv() Config {
	cfg := Config{
		Go:           env("GO", "go"),
		Dist:         env("DIST", "dist"),
		Tag:          gitTag(),
		Binaries:     strings.Fields(env("CROSS_BINARIES", beaconChain+" validator client-stats prysmctl")),
		Arm64CFlags:  os.Getenv("CGO_CFLAGS_LINUX_ARM64"),
		BLSTPortable: env("BLST_PORTABLE", "-D__BLST_PORTABLE__"),
		Ldflags:      os.Getenv("LDFLAGS"),
		Tagflag:      os.Getenv("TAGFLAG"),
		PGOBeacon:    os.Getenv("PGO_beacon_chain"),
		Mode:         env("BUILD_MODE", "dev"),
	}

	cfg.Targets = defaultTargets
	if spec := os.Getenv("CROSS_TARGETS"); spec != "" {
		cfg.Targets = parseTargets(spec)
	}

	return cfg
}

func (c Config) toolchain(t Target, zig string, osxcross *string) (cc, cxx, extra, pathPrefix string, err error) {
	switch t.OS {
	case Linux:
		cc = zig + " cc -target " + t.Triple
		cxx = zig + " c++ -target " + t.Triple
		if t.Arch == ARM64 {
			extra = c.Arm64CFlags
		}

	case Darwin:
		if *osxcross == "" {
			if *osxcross, err = provision("install-osxcross.sh"); err != nil {
				return
			}
		}
		pathPrefix = *osxcross + string(os.PathListSeparator)
		cc, cxx = filepath.Join(*osxcross, "oa64-clang"), filepath.Join(*osxcross, "oa64-clang++")
		if t.Arch == AMD64 {
			cc, cxx = filepath.Join(*osxcross, "o64-clang"), filepath.Join(*osxcross, "o64-clang++")
		}

	case Windows:
		if _, err = provision("install-mingw.sh"); err != nil {
			return
		}

		cc, cxx = "x86_64-w64-mingw32-gcc", "x86_64-w64-mingw32-g++"

	default:
		err = fmt.Errorf("unknown target OS %q", t.OS)
	}
	return
}

type builder struct {
	cfg        Config
	t          Target
	cc, cxx    string
	pathPrefix string
}

func (b builder) compile(out, pkg, cgoCFlags, pgo string) error {
	args := []string{"build"}
	if b.cfg.Tagflag != "" {
		args = append(args, b.cfg.Tagflag)
	}

	args = append(args, "-trimpath")
	if pgo != "" {
		args = append(args, pgo)
	}

	args = append(args, "-ldflags", b.cfg.Ldflags, "-o", out, pkg)

	cmd := exec.Command(b.cfg.Go, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(),
		"GOOS="+string(b.t.OS),
		"GOARCH="+string(b.t.Arch),
		"CGO_ENABLED=1",
		"CC="+b.cc,
		"CXX="+b.cxx,
		"CGO_CFLAGS="+cgoCFlags,
	)

	if b.pathPrefix != "" {
		cmd.Env = append(cmd.Env, "PATH="+b.pathPrefix+os.Getenv("PATH"))
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run: %w", err)
	}

	return nil
}

func (c Config) artifactCount() int {
	hasBeacon := false
	for _, b := range c.Binaries {
		if b == beaconChain {
			hasBeacon = true
		}
	}

	m := 0
	for _, t := range c.Targets {
		m += len(c.Binaries)
		if t.Arch == AMD64 && hasBeacon {
			m++
		}
	}

	return m
}

func provision(script string) (string, error) {
	cmd := exec.Command(filepath.Join("tools", "cross-toolchain", script))
	cmd.Stderr = os.Stderr

	path, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w", script, err)
	}

	return strings.TrimSpace(string(path)), nil
}

// defaultTargets is the build matrix used when CROSS_TARGETS is unset.
var defaultTargets = []Target{
	{Linux, AMD64, "x86_64-linux-gnu.2.31"},
	{Linux, ARM64, "aarch64-linux-gnu.2.31"},
	{Darwin, AMD64, "x86_64-macos"},
	{Darwin, ARM64, "aarch64-macos"},
	{Windows, AMD64, "x86_64-windows-gnu"},
}

func parseTargets(spec string) []Target {
	fields := strings.Fields(spec)

	targets := make([]Target, 0, len(fields))
	for _, field := range fields {
		parts := strings.SplitN(field, "/", 3)
		if len(parts) != 3 {
			fmt.Fprintf(os.Stderr, "cross: skipping malformed target %q\n", field)
			continue
		}

		targets = append(targets, Target{OS: OS(parts[0]), Arch: Arch(parts[1]), Triple: parts[2]})
	}

	return targets
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
