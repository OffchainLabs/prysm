package logging_test

import (
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v6/cmd/prysmctl/logging"
	prefixed "github.com/OffchainLabs/prysm/v6/runtime/logging/logrus-prefixed-formatter"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	joonix "github.com/joonix/log"
	"github.com/sirupsen/logrus"
)

type testCase struct {
	input  string
	output string
}

func TestTranslateFluentdtoUnstructuredLog(t *testing.T) {
	tests := []testCase{
		createTestCaseFluentdToText(t, &logrus.Entry{
			Data: logrus.Fields{
				"prefix": "p2p",
				"error":  "something really bad happened",
				"slot":   529,
			},
			Level:   logrus.DebugLevel,
			Message: "Failed to process something not very important",
		}),
		createTestCaseFluentdToText(t, &logrus.Entry{
			Data: logrus.Fields{
				"prefix": "core",
				"error":  "something really really bad happened",
				"slot":   530,
			},
			Level:   logrus.ErrorLevel,
			Message: "Failed to process something very important",
		}),
		createTestCaseFluentdToText(t, &logrus.Entry{
			Data: logrus.Fields{
				"prefix": "core",
				"slot":   100_000_000,
				"hash":   "0xabcdef",
			},
			Level:   logrus.InfoLevel,
			Message: "Processed something successfully",
		}),
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("scenario_%d", i), func(t *testing.T) {
			t.Logf("Input was %v", tt.input)
			got, err := logging.TranslateFluentdtoUnstructuredLog(tt.input)
			assert.NoError(t, err)
			require.Equal(t, tt.output, got, "Did not get expected output")
		})
	}
}

func createTestCaseFluentdToText(t *testing.T, e *logrus.Entry) testCase {
	return testCase{
		input:  logToString(t, fluentdFormat(t), e),
		output: logToString(t, textFormat(), e),
	}
}

type formatter interface {
	Format(entry *logrus.Entry) ([]byte, error)
}

func logToString(t *testing.T, f formatter, e *logrus.Entry) string {
	b, err := f.Format(e)
	require.NoError(t, err)
	return string(b)
}

func fluentdFormat(t *testing.T) formatter {
	f := joonix.NewFormatter()

	require.NoError(t, joonix.DisableTimestampFormat(f))
	return f

}

func textFormat() formatter {
	formatter := new(prefixed.TextFormatter)
	formatter.DisableTimestamp = true // Don't include timestamp since we don't have it
	formatter.DisableColors = false

	return formatter
}
