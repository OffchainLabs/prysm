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
