package driver

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

const (
	ENV_JSON_INDEX_PATH = "PACKAGE_JSON_INVENTORY"
	ENV_PACKAGES_BASE   = "PACKAGES_BASE"
	ENV_PWD             = "PWD"
	ENV_LOG_PATH        = "GOPACKAGESDRIVER_LOG_PATH"
	ENV_GO_TAGS         = "GOTAGS"
	ENV_RECORDER_PATH   = "GOPACKAGESDRIVER_RECORDER_PATH"
)

var errUnsetEnvVar = errors.New("required env var not set")

type environment struct {
	inventoryIndexPath string
	packagesBase       string
	pwd                string
	logPath            string
	goTags             []string
	recorderPath       string
}

func loadEnv() (*environment, error) {
	e := &environment{}
	e.goTags = strings.Split(os.Getenv(ENV_GO_TAGS), ",")
	e.inventoryIndexPath = os.Getenv(ENV_JSON_INDEX_PATH)
	if e.inventoryIndexPath == "" {
		return nil, errors.Wrap(errUnsetEnvVar, ENV_JSON_INDEX_PATH)
	}
	e.packagesBase = os.Getenv(ENV_PACKAGES_BASE)
	if e.packagesBase == "" {
		return nil, errors.Wrap(errUnsetEnvVar, ENV_PACKAGES_BASE)
	}
	e.pwd = os.Getenv(ENV_PWD)
	if e.pwd == "" {
		return nil, errors.Wrap(errUnsetEnvVar, ENV_PWD)
	}
	e.logPath = os.Getenv(ENV_LOG_PATH)
	if e.logPath == "" {
		e.logPath = filepath.Join(e.pwd, "genception.log")
	}
	e.recorderPath = os.Getenv(ENV_RECORDER_PATH)
	return e, nil
}
