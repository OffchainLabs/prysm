package stateutil

import (
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

func HistoricalSummariesRoot(summaries []*ethpb.HistoricalSummary) ([32]byte, error) {
	return ssz.SliceRoot(summaries, fieldparams.HistoricalRootsLength)
}
