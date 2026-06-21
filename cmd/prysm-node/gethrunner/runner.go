package gethrunner

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/eth/catalyst"
	"github.com/ethereum/go-ethereum/eth/tracers"
	gethlog "github.com/ethereum/go-ethereum/log"
	gethmetrics "github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/metrics/exp"
	"github.com/ethereum/go-ethereum/metrics/prometheus"
	gethnode "github.com/ethereum/go-ethereum/node"
)

var enableMetricsOnce sync.Once

func Run(ctx context.Context, args []string) error {
	cfg, err := ParseArgs(args, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	return RunWithConfig(ctx, cfg)
}

func RunWithConfig(ctx context.Context, cfg Config) error {
	if err := configureLogging(cfg.Verbosity); err != nil {
		return err
	}
	var shutdownMetrics func(context.Context) error
	if cfg.Metrics.Enabled {
		shutdown, err := startMetrics(ctx, cfg.Metrics)
		if err != nil {
			return err
		}
		shutdownMetrics = shutdown
	}
	defer func() {
		if shutdownMetrics == nil {
			return
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownMetrics(shutdownCtx); err != nil {
			gethlog.Warn("Failed to stop geth metrics server", "err", err)
		}
	}()

	stack, err := gethnode.New(&cfg.Node)
	if err != nil {
		return fmt.Errorf("create geth node: %w", err)
	}
	stackClosed := false
	defer func() {
		if !stackClosed {
			_ = stack.Close()
		}
	}()

	backend, err := eth.New(stack, &cfg.Eth)
	if err != nil {
		return fmt.Errorf("register geth eth service: %w", err)
	}
	stack.RegisterAPIs(tracers.APIs(backend.APIBackend))
	if err := catalyst.Register(stack, backend); err != nil {
		return fmt.Errorf("register geth engine api: %w", err)
	}
	if err := stack.Start(); err != nil {
		return fmt.Errorf("start geth node: %w", err)
	}

	gethlog.Info("Geth node started", "network", cfg.Network, "datadir", cfg.Node.DataDir, "authrpc", stack.HTTPAuthEndpoint())
	<-ctx.Done()
	gethlog.Info("Stopping geth node")

	if err := stack.Close(); err != nil {
		return err
	}
	stackClosed = true
	return nil
}

func configureLogging(verbosity string) error {
	level, err := parseVerbosity(verbosity)
	if err != nil {
		return err
	}
	gethlog.SetDefault(gethlog.NewLogger(gethlog.NewTerminalHandlerWithLevel(os.Stderr, level, true)))
	return nil
}

func startMetrics(ctx context.Context, cfg MetricsConfig) (func(context.Context) error, error) {
	enableMetricsOnce.Do(func() {
		gethmetrics.Enable()
		go gethmetrics.CollectProcessMetrics(3 * time.Second)
	})

	addr := net.JoinHostPort(cfg.Addr, strconv.Itoa(cfg.Port))
	mux := http.NewServeMux()
	mux.Handle("/debug/metrics", exp.ExpHandler(gethmetrics.DefaultRegistry))
	mux.Handle("/debug/metrics/prometheus", prometheus.Handler(gethmetrics.DefaultRegistry))
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
		close(errc)
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			gethlog.Warn("Failed to stop geth metrics server", "err", err)
		}
	}()
	select {
	case err := <-errc:
		if err != nil {
			return nil, fmt.Errorf("start geth metrics server: %w", err)
		}
	case <-time.After(100 * time.Millisecond):
	}
	gethlog.Info("Geth metrics server started", "addr", "http://"+addr+"/debug/metrics")
	return server.Shutdown, nil
}
