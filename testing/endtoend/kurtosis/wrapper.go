package kurtosis

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/endtoend/helpers"
	"github.com/kurtosis-tech/kurtosis/api/golang/core/lib/enclaves"
	"github.com/kurtosis-tech/kurtosis/api/golang/core/lib/services"
	"github.com/kurtosis-tech/kurtosis/api/golang/core/lib/starlark_run_config"
	"github.com/kurtosis-tech/kurtosis/api/golang/engine/lib/kurtosis_context"
	"google.golang.org/grpc"
)

// KurtosisWrapper drives a local Kurtosis engine for Kurtosis-backed E2E tests.
// It manages enclave lifecycle (creation, destruction).
type KurtosisWrapper struct {
	t           *testing.T
	ctx         context.Context
	kurtosisCtx *kurtosis_context.KurtosisContext
	enclaveName string
	enclaveCtx  *enclaves.EnclaveContext
}

func NewKurtosisWrapper(t *testing.T, ctx context.Context, name string) (*KurtosisWrapper, error) {
	kurtosisCtx, err := kurtosis_context.NewKurtosisContextFromLocalEngine()
	if err != nil {
		return nil, fmt.Errorf("get kurtosis context from local engine: %w", err)
	}

	return &KurtosisWrapper{
		t:           t,
		ctx:         ctx,
		kurtosisCtx: kurtosisCtx,
		enclaveName: name,
	}, nil
}

// CreateEnclave creates a new enclave with the wrapper's enclave name.
// Before creation, destroy any existing enclave with the same name
// for idempotency
func (kw *KurtosisWrapper) CreateEnclave() error {
	enclavesInfo, err := kw.kurtosisCtx.GetEnclaves(kw.ctx)
	if err != nil {
		return fmt.Errorf("failed to check for pre-existing Kurtosis enclaves: %s: %w", kw.enclaveName, err)
	}
	enclaveInfoMap := enclavesInfo.GetEnclavesByName()
	if _, exists := enclaveInfoMap[kw.enclaveName]; exists {
		kw.t.Logf("Enclave with name '%s' already exists; destroying it for idempotency", kw.enclaveName)
		if err := kw.DestroyEnclave(); err != nil {
			return fmt.Errorf("failed to destroy pre-existing Kurtosis enclave: %s: %w", kw.enclaveName, err)
		}
	}

	enclaveCtx, err := kw.kurtosisCtx.CreateEnclave(kw.ctx, kw.enclaveName)
	if err != nil {
		return fmt.Errorf("failed to create Kurtosis enclave: %s: %w", kw.enclaveName, err)
	}

	kw.enclaveCtx = enclaveCtx

	return nil
}

// DestroyEnclave destroys the enclave and reset enclave context and name.
func (kw *KurtosisWrapper) DestroyEnclave() error {
	err := kw.kurtosisCtx.DestroyEnclave(kw.ctx, kw.enclaveName)
	if err != nil {
		return fmt.Errorf("failed to destroy Kurtosis enclave: %s: %w", kw.enclaveName, err)
	}

	kw.enclaveCtx = nil
	return nil
}

// RunPackageWithNetworkConfig runs a Starlark package (mostly ethereum-package) with the given ID
// in the current enclave using the provided network config YAML file (networkConfigPath).
func (kw *KurtosisWrapper) RunPackageWithNetworkConfig(packageId string, networkConfigPath string) error {
	if kw.enclaveCtx == nil {
		return fmt.Errorf("enclave context is nil")
	}

	jsonParams, err := readYamlConfigAsJson(networkConfigPath)
	if err != nil {
		return fmt.Errorf("failed to process config file: %w", err)
	}

	kw.t.Logf("Running package '%s' with params: %s", packageId, jsonParams)

	runConfig := starlark_run_config.NewRunStarlarkConfig(
		starlark_run_config.WithSerializedParams(jsonParams),
	)

	runResult, err := kw.enclaveCtx.RunStarlarkRemotePackageBlocking(kw.ctx, packageId, runConfig)
	if err != nil {
		return fmt.Errorf("failed to run remote package: %w", err)
	}

	if runResult.InterpretationError != nil {
		return fmt.Errorf("starlark interpretation error: %v", runResult.InterpretationError)
	}

	if len(runResult.ValidationErrors) > 0 {
		return fmt.Errorf("starlark validation errors: %v", runResult.ValidationErrors)
	}

	kw.t.Logf("Starlark package executed successfully in enclave '%s'", kw.enclaveName)
	return nil
}

// StartService starts a previously-stopped service (e.g. a skip_start beacon
// node) by running a one-line Starlark script in the enclave.
func (kw *KurtosisWrapper) StartService(name string) error {
	if kw.enclaveCtx == nil {
		return fmt.Errorf("enclave context is nil")
	}
	script := fmt.Sprintf("def run(plan):\n    plan.start_service(%q)\n", name)
	if _, err := kw.enclaveCtx.RunStarlarkScriptBlocking(kw.ctx, script, starlark_run_config.NewRunStarlarkConfig()); err != nil {
		return fmt.Errorf("start service %q: %w", name, err)
	}
	return nil
}

// prysmCLServices returns all Prysm beacon (CL) service contexts in the enclave
// keyed by name, plus their names sorted ("cl-<i>-prysm-<el>").
func (kw *KurtosisWrapper) prysmCLServices() (map[services.ServiceName]*services.ServiceContext, []string, error) {
	// Empty map means "all services" in GetServiceContexts.
	all, err := kw.enclaveCtx.GetServiceContexts(map[string]bool{})
	if err != nil {
		return nil, nil, fmt.Errorf("list services: %w", err)
	}

	// Prysm beacon nodes are the CL services: "cl-<i>-prysm-<el>".
	var names []string
	for name, sc := range all {
		n := string(name)
		if strings.HasPrefix(n, "cl-") && strings.Contains(n, "prysm") {
			if isStoppedService(sc) {
				continue
			}
			names = append(names, n)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, nil, fmt.Errorf("no prysm CL beacon services found in enclave %q", kw.enclaveName)
	}
	return all, names, nil
}

// StoppedPrysmCLName returns the name of the stopped Prysm beacon (CL) service in the enclave, which is the skip_start sync node.
func (kw *KurtosisWrapper) StoppedPrysmCLName() ([]string, error) {
	all, err := kw.enclaveCtx.GetServiceContexts(map[string]bool{})
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	var stopped []string
	for name, sc := range all {
		n := string(name)
		if strings.HasPrefix(n, "cl-") && strings.Contains(n, "prysm") && isStoppedService(sc) {
			stopped = append(stopped, n)
		}
	}

	// NOTE: One for sync node, other for checkpoint sync node.
	if len(stopped) > 2 {
		return nil, fmt.Errorf("expected at most two stopped prysm CL nodes, found %v", stopped)
	}
	return stopped, nil
}

// NewGRPCConnections discovers the published gRPC ("rpc") port of each
// Prysm beacon node in the enclave and dials it.
func (kw *KurtosisWrapper) NewGRPCConnections() ([]*grpc.ClientConn, func(), error) {
	all, names, err := kw.prysmCLServices()
	if err != nil {
		return nil, nil, err
	}

	conns := make([]*grpc.ClientConn, 0, len(names))
	for _, n := range names {
		// Published gRPC port is the "rpc" port in the service's public ports.
		rpcPort, ok := all[services.ServiceName(n)].GetPublicPorts()["rpc"]
		if !ok {
			return nil, nil, fmt.Errorf("service %s has no published rpc port", n)
		}
		conn, err := helpers.NewLocalConnection(kw.ctx, int(rpcPort.GetNumber())) // lint:ignore uintcast -- a uint16 port never exceeds int.
		if err != nil {
			return nil, nil, fmt.Errorf("dial gRPC for %s: %w", n, err)
		}
		conns = append(conns, conn)
	}
	return conns, func() {
		for _, c := range conns {
			_ = c.Close()
		}
	}, nil
}

// NewBeaconRESTEndpoints discovers the published Beacon REST ("http") port of
// each Prysm beacon node and returns base URLs like "http://127.0.0.1:<port>".
func (kw *KurtosisWrapper) NewBeaconRESTEndpoints() ([]string, error) {
	all, names, err := kw.prysmCLServices()
	if err != nil {
		return nil, err
	}

	urls := make([]string, 0, len(names))
	for _, n := range names {
		httpPort, ok := all[services.ServiceName(n)].GetPublicPorts()["http"]
		if !ok {
			return nil, fmt.Errorf("service %s has no published http port", n)
		}
		urls = append(urls, fmt.Sprintf("http://127.0.0.1:%d", httpPort.GetNumber())) // lint:ignore uintcast -- a uint16 port never exceeds int.
	}
	return urls, nil
}

// NewAssertoorEndpoint discovers the published HTTP port of the "assertoor"
// service and returns its base URL like "http://127.0.0.1:<port>".
func (kw *KurtosisWrapper) NewAssertoorEndpoint() (string, error) {
	svc, err := kw.enclaveCtx.GetServiceContext("assertoor")
	if err != nil {
		return "", fmt.Errorf("get assertoor service: %w", err)
	}
	httpPort, ok := svc.GetPublicPorts()["http"]
	if !ok {
		return "", fmt.Errorf("assertoor service has no published http port")
	}
	return fmt.Sprintf("http://127.0.0.1:%d", httpPort.GetNumber()), nil // lint:ignore uintcast -- a uint16 port never exceeds int.
}

// isStoppedService returns true if the service has no published ports, which
// is how we identify the skip_start sync node in the enclave.
func isStoppedService(sc *services.ServiceContext) bool {
	return len(sc.GetPublicPorts()) == 0
}
