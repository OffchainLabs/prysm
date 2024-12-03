package execution

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
)

const (
	NewPayloadMethodV5               = "engine_newPayloadV5" // Do we really need this?
	GetInclusionListV1               = "engine_getInclusionListV1"
	UpdatePayloadWithInclusionListV1 = "engine_updatePayloadWithInclusionListV1"
)

// GetInclusionList fetches the inclusion list for a given parent hash by invoking the execution engine RPC.
// It uses a context with a timeout defined by the Beacon configuration.
// Implements: https://github.com/ethereum/execution-apis/pull/609
func (s *Service) GetInclusionList(ctx context.Context, parentHash [32]byte) ([][]byte, error) {
	ctx, span := trace.StartSpan(ctx, "execution.GetInclusionList")
	defer span.End()

	start := time.Now()
	defer func() {
		getInclusionListLatency.Observe(float64(time.Since(start).Milliseconds()))
	}()

	timeout := time.Duration(params.BeaconConfig().ExecutionEngineTimeoutValue) * time.Second
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(timeout))
	defer cancel()

	var result [][]byte
	err := s.rpcClient.CallContext(ctx, &result, GetInclusionListV1, common.Hash(parentHash))
	if err != nil {
		return nil, handleRPCError(err)
	}

	return result, nil
}

// UpdatePayloadWithInclusionList updates a payload with a provided inclusion list of transactions.
// It uses a context with a timeout defined by the Beacon configuration and returns the new payload ID.
// Implements: https://github.com/ethereum/execution-apis/pull/609
func (s *Service) UpdatePayloadWithInclusionList(ctx context.Context, payloadID primitives.PayloadID, txs [][]byte) (*primitives.PayloadID, error) {
	ctx, span := trace.StartSpan(ctx, "execution.UpdatePayloadWithInclusionList")
	defer span.End()

	start := time.Now()
	defer func() {
		updatePayloadWithInclusionListLatency.Observe(float64(time.Since(start).Milliseconds()))
	}()

	timeout := time.Duration(params.BeaconConfig().ExecutionEngineTimeoutValue) * time.Second
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(timeout))
	defer cancel()

	result := &engine.PayloadID{}
	err := s.rpcClient.CallContext(ctx, result, UpdatePayloadWithInclusionListV1, engine.PayloadID(payloadID), txs)
	if err != nil {
		return nil, handleRPCError(err)
	}

	return (*primitives.PayloadID)(result), nil
}
