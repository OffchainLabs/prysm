package blockchain

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
)

func engineExecutionRequests(envelope interfaces.ROExecutionPayloadEnvelope) enginev1.ExecutionRequester {
	if r := envelope.ExecutionRequestsPQ(); r != nil {
		return r
	}
	return envelope.ExecutionRequests()
}
