package state_native

import (
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

func (b *BeaconState) executionPayloadHeaderVal() *enginev1.ExecutionPayloadHeaderEPBS {
	return eth.CopyExecutionPayloadHeaderEPBS(b.executionPayloadHeader)
}
