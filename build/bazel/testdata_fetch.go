//go:build !bazel

package bazel

import (
	"path/filepath"
	"strings"

	"github.com/OffchainLabs/prysm/v7/build/externaldata"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("prefix", "bazel")

func fetchOnMiss(path string) bool {
	name, ok := archiveForPath(path)
	if !ok {
		return false
	}

	if err := externaldata.Fetch(name); err != nil {
		log.WithField("archive", name).WithError(err).Error("Failed to fetch external test data")
		return false
	}

	return true
}

func archiveForPath(path string) (string, bool) {
	path = filepath.ToSlash(path)
	first, rest, _ := strings.Cut(path, "/")

	switch first {
	case "tests":
		// Consensus spec tests: tests/{general,minimal,mainnet}/...
		category, _, _ := strings.Cut(rest, "/")
		switch category {
		case "general":
			return externaldata.ConsensusSpecTestsGeneral, true
		case "minimal":
			return externaldata.ConsensusSpecTestsMinimal, true
		case "mainnet":
			return externaldata.ConsensusSpecTestsMainnet, true
		}
		return "", false

	case "external":
		// external/<repo>/... → the <repo> archive.
		repo, _, _ := strings.Cut(rest, "/")
		if repo == "" {
			return "", false
		}
		return repo, true
	}

	// BLS test vectors live at the root under per-category dirs.
	if blsCategories[first] {
		return externaldata.BLSSpecTests, true
	}

	return "", false
}

// blsCategories are the top-level directories the bls12-381 test archive unpacks
// into (it extracts into the test-data root, so its files appear directly under
// these names).
var blsCategories = map[string]bool{
	"aggregate":             true,
	"aggregate_verify":      true,
	"batch_verify":          true,
	"deserialization_G1":    true,
	"deserialization_G2":    true,
	"fast_aggregate_verify": true,
	"hash_to_G2":            true,
	"sign":                  true,
	"verify":                true,
}
