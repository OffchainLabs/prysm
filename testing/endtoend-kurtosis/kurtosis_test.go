package endtoend_kurtosis

import (
	"context"
	"fmt"
	"testing"

	"github.com/kurtosis-tech/kurtosis/api/golang/core/lib/enclaves"
	"github.com/kurtosis-tech/kurtosis/api/golang/engine/lib/kurtosis_context"
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

func (kw *KurtosisWrapper) RunPackageWithNetworkConfig() {
	panic("TODO")
}
