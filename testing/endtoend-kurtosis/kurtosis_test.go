package endtoend_kurtosis

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"github.com/kurtosis-tech/kurtosis/api/golang/core/lib/enclaves"
	"github.com/kurtosis-tech/kurtosis/api/golang/core/lib/starlark_run_config"
	"github.com/kurtosis-tech/kurtosis/api/golang/engine/lib/kurtosis_context"
	"gopkg.in/yaml.v3"
)

type KurtosisWrapper struct {
	t           *testing.T
	ctx         context.Context
	kurtosisCtx *kurtosis_context.KurtosisContext
	enclaves    map[string]*enclaves.EnclaveContext
}

func NewKurtosisWrapper(t *testing.T, ctx context.Context) (*KurtosisWrapper, error) {
	kurtosisCtx, err := kurtosis_context.NewKurtosisContextFromLocalEngine()
	if err != nil {
		return nil, fmt.Errorf("get kurtosis context from local engine: %w", err)
	}

	return &KurtosisWrapper{
		t:           t,
		ctx:         ctx,
		kurtosisCtx: kurtosisCtx,
		enclaves:    make(map[string]*enclaves.EnclaveContext),
	}, nil
}

func (kw *KurtosisWrapper) CreateEnclave(enclaveName string) error {
	// Before creation, destroy any existing enclave with the same name
	// for idempotency
	enclavesInfo, err := kw.kurtosisCtx.GetEnclaves(kw.ctx)
	if err != nil {
		return fmt.Errorf("failed to check for pre-existing Kurtosis enclaves: %s: %w", enclaveName, err)
	}
	enclaveInfoMap := enclavesInfo.GetEnclavesByName()
	_, exists := enclaveInfoMap[enclaveName]
	if exists {
		kw.t.Logf("Enclave with name '%s' already exists; destroying it for idempotency", enclaveName)
		if err := kw.kurtosisCtx.DestroyEnclave(kw.ctx, enclaveName); err != nil {
			return fmt.Errorf("failed to destroy pre-existing Kurtosis enclave: %s: %w", enclaveName, err)
		}
	}

	enclaveCtx, err := kw.kurtosisCtx.CreateEnclave(kw.ctx, enclaveName)
	if err != nil {
		return fmt.Errorf("failed to create Kurtosis enclave: %s: %w", enclaveName, err)
	}

	kw.enclaves[enclaveName] = enclaveCtx
	return nil
}

func (kw *KurtosisWrapper) DestroyEnclave(enclaveName string) error {
	_, exists := kw.enclaves[enclaveName]
	if !exists {
		return fmt.Errorf("enclave not found in wrapper: %s", enclaveName)
	}

	err := kw.kurtosisCtx.DestroyEnclave(context.Background(), enclaveName)
	if err != nil {
		return fmt.Errorf("failed to destroy Kurtosis enclave: %s: %w", enclaveName, err)
	}

	delete(kw.enclaves, enclaveName)
	return nil
}

func (kw *KurtosisWrapper) RunPackageWithNetworkConfig(enclaveName string, packageId string, networkConfigPath string) error {
	enclaveCtx, exists := kw.enclaves[enclaveName]
	if !exists {
		return fmt.Errorf("enclave not found in wrapper: %s", enclaveName)
	}

	jsonParams, err := kw.readYamlConfigAsJson(networkConfigPath)
	if err != nil {
		return fmt.Errorf("failed to process config file: %w", err)
	}
	kw.t.Logf("Running package '%s' with params: %s", packageId, jsonParams)

	runConfig := starlark_run_config.NewRunStarlarkConfig(
		starlark_run_config.WithSerializedParams(jsonParams),
	)

	runResult, err := enclaveCtx.RunStarlarkRemotePackageBlocking(kw.ctx, packageId, runConfig)
	if err != nil {
		return fmt.Errorf("failed to run remote package: %w", err)
	}

	if runResult.InterpretationError != nil {
		return fmt.Errorf("starlark interpretation error: %v", runResult.InterpretationError)
	}

	if len(runResult.ValidationErrors) > 0 {
		return fmt.Errorf("starlark validation errors: %v", runResult.ValidationErrors)
	}

	kw.t.Log("Starlark package executed successfully")
	return nil
}

func (kw *KurtosisWrapper) readYamlConfigAsJson(networkConfigPath string) (string, error) {
	realPath, err := bazel.Runfile(networkConfigPath)
	if err != nil {
		return "", fmt.Errorf("failed to find runfile '%s': %w", networkConfigPath, err)
	}

	yamlData, err := os.ReadFile(realPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	var body any
	if err := yaml.Unmarshal(yamlData, &body); err != nil {
		return "", fmt.Errorf("failed to unmarshal yaml: %w", err)
	}

	jsonData, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to marshal to json: %w", err)
	}

	return string(jsonData), nil
}
