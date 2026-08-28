package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/OffchainLabs/prysm/v7/build/externaldata"
)

type (
	kind  string
	suite string
)

const (
	kindMinimal             kind = "minimal"
	kindBuilder             kind = "builder"
	kindWeb3signer          kind = "web3signer"
	kindSlasher             kind = "slasher"
	kindSlashing            kind = "slashing"
	kindScenario            kind = "scenario"
	kindPostmerge           kind = "postmerge"
	kindStatediff           kind = "statediff"
	kindMainnet             kind = "mainnet"
	kindMulticlient         kind = "multiclient"
	kindScenarioMulticlient kind = "scenario-multiclient"
)

const (
	suitePresubmit     suite = "presubmit"
	suitePostsubmit    suite = "postsubmit"
	suiteScenarioTests suite = "scenario_tests"
)

const e2eTimeout = "60m"

const logDirPrefix = "prysm-e2e-logs-"

// javaBin is the JRE launcher web3signer needs on PATH at run time.
const javaBin = "java"

type spec struct {
	run            string // anchored -run regexp
	minimal        bool
	needLighthouse bool
	needWeb3signer bool
	needPrysmSh    bool
}

var kinds = map[kind]spec{
	kindMinimal:             {run: "^TestEndToEnd_MinimalConfig$", minimal: true},
	kindBuilder:             {run: "^TestEndToEnd_MinimalConfig_WithBuilder$", minimal: true},
	kindWeb3signer:          {run: "^TestEndToEnd_MinimalConfig_Web3Signer$", minimal: true, needWeb3signer: true},
	kindSlasher:             {run: "^TestEndToEnd_SlasherSimulator$", minimal: true},
	kindSlashing:            {run: "^TestEndToEnd_Slasher_MinimalConfig$", minimal: true},
	kindScenario:            {run: "^TestEndToEnd_MultiScenarioRun$", minimal: true},
	kindPostmerge:           {run: "^TestEndToEnd_MinimalConfig_PostMerge$", minimal: true},
	kindStatediff:           {run: "^TestEndToEnd_MinimalConfig_WithStateDiff$", minimal: true},
	kindMainnet:             {run: "^TestEndToEnd_MainnetConfig_ValidatorAtCurrentRelease$", needPrysmSh: true},
	kindMulticlient:         {run: "^TestEndToEnd_MainnetConfig_MultiClient$", needLighthouse: true},
	kindScenarioMulticlient: {run: "^TestEndToEnd_MultiScenarioRun_Multiclient$", needLighthouse: true},
}

var suites = map[suite][]kind{
	suitePresubmit:     {kindMinimal, kindStatediff, kindSlashing, kindSlasher},
	suitePostsubmit:    {kindBuilder, kindPostmerge, kindMainnet, kindMulticlient},
	suiteScenarioTests: {kindScenario, kindScenarioMulticlient},
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
		return fmt.Errorf("resolving dist path: %w", err)

	}

	colorEnv := "E2E_LOG_COLOR=0"
	if isTerminal(os.Stdout) {
		colorEnv = "E2E_LOG_COLOR=1"
	}

	label, targets, err := selectTargets(os.Args[1:])
	if err != nil {
		return fmt.Errorf("selecting e2e targets: %w", err)
	}

	if err := os.MkdirAll(dist, 0o750); err != nil {
		return fmt.Errorf("creating dist dir: %w", err)
	}

	// Kill any devnet processes left over from a previous run, then wipe the data
	// directories they left behind.
	cleanupStaleProcs(dist)
	cleanupStaleData()

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, shutdownSignals...)
	go func() {
		<-sigc
		procMu.Lock()
		if currentProc != nil {
			if err := killProcGroup(currentProc.Pid); err != nil {
				fmt.Fprintln(os.Stderr, "e2e: tearing down devnet process group:", err)
			}
		}
		procMu.Unlock()

		fmt.Fprintln(os.Stderr, "\n❌ e2e: interrupted — devnet torn down")
		os.Exit(130)
	}()

	if err := installGeth(goBin, dist); err != nil {
		return fmt.Errorf("building geth: %w", err)
	}

	built := map[string]bool{}
	for i, k := range targets {
		cfg := kinds[k]
		binTags, testTags := "", "develop"
		if cfg.minimal {
			binTags, testTags = "minimal", "develop,minimal"
		}

		if !built[binTags] {
			if err := buildPrysmBins(goBin, dist, binTags); err != nil {
				return fmt.Errorf("building prysm binaries: %w", err)
			}

			built[binTags] = true
		}

		if cfg.needLighthouse {
			if err := provisionLighthouse(dist); err != nil {
				return fmt.Errorf("provisioning lighthouse: %w", err)
			}
		}
		if cfg.needWeb3signer {
			if err := provisionWeb3signer(dist); err != nil {
				return fmt.Errorf("provisioning web3signer: %w", err)
			}
		}
		if cfg.needPrysmSh {
			if err := provisionPrysmSh(dist); err != nil {
				return fmt.Errorf("provisioning prysm.sh: %w", err)
			}
		}

		logDir, err := os.MkdirTemp("", logDirPrefix)
		if err != nil {
			return fmt.Errorf("creating log directory: %w", err)
		}

		fmt.Fprintf(os.Stderr, "e2e: [%d/%d] %s (run=%s, tags=%s)\n  PRYSM_BIN=%s\n  E2E_LOG_PATH=%s\n",
			i+1, len(targets), k, cfg.run, testTags, dist, logDir)

		args := []string{"test", "-tags=" + testTags, "-run", cfg.run, "-timeout", e2eTimeout, "-v", "-count=1", "./testing/endtoend"}
		if err := runGoTest(goBin, args, []string{"PRYSM_BIN=" + dist, "E2E_LOG_PATH=" + logDir, colorEnv}); err != nil {
			return fmt.Errorf("scenario %s failed (logs: %s): %w", k, logDir, err)
		}
	}

	fmt.Printf("✅ e2e: %s passed (%d scenarios)\n", label, len(targets))

	return nil
}

func selectTargets(args []string) (string, []kind, error) {
	var names []string
	for _, arg := range args {
		if arg != "" && !strings.HasPrefix(arg, "-") {
			names = append(names, arg)
		}
	}

	if len(names) == 0 {
		names = []string{string(suitePresubmit)}
	}

	var targets []kind
	seen := map[kind]bool{}
	for _, name := range names {
		expanded, err := resolveTarget(name)
		if err != nil {
			return "", nil, err
		}

		for _, k := range expanded {
			if !seen[k] {
				seen[k] = true
				targets = append(targets, k)
			}
		}
	}

	return strings.Join(names, " "), targets, nil
}

// resolveTarget expands a single suite or scenario name into its constituent kinds.
func resolveTarget(name string) ([]kind, error) {
	if members, ok := suites[suite(name)]; ok {
		return members, nil
	}

	if k := kind(name); kinds[k].run != "" {
		return []kind{k}, nil
	}

	return nil, fmt.Errorf("unknown e2e target %q (kinds: %s; suites: %s)", name, kindNames(), suiteNames())
}

func kindNames() string {
	var names []string
	for kind := range kinds {
		names = append(names, string(kind))
	}

	sort.Strings(names)
	return strings.Join(names, " ")
}

func suiteNames() string {
	return strings.Join([]string{string(suitePresubmit), string(suitePostsubmit), string(suiteScenarioTests)}, " ")
}

var (
	procMu      sync.Mutex
	currentProc *os.Process
)

func runGoTest(goBin string, args, extraEnv []string) error {
	cmd := exec.Command(goBin, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), extraEnv...)

	setNewProcGroup(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting go test: %w", err)
	}

	procMu.Lock()
	currentProc = cmd.Process
	procMu.Unlock()

	err := cmd.Wait()

	if killErr := killProcGroup(cmd.Process.Pid); killErr != nil {
		fmt.Fprintln(os.Stderr, "e2e: reaping devnet process group:", killErr)
	}

	procMu.Lock()
	currentProc = nil
	procMu.Unlock()

	return err
}

func cleanupStaleProcs(dist string) {
	var killed bool
	for _, name := range []string{"bootnode", "beacon-chain", "validator", "geth"} {
		// #nosec G204 -- dist is a developer-controlled path from the DIST env var, not external input.
		if err := exec.Command("pkill", "-f", filepath.Join(dist, name)).Run(); err == nil {
			killed = true
		}
	}

	if killed {
		fmt.Fprintln(os.Stderr, "e2e: cleaned up stale devnet process(es) from a previous run")
	}
}

// cleanupStaleData removes the devnet data directories left behind by a previous run.
func cleanupStaleData() {
	matches, err := filepath.Glob(filepath.Join(os.TempDir(), "shard-*"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e: globbing stale devnet data:", err)
		return
	}

	var removed bool
	for _, dir := range matches {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: removing stale devnet data %s: %v\n", dir, err)
			continue
		}
		removed = true
	}

	if removed {
		fmt.Fprintln(os.Stderr, "e2e: cleaned up stale devnet data from a previous run")
	}
}

func buildPrysmBins(goBin, dist, tags string) error {
	fmt.Fprintf(os.Stderr, "e2e: building binaries (tags=%q) → %s\n", tags, dist)
	for _, b := range []struct{ name, pkg string }{
		{"beacon-chain", "./cmd/beacon-chain"},
		{"validator", "./cmd/validator"},
		{"bootnode", "./tools/bootnode"},
	} {
		if err := goBuild(goBin, dist, b.name, b.pkg, tags); err != nil {
			return fmt.Errorf("building %s: %w", b.name, err)
		}
	}
	return nil
}

func installGeth(goBin, dist string) error {
	out, err := exec.Command(goBin, "list", "-m", "-f", "{{.Version}}", "github.com/ethereum/go-ethereum").Output()
	if err != nil {
		return fmt.Errorf("resolving go-ethereum version: %w", err)
	}

	version := strings.TrimSpace(string(out))
	fmt.Fprintf(os.Stderr, "  install geth@%s\n", version)
	// #nosec G204 -- version comes from `go list` on the pinned go.mod dependency, not external input.
	cmd := exec.Command(goBin, "install", "github.com/ethereum/go-ethereum/cmd/geth@"+version)

	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1", "GOBIN="+dist)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting go install geth: %w", err)
	}

	return nil
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

func provisionLighthouse(dist string) error {
	if _, ok := externaldata.LighthouseTriple(); !ok {
		return fmt.Errorf("lighthouse is not published for %s/%s at the pinned version; the multiclient "+
			"scenario can't run here (supported: linux/amd64, linux/arm64, darwin/arm64)",
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

func provisionWeb3signer(dist string) error {
	if out, err := exec.Command(javaBin, "-version").CombinedOutput(); err != nil {
		return fmt.Errorf("web3signer needs a JRE, but `%s -version` failed: %w\n%s", javaBin, err, out)
	}

	if err := externaldata.Fetch(externaldata.Web3signer); err != nil {
		return fmt.Errorf("fetching web3signer: %w", err)
	}

	dir, ok := externaldata.DestDir(externaldata.Web3signer)
	if !ok {
		return fmt.Errorf("could not locate fetched web3signer")
	}

	if err := symlink(filepath.Join(dir, "bin", "web3signer"), filepath.Join(dist, "web3signer")); err != nil {
		return fmt.Errorf("symlinking web3signer: %w", err)
	}

	return nil
}

// provisionPrysmSh makes the repo's prysm.sh wrapper available to the harness as the
// "prysm_sh" binary.
func provisionPrysmSh(dist string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolving working directory: %w", err)
	}

	return symlink(filepath.Join(wd, "prysm.sh"), filepath.Join(dist, "prysm_sh"))
}

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
// whether the harness should colorize.
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
