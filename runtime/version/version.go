// Package version executes and returns the version string
// for the currently running process.
package version

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// The value of these vars are set through linker options.
var gitCommit = "Local build"
var buildDate = "Moments ago"
var buildDateUnix = "0"
var gitTag = "Unknown"

// Version returns the version string of this build.
func Version() string {
	if buildDate == "{DATE}" {
		now := time.Now().Format(time.RFC3339)
		buildDate = now
	}
	if buildDateUnix == "{DATE_UNIX}" {
		buildDateUnix = strconv.Itoa(int(time.Now().Unix()))
	}
	return fmt.Sprintf("%s. Built at: %s", BuildData(), buildDate)
}

// SemanticVersion returns the Major.Minor.Patch version of this build.
func SemanticVersion() string {
	return gitTag
}

// GitCommit returns the current build commit hash.
func GitCommit() string {
	return resolvedGitCommit()
}

// BuildData returns the git tag and commit of the current build.
func BuildData() string {
	return fmt.Sprintf("Prysm/%s/%s", gitTag, resolvedGitCommit())
}

func resolvedGitCommit() string {
	// if doing a local build, these values are not interpolated
	if gitCommit == "{STABLE_GIT_COMMIT}" {
		commit, err := exec.Command("git", "rev-parse", "HEAD").Output()
		if err != nil {
			log.Println(err)
		} else {
			gitCommit = strings.TrimRight(string(commit), "\r\n")
		}
	}
	return gitCommit
}

// GetCommitPrefix returns the first 4 characters of the git commit.
// This is used for graffiti generation per the client identification spec.
// Note: BuildData() must be called before this (happens at startup via Version()).
func GetCommitPrefix() string {
	if len(gitCommit) < 4 {
		return gitCommit
	}
	return gitCommit[:4]
}
