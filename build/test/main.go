// Command test runs the Prysm unit-test passes via gotestsum, replacing the bash
// `test` / `test-race` Makefile recipes. It wraps gotestsum (which still does the
// actual `go test`, flaky-rerun, and formatting) with three things that are clearer
// in Go than in shell:
//
//   - dispatch/validation of the requested passes (mainnet, minimal),
//   - a single `go list` per pass (the bash version listed twice — once to count,
//     once to enumerate),
//   - a streaming "[X/N] packages" progress prefix over gotestsum's output.
//
// Two passes exist: "mainnet" (the full module minus E2E and the minimal-config
// packages) and "minimal" (the -tags=minimal subset). With no pass named both run;
// the -race flag runs the mainnet package set with the race detector instead.
//
// Configuration comes from the environment the Makefile exports (TEST_EXCLUDE,
// MINIMAL_PKGS, TEST_TAGFLAG, MINIMAL_TAGFLAG, GOTESTSUM_FLAGS, RERUN_ATTEMPTS);
// defaults keep it runnable standalone. Run from the repository root.
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

// validKinds are the test passes, in canonical run order.
var validKinds = []string{"mainnet", "minimal"}

// errTestsFailed signals a real test failure: the summary line is already printed,
// so main exits non-zero without adding its own "❌ test:" prefix.
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
	rerun := env("RERUN_ATTEMPTS", "5")
	gotestsumFlags := strings.Fields(env("GOTESTSUM_FLAGS",
		"--format=pkgname --no-color=false --hide-summary=skipped --rerun-fails=5 --rerun-fails-max-failures=1000"))

	kinds, err := selectKinds(flag.Args(), *race)
	if err != nil {
		return err
	}

	failed := false
	for _, k := range kinds {
		pkgs, header, tagKey, tagDefault, err := passSpec(goBin, k)
		if err != nil {
			return err
		}
		testFlags := tagFlags(tagKey, tagDefault)
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
		fmt.Printf("❌ Some failure: a test failed all %s attempts\n", rerun)
		return errTestsFailed
	}
	suffix := ""
	if *race {
		suffix = " with -race"
	}
	fmt.Printf("✅ All tests passed (%s%s; any 'failure' above was a flake recovered within %s attempts)\n",
		strings.Join(kinds, " "), suffix, rerun)
	return nil
}

// passSpec returns the package set, header line, and tag-flag env key/default for
// one pass. The minimal package patterns are expanded by gotestsum/go test; we list
// them only to count packages for the progress total.
func passSpec(goBin, kind string) (pkgs []string, header, tagKey, tagDefault string, err error) {
	switch kind {
	case "mainnet":
		pkgs, err = mainnetPackages(goBin)
		return pkgs, "=== mainnet pass ===", "TEST_TAGFLAG", "-tags=develop", err
	case "minimal":
		pkgs, err = goList(goBin, strings.Fields(env("MINIMAL_PKGS", defaultMinimalPkgs))...)
		return pkgs, "=== minimal pass (-tags=minimal) ===", "MINIMAL_TAGFLAG", "-tags=develop,minimal", err
	}
	return nil, "", "", "", fmt.Errorf("unknown pass %q", kind)
}

const defaultMinimalPkgs = "./testing/spectest/minimal/... ./beacon-chain/rpc/prysm/v1alpha1/beacon " +
	"./beacon-chain/rpc/prysm/v1alpha1/validator ./config/fieldparams"

const defaultTestExclude = `/testing/endtoend|/testing/spectest/minimal|` +
	`/beacon-chain/rpc/prysm/v1alpha1/beacon$|/beacon-chain/rpc/prysm/v1alpha1/validator$`

// mainnetPackages is `go list ./...` minus the TEST_EXCLUDE packages (E2E + the
// minimal-config packages, which run in their own pass).
func mainnetPackages(goBin string) ([]string, error) {
	all, err := goList(goBin, "./...")
	if err != nil {
		return nil, err
	}
	exclude, err := regexp.Compile(env("TEST_EXCLUDE", defaultTestExclude))
	if err != nil {
		return nil, fmt.Errorf("invalid TEST_EXCLUDE regexp: %w", err)
	}
	pkgs := all[:0]
	for _, p := range all {
		if !exclude.MatchString(p) {
			pkgs = append(pkgs, p)
		}
	}
	return pkgs, nil
}

// selectKinds validates the named passes against validKinds. With none named it
// defaults to all passes — except under -race, which defaults to mainnet only
// (preserving `make test-race`; minimal-with-race is opt-in via `make test-race minimal`).
func selectKinds(args []string, race bool) ([]string, error) {
	if len(args) == 0 {
		if race {
			return []string{"mainnet"}, nil
		}
		return validKinds, nil
	}
	for _, a := range args {
		if !slices.Contains(validKinds, a) {
			return nil, fmt.Errorf("not a test pass: %s (passes: %s)", a, strings.Join(validKinds, " "))
		}
	}
	return args, nil
}

// gotestsum runs `go tool gotestsum` over pkgs and streams its output through the
// [X/N] progress prefix. It returns an error iff gotestsum exits non-zero.
func gotestsum(goBin string, gotestsumFlags, pkgs, testFlags []string) error {
	args := append([]string{"tool", "gotestsum"}, gotestsumFlags...)
	args = append(args, "--packages="+strings.Join(pkgs, " "), "--")
	args = append(args, testFlags...)

	cmd := exec.Command(goBin, args...)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	streamProgress(stdout, len(pkgs))
	return cmd.Wait()
}

// statusIcon matches gotestsum's per-package status glyphs (✓ pass, ✖ fail, ∅ no
// tests, ↻ rerun) anywhere on the line — a leading ANSI color code may precede it.
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
		if statusIcon.MatchString(line) {
			count++
			fmt.Printf("[%*d/%d] %s\n", width, count, total, line)
		} else {
			fmt.Println(line)
		}
	}
}

// tagFlags returns the test build-tag flag as a one-element slice (e.g.
// ["-tags=develop"]), from the named env var or the given default.
func tagFlags(key, fallback string) []string {
	return []string{env(key, fallback)}
}

// goList runs `go list <patterns>` and returns the non-empty import paths.
func goList(goBin string, patterns ...string) ([]string, error) {
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
