package structs

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestSignedInclusionListConversion(t *testing.T) {
	txs, err := enginev1.NewProgressiveTransactionList([][]byte{{0x02, 0x01}, {0xf8}})
	require.NoError(t, err)
	signed := &eth.SignedInclusionList{
		Message: &eth.InclusionList{
			Slot:           10,
			ValidatorIndex: 123,
			DependentRoot:  bytesutil.PadTo([]byte{0xcf}, fieldparams.RootLength),
			Transactions:   txs,
		},
		Signature: bytesutil.PadTo([]byte{0x1b}, fieldparams.BLSSignatureLength),
	}

	t.Run("round trip", func(t *testing.T) {
		j := SignedInclusionListFromConsensus(signed)
		require.Equal(t, "10", j.Message.Slot)
		require.Equal(t, "123", j.Message.ValidatorIndex)
		require.DeepEqual(t, []string{"0x0201", "0xf8"}, j.Message.Transactions)
		got, err := j.ToConsensus()
		require.NoError(t, err)
		require.DeepEqual(t, signed, got)
	})
	t.Run("no transactions is an empty array", func(t *testing.T) {
		require.DeepEqual(t, []string{}, InclusionListFromConsensus(&eth.InclusionList{}).Transactions)
	})
	t.Run("nil", func(t *testing.T) {
		require.Equal(t, (*SignedInclusionList)(nil), SignedInclusionListFromConsensus(nil))
		_, err := (*SignedInclusionList)(nil).ToConsensus()
		require.ErrorContains(t, "SignedInclusionList", err)
		_, err = (&SignedInclusionList{}).ToConsensus()
		require.ErrorContains(t, "Message", err)
	})
	for _, tc := range []struct {
		name   string
		mutate func(*SignedInclusionList)
		msg    string
	}{
		{name: "slot", mutate: func(s *SignedInclusionList) { s.Message.Slot = "x" }, msg: "Message.Slot"},
		{name: "validator index", mutate: func(s *SignedInclusionList) { s.Message.ValidatorIndex = "-1" }, msg: "Message.ValidatorIndex"},
		{name: "dependent root length", mutate: func(s *SignedInclusionList) { s.Message.DependentRoot = "0x01" }, msg: "Message.DependentRoot"},
		{name: "transaction hex", mutate: func(s *SignedInclusionList) { s.Message.Transactions = []string{"0x0g"} }, msg: "Message.Transactions[0]"},
		{name: "signature length", mutate: func(s *SignedInclusionList) { s.Signature = hexutil.Encode([]byte{1}) }, msg: "Signature"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			j := SignedInclusionListFromConsensus(signed)
			tc.mutate(j)
			_, err := j.ToConsensus()
			require.ErrorContains(t, tc.msg, err)
		})
	}
}
