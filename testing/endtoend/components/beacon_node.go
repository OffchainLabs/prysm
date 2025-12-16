// Package components defines utilities to spin up actual
// beacon node and validator processes as needed by end to end tests.
package components

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	cmdshared "github.com/OffchainLabs/prysm/v7/cmd"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/genesis"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/io/file"
	"github.com/OffchainLabs/prysm/v7/runtime/interop"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/helpers"
	e2e "github.com/OffchainLabs/prysm/v7/testing/endtoend/params"
	e2etypes "github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

var _ e2etypes.ComponentRunner = (*BeaconNode)(nil)
var _ e2etypes.ComponentRunner = (*BeaconNodeSet)(nil)
var _ e2etypes.MultipleComponentRunners = (*BeaconNodeSet)(nil)
var _ e2etypes.BeaconNodeSet = (*BeaconNodeSet)(nil)
var _ e2etypes.RestartableBeaconNodeSet = (*BeaconNodeSet)(nil)

// BeaconNodeSet represents set of beacon nodes.
type BeaconNodeSet struct {
	e2etypes.ComponentRunner
	config  *e2etypes.E2EConfig
	nodes   []e2etypes.ComponentRunner
	enr     string
	ids     []string
	started chan struct{}
}

// SetENR assigns ENR to the set of beacon nodes.
func (s *BeaconNodeSet) SetENR(enr string) {
	s.enr = enr
}

// NewBeaconNodes creates and returns a set of beacon nodes.
func NewBeaconNodes(config *e2etypes.E2EConfig) *BeaconNodeSet {
	return &BeaconNodeSet{
		config:  config,
		started: make(chan struct{}, 1),
	}
}

// Start starts all the beacon nodes in set.
func (s *BeaconNodeSet) Start(ctx context.Context) error {
	if s.enr == "" {
		return errors.New("empty ENR")
	}

	// Create beacon nodes.
	nodes := make([]e2etypes.ComponentRunner, e2e.TestParams.BeaconNodeCount)
	for i := 0; i < e2e.TestParams.BeaconNodeCount; i++ {
		nodes[i] = NewBeaconNode(s.config, i, s.enr)
	}
	s.nodes = nodes

	// Wait for all nodes to finish their job (blocking).
	// Once nodes are ready passed in handler function will be called.
	return helpers.WaitOnNodes(ctx, nodes, func() {
		if s.config.UseFixedPeerIDs {
			for i := range nodes {
				s.ids = append(s.ids, nodes[i].(*BeaconNode).peerID)
			}
			s.config.PeerIDs = s.ids
		}
		// All nodes started, close channel, so that all services waiting on a set, can proceed.
		close(s.started)
	})
}

// Started checks whether beacon node set is started and all nodes are ready to be queried.
func (s *BeaconNodeSet) Started() <-chan struct{} {
	return s.started
}

// Pause pauses the component and its underlying process.
func (s *BeaconNodeSet) Pause() error {
	for _, n := range s.nodes {
		if err := n.Pause(); err != nil {
			return err
		}
	}
	return nil
}

// Resume resumes the component and its underlying process.
func (s *BeaconNodeSet) Resume() error {
	for _, n := range s.nodes {
		if err := n.Resume(); err != nil {
			return err
		}
	}
	return nil
}

// Stop stops the component and its underlying process.
func (s *BeaconNodeSet) Stop() error {
	for _, n := range s.nodes {
		if err := n.Stop(); err != nil {
			return err
		}
	}
	return nil
}

// PauseAtIndex pauses the component and its underlying process at the desired index.
func (s *BeaconNodeSet) PauseAtIndex(i int) error {
	if i >= len(s.nodes) {
		return errors.Errorf("provided index exceeds slice size: %d >= %d", i, len(s.nodes))
	}
	return s.nodes[i].Pause()
}

// ResumeAtIndex resumes the component and its underlying process at the desired index.
func (s *BeaconNodeSet) ResumeAtIndex(i int) error {
	if i >= len(s.nodes) {
		return errors.Errorf("provided index exceeds slice size: %d >= %d", i, len(s.nodes))
	}
	return s.nodes[i].Resume()
}

// StopAtIndex stops the component and its underlying process at the desired index.
func (s *BeaconNodeSet) StopAtIndex(i int) error {
	if i >= len(s.nodes) {
		return errors.Errorf("provided index exceeds slice size: %d >= %d", i, len(s.nodes))
	}
	return s.nodes[i].Stop()
}

// ComponentAtIndex returns the component at the provided index.
func (s *BeaconNodeSet) ComponentAtIndex(i int) (e2etypes.ComponentRunner, error) {
	if i >= len(s.nodes) {
		return nil, errors.Errorf("provided index exceeds slice size: %d >= %d", i, len(s.nodes))
	}
	return s.nodes[i], nil
}

// RestartAtIndex stops the beacon node at the given index and restarts it
// with the provided extra flags appended to the existing configuration.
// The restarted node preserves its data directory (does not clear DB).
func (s *BeaconNodeSet) RestartAtIndex(ctx context.Context, i int, extraFlags []string) error {
	if i >= len(s.nodes) {
		return errors.Errorf("provided index exceeds slice size: %d >= %d", i, len(s.nodes))
	}

	// Get the existing node to extract its configuration
	oldNode, ok := s.nodes[i].(*BeaconNode)
	if !ok {
		return errors.New("node at index is not a BeaconNode")
	}

	// Backup the log file before restart so we don't lose pre-restart logs
	oldLogPath := path.Join(e2e.TestParams.LogPath, fmt.Sprintf(e2e.BeaconNodeLogFileName, i))
	backupLogPath := path.Join(e2e.TestParams.LogPath, fmt.Sprintf("beacon-%d-pre-restart.log", i))
	if err := copyFile(oldLogPath, backupLogPath); err != nil {
		log.WithError(err).Warnf("Failed to backup log file before restart (non-fatal)")
	} else {
		log.Infof("Backed up beacon node %d log to %s", i, backupLogPath)
	}

	// Stop the node for restart (sets restarting flag to prevent errgroup failure)
	log.Infof("Stopping beacon node %d for restart", i)
	if err := oldNode.StopForRestart(); err != nil {
		return errors.Wrap(err, "failed to stop node for restart")
	}

	// Wait a moment for the process to fully terminate
	time.Sleep(2 * time.Second)

	// Create a new config with extra flags
	newConfig := *s.config
	newConfig.BeaconFlags = append(slices.Clone(s.config.BeaconFlags), extraFlags...)

	// Create a new node that will preserve the data directory
	newNode := NewBeaconNodeForRestart(&newConfig, oldNode.index, oldNode.enr)
	s.nodes[i] = newNode

	// Start the new node in a goroutine
	startErrCh := make(chan error, 1)
	go func() {
		if err := newNode.Start(ctx); err != nil {
			// Only report error if context wasn't cancelled
			if ctx.Err() == nil {
				startErrCh <- err
			}
		}
	}()

	// Wait for node to start or timeout
	select {
	case <-newNode.Started():
		log.Infof("Beacon node %d restarted successfully with extra flags: %v", i, extraFlags)
		return nil
	case err := <-startErrCh:
		return errors.Wrap(err, "failed to start restarted beacon node")
	case <-time.After(2 * time.Minute):
		return errors.New("timeout waiting for restarted beacon node to start")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PreRestartHook is a function called after stopping a node but before restarting it.
// It receives the BeaconNodeSet and node index, allowing modification of the node's database.
type PreRestartHook func(s *BeaconNodeSet, nodeIndex int) error

// RestartAtIndexWithPreHook is like RestartAtIndex but calls the provided hook function
// after stopping the node and before restarting it. This allows modification of the
// node's database (e.g., setting custody info) while the node is stopped.
func (s *BeaconNodeSet) RestartAtIndexWithPreHook(ctx context.Context, i int, extraFlags []string, preHook PreRestartHook) error {
	if i >= len(s.nodes) {
		return errors.Errorf("provided index exceeds slice size: %d >= %d", i, len(s.nodes))
	}

	// Get the existing node to extract its configuration
	oldNode, ok := s.nodes[i].(*BeaconNode)
	if !ok {
		return errors.New("node at index is not a BeaconNode")
	}

	// Backup the log file before restart so we don't lose pre-restart logs
	oldLogPath := path.Join(e2e.TestParams.LogPath, fmt.Sprintf(e2e.BeaconNodeLogFileName, i))
	backupLogPath := path.Join(e2e.TestParams.LogPath, fmt.Sprintf("beacon-%d-pre-restart.log", i))
	if err := copyFile(oldLogPath, backupLogPath); err != nil {
		log.WithError(err).Warnf("Failed to backup log file before restart (non-fatal)")
	} else {
		log.Infof("Backed up beacon node %d log to %s", i, backupLogPath)
	}

	// Stop the node for restart (sets restarting flag to prevent errgroup failure)
	log.Infof("Stopping beacon node %d for restart", i)
	if err := oldNode.StopForRestart(); err != nil {
		return errors.Wrap(err, "failed to stop node for restart")
	}

	// Wait a moment for the process to fully terminate
	time.Sleep(2 * time.Second)

	// Execute the pre-restart hook if provided
	if preHook != nil {
		log.Infof("Executing pre-restart hook for beacon node %d", i)
		if err := preHook(s, i); err != nil {
			return errors.Wrap(err, "pre-restart hook failed")
		}
	}

	// Create a new config with extra flags
	newConfig := *s.config
	newConfig.BeaconFlags = append(slices.Clone(s.config.BeaconFlags), extraFlags...)

	// Create a new node that will preserve the data directory
	newNode := NewBeaconNodeForRestart(&newConfig, oldNode.index, oldNode.enr)
	s.nodes[i] = newNode

	// Start the new node in a goroutine
	startErrCh := make(chan error, 1)
	go func() {
		if err := newNode.Start(ctx); err != nil {
			// Only report error if context wasn't cancelled
			if ctx.Err() == nil {
				startErrCh <- err
			}
		}
	}()

	// Wait for node to start or timeout
	select {
	case <-newNode.Started():
		log.Infof("Beacon node %d restarted successfully with extra flags: %v", i, extraFlags)
		return nil
	case err := <-startErrCh:
		return errors.Wrap(err, "failed to start restarted beacon node")
	case <-time.After(2 * time.Minute):
		return errors.New("timeout waiting for restarted beacon node to start")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// BeaconNode represents beacon node.
type BeaconNode struct {
	e2etypes.ComponentRunner
	config     *e2etypes.E2EConfig
	started    chan struct{}
	index      int
	enr        string
	peerID     string
	cmd        *exec.Cmd
	isRestart  bool       // If true, don't clear DB on start
	restarting atomic.Bool // Set to true when being intentionally stopped for restart
}

// NewBeaconNode creates and returns a beacon node.
func NewBeaconNode(config *e2etypes.E2EConfig, index int, enr string) *BeaconNode {
	return &BeaconNode{
		config:    config,
		index:     index,
		enr:       enr,
		started:   make(chan struct{}, 1),
		isRestart: false,
	}
}

// NewBeaconNodeForRestart creates a beacon node configured for restart (preserves DB).
func NewBeaconNodeForRestart(config *e2etypes.E2EConfig, index int, enr string) *BeaconNode {
	return &BeaconNode{
		config:    config,
		index:     index,
		enr:       enr,
		started:   make(chan struct{}, 1),
		isRestart: true,
	}
}

func (node *BeaconNode) saveGenesis(ctx context.Context) (string, error) {
	// The deposit contract starts with an empty trie, we use the BeaconState to "pre-mine" the validator registry,
	g, err := GenerateGenesis(ctx)
	if err != nil {
		return "", err
	}

	root, err := g.HashTreeRoot(ctx)
	if err != nil {
		return "", err
	}
	lbhr, err := g.LatestBlockHeader().HashTreeRoot()
	if err != nil {
		return "", err
	}
	log.WithField("forkVersion", g.Fork().CurrentVersion).
		WithField("latestBlockHeaderRoot", fmt.Sprintf("%#x", lbhr)).
		WithField("stateRoot", fmt.Sprintf("%#x", root)).
		Infof("BeaconState info")

	genesisBytes, err := g.MarshalSSZ()
	if err != nil {
		return "", err
	}
	genesisDir := path.Join(e2e.TestParams.TestPath, fmt.Sprintf("genesis/%d", node.index))
	if err := file.MkdirAll(genesisDir); err != nil {
		return "", err
	}
	genesisPath := path.Join(genesisDir, "genesis.ssz")
	return genesisPath, file.WriteFile(genesisPath, genesisBytes)
}

func (node *BeaconNode) saveConfig() (string, error) {
	cfg := params.BeaconConfig().Copy()
	cfgBytes := params.ConfigToYaml(cfg)
	cfgDir := path.Join(e2e.TestParams.TestPath, fmt.Sprintf("config/%d", node.index))
	if err := file.MkdirAll(cfgDir); err != nil {
		return "", err
	}
	cfgPath := path.Join(cfgDir, "beacon-config.yaml")
	return cfgPath, file.WriteFile(cfgPath, cfgBytes)
}

// Start starts a fresh beacon node, connecting to all passed in beacon nodes.
func (node *BeaconNode) Start(ctx context.Context) error {
	binaryPath, found := bazel.FindBinary("cmd/beacon-chain", "beacon-chain")
	if !found {
		log.Info(binaryPath)
		return errors.New("beacon chain binary not found")
	}

	config, index, enr := node.config, node.index, node.enr
	stdOutFile, err := helpers.DeleteAndCreateFile(e2e.TestParams.LogPath, fmt.Sprintf(e2e.BeaconNodeLogFileName, index))
	if err != nil {
		return err
	}
	expectedNumOfPeers := e2e.TestParams.BeaconNodeCount + e2e.TestParams.LighthouseBeaconNodeCount - 1
	if node.config.TestSync {
		expectedNumOfPeers += 1
	}
	if node.config.TestCheckpointSync {
		expectedNumOfPeers += 1
	}
	jwtPath := path.Join(e2e.TestParams.TestPath, "eth1data/"+strconv.Itoa(node.index)+"/")
	if index == 0 {
		jwtPath = path.Join(e2e.TestParams.TestPath, "eth1data/miner/")
	}
	jwtPath = path.Join(jwtPath, "geth/jwtsecret")

	genesisPath, err := node.saveGenesis(ctx)
	if err != nil {
		return err
	}
	cfgPath, err := node.saveConfig()
	if err != nil {
		return err
	}
	args := []string{
		fmt.Sprintf("--%s=%s", genesis.StatePath.Name, genesisPath),
		fmt.Sprintf("--%s=%s/eth2-beacon-node-%d", cmdshared.DataDirFlag.Name, e2e.TestParams.TestPath, index),
		fmt.Sprintf("--%s=%s", cmdshared.LogFileName.Name, stdOutFile.Name()),
		fmt.Sprintf("--%s=%s", flags.DepositContractFlag.Name, params.BeaconConfig().DepositContractAddress),
		fmt.Sprintf("--%s=%d", flags.RPCPort.Name, e2e.TestParams.Ports.PrysmBeaconNodeRPCPort+index),
		fmt.Sprintf("--%s=http://127.0.0.1:%d", flags.ExecutionEngineEndpoint.Name, e2e.TestParams.Ports.Eth1ProxyPort+index),
		fmt.Sprintf("--%s=%s", flags.ExecutionJWTSecretFlag.Name, jwtPath),
		fmt.Sprintf("--%s=%d", flags.MinSyncPeers.Name, 1),
		fmt.Sprintf("--%s=%d", cmdshared.P2PUDPPort.Name, e2e.TestParams.Ports.PrysmBeaconNodeUDPPort+index),
		fmt.Sprintf("--%s=%d", cmdshared.P2PQUICPort.Name, e2e.TestParams.Ports.PrysmBeaconNodeQUICPort+index),
		fmt.Sprintf("--%s=%d", cmdshared.P2PTCPPort.Name, e2e.TestParams.Ports.PrysmBeaconNodeTCPPort+index),
		fmt.Sprintf("--%s=%d", cmdshared.P2PMaxPeers.Name, expectedNumOfPeers),
		fmt.Sprintf("--%s=%d", flags.MonitoringPortFlag.Name, e2e.TestParams.Ports.PrysmBeaconNodeMetricsPort+index),
		fmt.Sprintf("--%s=%d", flags.HTTPServerPort.Name, e2e.TestParams.Ports.PrysmBeaconNodeHTTPPort+index),
		fmt.Sprintf("--%s=%d", flags.ContractDeploymentBlock.Name, 0),
		fmt.Sprintf("--%s=%d", flags.MinPeersPerSubnet.Name, 0),
		fmt.Sprintf("--%s=%d", cmdshared.RPCMaxPageSizeFlag.Name, params.BeaconConfig().MinGenesisActiveValidatorCount),
		fmt.Sprintf("--%s=%s", cmdshared.BootstrapNode.Name, enr),
		fmt.Sprintf("--%s=%s", cmdshared.VerbosityFlag.Name, "debug"),
		fmt.Sprintf("--%s=%d", flags.BlockBatchLimitBurstFactor.Name, 8),
		fmt.Sprintf("--%s=%d", flags.BlobBatchLimitBurstFactor.Name, 16),
		fmt.Sprintf("--%s=%d", flags.BlobBatchLimit.Name, 256),
		fmt.Sprintf("--%s=%s", cmdshared.ChainConfigFileFlag.Name, cfgPath),
		"--" + cmdshared.ValidatorMonitorIndicesFlag.Name + "=1",
		"--" + cmdshared.ValidatorMonitorIndicesFlag.Name + "=2",
		"--" + cmdshared.AcceptTosFlag.Name,
	}
	// Only clear DB on initial start, not on restart
	if !node.isRestart {
		args = append(args, "--"+cmdshared.ForceClearDB.Name)
	}
	if config.UsePprof {
		args = append(args, "--pprof", fmt.Sprintf("--pprofport=%d", e2e.TestParams.Ports.PrysmBeaconNodePprofPort+index))
	}
	// Only add in the feature flags if we either aren't performing a control test
	// on our features or the beacon index is a multiplier of 2 (idea is to split nodes
	// equally down the line with one group having feature flags and the other without
	// feature flags; this is to allow A-B testing on new features)
	if !config.TestFeature || index != 1 {
		args = append(args, features.E2EBeaconChainFlags...)
	}
	if config.UseBuilder {
		args = append(args, fmt.Sprintf("--%s=%s:%d", flags.MevRelayEndpoint.Name, "http://127.0.0.1", e2e.TestParams.Ports.Eth1ProxyPort+index))
	}
	args = append(args, config.BeaconFlags...)

	cmd := exec.CommandContext(ctx, binaryPath, args...) // #nosec G204 -- Safe
	// Write stderr to log files.
	stderr, err := os.Create(path.Join(e2e.TestParams.LogPath, fmt.Sprintf("beacon_node_%d_stderr.log", index)))
	if err != nil {
		return err
	}
	defer func() {
		if err := stderr.Close(); err != nil {
			log.WithError(err).Error("Failed to close stderr file")
		}
	}()
	cmd.Stderr = stderr
	log.Infof("Starting beacon chain %d with flags: %s", index, strings.Join(args, " "))
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("failed to start beacon node: %w", err)
	}

	if err = helpers.WaitForTextInFile(stdOutFile, "Beacon chain gRPC server listening"); err != nil {
		return fmt.Errorf("could not find multiaddr for node %d, this means the node had issues starting: %w", index, err)
	}

	if config.UseFixedPeerIDs {
		peerId, err := helpers.FindFollowingTextInFile(stdOutFile, "Running node with peer id of ")
		if err != nil {
			return fmt.Errorf("could not find peer id: %w", err)
		}
		node.peerID = peerId
	}

	// Mark node as ready.
	close(node.started)

	node.cmd = cmd
	err = cmd.Wait()
	// If the node was intentionally stopped for restart, don't propagate the error
	// to avoid failing the errgroup
	if err != nil && node.restarting.Load() {
		log.Infof("Beacon node %d stopped for restart (not an error)", index)
		return nil
	}
	return err
}

// Started checks whether beacon node is started and ready to be queried.
func (node *BeaconNode) Started() <-chan struct{} {
	return node.started
}

// Pause pauses the component and its underlying process.
func (node *BeaconNode) Pause() error {
	return node.cmd.Process.Signal(syscall.SIGSTOP)
}

// Resume resumes the component and its underlying process.
func (node *BeaconNode) Resume() error {
	return node.cmd.Process.Signal(syscall.SIGCONT)
}

// Stop stops the component and its underlying process.
func (node *BeaconNode) Stop() error {
	return node.cmd.Process.Kill()
}

// StopForRestart stops the component for restart, setting a flag so the errgroup knows
// this is an intentional stop and shouldn't fail the test.
func (node *BeaconNode) StopForRestart() error {
	node.restarting.Store(true)
	return node.cmd.Process.Kill()
}

// IsRestarting returns true if the node was intentionally stopped for restart.
func (node *BeaconNode) IsRestarting() bool {
	return node.restarting.Load()
}

func (node *BeaconNode) UnderlyingProcess() *os.Process {
	return node.cmd.Process
}

func GenerateGenesis(ctx context.Context) (state.BeaconState, error) {
	if e2e.TestParams.Eth1GenesisBlock == nil {
		return nil, errors.New("Cannot construct bellatrix block, e2e.TestParams.Eth1GenesisBlock == nil")
	}
	gb := e2e.TestParams.Eth1GenesisBlock
	t := e2e.TestParams.CLGenesisTime
	pcreds := e2e.TestParams.NumberOfExecutionCreds
	nvals := params.BeaconConfig().MinGenesisActiveValidatorCount
	version := e2etypes.GenesisFork()
	return interop.NewPreminedGenesis(ctx, t, nvals, pcreds, version, gb)
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0644)
}

// Bucket and key names for custody info in BoltDB - must match beacon-chain/db/kv/schema.go
var (
	custodyBucket            = []byte("custody")
	groupCountKey            = []byte("group-count")
	earliestAvailableSlotKey = []byte("earliest-available-slot")
)

// SetEarliestSlotForNode directly sets the earliestAvailableSlot in a beacon node's database
// without modifying the custody group count. The node MUST be stopped before calling this function.
// This is used for testing to verify that earliestAvailableSlot never decreases.
func (s *BeaconNodeSet) SetEarliestSlotForNode(nodeIndex int, earliestSlot primitives.Slot) error {
	if nodeIndex >= len(s.nodes) {
		return errors.Errorf("node index %d out of range (max %d)", nodeIndex, len(s.nodes)-1)
	}

	// Construct the path to the beacon node's database
	// The beacon node stores its DB in a "beaconchaindata" subdirectory
	dataDir := fmt.Sprintf("%s/eth2-beacon-node-%d", e2e.TestParams.TestPath, nodeIndex)
	dbPath := path.Join(dataDir, "beaconchaindata", "beaconchain.db")

	// Open the BoltDB database
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return errors.Wrap(err, "failed to open beacon node database")
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.WithError(closeErr).Error("Failed to close beacon node database")
		}
	}()

	// Update only the earliest available slot
	if err := db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(custodyBucket)
		if err != nil {
			return errors.Wrap(err, "create custody bucket")
		}

		// Store the earliest available slot
		slotBytes := bytesutil.Uint64ToBytesBigEndian(uint64(earliestSlot))
		if err := bucket.Put(earliestAvailableSlotKey, slotBytes); err != nil {
			return errors.Wrap(err, "put earliest available slot")
		}

		return nil
	}); err != nil {
		return errors.Wrap(err, "update earliest slot in database")
	}

	log.WithFields(map[string]any{
		"nodeIndex":             nodeIndex,
		"earliestAvailableSlot": earliestSlot,
		"dbPath":                dbPath,
	}).Info("Set earliest available slot directly in beacon node database")

	return nil
}

// SetCustodyInfoForNode directly modifies the custody info in a beacon node's database.
// The node MUST be stopped before calling this function.
// This is used for testing to simulate the state where maintainCustodyInfo() has updated
// the earliestAvailableSlot to a higher value.
func (s *BeaconNodeSet) SetCustodyInfoForNode(nodeIndex int, earliestSlot primitives.Slot, custodyGroupCount uint64) error {
	if nodeIndex >= len(s.nodes) {
		return errors.Errorf("node index %d out of range (max %d)", nodeIndex, len(s.nodes)-1)
	}

	// Construct the path to the beacon node's database
	// The beacon node stores its DB in a "beaconchaindata" subdirectory
	dataDir := fmt.Sprintf("%s/eth2-beacon-node-%d", e2e.TestParams.TestPath, nodeIndex)
	dbPath := path.Join(dataDir, "beaconchaindata", "beaconchain.db")

	// Open the BoltDB database
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return errors.Wrap(err, "failed to open beacon node database")
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.WithError(closeErr).Error("Failed to close beacon node database")
		}
	}()

	// Update the custody info
	if err := db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(custodyBucket)
		if err != nil {
			return errors.Wrap(err, "create custody bucket")
		}

		// Store the earliest available slot
		slotBytes := bytesutil.Uint64ToBytesBigEndian(uint64(earliestSlot))
		if err := bucket.Put(earliestAvailableSlotKey, slotBytes); err != nil {
			return errors.Wrap(err, "put earliest available slot")
		}

		// Store the custody group count
		countBytes := bytesutil.Uint64ToBytesBigEndian(custodyGroupCount)
		if err := bucket.Put(groupCountKey, countBytes); err != nil {
			return errors.Wrap(err, "put custody group count")
		}

		return nil
	}); err != nil {
		return errors.Wrap(err, "update custody info in database")
	}

	// Verify the write by reading back the values
	var verifiedSlot, verifiedCount uint64
	if err := db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(custodyBucket)
		if bucket == nil {
			return errors.New("custody bucket not found after write")
		}

		slotBytes := bucket.Get(earliestAvailableSlotKey)
		if slotBytes != nil {
			verifiedSlot = bytesutil.BytesToUint64BigEndian(slotBytes)
		}

		countBytes := bucket.Get(groupCountKey)
		if countBytes != nil {
			verifiedCount = bytesutil.BytesToUint64BigEndian(countBytes)
		}

		return nil
	}); err != nil {
		return errors.Wrap(err, "verify custody info after write")
	}

	log.WithFields(map[string]any{
		"nodeIndex":             nodeIndex,
		"earliestAvailableSlot": earliestSlot,
		"custodyGroupCount":     custodyGroupCount,
		"verifiedSlot":          verifiedSlot,
		"verifiedCount":         verifiedCount,
		"dbPath":                dbPath,
	}).Info("Set custody info directly in beacon node database")

	if verifiedSlot != uint64(earliestSlot) || verifiedCount != custodyGroupCount {
		return errors.Errorf("verification failed: wrote slot=%d count=%d, read slot=%d count=%d",
			earliestSlot, custodyGroupCount, verifiedSlot, verifiedCount)
	}

	return nil
}
