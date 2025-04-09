package utils

import (
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"github.com/ghodss/yaml"
	jsoniter "github.com/json-iterator/go"
	"github.com/prysmaticlabs/prysm/v5/io/file"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

var json = jsoniter.Config{
	EscapeHTML:             true,
	SortMapKeys:            true,
	ValidateJsonRawMessage: true,
	TagKey:                 "spec-name",
}.Froze()

// UnmarshalYaml using a customized json encoder that supports "spec-name"
// override tag.
func UnmarshalYaml(y []byte, dest interface{}) error {
	j, err := yaml.YAMLToJSON(y)
	if err != nil {
		return err
	}
	return json.Unmarshal(j, dest)
}

// TestFolders sets the proper config and returns the result of ReadDir
// on the passed in eth2-spec-tests directory along with its path.
func TestFolders(t testing.TB, config, forkOrPhase, folderPath string) ([]os.DirEntry, string) {
	testsFolderPath := path.Join("tests", config, forkOrPhase, folderPath)
	filepath, err := bazel.Runfile(testsFolderPath)
	require.NoError(t, err)
	testFolders, err := os.ReadDir(filepath)
	require.NoError(t, err)

	if len(testFolders) == 0 {
		t.Fatalf("No test folders found at %s", testsFolderPath)
	}
	err = saveSpecTest(testsFolderPath)
	require.NoError(t, err)
	return testFolders, testsFolderPath
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
