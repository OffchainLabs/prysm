package gethrunner

import (
	"errors"
	"flag"
	"io"
	"path/filepath"
	"strings"
	"testing"

	gethnode "github.com/ethereum/go-ethereum/node"
)

func TestParseArgsDefaults(t *testing.T) {
	cfg, err := ParseArgs([]string{"geth"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Network != "mainnet" {
		t.Fatalf("network = %q, want mainnet", cfg.Network)
	}
	if cfg.Eth.NetworkId != 1 {
		t.Fatalf("network ID = %d, want 1", cfg.Eth.NetworkId)
	}
	if cfg.Node.HTTPHost != "" {
		t.Fatalf("HTTPHost = %q, want disabled", cfg.Node.HTTPHost)
	}
	if strings.Join(cfg.Node.HTTPModules, ",") != defaultHTTPAPI {
		t.Fatalf("HTTP modules = %v, want %s", cfg.Node.HTTPModules, defaultHTTPAPI)
	}
	if cfg.Node.AuthPort != 8551 {
		t.Fatalf("auth port = %d, want 8551", cfg.Node.AuthPort)
	}
}

func TestParseArgsCuratedFlags(t *testing.T) {
	cfg, err := ParseArgs([]string{
		"geth",
		"--network", "sepolia",
		"--datadir", "/tmp/prysm-geth",
		"--authrpc.addr", "127.0.0.1",
		"--authrpc.port", "9551",
		"--authrpc.jwtsecret", "/tmp/jwt.hex",
		"--http",
		"--http.addr", "0.0.0.0",
		"--http.port", "9545",
		"--http.api", "eth,net",
		"--metrics",
		"--metrics.addr", "127.0.0.2",
		"--metrics.port", "7070",
		"--verbosity", "debug",
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Eth.NetworkId != 11155111 {
		t.Fatalf("network ID = %d, want 11155111", cfg.Eth.NetworkId)
	}
	if cfg.Node.DataDir != "/tmp/prysm-geth" {
		t.Fatalf("datadir = %q", cfg.Node.DataDir)
	}
	if cfg.Node.AuthAddr != "127.0.0.1" || cfg.Node.AuthPort != 9551 || cfg.Node.JWTSecret != "/tmp/jwt.hex" {
		t.Fatalf("auth config = %s:%d %q", cfg.Node.AuthAddr, cfg.Node.AuthPort, cfg.Node.JWTSecret)
	}
	if cfg.Node.HTTPHost != "0.0.0.0" || cfg.Node.HTTPPort != 9545 {
		t.Fatalf("http config = %s:%d", cfg.Node.HTTPHost, cfg.Node.HTTPPort)
	}
	if strings.Join(cfg.Node.HTTPModules, ",") != "eth,net" {
		t.Fatalf("HTTP modules = %v", cfg.Node.HTTPModules)
	}
	if !cfg.Metrics.Enabled || cfg.Metrics.Addr != "127.0.0.2" || cfg.Metrics.Port != 7070 {
		t.Fatalf("metrics config = %+v", cfg.Metrics)
	}
}

func TestParseArgsDefaultTestnetDataDir(t *testing.T) {
	cfg, err := ParseArgs([]string{"geth", "--network", "holesky"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(gethnode.DefaultDataDir(), "holesky")
	if cfg.Node.DataDir != want {
		t.Fatalf("datadir = %q, want %q", cfg.Node.DataDir, want)
	}
}

func TestParseArgsInvalidNetwork(t *testing.T) {
	_, err := ParseArgs([]string{"geth", "--network", "badnet"}, io.Discard)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported geth network") {
		t.Fatalf("error = %q", err)
	}
}

func TestParseArgsHelp(t *testing.T) {
	_, err := ParseArgs([]string{"geth", "--help"}, io.Discard)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("error = %v, want flag.ErrHelp", err)
	}
}
