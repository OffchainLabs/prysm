package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// genSSZ runs the methodical SSZ generator for every ssz_methodical target
// declared across proto/**/BUILD.bazel.
//
// The Bazel ssz_methodical rule (see //tools:methodical.bzl) hands methodical a
// pre-built package inventory through the genception GOPACKAGESDRIVER because
// the go toolchain is not usable inside the Bazel sandbox. Here we run outside
// any sandbox, so methodical's golang.org/x/tools/go/packages loads resolve
// against the real module via the standard `go list` driver -- none of the
// genception plumbing (custom driver, JSON inventory, .pb.go staging) is
// needed. methodical loads the package named by the config file's `package:`
// field straight from the module.
//
// Each target is generated twice: once for mainnet (default build tags) and
// once for minimal (--build-tags=minimal, which makes methodical's package
// loader pick up the //go:build minimal .pb.go sources written by `make gen
// proto`), and always written as a //go:build !minimal file plus a
// <name>.minimal.ssz.go twin. This requires `make gen proto` to have run first
// so both .pb.go variants are on disk.
func genSSZ() error {
	targets, err := loadMethodicalTargets()
	if err != nil {
		return fmt.Errorf("load methodical targets: %w", err)
	}

	// Progressive merkleization is ON by default: gloas (EIP-7688) mandates it
	// and the consensus-spec fixtures expect the progressive hash_tree_root
	// values. This mirrors the //tools:disable_progressive_merkleization
	// default in .bazelrc. Set SSZ_PROGRESSIVE=0 to generate the bounded form.
	progressive := true
	if v, ok := os.LookupEnv("SSZ_PROGRESSIVE"); ok {
		if parsed, err := strconv.ParseBool(v); err == nil {
			progressive = parsed
		}
	}
	if !progressive {
		fmt.Println("SSZ_PROGRESSIVE=0: generating with progressive merkleization OFF")
	}

	for _, t := range targets {
		if err := genMethodical(t, !progressive); err != nil {
			return fmt.Errorf("gen methodical %s/%s: %w", t.pkg, t.out, err)
		}
	}

	return nil
}

// genMethodical generates a single target for both networks and always writes a
// build-tagged mainnet/minimal pair.
//
// We deliberately do not collapse to one untagged file when the two outputs
// match. Whether a target differs across networks depends on the progressive
// flag (progressive collections merkleize size-independently, so a target that
// differs under bounded merkleization can become identical under progressive)
// and on the bounded sizes themselves. A compare-and-collapse would make the
// .minimal.ssz.go twins appear and disappear as those inputs change; we accept
// the duplication of an identical twin to keep the generated file set stable.
func genMethodical(t methodicalTarget, disableProgressive bool) error {
	out := filepath.Join(t.pkg, t.out)
	fmt.Printf("generating %s\n", out)

	// methodical stamps the //go:build header itself (--go-build-constraint), so
	// both this harness and the Bazel ssz_methodical rule produce byte-identical
	// files. The mainnet variant loads the default sources and is constrained to
	// !minimal; the minimal variant loads -tags minimal sources and is
	// constrained to minimal.
	mainnet, err := methodicalOne(t, disableProgressive, nil, "!minimal")
	if err != nil {
		return fmt.Errorf("mainnet: %w", err)
	}

	minimal, err := methodicalOne(t, disableProgressive, []string{"minimal"}, "minimal")
	if err != nil {
		return fmt.Errorf("minimal: %w", err)
	}

	if err := os.WriteFile(out, []byte(mainnet), 0o600); err != nil {
		return fmt.Errorf("writeFile: %w", err)
	}

	minOut := filepath.Join(t.pkg, strings.TrimSuffix(t.out, ".ssz.go")+".minimal.ssz.go")
	if err := os.WriteFile(minOut, []byte(minimal), 0o600); err != nil {
		return fmt.Errorf("writeFile: %w", err)
	}

	return nil
}

// methodicalOne runs `go tool ssz gen` for one target and returns the generated
// source. buildTags selects which tag-gated .pb.go variant methodical's package
// loader sees (nil for the default/mainnet build); buildConstraint is the
// //go:build header methodical stamps on the output ("" for none).
func methodicalOne(t methodicalTarget, disableProgressive bool, buildTags []string, buildConstraint string) (string, error) {
	tmp, err := os.CreateTemp("", "methodical-*.go")
	if err != nil {
		return "", fmt.Errorf("createTemp: %w", err)
	}

	tmpName := tmp.Name()
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close: %w", err)
	}

	defer func() { _ = os.Remove(tmpName) }()

	args := []string{
		"tool", "ssz", "gen",
		"--config=" + filepath.Join(t.pkg, t.configFile),
		"--output=" + tmpName,
	}
	if t.overridePkg != "" {
		args = append(args, "--override-package-name="+t.overridePkg)
	}
	if disableProgressive {
		args = append(args, "--disable-progressive")
	}
	if len(buildTags) > 0 {
		args = append(args, "--build-tags="+strings.Join(buildTags, ","))
	}
	if buildConstraint != "" {
		args = append(args, "--go-build-constraint="+buildConstraint)
	}

	if err := sh("go", args...); err != nil {
		return "", fmt.Errorf("sh: %w", err)
	}

	data, err := os.ReadFile(tmpName) // #nosec G304 -- tmpName is our own os.CreateTemp output
	if err != nil {
		return "", fmt.Errorf("readFile: %w", err)
	}

	return string(data), nil
}
