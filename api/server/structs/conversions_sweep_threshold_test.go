package structs_test

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// TestSetSweepThresholdRequestsFromConsensus_Count guards the element count and order.
// A botched double-append here previously turned N requests into 2N-1, duplicating the
// first entry, which is invisible unless the count is asserted.
func TestSetSweepThresholdRequestsFromConsensus_Count(t *testing.T) {
	consensus := make([]*enginev1.SetSweepThresholdRequest, 0, 3)
	for i := range 3 {
		consensus = append(consensus, &enginev1.SetSweepThresholdRequest{
			SourceAddress:   bytes.Repeat([]byte{0x11}, 20),
			ValidatorPubkey: bytes.Repeat([]byte{byte(0xa0 + i)}, 48),
			Threshold:       uint64(33+i) * 1_000_000_000,
		})
	}

	got := structs.SetSweepThresholdRequestsFromConsensus(consensus)
	require.Equal(t, len(consensus), len(got))
	for i, want := range consensus {
		require.Equal(t, hexEncode(want.ValidatorPubkey), got[i].ValidatorPubkey)
	}

	require.Equal(t, 0, len(structs.SetSweepThresholdRequestsFromConsensus(nil)))
}

func hexEncode(b []byte) string {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 0, 2+2*len(b))
	out = append(out, '0', 'x')
	for _, c := range b {
		out = append(out, hexDigits[c>>4], hexDigits[c&0x0f])
	}
	return string(out)
}
