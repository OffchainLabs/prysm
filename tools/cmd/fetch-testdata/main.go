// Command fetch-testdata downloads every external test-data archive (consensus
// spec tests, BLS vectors, network configs) into the local test-data cache. It is
// the eager counterpart to the lazy, on-demand fetch performed by build/bazel's
// Runfile: tests self-provision the data they touch, while `make testdata` (which
// runs this command) pre-fetches everything — handy for CI warm-up or offline work.
//
// The cache location honors $PRYSM_TESTDATA (default <repo>/third_party/testdata).
package main

import (
	"github.com/OffchainLabs/prysm/v7/build/externaldata"
	"github.com/sirupsen/logrus"
)

func main() {
	logrus.WithFields(logrus.Fields{
		"count": len(externaldata.Names()),
		"dir":   externaldata.Root(),
	}).Info("Fetching external test-data archives")

	if err := externaldata.FetchAll(); err != nil {
		logrus.WithError(err).Fatal("Failed to fetch test data")
	}

	logrus.Info("Test data ready")
}
