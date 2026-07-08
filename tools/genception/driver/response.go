package driver

import (
	"runtime"
	"strconv"
	"strings"
)

// driverResponse is a copy of packages.DriverResponse, but with the
// `Packages` field using flatPackage instead of packages.Package.
type driverResponse struct {
	NotHandled bool
	Compiler   string
	Arch       string
	// GoVersion is the minor version of the Go toolchain (e.g. 21 for go1.21.x).
	// The driver protocol defines this field; zero means unknown.
	GoVersion int
	Roots     []string `json:",omitempty"`
	Packages  []*flatPackage
}

// goVersion reports the Go minor version of the toolchain that built genception,
// which inside bazel is the rules_go toolchain used for codegen.
func goVersion() int {
	return parseGoMinorVersion(runtime.Version())
}

// parseGoMinorVersion extracts the minor version from a runtime.Version() string
// such as "go1.21.5", "go1.22rc1", or "go1.21". It returns 0 (unknown) for any
// value it can't parse, e.g. "devel ..." development builds.
func parseGoMinorVersion(v string) int {
	const prefix = "go1."
	if !strings.HasPrefix(v, prefix) {
		return 0
	}
	v = v[len(prefix):]
	end := 0
	for end < len(v) && v[end] >= '0' && v[end] <= '9' {
		end++
	}
	n, err := strconv.Atoi(v[:end])
	if err != nil {
		return 0
	}
	return n
}
