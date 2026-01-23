// Package logs creates a Multi writer instance that
// write all logs that are written to stdout.
package logs

import (
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/io/file"
	prefixed "github.com/OffchainLabs/prysm/v7/runtime/logging/logrus-prefixed-formatter"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

var ephemeralLogFileVerbosity = logrus.DebugLevel

// SetLoggingLevel sets the base logging level for logrus.
func SetLoggingLevel(lvl logrus.Level) {
	logrus.SetLevel(max(lvl, ephemeralLogFileVerbosity))
}

func addLogWriter(w io.Writer) {
	mw := io.MultiWriter(logrus.StandardLogger().Out, w)
	logrus.SetOutput(mw)
}

// ConfigurePersistentLogging adds a log-to-file writer. File content is identical to stdout.
func ConfigurePersistentLogging(logFileName string, format string, lvl logrus.Level, vmodule map[string]logrus.Level) error {
	logrus.WithField("logFileName", logFileName).Info("Logs will be made persistent")
	if err := file.MkdirAll(filepath.Dir(logFileName)); err != nil {
		return err
	}
	f, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, params.BeaconIoConfig().ReadWritePermissions) // #nosec G304
	if err != nil {
		return err
	}

	if format != "text" {
		addLogWriter(f)

		logrus.Info("File logging initialized")
		return nil
	}

	maxVmoduleLevel := logrus.PanicLevel
	for _, level := range vmodule {
		if level > maxVmoduleLevel {
			maxVmoduleLevel = level
		}
	}

	// Create formatter and writer hook
	formatter := new(prefixed.TextFormatter)
	formatter.TimestampFormat = "2006-01-02 15:04:05.00"
	formatter.FullTimestamp = true
	// If persistent log files are written - we disable the log messages coloring because
	// the colors are ANSI codes and seen as gibberish in the log files.
	formatter.DisableColors = true
	formatter.BaseVerbosity = lvl
	formatter.VModule = vmodule

	logrus.AddHook(&WriterHook{
		Formatter:     formatter,
		Writer:        f,
		AllowedLevels: logrus.AllLevels[:max(lvl, maxVmoduleLevel)+1],
	})

	logrus.Info("File logging initialized")
	return nil
}

// ConfigureEphemeralLogFile adds a log file that keeps 24 hours of logs with >debug verbosity.
func ConfigureEphemeralLogFile(datadirPath string, app string) error {
	logFilePath := filepath.Join(datadirPath, "logs", app+".log")
	if err := file.MkdirAll(filepath.Dir(logFilePath)); err != nil {
		return errors.Wrap(err, "failed to create directory")
	}

	// Create formatter and writer hook
	formatter := new(prefixed.TextFormatter)
	formatter.TimestampFormat = "2006-01-02 15:04:05.00"
	formatter.FullTimestamp = true
	// If persistent log files are written - we disable the log messages coloring because
	// the colors are ANSI codes and seen as gibberish in the log files.
	formatter.DisableColors = true

	// configure the lumberjack log writer to rotate logs every ~24 hours
	debugWriter := &lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    250, // MB, to avoid unbounded growth
		MaxBackups: 1,   // one backup in case of size-based rotations
		MaxAge:     1,   // days; files older than this are removed
	}

	logrus.AddHook(&WriterHook{
		Formatter:     formatter,
		Writer:        debugWriter,
		AllowedLevels: logrus.AllLevels[:ephemeralLogFileVerbosity+1],
	})

	logrus.Info("Ephemeral log file initialized")
	return nil
}

// MaskCredentialsLogging masks the url credentials before logging for security purpose
// [scheme:][//[userinfo@]host][/]path[?query][#fragment] -->  [scheme:][//[***]host][/***][#***]
// if the format is not matched nothing is done, string is returned as is.
func MaskCredentialsLogging(currUrl string) string {
	// error if the input is not a URL
	MaskedUrl := currUrl
	u, err := url.Parse(currUrl)
	if err != nil {
		return currUrl // Not a URL, nothing to do
	}
	// Mask the userinfo and the URI (path?query or opaque?query ) and fragment, leave the scheme and host(host/port)  untouched
	if u.User != nil {
		MaskedUrl = strings.Replace(MaskedUrl, u.User.String(), "***", 1)
	}
	if len(u.RequestURI()) > 1 { // Ignore the '/'
		MaskedUrl = strings.Replace(MaskedUrl, u.RequestURI(), "/***", 1)
	}
	if len(u.Fragment) > 0 {
		MaskedUrl = strings.Replace(MaskedUrl, u.RawFragment, "***", 1)
	}
	return MaskedUrl
}
