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
var supportedVersions []int

// unsupportedVersions contains fork versions that exist in the enums but are not yet
// enabled on any supported network. These versions are removed from All().
var unsupportedVersions = []int{Gloas}

var unsupportedVersionSet = map[int]struct{}{}

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

// All returns a list of all supported fork versions.
func All() []int {
	return supportedVersions
}

// Unsupported returns fork versions that exist in the enum but are not yet enabled.
func Unsupported() []int {
	return unsupportedVersions
}

// IsUnsupported reports whether the provided version is currently gate-kept.
func IsUnsupported(version int) bool {
	_, ok := unsupportedVersionSet[version]
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

	unsupportedVersionSet = make(map[int]struct{}, len(unsupportedVersions))
	for _, v := range unsupportedVersions {
		unsupportedVersionSet[v] = struct{}{}
	}
	sort.Ints(unsupportedVersions)

	supportedVersions = make([]int, 0, len(allVersions))
	for _, v := range allVersions {
		if _, skip := unsupportedVersionSet[v]; skip {
			continue
		}
		supportedVersions = append(supportedVersions, v)
	}
}
