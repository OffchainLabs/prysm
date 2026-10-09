package query

import (
	"reflect"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestSpecFieldName(t *testing.T) {
	type tagged struct {
		A                         uint64 `json:"spec_a,omitempty"`
		B                         uint64 `json:"-"`
		Eth1DataVotes             uint64
		LatestExecutionPayloadBid uint64
	}
	rt := reflect.TypeFor[tagged]()
	for i, want := range []string{"spec_a", "b", "eth1_data_votes", "latest_execution_payload_bid"} {
		require.Equal(t, want, specFieldName(rt.Field(i)))
	}
}
