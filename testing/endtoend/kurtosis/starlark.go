package kurtosis

import (
	"fmt"

	"github.com/kurtosis-tech/kurtosis/api/golang/core/lib/starlark_run_config"
)

// runStarlarkScript runs the given Starlark script in the enclave,
// returning an error if the script fails to run or if it returns a non-zero exit code.
func (kw *KurtosisWrapper) runStarlarkScript(action, script string) error {
	res, err := kw.enclaveCtx.RunStarlarkScriptBlocking(kw.ctx, script, starlark_run_config.NewRunStarlarkConfig())
	if err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	if res.InterpretationError != nil {
		return fmt.Errorf("%s: starlark interpretation error: %v", action, res.InterpretationError)
	}
	if len(res.ValidationErrors) > 0 {
		return fmt.Errorf("%s: starlark validation errors: %v", action, res.ValidationErrors)
	}
	if res.ExecutionError != nil {
		return fmt.Errorf("%s: starlark execution error: %v", action, res.ExecutionError)
	}
	return nil
}
