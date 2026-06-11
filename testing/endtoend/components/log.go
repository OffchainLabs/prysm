package components

import (
	"os"

	prefixed "github.com/OffchainLabs/prysm/v7/runtime/logging/logrus-prefixed-formatter"
	"github.com/sirupsen/logrus"
)

// Use the "package" field (not "prefix"): the prefixed formatter renders it as the
// "components:" segment AND omits it from the trailing key=value fields, whereas a
// "prefix" field is rendered as the segment but also printed as a redundant field.
var log = logrus.WithField("package", "components")

// init gives the e2e harness's own logs the same compact, prefixed format the Prysm
// binaries use (mirrors cmd/beacon-chain/main.go), so `make e2e` reads like a node's
// console rather than logrus's default key=value text.
//
// Color can't be auto-detected here: the harness runs under `go test`, whose pipe hides
// the real terminal. Instead build/e2e — which does see the TTY — sets E2E_LOG_COLOR=1
// when interactive; we honor that via DisableColors. ForceColors makes the color survive
// the `go test` pipe; ForceFormatting keeps the compact layout even when uncolored (CI),
// so CI logs stay readable and free of ANSI escapes.
func init() {
	formatter := new(prefixed.TextFormatter)
	formatter.TimestampFormat = "2006-01-02 15:04:05.00"
	formatter.FullTimestamp = true
	formatter.ForceFormatting = true
	formatter.ForceColors = true
	formatter.DisableColors = os.Getenv("E2E_LOG_COLOR") != "1"
	logrus.SetFormatter(formatter)
}
