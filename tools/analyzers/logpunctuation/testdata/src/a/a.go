package a

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("package", "a")

const done = "Done."

func messages() {
	log.Info("Synced new block")
	log.Info("Synced new block.")        // want `log message should not end with "." or "!"`
	log.Warn("Use at your own risk!")    // want `log message should not end with "." or "!"`
	log.Debug("Starting sync...")        // want `log message should not end with "." or "!"`
	log.Infof("Processed %d blocks.", 1) // want `log message should not end with "." or "!"`
	log.Error("Two sentences. " + "Second one.")
	log.Info(done)
	log.Println("Count:", 1, "blocks.") // want `log message should not end with "." or "!"`
	logrus.Info("Package func.")        // want `log message should not end with "." or "!"`
	log.Infof(`Raw "quoted" %d.`, 1)    // want `log message should not end with "." or "!"`
	log.Infof("Version %s", "1.0.")
	log.Info("Using version v1.2.3 of the client")
	log.Info(fmt.Sprintf("Dynamic %s.", "message"))
	log.Info("...")
	fmt.Println("Not a log message.")
}
