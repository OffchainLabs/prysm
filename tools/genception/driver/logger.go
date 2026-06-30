package driver

import (
	"os"

	"github.com/sirupsen/logrus"
)

// log is the package-internal logger; Logger is the same instance exported for the
// cmd entrypoint. Both are non-nil from package load and default to stderr until
// configureLog redirects them to the configured log file.
var log = logrus.New()
var Logger = log

// configureLog redirects logging to the file named by the environment, falling back to
// the default stderr output if the file cannot be opened. It is called once during
// driver construction rather than from init() so that importing the package has no
// filesystem side effects.
func configureLog(env environment) {
	file, err := os.OpenFile(env.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Info("Failed to log to file, using default stderr")
		return
	}
	log.Out = file
}
