package components

import (
	"os"

	prefixed "github.com/OffchainLabs/prysm/v7/runtime/logging/logrus-prefixed-formatter"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("package", "components")

func init() {
	formatter := new(prefixed.TextFormatter)
	formatter.TimestampFormat = "2006-01-02 15:04:05.00"
	formatter.FullTimestamp = true
	formatter.ForceFormatting = true
	formatter.ForceColors = true
	formatter.DisableColors = os.Getenv("E2E_LOG_COLOR") != "1"
	logrus.SetFormatter(formatter)
}
