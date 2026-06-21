package gethrunner

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"

	prysmversion "github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	gethlog "github.com/ethereum/go-ethereum/log"
	gethmetrics "github.com/ethereum/go-ethereum/metrics"
	gethnode "github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/params"
)

const (
	defaultNetwork = "mainnet"
	defaultHTTPAPI = "eth,net,web3"
)

type Config struct {
	Network   string
	Node      gethnode.Config
	Eth       ethconfig.Config
	Metrics   MetricsConfig
	Verbosity string
}

type MetricsConfig struct {
	Enabled bool
	Addr    string
	Port    int
}

type flagValues struct {
	network     string
	datadir     string
	datadirSet  bool
	authRPCAddr string
	authRPCPort int
	authRPCJWT  string
	http        bool
	httpAddr    string
	httpPort    int
	httpAPI     string
	metrics     bool
	metricsAddr string
	metricsPort int
	verbosity   string
}

func ParseArgs(args []string, output io.Writer) (Config, error) {
	if len(args) == 0 {
		args = []string{"geth"}
	}
	values := defaultFlagValues()
	fs := newFlagSet(args, &values, output)
	if err := fs.Parse(args[1:]); err != nil {
		return Config{}, err
	}
	if fs.NArg() > 0 {
		return Config{}, fmt.Errorf("geth does not accept positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	return values.config()
}

func defaultFlagValues() flagValues {
	nodeCfg := gethnode.DefaultConfig
	return flagValues{
		network:     defaultNetwork,
		datadir:     nodeCfg.DataDir,
		authRPCAddr: nodeCfg.AuthAddr,
		authRPCPort: nodeCfg.AuthPort,
		httpAddr:    "127.0.0.1",
		httpPort:    nodeCfg.HTTPPort,
		httpAPI:     defaultHTTPAPI,
		metricsAddr: gethmetrics.DefaultConfig.HTTP,
		metricsPort: gethmetrics.DefaultConfig.Port,
		verbosity:   "3",
	}
}

func newFlagSet(args []string, values *flagValues, output io.Writer) *flag.FlagSet {
	name := "geth"
	if len(args) > 0 && args[0] != "" {
		name = args[0]
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	if output != nil {
		fs.SetOutput(output)
	}
	fs.StringVar(&values.network, "network", values.network, "Ethereum network: mainnet, sepolia, holesky, or hoodi")
	fs.Func("datadir", "Data directory for Geth databases and keystore", func(value string) error {
		values.datadir = value
		values.datadirSet = true
		return nil
	})
	fs.StringVar(&values.authRPCAddr, "authrpc.addr", values.authRPCAddr, "Authenticated Engine API listen address")
	fs.IntVar(&values.authRPCPort, "authrpc.port", values.authRPCPort, "Authenticated Engine API listen port")
	fs.StringVar(&values.authRPCJWT, "authrpc.jwtsecret", values.authRPCJWT, "Path to a hex-encoded JWT secret for the Engine API")
	fs.BoolVar(&values.http, "http", values.http, "Enable public HTTP JSON-RPC")
	fs.StringVar(&values.httpAddr, "http.addr", values.httpAddr, "Public HTTP JSON-RPC listen address")
	fs.IntVar(&values.httpPort, "http.port", values.httpPort, "Public HTTP JSON-RPC listen port")
	fs.StringVar(&values.httpAPI, "http.api", values.httpAPI, "Comma-separated public HTTP JSON-RPC APIs")
	fs.BoolVar(&values.metrics, "metrics", values.metrics, "Enable Geth metrics collection and reporting")
	fs.StringVar(&values.metricsAddr, "metrics.addr", values.metricsAddr, "Metrics HTTP listen address")
	fs.IntVar(&values.metricsPort, "metrics.port", values.metricsPort, "Metrics HTTP listen port")
	fs.StringVar(&values.verbosity, "verbosity", values.verbosity, "Geth log verbosity: 0-5 or crit,error,warn,info,debug,trace")
	return fs
}

func (values flagValues) config() (Config, error) {
	nodeCfg := gethnode.DefaultConfig
	nodeCfg.Name = "geth"
	nodeCfg.Version = prysmversion.Version()
	nodeCfg.DataDir = values.datadir
	nodeCfg.AuthAddr = values.authRPCAddr
	nodeCfg.AuthPort = values.authRPCPort
	nodeCfg.JWTSecret = values.authRPCJWT
	nodeCfg.HTTPModules = splitCSV(values.httpAPI)
	if values.http {
		nodeCfg.HTTPHost = values.httpAddr
		nodeCfg.HTTPPort = values.httpPort
	}

	ethCfg := ethconfig.Defaults
	if err := applyNetwork(values.network, values.datadirSet, &nodeCfg, &ethCfg); err != nil {
		return Config{}, err
	}
	if _, err := parseVerbosity(values.verbosity); err != nil {
		return Config{}, err
	}

	return Config{
		Network: values.network,
		Node:    nodeCfg,
		Eth:     ethCfg,
		Metrics: MetricsConfig{
			Enabled: values.metrics,
			Addr:    values.metricsAddr,
			Port:    values.metricsPort,
		},
		Verbosity: values.verbosity,
	}, nil
}

func applyNetwork(network string, datadirSet bool, nodeCfg *gethnode.Config, ethCfg *ethconfig.Config) error {
	switch network {
	case "mainnet":
		ethCfg.NetworkId = 1
		ethCfg.Genesis = core.DefaultGenesisBlock()
		setDNSDiscoveryDefaults(ethCfg, params.MainnetGenesisHash)
	case "sepolia":
		ethCfg.NetworkId = 11155111
		ethCfg.Genesis = core.DefaultSepoliaGenesisBlock()
		setDNSDiscoveryDefaults(ethCfg, params.SepoliaGenesisHash)
		setDefaultTestnetDataDir(datadirSet, nodeCfg, "sepolia")
	case "holesky":
		ethCfg.NetworkId = 17000
		ethCfg.Genesis = core.DefaultHoleskyGenesisBlock()
		setDNSDiscoveryDefaults(ethCfg, params.HoleskyGenesisHash)
		setDefaultTestnetDataDir(datadirSet, nodeCfg, "holesky")
	case "hoodi":
		ethCfg.NetworkId = 560048
		ethCfg.Genesis = core.DefaultHoodiGenesisBlock()
		setDNSDiscoveryDefaults(ethCfg, params.HoodiGenesisHash)
		setDefaultTestnetDataDir(datadirSet, nodeCfg, "hoodi")
	default:
		return fmt.Errorf("unsupported geth network %q (want mainnet, sepolia, holesky, or hoodi)", network)
	}
	return nil
}

func setDNSDiscoveryDefaults(cfg *ethconfig.Config, genesis common.Hash) {
	if url := params.KnownDNSNetwork(genesis, "all"); url != "" {
		cfg.EthDiscoveryURLs = []string{url}
		cfg.SnapDiscoveryURLs = cfg.EthDiscoveryURLs
	}
}

func setDefaultTestnetDataDir(datadirSet bool, cfg *gethnode.Config, network string) {
	if !datadirSet && cfg.DataDir == gethnode.DefaultDataDir() {
		cfg.DataDir = filepath.Join(gethnode.DefaultDataDir(), network)
	}
}

func splitCSV(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func parseVerbosity(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "crit", "critical":
		return gethlog.FromLegacyLevel(0), nil
	case "error":
		return gethlog.FromLegacyLevel(1), nil
	case "warn", "warning":
		return gethlog.FromLegacyLevel(2), nil
	case "info":
		return gethlog.FromLegacyLevel(3), nil
	case "debug":
		return gethlog.FromLegacyLevel(4), nil
	case "trace":
		return gethlog.FromLegacyLevel(5), nil
	default:
		level, err := strconv.Atoi(value)
		if err != nil {
			return 0, fmt.Errorf("invalid verbosity %q", value)
		}
		return gethlog.FromLegacyLevel(level), nil
	}
}
