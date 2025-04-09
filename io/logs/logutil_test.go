package logs

import (
	"fmt"
	"os"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

var urltests = []struct {
	url       string
	maskedUrl string
}{
	{"https://a:b@xyz.net", "https://***@xyz.net"},
	{"https://eth-holesky.alchemyapi.io/v2/tOZG5mjl3.zl_nZdZTNIBUzsDq62R_dkOtY",
		"https://eth-holesky.alchemyapi.io/***"},
	{"https://google.com/search?q=golang", "https://google.com/***"},
	{"https://user@example.com/foo%2fbar", "https://***@example.com/***"},
	{"http://john@example.com/#x/y%2Fz", "http://***@example.com/#***"},
	{"https://me:pass@example.com/foo/bar?x=1&y=2", "https://***@example.com/***"},
}

func TestMaskCredentialsLogging(t *testing.T) {
	for _, test := range urltests {
		require.Equal(t, MaskCredentialsLogging(test.url), test.maskedUrl)
	}
}

func TestConfigurePersistantLogging(t *testing.T) {
	testParentDir := t.TempDir()

	// 1. Test creation of file in an existing parent directory
	logFileName := "test.log"
	existingDirectory := "test-1-existing-testing-dir"

	err := ConfigurePersistentLogging(fmt.Sprintf("%s/%s/%s", testParentDir, existingDirectory, logFileName))
	require.NoError(t, err)

	// 2. Test creation of file along with parent directory
	nonExistingDirectory := "test-2-non-existing-testing-dir"

	err = ConfigurePersistentLogging(fmt.Sprintf("%s/%s/%s", testParentDir, nonExistingDirectory, logFileName))
	require.NoError(t, err)

	// 3. Test creation of file in an existing parent directory with a non-existing sub-directory
	existingDirectory = "test-3-existing-testing-dir"
	nonExistingSubDirectory := "test-3-non-existing-sub-dir"
	err = os.Mkdir(fmt.Sprintf("%s/%s", testParentDir, existingDirectory), 0700)
	if err != nil {
		return
	}

	err = ConfigurePersistentLogging(fmt.Sprintf("%s/%s/%s/%s", testParentDir, existingDirectory, nonExistingSubDirectory, logFileName))
	require.NoError(t, err)

	//4. Create log file in a directory without 700 permissions
	existingDirectory = "test-4-existing-testing-dir"
	err = os.Mkdir(fmt.Sprintf("%s/%s", testParentDir, existingDirectory), 0750)
	if err != nil {
		return
	}
}
