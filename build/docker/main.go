// Command docker builds the Prysm container images with `docker buildx`, replacing
// the Bazel rules_oci pipeline (Phase 5). It (1) builds the linux portable binaries
// via the Phase-4 cross-build (the crossbuild package, in-process), then (2) assembles
// one image per binary from tools/docker/Dockerfile.
//
// Modes (env MODE):
//
//	load (default) — build for the HOST arch only and --load into the local daemon,
//	                 tagged prysm/<bin>:<tag>. For local testing (multi-arch can't be --load'ed).
//	push           — build linux/amd64+linux/arm64 and --push the multi-arch manifest to
//	                 <REGISTRY>/<repo>:<tag>. Requires registry auth + a buildx builder that
//	                 supports multi-platform (docker-container driver).
//
// The cross-build env (GO, DIST, GIT_TAG, LDFLAGS, ...) is inherited from the Makefile;
// the binaries are named with GIT_TAG, while the image tag uses TAG (DOCKER_TAG), matching
// the dist/<bin>-<tag>-linux-<arch> path the Dockerfile COPYs. Run from the repo root.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/OffchainLabs/prysm/v7/build/crossbuild"
)

const dockerfile = "tools/docker/Dockerfile"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "❌ docker:", err)
		os.Exit(1)
	}
}

func run() error {
	mode := env("MODE", "load")
	tag := env("TAG", crossbuild.GitTag())
	registry := env("REGISTRY", "gcr.io/offchainlabs/prysm")
	binaries := strings.Fields(env("DOCKER_BINARIES", "beacon-chain validator prysmctl"))

	linuxTargets := os.Getenv("CROSS_TARGETS_LINUX")
	if linuxTargets == "" {
		return fmt.Errorf("CROSS_TARGETS_LINUX must be set (the linux/<arch>/<triple> entries)")
	}

	// Pick the platforms to image and the targets to cross-build.
	var platforms string
	var buildTargets []crossbuild.Target
	var builderArgs []string
	if mode == "push" {
		platforms = "linux/amd64,linux/arm64"
		buildTargets = crossbuild.ParseTargets(linuxTargets)
		// Multi-arch needs the docker-container buildx driver; the default 'docker' driver can't.
		if err := exec.Command("docker", "buildx", "inspect", "prysm-builder").Run(); err != nil {
			fmt.Fprintln(os.Stderr, "docker: creating buildx builder 'prysm-builder' (docker-container driver)")
			create := exec.Command("docker", "buildx", "create", "--name", "prysm-builder", "--driver", "docker-container")
			create.Stderr = os.Stderr
			if err := create.Run(); err != nil {
				return fmt.Errorf("creating buildx builder: %w", err)
			}
		}
		builderArgs = []string{"--builder", "prysm-builder"}
	} else {
		host := runtime.GOARCH
		platforms = "linux/" + host
		for _, t := range crossbuild.ParseTargets(linuxTargets) {
			if t.Arch == host {
				buildTargets = []crossbuild.Target{t}
			}
		}
		if len(buildTargets) == 0 {
			return fmt.Errorf("no linux target for host arch %q in CROSS_TARGETS", host)
		}
	}

	// --- 1. build the (portable) binaries the images embed (in-process cross-build) ---------
	fmt.Fprintf(os.Stderr, "docker: building binaries %v for %v\n", binaries, buildTargets)
	cfg := crossbuild.ConfigFromEnv()
	cfg.Binaries = binaries
	cfg.Targets = buildTargets
	if err := cfg.Build(); err != nil {
		return err
	}

	// --- 2. assemble one image per binary ---------------------------------------------------
	for _, bin := range binaries {
		var tags []string
		var out string
		if mode == "push" {
			repo, err := repoFor(bin)
			if err != nil {
				return err
			}
			repo = registry + "/" + repo
			tags = []string{"-t", repo + ":" + tag}
			if bin == "beacon-chain" {
				tags = append(tags, "-t", repo+":"+tag+"-portable")
			}
			out = "--push"
		} else {
			repo := "prysm/" + bin
			tags = []string{"-t", repo + ":" + tag}
			if bin == "beacon-chain" {
				tags = append(tags, "-t", repo+":"+tag+"-portable")
			}
			out = "--load"
		}

		fmt.Fprintf(os.Stderr, "docker: (%s) %s -> %s  [%s]\n", mode, bin, strings.Join(tags, " "), platforms)
		args := []string{"buildx"}
		args = append(args, builderArgs...)
		args = append(args, "build", out, "--platform", platforms,
			"--build-arg", "BIN="+bin, "--build-arg", "TAG="+tag)
		args = append(args, tags...)
		args = append(args, "-f", dockerfile, ".")

		cmd := exec.Command("docker", args...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("docker buildx %s: %w", bin, err)
		}
	}

	fmt.Fprintf(os.Stderr, "✅ docker (%s): built images for %v at tag %s\n", mode, binaries, tag)
	return nil
}

// repoFor maps a binary to its push repo (relative to REGISTRY), matching the Bazel
// image targets (note prysmctl's cmd/ path).
func repoFor(bin string) (string, error) {
	switch bin {
	case "beacon-chain":
		return "beacon-chain", nil
	case "validator":
		return "validator", nil
	case "prysmctl":
		return "cmd/prysmctl", nil
	default:
		return "", fmt.Errorf("no image repo defined for %q", bin)
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
