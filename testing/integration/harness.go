// Package integration provides a lightweight test harness for spinning up
// a small Prysm cluster (beacon nodes, geth nodes, validators) from genesis.
// Designed for fast, focused integration tests — not a replacement for the
// full e2e suite.
package integration

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	gloas "github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/build/bazel"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/interop"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/core"
)

// Harness manages a small test cluster.
type Harness struct {
	t      *testing.T
	cfg    *Config
	tmpDir string

	geths      []*process
	beacons    []*process
	validators []*process

	genesisTime    time.Time
	jwtSecret      string
	genesisSSZPath string
	configYAMLPath string
}

type process struct {
	cmd    *exec.Cmd
	logDir string
	index  int
}

// NewHarness creates a harness with the given config. Call Start() to launch.
func NewHarness(t *testing.T, cfg *Config) *Harness {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	tmpDir, err := os.MkdirTemp("", "prysm-integration-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Logs and data: %s", tmpDir)
	return &Harness{
		t:      t,
		cfg:    cfg,
		tmpDir: tmpDir,
	}
}

// Start builds binaries, generates genesis, and starts all nodes.
func (h *Harness) Start() {
	h.t.Helper()

	h.t.Log("Building binaries...")
	h.buildBinaries()

	h.t.Log("Generating genesis...")
	h.generateJWTSecret()
	h.generateGenesis()

	h.t.Log("Starting geth nodes...")
	for i := 0; i < h.cfg.NumGethNodes; i++ {
		h.startGeth(i)
	}
	// Give geth a moment to initialize.
	time.Sleep(2 * time.Second)

	h.t.Log("Starting beacon nodes...")
	// Start beacon-0 first, extract its ENR, then start the rest with it as a peer.
	h.startBeacon(0)
	enr := h.waitForENR(0)
	h.t.Logf("  beacon-0 ENR: %s", enr)
	for i := 1; i < h.cfg.NumBeaconNodes; i++ {
		h.startBeacon(i, enr)
	}

	h.t.Log("Waiting for beacon nodes to be ready...")
	for i := 0; i < h.cfg.NumBeaconNodes; i++ {
		h.waitForBeaconAPI(i)
	}

	h.t.Log("Starting validators...")
	for i := 0; i < h.cfg.NumBeaconNodes; i++ {
		h.startValidator(i)
	}

	h.t.Cleanup(func() {
		h.t.Log("Stopping cluster...")
		h.stopAll()
		h.checkLogsForProblems()
		if h.t.Failed() {
			h.dumpLogs()
		}
		_ = os.RemoveAll(h.tmpDir)
	})

	h.t.Log("Cluster started.")
}

// --- Binary building ---

func (h *Harness) binaryPath(name string) string {
	return filepath.Join(h.tmpDir, "bin", name)
}

func (h *Harness) buildBinaries() {
	h.t.Helper()
	binDir := filepath.Join(h.tmpDir, "bin")
	require.NoError(h.t, os.MkdirAll(binDir, 0o755))

	// Try bazel runfiles first (available when run via `bazel test`).
	if h.tryBazelBinaries() {
		return
	}

	// Fall back to go build.
	repoRoot := findRepoRoot(h.t)
	for name, pkg := range map[string]string{
		"beacon-chain": "./cmd/beacon-chain",
		"validator":    "./cmd/validator",
	} {
		h.goBuild(repoRoot, pkg, name)
	}
	h.prepareGeth(repoRoot)
}

func (h *Harness) tryBazelBinaries() (found bool) {
	// bazel.FindBinary panics if not built with bazel.
	defer func() {
		if r := recover(); r != nil {
			found = false
		}
	}()

	// Prysm binaries from bazel runfiles.
	for _, b := range [][2]string{
		{"cmd/beacon-chain", "beacon-chain"},
		{"cmd/validator", "validator"},
	} {
		path, ok := bazel.FindBinary(b[0], b[1])
		if !ok {
			return false
		}
		dst := h.binaryPath(b[1])
		require.NoError(h.t, os.Symlink(path, dst))
		h.t.Logf("  bazel binary %s -> %s", b[1], path)
	}

	// Geth: prefer GETH_BINARY env var, then bazel runfiles.
	if h.cfg.GethBinary != "" {
		dst := h.binaryPath("geth")
		require.NoError(h.t, os.Symlink(h.cfg.GethBinary, dst))
		h.t.Logf("  using pre-built geth: %s", h.cfg.GethBinary)
	} else {
		path, ok := bazel.FindBinary("cmd/geth", "geth")
		if !ok {
			return false
		}
		dst := h.binaryPath("geth")
		require.NoError(h.t, os.Symlink(path, dst))
		h.t.Logf("  bazel binary geth -> %s", path)
	}

	return true
}

func (h *Harness) prepareGeth(repoRoot string) {
	h.t.Helper()

	// Option 1: Pre-built binary via GETH_BINARY.
	if h.cfg.GethBinary != "" {
		h.t.Logf("  using pre-built geth: %s", h.cfg.GethBinary)
		// Symlink into our bin dir so binaryPath("geth") works.
		dst := h.binaryPath("geth")
		require.NoError(h.t, os.Symlink(h.cfg.GethBinary, dst))
		return
	}

	// Option 2: Clone and build from a custom repo/branch.
	if h.cfg.GethRepo != "" {
		h.buildGethFromRepo()
		return
	}

	// Option 3: Build from the go module dependency.
	h.goBuild(repoRoot, "github.com/ethereum/go-ethereum/cmd/geth", "geth")
}

func (h *Harness) buildGethFromRepo() {
	h.t.Helper()
	gethSrc := filepath.Join(h.tmpDir, "geth-src")

	branch := h.cfg.GethBranch
	if branch == "" {
		branch = "main"
	}

	h.t.Logf("  cloning geth from %s (branch: %s)...", h.cfg.GethRepo, branch)
	cloneURL := fmt.Sprintf("https://%s.git", h.cfg.GethRepo)
	cmd := exec.Command("git", "clone", "--depth=1", "--branch", branch, cloneURL, gethSrc)
	output, err := cmd.CombinedOutput()
	require.NoError(h.t, err, "git clone failed: %s", string(output))

	h.t.Logf("  building geth from source...")
	out := h.binaryPath("geth")
	cmd = exec.Command("go", "build", "-o", out, "./cmd/geth")
	cmd.Dir = gethSrc
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	output, err = cmd.CombinedOutput()
	require.NoError(h.t, err, "geth build failed: %s", string(output))
}

func (h *Harness) goBuild(dir, pkg, name string) {
	h.t.Helper()
	out := h.binaryPath(name)
	h.t.Logf("  building %s...", name)
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	output, err := cmd.CombinedOutput()
	require.NoError(h.t, err, "failed to build %s: %s", name, string(output))
}

// --- Genesis ---

func (h *Harness) generateJWTSecret() {
	h.t.Helper()
	secret := make([]byte, 32)
	_, err := rand.Read(secret)
	require.NoError(h.t, err)
	h.jwtSecret = hex.EncodeToString(secret)

	// Write JWT secret file for each geth + beacon pair.
	for i := 0; i < max(h.cfg.NumGethNodes, h.cfg.NumBeaconNodes); i++ {
		jwtPath := h.jwtSecretPath(i)
		require.NoError(h.t, os.MkdirAll(filepath.Dir(jwtPath), 0o755))
		require.NoError(h.t, os.WriteFile(jwtPath, []byte(h.jwtSecret), 0o600))
	}
}

func (h *Harness) jwtSecretPath(index int) string {
	return filepath.Join(h.tmpDir, fmt.Sprintf("node-%d", index), "jwt.hex")
}

func (h *Harness) generateGenesis() {
	h.t.Helper()

	bcfg := h.cfg.BeaconConfig()
	params.OverrideBeaconConfig(bcfg)

	h.genesisTime = time.Now().Add(10 * time.Second)

	// Geth genesis.
	gethGenesis := interop.GethTestnetGenesis(h.genesisTime, bcfg)
	for i := 0; i < h.cfg.NumGethNodes; i++ {
		h.writeGethGenesis(i, gethGenesis)
	}

	// Beacon genesis state.
	gb := gethGenesis.ToBlock()
	st, err := interop.NewPreminedGenesis(
		context.Background(),
		h.genesisTime,
		uint64(h.cfg.NumValidators),
		0, // pregenesis creds
		forkVersion(h.cfg.GenesisFork),
		gb,
	)
	require.NoError(h.t, err, "failed to create genesis state")

	// NewPreminedGenesis only supports up to Fulu. Upgrade to Gloas if needed.
	if h.cfg.GenesisFork >= version.Gloas {
		st, err = gloas.UpgradeToGloas(st)
		require.NoError(h.t, err, "failed to upgrade genesis state to gloas")

		// Update LatestBlockHeader to match the Gloas genesis block body.
		gloasBody := gloasGenesisBlockBody()
		bodyRoot, err := gloasBody.HashTreeRoot()
		require.NoError(h.t, err, "failed to hash gloas genesis block body")
		require.NoError(h.t, st.SetLatestBlockHeader(
			&ethpb.BeaconBlockHeader{
				ParentRoot: params.BeaconConfig().ZeroHash[:],
				StateRoot:  params.BeaconConfig().ZeroHash[:],
				BodyRoot:   bodyRoot[:],
			},
		))
	}

	sszBytes, err := st.MarshalSSZ()
	require.NoError(h.t, err)

	h.genesisSSZPath = filepath.Join(h.tmpDir, "genesis.ssz")
	require.NoError(h.t, os.WriteFile(h.genesisSSZPath, sszBytes, 0o644))

	// Write chain config YAML so beacon + validator use our custom config.
	h.configYAMLPath = filepath.Join(h.tmpDir, "config.yaml")
	require.NoError(h.t, os.WriteFile(h.configYAMLPath, params.ConfigToYaml(bcfg), 0o644))
}

// waitForENR watches the beacon node log for its ENR and returns it.
func (h *Harness) waitForENR(index int) string {
	h.t.Helper()
	logFile := filepath.Join(h.tmpDir, "logs", fmt.Sprintf("beacon-%d.log", index))
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(logFile)
		if err == nil {
			cleaned := ansiRegex.ReplaceAllString(string(data), "")
			scanner := bufio.NewScanner(strings.NewReader(cleaned))
			for scanner.Scan() {
				line := scanner.Text()
				if idx := strings.Index(line, "enr:"); idx >= 0 {
					enr := line[idx:]
					if end := strings.IndexAny(enr, " \t\n"); end > 0 {
						enr = enr[:end]
					}
					return enr
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	h.t.Fatal("timed out waiting for beacon ENR")
	return ""
}

// waitForBeaconAPI polls the beacon node's REST API until it responds or times out.
func (h *Harness) waitForBeaconAPI(index int) {
	h.t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/eth/v1/node/health", beaconGRPCPort(index))
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			h.t.Logf("  beacon-%d API ready", index)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	h.t.Fatalf("beacon-%d API not ready after 30s", index)
}

func gloasGenesisBlockBody() *ethpb.BeaconBlockBodyGloas {
	return &ethpb.BeaconBlockBodyGloas{
		RandaoReveal: make([]byte, 96),
		Eth1Data: &ethpb.Eth1Data{
			DepositRoot: make([]byte, 32),
			BlockHash:   make([]byte, 32),
		},
		Graffiti: make([]byte, 32),
		SyncAggregate: &ethpb.SyncAggregate{
			SyncCommitteeBits:      make([]byte, fieldparams.SyncCommitteeLength/8),
			SyncCommitteeSignature: make([]byte, fieldparams.BLSSignatureLength),
		},
		SignedExecutionPayloadBid: &ethpb.SignedExecutionPayloadBid{
			Message: &ethpb.ExecutionPayloadBid{
				ParentBlockHash:    make([]byte, 32),
				ParentBlockRoot:    make([]byte, 32),
				BlockHash:          make([]byte, 32),
				PrevRandao:         make([]byte, 32),
				FeeRecipient:       make([]byte, 20),
				BlobKzgCommitments: make([][]byte, 0),
			},
			Signature: make([]byte, fieldparams.BLSSignatureLength),
		},
		PayloadAttestations: make([]*ethpb.PayloadAttestation, 0),
	}
}

func forkVersion(fork int) int {
	// NewPreminedGenesis only supports up to Fulu currently.
	// For Gloas, generate at Fulu and upgrade via UpgradeToGloas.
	if fork >= version.Gloas {
		return version.Fulu
	}
	return fork
}

func (h *Harness) writeGethGenesis(index int, genesis *core.Genesis) {
	h.t.Helper()
	dir := filepath.Join(h.tmpDir, fmt.Sprintf("geth-%d", index))
	require.NoError(h.t, os.MkdirAll(dir, 0o755))

	data, err := json.Marshal(genesis)
	require.NoError(h.t, err)
	require.NoError(h.t, os.WriteFile(filepath.Join(dir, "genesis.json"), data, 0o644))
}

// --- Geth ---

func (h *Harness) startGeth(index int) {
	h.t.Helper()
	dataDir := filepath.Join(h.tmpDir, fmt.Sprintf("geth-%d", index))

	// Init geth with genesis.
	gethBin := h.binaryPath("geth")
	initCmd := exec.Command(gethBin, "init",
		"--datadir", dataDir,
		filepath.Join(dataDir, "genesis.json"),
	)
	output, err := initCmd.CombinedOutput()
	require.NoError(h.t, err, "geth init failed: %s", string(output))

	// Start geth.
	args := []string{
		"--datadir", dataDir,
		"--networkid", "1337",
		"--nat", "none",
		"--nodiscover",
		"--http",
		"--http.addr", "127.0.0.1",
		"--http.port", fmt.Sprintf("%d", gethHTTPPort(index)),
		"--http.api", "eth,net,engine,admin",
		"--authrpc.addr", "127.0.0.1",
		"--authrpc.port", fmt.Sprintf("%d", gethAuthRPCPort(index)),
		"--authrpc.jwtsecret", h.jwtSecretPath(index),
		"--authrpc.vhosts=*",
		"--port", fmt.Sprintf("%d", gethP2PPort(index)),
		"--syncmode", "full",
		"--miner.gaslimit", fmt.Sprintf("%d", params.BeaconConfig().DefaultBuilderGasLimit),
	}

	p := h.startProcess("geth", gethBin, args, dataDir, index)
	h.geths = append(h.geths, p)
}

// --- Beacon Node ---

func (h *Harness) startBeacon(index int, peerENRs ...string) {
	h.t.Helper()
	dataDir := filepath.Join(h.tmpDir, fmt.Sprintf("beacon-%d", index))
	require.NoError(h.t, os.MkdirAll(dataDir, 0o755))

	// Map beacon node to geth node (1:1 for now).
	gethIndex := index % h.cfg.NumGethNodes

	args := []string{
		"--datadir", dataDir,
		"--chain-config-file", h.configYAMLPath,
		"--min-sync-peers=0",
		"--genesis-state", h.genesisSSZPath,
		"--interop-eth1data-votes",
		"--contract-deployment-block=0",
		"--execution-endpoint", gethAuthEndpoint(gethIndex),
		"--jwt-secret", h.jwtSecretPath(gethIndex),
		"--rpc-port", fmt.Sprintf("%d", beaconRPCPort(index)),
		"--grpc-gateway-port", fmt.Sprintf("%d", beaconGRPCPort(index)),
		"--p2p-udp-port", fmt.Sprintf("%d", beaconP2PUDPPort(index)),
		"--p2p-tcp-port", fmt.Sprintf("%d", beaconP2PTCPPort(index)),
		"--monitoring-port", fmt.Sprintf("%d", beaconMonitorPort(index)),
		"--minimum-peers-per-subnet=0",
		"--suggested-fee-recipient=0x0000000000000000000000000000000000000001",
		"--p2p-static-id",
		"--bootstrap-node=",
		"--accept-terms-of-use",
		"--force-clear-db",
		"--verbosity", "debug",
		"--disable-log-colors",
	}

	for _, enr := range peerENRs {
		args = append(args, "--peer", enr)
	}

	p := h.startProcess("beacon", h.binaryPath("beacon-chain"), args, dataDir, index)
	h.beacons = append(h.beacons, p)
}

// startSyncBeacon starts a beacon node configured for initial sync (min-sync-peers=1).
func (h *Harness) startSyncBeacon(index int, peerENRs ...string) {
	h.t.Helper()
	dataDir := filepath.Join(h.tmpDir, fmt.Sprintf("beacon-%d", index))
	require.NoError(h.t, os.MkdirAll(dataDir, 0o755))

	gethIndex := index % h.cfg.NumGethNodes

	args := []string{
		"--datadir", dataDir,
		"--chain-config-file", h.configYAMLPath,
		"--min-sync-peers=1",
		"--genesis-state", h.genesisSSZPath,
		"--interop-eth1data-votes",
		"--contract-deployment-block=0",
		"--execution-endpoint", gethAuthEndpoint(gethIndex),
		"--jwt-secret", h.jwtSecretPath(gethIndex),
		"--rpc-port", fmt.Sprintf("%d", beaconRPCPort(index)),
		"--grpc-gateway-port", fmt.Sprintf("%d", beaconGRPCPort(index)),
		"--p2p-udp-port", fmt.Sprintf("%d", beaconP2PUDPPort(index)),
		"--p2p-tcp-port", fmt.Sprintf("%d", beaconP2PTCPPort(index)),
		"--monitoring-port", fmt.Sprintf("%d", beaconMonitorPort(index)),
		"--minimum-peers-per-subnet=0",
		"--suggested-fee-recipient=0x0000000000000000000000000000000000000001",
		"--p2p-static-id",
		"--bootstrap-node=",
		"--accept-terms-of-use",
		"--force-clear-db",
		"--verbosity", "debug",
		"--disable-log-colors",
	}

	for _, enr := range peerENRs {
		args = append(args, "--peer", enr)
	}

	p := h.startProcess("beacon", h.binaryPath("beacon-chain"), args, dataDir, index)
	h.beacons = append(h.beacons, p)
}

// --- Validator ---

func (h *Harness) startValidator(index int) {
	h.t.Helper()
	dataDir := filepath.Join(h.tmpDir, fmt.Sprintf("validator-%d", index))
	require.NoError(h.t, os.MkdirAll(dataDir, 0o755))

	vpn := h.cfg.ValidatorsPerNode()
	offset := index * vpn

	args := []string{
		"--datadir", dataDir,
		"--chain-config-file", h.configYAMLPath,
		"--interop-num-validators", fmt.Sprintf("%d", vpn),
		"--interop-start-index", fmt.Sprintf("%d", offset),
		"--beacon-rpc-provider", fmt.Sprintf("127.0.0.1:%d", beaconRPCPort(index)),
		"--monitoring-port", fmt.Sprintf("%d", validatorMonitorPort(index)),
		"--grpc-gateway-port", fmt.Sprintf("%d", validatorRPCPort(index)),
		"--accept-terms-of-use",
		"--force-clear-db",
		"--verbosity", "info",
		"--disable-log-colors",
	}

	p := h.startProcess("validator", h.binaryPath("validator"), args, dataDir, index)
	h.validators = append(h.validators, p)
}

// --- Process management ---

func (h *Harness) startProcess(name, binary string, args []string, dataDir string, index int) *process {
	h.t.Helper()
	logDir := filepath.Join(h.tmpDir, "logs")
	require.NoError(h.t, os.MkdirAll(logDir, 0o755))

	logFile := filepath.Join(logDir, fmt.Sprintf("%s-%d.log", name, index))
	f, err := os.Create(logFile)
	require.NoError(h.t, err)

	cmd := exec.Command(binary, args...)
	cmd.Stdout = f
	cmd.Stderr = f

	h.t.Logf("Starting %s-%d: %s", name, index, cmd.String())
	require.NoError(h.t, cmd.Start(), "failed to start %s-%d", name, index)

	return &process{cmd: cmd, logDir: logDir, index: index}
}

// AddSyncNode starts a new geth + beacon node pair for sync testing.
// It uses the next available index and peers with existing beacon nodes.
// No validator is started — the node only syncs. Returns the beacon node index.
func (h *Harness) AddSyncNode() int {
	h.t.Helper()
	gethIndex := len(h.geths)
	beaconIndex := len(h.beacons)

	// Write JWT secret for the new node.
	jwtPath := h.jwtSecretPath(gethIndex)
	require.NoError(h.t, os.MkdirAll(filepath.Dir(jwtPath), 0o755))
	require.NoError(h.t, os.WriteFile(jwtPath, []byte(h.jwtSecret), 0o600))

	// Write geth genesis for the new node.
	bcfg := h.cfg.BeaconConfig()
	gethGenesis := interop.GethTestnetGenesis(h.genesisTime, bcfg)
	h.writeGethGenesis(gethIndex, gethGenesis)

	h.t.Logf("Starting sync geth-%d...", gethIndex)
	h.startGeth(gethIndex)
	time.Sleep(2 * time.Second)

	// Collect ENRs from existing beacon nodes.
	var enrs []string
	for i := range h.beacons {
		enr := h.waitForENR(i)
		enrs = append(enrs, enr)
	}

	// Peer geth-new with existing geth nodes so it can sync execution payloads.
	h.peerGethNodes(gethIndex)

	// Temporarily bump NumGethNodes so startBeacon maps to the new geth.
	origGethNodes := h.cfg.NumGethNodes
	h.cfg.NumGethNodes = gethIndex + 1
	h.t.Logf("Starting sync beacon-%d...", beaconIndex)
	h.startSyncBeacon(beaconIndex, enrs...)
	h.cfg.NumGethNodes = origGethNodes

	return beaconIndex
}

// peerGethNodes adds geth-0's enode to the given geth node via admin_addPeer.
func (h *Harness) peerGethNodes(newIndex int) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get enode from geth-0.
	enode, err := h.gethRPC(ctx, 0, "admin_nodeInfo")
	if err != nil {
		h.t.Logf("Warning: could not get geth-0 enode: %v", err)
		return
	}

	var info struct {
		Enode string `json:"enode"`
	}
	if err := json.Unmarshal(enode, &info); err != nil {
		h.t.Logf("Warning: could not parse geth-0 nodeInfo: %v", err)
		return
	}

	// Add geth-0 as peer to geth-new.
	_, err = h.gethRPC(ctx, newIndex, "admin_addPeer", info.Enode)
	if err != nil {
		h.t.Logf("Warning: could not add geth peer: %v", err)
		return
	}
	h.t.Logf("Peered geth-%d with geth-0 (enode: %s)", newIndex, info.Enode[:50]+"...")
}

// gethRPC makes a JSON-RPC call to the given geth node's HTTP endpoint.
func (h *Harness) gethRPC(ctx context.Context, index int, method string, params ...any) (json.RawMessage, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d", gethHTTPPort(index))
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var result struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, fmt.Errorf("RPC error: %s", result.Error.Message)
	}
	return result.Result, nil
}

func (h *Harness) stopAll() {
	for _, p := range h.validators {
		killProcess(p)
	}
	for _, p := range h.beacons {
		killProcess(p)
	}
	for _, p := range h.geths {
		killProcess(p)
	}
}

func (h *Harness) dumpLogs() {
	h.t.Helper()
	logDir := filepath.Join(h.tmpDir, "logs")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(logDir, e.Name()))
		if err != nil {
			continue
		}
		// Show last 10 lines of each log.
		lines := strings.Split(string(data), "\n")
		start := 0
		if len(lines) > 10 {
			start = len(lines) - 10
		}
		h.t.Logf("=== %s (last %d lines) ===\n%s", e.Name(), len(lines)-start, strings.Join(lines[start:], "\n"))
	}
}

var (
	ansiRe     = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	logLevelRe = regexp.MustCompile(`\s+(ERROR|WARN|WARNING|FATAL)\s+`)
)

// checkLogsForProblems scans beacon and validator logs for ERROR/FATAL lines
// and fails the test if any unexpected ones are found.
func (h *Harness) checkLogsForProblems() {
	h.t.Helper()
	logDir := filepath.Join(h.tmpDir, "logs")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}

	allowed := loadAllowedPatterns()

	var errors []string
	var warnings []string

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "beacon-") && !strings.HasPrefix(name, "validator-") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(logDir, name))
		if err != nil {
			continue
		}
		cleaned := ansiRe.ReplaceAllString(string(data), "")
		scanner := bufio.NewScanner(strings.NewReader(cleaned))
		for scanner.Scan() {
			line := scanner.Text()
			m := logLevelRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			level := m[1]
			if isAllowed(line, allowed) {
				continue
			}
			entry := fmt.Sprintf("[%s] %s", name, line)
			switch level {
			case "ERROR", "FATAL":
				errors = append(errors, entry)
			case "WARN", "WARNING":
				warnings = append(warnings, entry)
			}
		}
	}

	if len(warnings) > 0 {
		h.t.Logf("=== %d warning(s) found in logs ===", len(warnings))
		for _, w := range warnings {
			h.t.Logf("  WARN: %s", w)
		}
	}

	if len(errors) > 0 {
		h.t.Logf("=== %d error(s) found in logs ===", len(errors))
		for _, e := range errors {
			h.t.Logf("  ERROR: %s", e)
		}
		h.t.Errorf("Test failed: %d ERROR/FATAL line(s) found in beacon/validator logs", len(errors))
	}
}

// loadAllowedPatterns reads allowed_logs.txt from the same directory as this source file.
func loadAllowedPatterns() []string {
	// Try bazel runfiles first, fall back to source-relative path.
	path, err := bazel.Runfile("testing/integration/allowed_logs.txt")
	if err != nil {
		_, thisFile, _, _ := runtime.Caller(0)
		path = filepath.Join(filepath.Dir(thisFile), "allowed_logs.txt")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var patterns []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

func isAllowed(line string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(line, p) {
			return true
		}
	}
	return false
}

func killProcess(p *process) {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return
	}
	_ = p.cmd.Process.Kill()
	_ = p.cmd.Wait()
}

// --- Helpers ---

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod found)")
		}
		dir = parent
	}
}
