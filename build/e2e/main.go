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

// lighthouse (and thus the multiclient scenarios) is only published for this platform.
const (
	osLinux   = "linux"
	archAMD64 = "amd64"
)

type spec struct {
	run            string // anchored -run regexp
	minimal        bool
	needLighthouse bool
	needWeb3signer bool
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
	kindMainnet:             {run: "^TestEndToEnd_MainnetConfig_ValidatorAtCurrentRelease$"},
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

	if err := os.MkdirAll(dist, 0o755); err != nil {
		return fmt.Errorf("creating dist dir: %w", err)
	}

	// Kill any devnet processes left over from a previous run.
	cleanupStaleProcs(dist)

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
	name := string(suitePresubmit)
	for _, arg := range args {
		if arg != "" && !strings.HasPrefix(arg, "-") {
			name = arg
			break
		}
	}

	if members, ok := suites[suite(name)]; ok {
		return name, members, nil
	}

	if k := kind(name); isPublicKind(k) {
		if _, ok := kinds[k]; ok {
			return name, []kind{k}, nil
		}
	}

	return "", nil, fmt.Errorf("unknown e2e target %q (kinds: %s; suites: %s)", name, publicKindNames(), suiteNames())
}

func isPublicKind(k kind) bool {
	return k != kindScenarioMulticlient
}

func publicKindNames() string {
	var names []string
	for kind := range kinds {
		if isPublicKind(kind) {
			names = append(names, string(kind))
		}
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

	killProcGroup(cmd.Process.Pid)

	procMu.Lock()
	currentProc = nil
	procMu.Unlock()

	return err
}

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
	if runtime.GOOS != osLinux || runtime.GOARCH != archAMD64 {
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

func provisionWeb3signer(dist string) error {
	if _, err := exec.LookPath(javaBin); err != nil {
		return fmt.Errorf("web3signer needs a JRE: `java` not found on PATH")
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
