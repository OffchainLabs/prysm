package prefixed_test

import (
	"fmt"
	"regexp"
	"testing"

	prefixed "github.com/OffchainLabs/prysm/v6/runtime/logging/logrus-prefixed-formatter"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type LogOutput struct {
	buffer string
}

func (o *LogOutput) Write(p []byte) (int, error) {
	o.buffer += string(p)
	return len(p), nil
}

func (o *LogOutput) GetValue() string {
	return o.buffer
}

func TestFormatter_logfmt_output(t *testing.T) {
	tests := []struct {
		name     string
		callback func(l *logrus.Logger)
		expected string
	}{
		{
			name: "should output simple message",
			callback: func(l *logrus.Logger) {
				l.Debug("test")
			},
			expected: "level=debug msg=test\n",
		},
		{
			name: "should output message with additional field",
			callback: func(l *logrus.Logger) {
				l.WithFields(logrus.Fields{"animal": "walrus"}).Debug("test")
			},
			expected: "level=debug msg=test animal=walrus\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := new(LogOutput)
			formatter := new(prefixed.TextFormatter)
			formatter.DisableTimestamp = true
			log := logrus.New()
			log.Out = output
			log.Formatter = formatter
			log.Level = logrus.DebugLevel

			tt.callback(log)
			require.Equal(t, output.GetValue(), tt.expected)
		})
	}
}

func TestFormatter_formatted_output(t *testing.T) {
	output := new(LogOutput)
	formatter := new(prefixed.TextFormatter)
	formatter.DisableTimestamp = true
	formatter.ForceFormatting = true
	log := logrus.New()
	log.Out = output
	log.Formatter = formatter
	log.Level = logrus.DebugLevel

	log.Debug("test")
	require.Equal(t, output.GetValue(), "DEBUG test\n")
}

func TestFormatter_SuppressErrorStackTraces(t *testing.T) {
	formatter := new(prefixed.TextFormatter)
	formatter.ForceFormatting = true
	log := logrus.New()
	log.Formatter = formatter
	output := new(LogOutput)
	log.Out = output

	errFn := func() error {
		return errors.New("inner")
	}

	log.WithError(errors.Wrap(errFn(), "outer")).Error("test")
	require.Equal(t, true, regexp.MustCompile(`test error=outer: inner\n\s*$`).MatchString(output.GetValue()), fmt.Sprintf("wrong log output: %s", output.GetValue()))
}

func TestFormatter_EscapesControlCharacters(t *testing.T) {
	formatter := new(prefixed.TextFormatter)
	formatter.ForceFormatting = true
	log := logrus.New()
	log.Formatter = formatter
	output := new(LogOutput)
	log.Out = output

	log.WithField("test", "foo\nbar").Error("testing things")
	require.Equal(t, "[0000] ERROR testing things test=foobar"+"\n", output.GetValue())
}
