package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"sort"
)

const (
	builderDockerfile = "tools/cross-toolchain/Dockerfile.builder"
	crossDockerfile   = "tools/docker/Dockerfile.cross"

	// buildPlatform pins the container to a single platform on every host:
	// - amd64 hosts run it natively
	// - arm64 hosts, e.g. Apple Silicon, via Docker's emulation.
	// This is what makes the build 100% reproducible across hosts.
	// However, it considerably slows down builds on arm64 hosts.
	buildPlatform = "linux/amd64"
)

var toolchainInputs = []string{
	builderDockerfile,
	"tools/cross-toolchain/install-zig.sh",
	"tools/cross-toolchain/install-mingw.sh",
	"tools/cross-toolchain/install-osxcross.sh",
	"tools/cross-toolchain/install_osxcross.sh",
	"tools/cross-toolchain/link_osxcross.sh",
	"tools/cross-toolchain/common_osxcross.sh",
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "❌ cross (docker):", err)

		os.Exit(1)
	}
}

func run() error {
	tag, err := builderTag()
	if err != nil {
		return fmt.Errorf("builder tag: %w", err)
	}

	if err := ensureBuilder(tag); err != nil {
		return fmt.Errorf("ensure builder: %w", err)
	}

	if err := crossBuild(tag); err != nil {
		return fmt.Errorf("cross build: %w", err)
	}

	return nil
}

func builderTag() (string, error) {
	h := sha256.New()
	inputs := slices.Clone(toolchainInputs)
	sort.Strings(inputs)

	for _, input := range inputs {
		b, err := os.ReadFile(input)
		if err != nil {
			return "", fmt.Errorf("hashing toolchain inputs: %w", err)
		}

		_, err = fmt.Fprintf(h, "%s\x00", input)
		if err != nil {
			return "", fmt.Errorf("fprintf: %w", err)
		}

		h.Write(b)
	}

	return fmt.Sprintf("prysm-cross-builder:%x-linux-amd64", h.Sum(nil)[:6]), nil
}

func ensureBuilder(tag string) error {
	if exec.Command("docker", "image", "inspect", tag).Run() == nil {
		fmt.Fprintf(os.Stderr, "cross: reusing builder image %s\n", tag)

		return nil
	}

	fmt.Fprintf(os.Stderr, "cross: building toolchain image %s (one-time; downloads SDK + builds osxcross)\n", tag)
	args := []string{
		"build",
		"--platform", buildPlatform,
		"-t", tag,
		"-f", builderDockerfile,
		".",
	}

	if err := dockerRun(args); err != nil {
		return fmt.Errorf("docker run: %w%s", err, emulationHint())
	}

	return nil
}

func emulationHint() string {
	if runtime.GOARCH == "amd64" {
		return ""
	}

	return fmt.Sprintf("\n\nthe builder runs as %s but this host is %s. If you see "+
		"\"exec format error\", register QEMU emulation once with:\n"+
		"    docker run --privileged --rm tonistiigi/binfmt --install amd64", buildPlatform, runtime.GOARCH)
}

func crossBuild(tag string) error {
	dist := env("DIST", "dist")
	if err := os.MkdirAll(dist, 0o755); err != nil {
		return fmt.Errorf("mkdir dist: %w", err)
	}

	args := []string{
		"build",
		"--platform", buildPlatform,
		"--build-arg", "BUILDER=" + tag,
	}

	for _, k := range []string{
		"GIT_TAG", "LDFLAGS", "TAGFLAG", "BUILD_MODE", "PGO_beacon_chain",
		"CGO_CFLAGS_LINUX_ARM64", "BLST_PORTABLE", "CROSS_BINARIES", "CROSS_TARGETS",
	} {
		args = append(args, "--build-arg", k+"="+os.Getenv(k))
	}

	args = append(args,
		"--target", "artifacts",
		"--output", "type=local,dest="+dist,
		"-f", crossDockerfile,
		".",
	)

	fmt.Fprintf(os.Stderr, "cross: building binaries in container → %s/\n", dist)
	if err := dockerRun(args); err != nil {
		return fmt.Errorf("docker run: %w%s", err, emulationHint())
	}

	fmt.Fprintf(os.Stderr, "✅ cross (docker): artifacts → %s/\n", dist)

	return nil
}

func dockerRun(args []string) error {
	cmd := exec.Command("docker", args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=1")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker %s: %w", args[0], err)
	}

	return nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}
