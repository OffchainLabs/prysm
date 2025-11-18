package version

import (
	"sort"

	"github.com/pkg/errors"
)

const (
	Phase0 = iota
	Altair
	Bellatrix
	Capella
	Deneb
	Electra
	Fulu
	Gloas
)

var versionToString = map[int]string{
	Phase0:    "phase0",
	Altair:    "altair",
	Bellatrix: "bellatrix",
	Capella:   "capella",
	Deneb:     "deneb",
	Electra:   "electra",
	Fulu:      "fulu",
	Gloas:     "gloas",
}

// stringToVersion and allVersions are populated in init()
var stringToVersion = map[string]int{}
var allVersions []int
var releasedVersions []int

// featureGatedVersions contains fork versions that exist in the enums but are not yet
// enabled on any supported network. These versions are removed from Released().
var featureGatedVersions = []int{Gloas}

var featureGatedVersionSet = map[int]struct{}{}

// ErrUnrecognizedVersionName means a string does not match the list of canonical version names.
var ErrUnrecognizedVersionName = errors.New("version name doesn't map to a known value in the enum")

// FromString translates a canonical version name to the version number.
func FromString(name string) (int, error) {
	v, ok := stringToVersion[name]
	if !ok {
		return 0, errors.Wrap(ErrUnrecognizedVersionName, name)
	}
	return v, nil
}

// String returns the canonical string form of a version.
// Unrecognized versions won't generate an error and are represented by the string "unknown version".
func String(version int) string {
	name, ok := versionToString[version]
	if !ok {
		return "unknown version"
	}
	return name
}

// All returns a list of all known fork versions.
func All() []int {
	return allVersions
}

// Released returns the list of fork versions that are not feature gated.
func Released() []int {
	return releasedVersions
}

// FeatureGated returns fork versions that exist in the enum but are not yet enabled.
func FeatureGated() []int {
	return featureGatedVersions
}

// IsFeatureGated reports whether the provided version is currently gate-kept.
func IsFeatureGated(version int) bool {
	_, ok := featureGatedVersionSet[version]
	return ok
}

func init() {
	allVersions = make([]int, len(versionToString))
	i := 0
	for v, s := range versionToString {
		allVersions[i] = v
		stringToVersion[s] = v
		i++
	}
	sort.Ints(allVersions)

	featureGatedVersionSet = make(map[int]struct{}, len(featureGatedVersions))
	for _, v := range featureGatedVersions {
		featureGatedVersionSet[v] = struct{}{}
	}
	sort.Ints(featureGatedVersions)

	releasedVersions = make([]int, 0, len(allVersions))
	for _, v := range allVersions {
		if _, skip := featureGatedVersionSet[v]; skip {
			continue
		}
		releasedVersions = append(releasedVersions, v)
	}
}
