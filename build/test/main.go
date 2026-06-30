package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// kind is a unit-test pass.
type kind string

const (
	mainnet         kind = "mainnet"
	mainnetSpectest kind = "mainnet-spectest"
	minimal         kind = "minimal"
	minimalSpectest kind = "minimal-spectest"
)

// validKinds are the test passes, in canonical run order.
var validKinds = []kind{mainnet, mainnetSpectest, minimal, minimalSpectest}

const (
	totalRuns = 5
	rerunMax  = 1000
)

// joinKinds renders kinds as a space-separated string (for messages / summaries).
func joinKinds(kinds []kind) string {
	ss := make([]string, len(kinds))
	for i, k := range kinds {
		ss[i] = string(k)
	}
	return strings.Join(ss, " ")
}

var errTestsFailed = errors.New("tests failed")

func main() {
	if err := run(); err != nil {
		if !errors.Is(err, errTestsFailed) {
			fmt.Fprintln(os.Stderr, "❌ test:", err)
		}

		os.Exit(1)
	}
}

func run() error {
	race := flag.Bool("race", false, "run the selected pass(es) with the race detector")
	flag.Parse()

	goBin := env("GO", "go")
	gotestsumFlags := []string{
		"--format=pkgname",
		"--no-color=false",
		"--hide-summary=skipped",
		fmt.Sprintf("--rerun-fails=%d", totalRuns-1),
		fmt.Sprintf("--rerun-fails-max-failures=%d", rerunMax),
	}

	kinds, err := selectKinds(flag.Args(), *race)
	if err != nil {
		return fmt.Errorf("selecting test passes: %w", err)
	}

	failed := false
	for _, kind := range kinds {
		pkgs, header, tagFlag, err := passSpec(goBin, kind)
		if err != nil {
			return fmt.Errorf("pass spec for %s: %w", kind, err)
		}

		testFlags := []string{tagFlag}
		if *race {
			testFlags = append(testFlags, "-race")
		}

		fmt.Printf("\n%s\n", header)
		if err := gotestsum(goBin, gotestsumFlags, pkgs, testFlags); err != nil {
			failed = true
		}
	}

	fmt.Println()
	if failed {
		fmt.Printf("❌ Some failure: a test failed all %d attempts\n", totalRuns)
		return errTestsFailed
	}

	suffix := ""
	if *race {
		suffix = " with -race"
	}

	fmt.Printf(
		"✅ All tests passed (%s%s; any test in the `=== FAIL` section above was a flake recovered within %d attempts)\n",
		joinKinds(kinds), suffix, totalRuns,
	)

	return nil
}

func passSpec(goBin string, k kind) (pkgs []string, header, tagFlag string, err error) {
	switch k {
	case mainnet:
		pkgs, err = mainnetPackages(goBin)
		return pkgs, "=== mainnet pass (excluding spectests) ===", "-tags=develop", err

	case mainnetSpectest:
		pkgs, err = mainnetSpectestPackages(goBin)
		return pkgs, "=== mainnet spectest pass ===", "-tags=develop", err

	case minimal:
		pkgs, err = goList(goBin, minimalPkgs...)
		return pkgs, "=== minimal pass (-tags=minimal, excluding spectests) ===", "-tags=develop,minimal", err

	case minimalSpectest:
		pkgs, err = goList(goBin, "./testing/spectest/minimal/...")
		return pkgs, "=== minimal spectest pass (-tags=minimal) ===", "-tags=develop,minimal", err
	}

	return nil, "", "", fmt.Errorf("unknown pass %q", k)
}

var minimalPkgs = []string{
	"./beacon-chain/rpc/prysm/v1alpha1/beacon",
	"./beacon-chain/rpc/prysm/v1alpha1/validator",
	"./config/fieldparams",
}

// excludeRe matches the packages dropped from the mainnet pass: E2E (heavy), all
// spec-tests (which run in their own spectest / minimal passes), and the minimal-config
// packages (which run in the -tags=minimal pass).
var excludeRe = regexp.MustCompile(strings.Join([]string{
	`/testing/endtoend`,
	`/testing/spectest/`,
	`/beacon-chain/rpc/prysm/v1alpha1/beacon$`,
	`/beacon-chain/rpc/prysm/v1alpha1/validator$`,
}, "|"))

// mainnetPackages is `go list ./...` minus the excludeRe packages.
func mainnetPackages(goBin string) ([]string, error) {
	all, err := goList(goBin, "./...")
	if err != nil {
		return nil, fmt.Errorf("listing all packages: %w", err)
	}

	pkgs := all[:0]
	for _, p := range all {
		if !excludeRe.MatchString(p) {
			pkgs = append(pkgs, p)
		}
	}

	return pkgs, nil
}

// minimalSpectestRe matches the minimal-config spec-tests, dropped from the mainnet
// spectest pass because they run in the minimal-spectest pass (-tags=minimal) instead.
var minimalSpectestRe = regexp.MustCompile(`/testing/spectest/minimal`)

// mainnetSpectestPackages is `go list ./testing/spectest/...` minus the minimal spec-tests.
func mainnetSpectestPackages(goBin string) ([]string, error) {
	all, err := goList(goBin, "./testing/spectest/...")
	if err != nil {
		return nil, fmt.Errorf("listing spectest packages: %w", err)
	}

	pkgs := all[:0]
	for _, p := range all {
		if !minimalSpectestRe.MatchString(p) {
			pkgs = append(pkgs, p)
		}
	}

	return pkgs, nil
}

func selectKinds(args []string, race bool) ([]kind, error) {
	if len(args) == 0 {
		if race {
			return []kind{mainnet, mainnetSpectest}, nil
		}

		return validKinds, nil
	}

	kinds := make([]kind, 0, len(args))
	for _, arg := range args {
		k := kind(arg)
		if !slices.Contains(validKinds, k) {
			return nil, fmt.Errorf("not a test pass: %s (passes: %s)", arg, joinKinds(validKinds))
		}

		kinds = append(kinds, k)
	}

	return kinds, nil
}

func gotestsum(goBin string, gotestsumFlags, pkgs, testFlags []string) error {
	args := append([]string{"tool", "gotestsum"}, gotestsumFlags...)
	args = append(args, "--packages="+strings.Join(pkgs, " "), "--")
	args = append(args, testFlags...)

	cmd := exec.Command(goBin, args...) // #nosec G204 -- goBin is the resolved go toolchain binary; this is a build/test orchestration tool
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("getting gotestsum stdout: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting gotestsum: %w", err)
	}

	streamProgress(stdout, len(pkgs))

	return cmd.Wait()
}

var statusIcon = regexp.MustCompile(`[✓✖∅↻]`)

// streamProgress copies r to stdout, prefixing each package-status line with a
// right-aligned "[count/total]" running counter.
func streamProgress(r io.Reader, total int) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024) // spec-test package lines can be long
	width := len(strconv.Itoa(total))
	count := 0

	for sc.Scan() {
		line := sc.Text()
		if !statusIcon.MatchString(line) {
			fmt.Println(line)
			continue
		}

		count++
		fmt.Printf("[%*d/%d] %s\n", width, count, total, line)
	}
}

// goList runs `go list <patterns>` and returns the non-empty import paths.
func goList(goBin string, patterns ...string) ([]string, error) {
	// #nosec G204 -- goBin is the resolved go toolchain binary; this is a build/test orchestration tool
	out, err := exec.Command(goBin, append([]string{"list"}, patterns...)...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("go list %s: %w\n%s", strings.Join(patterns, " "), err, ee.Stderr)
		}

		return nil, fmt.Errorf("go list %s: %w", strings.Join(patterns, " "), err)
	}

	var pkgs []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			pkgs = append(pkgs, line)
		}
	}

	return pkgs, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}
