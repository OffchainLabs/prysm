package utils

import (
	"errors"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v7/build/bazel"
	"github.com/OffchainLabs/prysm/v7/io/file"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ghodss/yaml"
	jsoniter "github.com/json-iterator/go"
)

// realOnlyCases need real verification to reach their post state but carry no
// bls_setting to say so. Gating the case rather than the handler keeps its
// sibling cases covered; keep the list minimal.
var realOnlyCases = map[string]bool{
	"fork_invalid_builder_deposit_followed_by_valid_builder_deposit": true,
	"fork_invalid_validator_deposit_followed_by_builder_credentials": true,
	"process_payload_attestation_invalid_signature":                  true,
}

// unmarshalLimitCases are vectors whose only defect is a list longer than its
// spec maximum. Gloas moved those maximums out of the SSZ type definitions and
// into the state transition, but prysm still enforces them in UnmarshalSSZ, so
// the vector fails to decode and never reaches the transition the runner is
// exercising. The transition-level checks are covered by unit tests on
// VerifyOperationLengths, validateExecutionRequestLengths and
// IsValidAttestationIndices.
var unmarshalLimitCases = map[string]bool{
	"invalid_too_many_attestations":             true,
	"invalid_too_many_attester_slashings":       true,
	"invalid_too_many_attesting_indices":        true,
	"invalid_too_many_bls_to_execution_changes": true,
	"invalid_too_many_builder_deposit_requests": true,
	"invalid_too_many_builder_exit_requests":    true,
	"invalid_too_many_consolidation_requests":   true,
	"invalid_too_many_payload_attestations":     true,
	"invalid_too_many_proposer_slashings":       true,
	"invalid_too_many_voluntary_exits":          true,
	"invalid_too_many_withdrawal_requests":      true,
}

// bls_setting values used by the consensus spec test vectors.
const (
	blsSettingOptional = 0 // Either backend.
	blsSettingRequired = 1 // Real backend only.
	blsSettingStubbed  = 2 // Fake backend only.
)

var json = jsoniter.Config{
	EscapeHTML:             true,
	SortMapKeys:            true,
	ValidateJsonRawMessage: true,
	TagKey:                 "spec-name",
}.Froze()

// UnmarshalYaml using a customized json encoder that supports "spec-name"
// override tag.
func UnmarshalYaml(y []byte, dest any) error {
	j, err := yaml.YAMLToJSON(y)
	if err != nil {
		return err
	}
	return json.Unmarshal(j, dest)
}

// TestFolders sets the proper config and returns the result of ReadDir
// on the passed in eth2-spec-tests directory along with its path.
//
// Cases whose bls_setting does not match the compiled-in backend are dropped, and a handler
// left with none is skipped rather than passed.
func TestFolders(t testing.TB, config, forkOrPhase, folderPath string) ([]os.DirEntry, string) {
	testsFolderPath := path.Join("tests", config, forkOrPhase, folderPath)
	filepath, err := bazel.Runfile(testsFolderPath)
	require.NoError(t, err)
	testFolders, err := os.ReadDir(filepath)
	require.NoError(t, err)

	if len(testFolders) == 0 {
		t.Fatalf("No test folders found at %s", testsFolderPath)
	}

	// Collect only "runnable" cases by filtering out
	// those whose bls_setting does not match the compiled-in backend.
	cases, runnableCases := 0, 0
	runnable := make([]os.DirEntry, 0, len(testFolders))
	for _, folder := range testFolders {
		if !folder.IsDir() {
			runnable = append(runnable, folder)
			continue
		}
		cases++
		if FakeCrypto && realOnlyCases[folder.Name()] {
			continue
		}
		if unmarshalLimitCases[folder.Name()] {
			t.Logf("Skipping %s: list limit is enforced at unmarshal time", folder.Name())
			continue
		}
		ok, err := blsSettingRunnable(path.Join(filepath, folder.Name()))
		require.NoError(t, err)
		if ok {
			runnableCases++
			runnable = append(runnable, folder)
		}
	}
	if cases > 0 && runnableCases == 0 {
		t.Skipf("None of the %d cases at %s suit this backend (fake_crypto=%v)", cases, testsFolderPath, FakeCrypto)
	}

	require.NoError(t, saveSpecTest(testsFolderPath))
	return runnable, testsFolderPath
}

func saveSpecTest(testFolder string) error {
	baseDir := os.Getenv("SPEC_TEST_REPORT_OUTPUT_DIR")
	if baseDir == "" {
		return nil // Do nothing if spec test report not requested.
	}
	fullPath := path.Join(baseDir, fmt.Sprintf("%x_tests.txt", testFolder))
	err := file.WriteFile(fullPath, []byte(testFolder))
	if err != nil {
		return err
	}
	return nil
}

// blsSettingRunnable reports whether a case directory suits the compiled-in
// backend. No meta.yaml means run: some handlers enumerate intermediate dirs.
func blsSettingRunnable(caseDir string) (bool, error) {
	metaYaml, err := os.ReadFile(path.Join(caseDir, "meta.yaml")) // #nosec G304 -- test-only path built from the spec test directory
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading %s/meta.yaml: %w", caseDir, err)
	}
	meta := &struct {
		BLSSetting int `json:"bls_setting"`
	}{}
	if err := UnmarshalYaml(metaYaml, meta); err != nil {
		return false, fmt.Errorf("unmarshalling %s/meta.yaml: %w", caseDir, err)
	}

	switch meta.BLSSetting {
	// Optional: run on either backend. Return true regardless of FakeCrypto.
	case blsSettingOptional:
		return true, nil
	// Required: run only on real backend.
	case blsSettingRequired:
		return !FakeCrypto, nil
	// Stubbed: run only on fake backend.
	case blsSettingStubbed:
		return FakeCrypto, nil
	// Unknown: loudly fail the test.
	default:
		return false, fmt.Errorf("unknown bls_setting %d in %s/meta.yaml", meta.BLSSetting, caseDir)
	}
}
