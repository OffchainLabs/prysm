//go:build !bazel

package bazel

import (
	"path/filepath"
	"strings"

	"github.com/OffchainLabs/prysm/v7/build/externaldata"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("prefix", "bazel")

// fetchArchive fetches the named external-data archive.
var fetchArchive = externaldata.Fetch

func fetchOnMiss(path string) bool {
	name, ok := archiveForPath(path)
	if !ok {
		return false
	}

	if err := fetchArchive(name); err != nil {
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
		if name, ok := consensusSpecTestArchives[category]; ok {
			return name, true
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

// consensusSpecTestArchives maps a consensus-spec test category.
var consensusSpecTestArchives = map[string]string{
	"general": externaldata.ConsensusSpecTestsGeneral,
	"minimal": externaldata.ConsensusSpecTestsMinimal,
	"mainnet": externaldata.ConsensusSpecTestsMainnet,
}

// blsCategories are the top-level directories the bls12-381 test archive unpacks
// into.
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
