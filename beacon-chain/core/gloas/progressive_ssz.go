package gloas

import (
	"github.com/OffchainLabs/prysm/v7/config/features"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

func progressiveSSZEnabled(stateVersion int) bool {
	return stateVersion >= version.Gloas && features.Get().EnableProgressiveSSZ
}

func executionRequestsHashTreeRoot(stateVersion int, requests *enginev1.ExecutionRequests) ([32]byte, error) {
	if progressiveSSZEnabled(stateVersion) {
		return requests.HashTreeRootProgressive()
	}
	return requests.HashTreeRoot()
}

func emptyExecutionRequestsHashTreeRootForGloas() ([32]byte, error) {
	if progressiveSSZEnabled(version.Gloas) {
		return enginev1.EmptyExecutionRequestsHashTreeRootProgressive()
	}
	return enginev1.EmptyExecutionRequestsHashTreeRoot()
}
