// Command e2e runs Prysm's end-to-end tests under the Go toolchain, replacing the
// Bazel e2e targets (Phase 8 of the Bazel->Go-toolchain migration). It (1) builds the
// binaries the e2e harness launches (beacon-chain, validator, bootnode + geth from the
// pinned go-ethereum dep) into DIST, (2) provisions the external binaries a scenario
// needs (lighthouse / web3signer, fetched + sha256-verified by build/externaldata and
// symlinked into DIST), then (3) runs `go test ./testing/endtoend` with the right build
// tags and -run filter, injecting the binary dir via $PRYSM_BIN.
//
// Driven by `make e2e [kind|suite]`; run from the repo root. A kind is a single scenario
// (one Go test func); a suite (presubmit/postsubmit/scenario_tests) runs the bundle the
// matching Bazel test_suite covered, in sequence. The e2e components resolve every binary
// via build/bazel.FindBinary, which (off Bazel) looks in $PRYSM_BIN then dist/.
//
// Platform: lighthouse ships linux/amd64 only and web3signer needs a JRE — matching what
// Bazel ran (a linux x86_64 host). Scenarios that don't need them run anywhere geth builds.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/OffchainLabs/prysm/v7/build/externaldata"
)

// kind is one e2e scenario: the Go test it maps to, whether it uses the minimal
// consensus config (→ -tags=minimal), and which external binaries it launches.
type kind struct {
	name           string
	run            string // anchored -run regexp
	minimal        bool
	needLighthouse bool
	needWeb3signer bool
}

// kinds is the per-scenario set. The first ten are public (`make e2e <name>`, listed in
// the Makefile E2E_KINDS); scenario-multiclient is internal — only the scenario_tests
// suite uses it (it mirrors Bazel's go_mainnet_scenario_test).
var kinds = []kind{
	{"minimal", "^TestEndToEnd_MinimalConfig$", true, false, false},
	{"builder", "^TestEndToEnd_MinimalConfig_WithBuilder$", true, false, false},
	{"web3signer", "^TestEndToEnd_MinimalConfig_Web3Signer$", true, false, true},
	{"slasher", "^TestEndToEnd_SlasherSimulator$", true, false, false},
	{"slashing", "^TestEndToEnd_Slasher_MinimalConfig$", true, false, false},
	{"scenario", "^TestEndToEnd_MultiScenarioRun$", true, false, false},
	{"postmerge", "^TestEndToEnd_MinimalConfig_PostMerge$", true, false, false},
	{"statediff", "^TestEndToEnd_MinimalConfig_WithStateDiff$", true, false, false},
	{"mainnet", "^TestEndToEnd_MainnetConfig_ValidatorAtCurrentRelease$", false, false, false},
	{"multiclient", "^TestEndToEnd_MainnetConfig_MultiClient$", false, true, false},
	{"scenario-multiclient", "^TestEndToEnd_MultiScenarioRun_Multiclient$", false, true, false},
}

// suites group scenarios the way the Bazel test_suites did (testing/endtoend/BUILD.bazel):
//
//	presubmit      = go_default_test
//	postsubmit     = go_builder_test + go_minimal_postmerge_test + go_mainnet_test
//	scenario_tests = go_minimal_scenario_test + go_mainnet_scenario_test
//
// Minimal scenarios are ordered before mainnet ones so the launched binaries are rebuilt
// at most once per consensus config.
var suites = map[string][]string{
	"presubmit":      {"minimal", "statediff", "slashing", "slasher"},
	"postsubmit":     {"builder", "postmerge", "mainnet", "multiclient"},
	"scenario_tests": {"scenario", "scenario-multiclient"},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "❌ e2e:", err)
		os.Exit(1)
	}
}

func run() error {
	goBin := env("GO", "go")
	dist, err := filepath.Abs(env("DIST", "dist"))
	if err != nil {
		return err
	}
	timeout := env("E2E_TIMEOUT", "60m")
	// Tell the harness to colorize its logs when we're attached to a terminal. It runs
	// under `go test` (a pipe), so it can't detect the TTY itself — we forward our own.
	colorEnv := "E2E_LOG_COLOR=0"
	if isTerminal(os.Stdout) {
		colorEnv = "E2E_LOG_COLOR=1"
	}

	label, targets, err := selectTargets(os.Args[1:])
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dist, 0o755); err != nil {
		return err
	}

	// Safeguard 1: kill any devnet processes left over from a previous run (e.g. one that
	// hit `go test`'s -timeout, which orphans the launched binaries). They hold the e2e's
	// fixed ports, so a stale bootnode would make this run fail to bind.
	cleanupStaleProcs(dist)

	// Safeguard 2: a killed/interrupted run shouldn't leak the devnet. Each scenario's
	// `go test` runs in its own process group (see runGoTest); forward Ctrl-C/SIGTERM as a
	// group kill so the whole subtree dies with us.
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, shutdownSignals...)
	go func() {
		<-sigc
		procMu.Lock()
		if currentProc != nil {
			killProcGroup(currentProc.Pid)
		}
		procMu.Unlock()
		fmt.Fprintln(os.Stderr, "\n❌ e2e: interrupted — devnet torn down")
		os.Exit(130)
	}()

	// geth (the EL client) is consensus-config-agnostic — build it once, in its own
	// module context (see installGeth). Every scenario uses it.
	if err := installGeth(goBin, dist); err != nil {
		return fmt.Errorf("building geth: %w", err)
	}

	// Build the launched Prysm binaries lazily per consensus config (tag set), so a suite
	// that mixes minimal + mainnet rebuilds them at most once each.
	built := map[string]bool{}
	for i, k := range targets {
		binTags, testTags := "", "develop"
		if k.minimal {
			binTags, testTags = "minimal", "develop,minimal"
		}
		if !built[binTags] {
			if err := buildPrysmBins(goBin, dist, binTags); err != nil {
				return err
			}
			built[binTags] = true
		}
		if k.needLighthouse {
			if err := provisionLighthouse(dist); err != nil {
				return err
			}
		}
		if k.needWeb3signer {
			if err := provisionWeb3signer(dist); err != nil {
				return err
			}
		}

		logDir, err := os.MkdirTemp("", "prysm-e2e-logs-")
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "e2e: [%d/%d] %s (run=%s, tags=%s)\n  PRYSM_BIN=%s\n  E2E_LOG_PATH=%s\n",
			i+1, len(targets), k.name, k.run, testTags, dist, logDir)

		args := []string{"test", "-tags=" + testTags, "-run", k.run, "-timeout", timeout, "-v", "-count=1", "./testing/endtoend"}
		if err := runGoTest(goBin, args, []string{"PRYSM_BIN=" + dist, "E2E_LOG_PATH=" + logDir, colorEnv}); err != nil {
			return fmt.Errorf("scenario %s failed (logs: %s): %w", k.name, logDir, err)
		}
	}

	if len(targets) == 1 {
		fmt.Printf("✅ e2e: %s passed\n", label)
	} else {
		fmt.Printf("✅ e2e: %s passed (%d scenarios)\n", label, len(targets))
	}
	return nil
}

// selectTargets resolves the positional arg to the scenarios to run: a suite expands to
// its bundle, a kind to itself, and no arg to the default minimal scenario. It returns the
// chosen label (for logging) and the ordered scenario list.
func selectTargets(args []string) (string, []kind, error) {
	name := "minimal"
	for _, a := range args {
		if a != "" && !strings.HasPrefix(a, "-") {
			name = a
			break
		}
	}
	if members, ok := suites[name]; ok {
		ks := make([]kind, 0, len(members))
		for _, m := range members {
			k, ok := kindByName(m)
			if !ok {
				return "", nil, fmt.Errorf("internal: suite %q references unknown kind %q", name, m)
			}
			ks = append(ks, k)
		}
		return name, ks, nil
	}
	if k, ok := kindByName(name); ok && isPublicKind(name) {
		return name, []kind{k}, nil
	}
	return "", nil, fmt.Errorf("unknown e2e target %q (kinds: %s; suites: %s)",
		name, publicKindNames(), suiteNames())
}

func kindByName(name string) (kind, bool) {
	for _, k := range kinds {
		if k.name == name {
			return k, true
		}
	}
	return kind{}, false
}

// isPublicKind reports whether a kind is selectable directly (excludes the internal
// scenario-multiclient, which only the scenario_tests suite reaches).
func isPublicKind(name string) bool {
	return name != "scenario-multiclient"
}

func publicKindNames() string {
	var names []string
	for _, k := range kinds {
		if isPublicKind(k.name) {
			names = append(names, k.name)
		}
	}
	return strings.Join(names, " ")
}

func suiteNames() string {
	// Stable order for the error message.
	return "presubmit postsubmit scenario_tests"
}

// currentProc tracks the in-flight `go test` process so the signal handler can tear down
// its process group; guarded by procMu since the handler runs on its own goroutine.
var (
	procMu      sync.Mutex
	currentProc *os.Process
)

// runGoTest runs one scenario's `go test` in its own process group (so the launched
// devnet is a killable subtree), records it for the signal handler, and on exit kills the
// group — reaping any children the test orphaned (e.g. on a -timeout panic).
func runGoTest(goBin string, args, extraEnv []string) error {
	cmd := exec.Command(goBin, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), extraEnv...)
	setNewProcGroup(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	procMu.Lock()
	currentProc = cmd.Process
	procMu.Unlock()

	err := cmd.Wait()

	killProcGroup(cmd.Process.Pid)
	procMu.Lock()
	currentProc = nil
	procMu.Unlock()
	return err
}

// cleanupStaleProcs best-effort kills devnet binaries from a previous run that are still
// holding the e2e's fixed ports (pkill matches the absolute dist path so it only targets
// this checkout's binaries). No-op where pkill is absent or nothing matches.
func cleanupStaleProcs(dist string) {
	var killed bool
	for _, name := range []string{"bootnode", "beacon-chain", "validator", "geth"} {
		if err := exec.Command("pkill", "-f", filepath.Join(dist, name)).Run(); err == nil {
			killed = true
		}
	}
	if killed {
		fmt.Fprintln(os.Stderr, "e2e: cleaned up stale devnet process(es) from a previous run")
	}
}

// buildPrysmBins compiles the Prysm binaries the harness launches (beacon-chain, validator,
// bootnode) into dist/ with the given build tags (empty for mainnet, "minimal" otherwise).
func buildPrysmBins(goBin, dist, tags string) error {
	fmt.Fprintf(os.Stderr, "e2e: building binaries (tags=%q) → %s\n", tags, dist)
	for _, b := range []struct{ name, pkg string }{
		{"beacon-chain", "./cmd/beacon-chain"},
		{"validator", "./cmd/validator"},
		{"bootnode", "./tools/bootnode"},
	} {
		if err := goBuild(goBin, dist, b.name, b.pkg, tags); err != nil {
			return err
		}
	}
	return nil
}

// installGeth builds geth into dist/ using `go install <pkg>@<version>`, which resolves
// geth's full dependency closure in an isolated module context (independent of Prysm's
// go.mod/go.sum). The version is read from Prysm's pin so the e2e geth matches the dep.
func installGeth(goBin, dist string) error {
	out, err := exec.Command(goBin, "list", "-m", "-f", "{{.Version}}", "github.com/ethereum/go-ethereum").Output()
	if err != nil {
		return fmt.Errorf("resolving go-ethereum version: %w", err)
	}
	version := strings.TrimSpace(string(out))
	fmt.Fprintf(os.Stderr, "  install geth@%s\n", version)
	cmd := exec.Command(goBin, "install", "github.com/ethereum/go-ethereum/cmd/geth@"+version)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	// `go install pkg@version` resolves in its own module context (ignores this module's
	// go.mod/go.sum); GOBIN drops the binary directly in dist/.
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1", "GOBIN="+dist)
	return cmd.Run()
}

// goBuild compiles pkg into dist/name with the given build tags (cgo enabled, as the
// Prysm binaries and geth need it).
func goBuild(goBin, dist, name, pkg, tags string) error {
	args := []string{"build", "-trimpath"}
	if tags != "" {
		args = append(args, "-tags="+tags)
	}
	args = append(args, "-o", filepath.Join(dist, name), pkg)
	cmd := exec.Command(goBin, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	fmt.Fprintf(os.Stderr, "  build %s\n", name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("building %s: %w", name, err)
	}
	return nil
}

// provisionLighthouse fetches the pinned lighthouse release and symlinks dist/lighthouse
// at its binary, so FindBinary("external/lighthouse", "lighthouse") resolves it.
func provisionLighthouse(dist string) error {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return fmt.Errorf("lighthouse is published for linux/amd64 only; this scenario can't run on %s/%s",
			runtime.GOOS, runtime.GOARCH)
	}
	if err := externaldata.Fetch(externaldata.Lighthouse); err != nil {
		return fmt.Errorf("fetching lighthouse: %w", err)
	}
	dir, ok := externaldata.DestDir(externaldata.Lighthouse)
	if !ok {
		return fmt.Errorf("could not locate fetched lighthouse")
	}
	return symlink(filepath.Join(dir, "lighthouse"), filepath.Join(dist, "lighthouse"))
}

// provisionWeb3signer fetches the pinned web3signer release and symlinks dist/web3signer
// at its launcher (bin/web3signer); the Gradle launcher resolves its own APP_HOME through
// the symlink, finding the sibling lib/. Requires a JRE on PATH at run time.
func provisionWeb3signer(dist string) error {
	if _, err := exec.LookPath("java"); err != nil {
		return fmt.Errorf("web3signer needs a JRE: `java` not found on PATH")
	}
	if err := externaldata.Fetch(externaldata.Web3signer); err != nil {
		return fmt.Errorf("fetching web3signer: %w", err)
	}
	dir, ok := externaldata.DestDir(externaldata.Web3signer)
	if !ok {
		return fmt.Errorf("could not locate fetched web3signer")
	}
	return symlink(filepath.Join(dir, "bin", "web3signer"), filepath.Join(dist, "web3signer"))
}

// symlink (re)creates dst as a symlink to src, after verifying src exists.
func symlink(src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("expected %s after fetch: %w", src, err)
	}
	_ = os.Remove(dst)
	if err := os.Symlink(src, dst); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", dst, src, err)
	}
	return nil
}

// isTerminal reports whether f is attached to a character device (a TTY), used to decide
// whether the harness should colorize. Stdlib-only heuristic — no terminal dep needed.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
