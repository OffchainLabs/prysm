package endtoend_kurtosis

import (
	"os/exec"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/bazelbuild/rules_go/go/tools/bazel"
)

const (
	beaconChainImageTarget = "cmd/beacon-chain/oci_image_tarball/tarball.tar"
	validatorImageTarget   = "cmd/validator/oci_image_tarball/tarball.tar"

	beaconChainImageName = "gcr.io/offchainlabs/prysm/beacon-chain:latest"
	validatorImageName   = "gcr.io/offchainlabs/prysm/validator:latest"
)

// LoadPrysmDockerImages loads the Prysm beacon-chain and validator Docker images
// into the local Docker daemon with verification.
func LoadPrysmDockerImages(t *testing.T) {
	// Load the beacon-chain image.
	loadDockerImage(t, beaconChainImageTarget)
	verifyImageLoaded(t, beaconChainImageName)

	// Load the validator image.
	loadDockerImage(t, validatorImageTarget)
	verifyImageLoaded(t, validatorImageName)
}

// loadDockerImage loads a Docker image from a Bazel runfile path into the local Docker daemon.
func loadDockerImage(t *testing.T, runfilePath string) {
	filePath, err := bazel.Runfile(runfilePath)
	require.NoError(t, err, "Failed to find runfile: %s", runfilePath)

	cmd := exec.Command("docker", "load", "-i", filePath)
	require.NoError(t, cmd.Run(), "Failed to load docker image from file: %s", filePath)
}

// verifyImageLoaded checks if a Docker image with the given name exists in the local Docker daemon.
func verifyImageLoaded(t *testing.T, imageName string) {
	cmd := exec.Command("docker", "image", "inspect", imageName)
	require.NoError(t, cmd.Run(), "Failed to verify image: %s", imageName)
}
