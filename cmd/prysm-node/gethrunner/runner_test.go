package gethrunner

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestRunWithConfigStartStop(t *testing.T) {
	cfg, err := ParseArgs([]string{
		"geth",
		"--network", "hoodi",
		"--datadir", t.TempDir(),
		"--authrpc.addr", "127.0.0.1",
		"--authrpc.port", "0",
		"--verbosity", "error",
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Node.P2P.ListenAddr = "127.0.0.1:0"
	cfg.Node.P2P.NoDiscovery = true
	cfg.Node.P2P.DiscoveryV4 = false
	cfg.Node.P2P.DiscoveryV5 = false
	cfg.Node.P2P.BootstrapNodes = nil
	cfg.Node.P2P.BootstrapNodesV5 = nil
	cfg.Node.P2P.NAT = nil

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		errc <- RunWithConfig(ctx, cfg)
	}()
	time.Sleep(500 * time.Millisecond)
	cancel()

	select {
	case err := <-errc:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for geth runner shutdown")
	}
}
