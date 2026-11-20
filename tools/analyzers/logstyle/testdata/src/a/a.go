package a

import (
	"errors"

	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("package", "a")

func formats() {
	log.Infof("Plain message")                   // want `Infof has no format arguments: use Info`
	log.WithError(errors.New("x")).Errorf("Bad") // want `Errorf has no format arguments: use Error`
	logrus.Infof("Package func")                 // want `Infof has no format arguments: use Info`
	log.Infof("Slot %d", 1)
	args := []any{1}
	log.Infof("Slot %d", args...)
	log.Info("Plain message")
}
