package events

import "github.com/sirupsen/logrus"

var logger = logrus.StandardLogger()
var log = logger.WithField("prefix", "events")
