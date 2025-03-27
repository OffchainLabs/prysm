package state_native

import (
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
)

func (b *BeaconState) executionPayloadHeaderVal() *enginev1.ExecutionPayloadHeaderEPBS {
	return eth.CopyExecutionPayloadHeaderEPBS(b.latestExecutionPayloadHeaderEPBS)
}
