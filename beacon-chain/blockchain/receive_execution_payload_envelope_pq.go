package blockchain

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
)

// No execution client knows the PQ request types yet, so the engine only sees the Gloas ones.
func engineExecutionRequests(envelope interfaces.ROExecutionPayloadEnvelope) *enginev1.ExecutionRequestsGloas {
	r := envelope.ExecutionRequestsPQ()
	if r == nil {
		return envelope.ExecutionRequests()
	}
	return &enginev1.ExecutionRequestsGloas{
		Deposits:        r.Deposits,
		Withdrawals:     r.Withdrawals,
		Consolidations:  r.Consolidations,
		BuilderDeposits: r.BuilderDeposits,
	}
}
