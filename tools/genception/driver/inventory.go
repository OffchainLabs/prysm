package driver

import (
	"encoding/json"
	"os"
)

// loadJsonListing reads the list of json package index files created by the bazel gopackagesdriver aspect:
// https://github.com/bazelbuild/rules_go/blob/master/go/tools/gopackagesdriver/aspect.bzl
// This list is serialized as a []string paths, relative to the bazel exec root.
func loadJsonListing(env environment) ([]string, error) {
	path := env.inventoryIndexPath
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	log.WithField("path", path).Info("Read json index file")

	var um []string
	if err := json.Unmarshal(b, &um); err != nil {
		return nil, err
	}

	return um, nil
}
