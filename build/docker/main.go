// Command docker assembles the Prysm OCI container images.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	dockerfile = "tools/docker/Dockerfile"
	builder    = "prysm-builder"

	beaconChain = "beacon-chain"
	validator   = "validator"
	prysmctl    = "prysmctl"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "❌ docker:", err)
		os.Exit(1)
	}
}

func run() error {
	dist := env("DIST", "dist")
	binTag := env("GIT_TAG", gitDescribe())
	imageTag := env("DOCKER_TAG", binTag)
	registry := env("DOCKER_REGISTRY", "gcr.io/offchainlabs/prysm")
	binaries := strings.Fields(env("DOCKER_BINARIES", beaconChain+" validator prysmctl"))
	arches := strings.Fields(env("DOCKER_ARCHES", "amd64 arm64"))

	if err := ensureBuilder(); err != nil {
		return fmt.Errorf("ensure buildx builder: %w", err)
	}

	total := len(binaries) * len(arches)
	n := 0
	for _, arch := range arches {
		for _, bin := range binaries {
			n++

			binPath := filepath.Join(dist, fmt.Sprintf("%s-%s-linux-%s", bin, binTag, arch))
			if _, err := os.Stat(binPath); err != nil {
				return fmt.Errorf("missing binary %s", binPath)
			}

			repo, err := repoFor(bin)
			if err != nil {
				return fmt.Errorf("repo for: %w", err)
			}

			ref := registry + "/" + repo + ":" + imageTag
			out := filepath.Join(dist, fmt.Sprintf("%s-%s-linux-%s.tar", bin, binTag, arch))

			args := []string{
				"buildx", "--builder", builder, "build",
				"--platform", "linux/" + arch,
				"--build-arg", "BIN=" + bin,
				"--build-arg", "TAG=" + binTag,
				"-t", ref,
			}

			if bin == beaconChain {
				args = append(args, "-t", registry+"/"+repo+":"+imageTag+"-portable")
			}

			args = append(args,
				"--output", "type=docker,dest="+out,
				"-f", dockerfile, ".",
			)

			fmt.Fprintf(os.Stderr, "[%d/%d] → docker/%s  %s  → %s\n", n, total, arch, bin, out)
			cmd := exec.Command("docker", args...)
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("docker buildx %s (linux/%s): %w%s", bin, arch, err, emulationHint(arch))
			}
		}
	}

	fmt.Fprintf(os.Stderr, "✅ docker: wrote %d image(s) → %s/  (docker load -i <file>)\n", n, dist)
	return nil
}

func ensureBuilder() error {
	if exec.Command("docker", "buildx", "inspect", builder).Run() == nil {
		return nil
	}

	fmt.Fprintf(os.Stderr, "docker: creating buildx builder %q (docker-container driver)\n", builder)
	cmd := exec.Command("docker", "buildx", "create", "--name", builder, "--driver", "docker-container")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cmd run: %w", err)
	}

	return nil
}

func repoFor(bin string) (string, error) {
	switch bin {
	case beaconChain:
		return beaconChain, nil
	case validator:
		return validator, nil
	case prysmctl:
		// This inconsistency is here to maintain the inconsistency we had in the past using Bazel.
		return "cmd/" + prysmctl, nil
	default:
		return "", fmt.Errorf("no image repo defined for %q", bin)
	}
}

func emulationHint(arch string) string {
	if arch == runtime.GOARCH {
		return ""
	}

	return fmt.Sprintf("\n\nbuilding linux/%s on a %s host runs the image's rootfs stage under "+
		"emulation. If you see \"exec format error\", register QEMU once with:\n"+
		"    docker run --privileged --rm tonistiigi/binfmt --install %s", arch, runtime.GOARCH, arch)
}

func gitDescribe() string {
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
