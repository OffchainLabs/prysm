// Copyright 2015 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package bazel

import (
	"path/filepath"
	"strings"

	"github.com/OffchainLabs/prysm/v7/build/externaldata"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("prefix", "bazel")

// fetchOnMiss is invoked by Runfile when a path cannot be resolved from disk. It
// maps the path's leading segment(s) to the external-data archive that provides
// it and downloads that archive (idempotently — a cached archive is a no-op).
// This makes external test data a lazy fixture: a test that needs a file triggers
// exactly the archive holding it, the first time it is requested. Paths with no
// known mapping return false (the caller then reports the normal "not found").
//
// Under Bazel these archives are http_archive runfiles, so this hook only exists
// in the non-bazel build (see bazel.go).
func fetchOnMiss(path string) bool {
	name, ok := archiveForPath(path)
	if !ok {
		return false
	}
	if err := externaldata.Fetch(name); err != nil {
		// Surface the cause (network/sha mismatch); the caller still reports the
		// generic "could not locate" miss to keep Runfile's signature.
		log.WithField("archive", name).WithError(err).Error("Failed to fetch external test data")
		return false
	}
	return true
}

// archiveForPath returns the external-data archive name that provides the given
// runfile path, mirroring the dest layout in externaldata's manifest (and the
// former hack/testdata.sh).
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
