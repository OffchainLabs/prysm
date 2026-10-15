package beacon_api

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/mock/gomock"
)

func TestProduceInclusionList(t *testing.T) {
	const endpoint = "/eth/v1/validator/inclusion_list?slot=42"

	t.Run("ok", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		handler := mock.NewMockHandler(ctrl)
		handler.EXPECT().Get(gomock.Any(), endpoint, &structs.ProduceInclusionListResponse{}).Return(nil).SetArg(
			2, structs.ProduceInclusionListResponse{Data: []string{"0x0201", "0xf802"}},
		).Times(1)
		client := &beaconApiValidatorClient{handler: handler}
		txs, err := client.ProduceInclusionList(t.Context(), 42)
		require.NoError(t, err)
		require.DeepEqual(t, [][]byte{{0x02, 0x01}, {0xf8, 0x02}}, txs)
	})
	t.Run("empty", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		handler := mock.NewMockHandler(ctrl)
		handler.EXPECT().Get(gomock.Any(), endpoint, gomock.Any()).Return(nil).SetArg(
			2, structs.ProduceInclusionListResponse{Data: []string{}},
		).Times(1)
		client := &beaconApiValidatorClient{handler: handler}
		txs, err := client.ProduceInclusionList(t.Context(), 42)
		require.NoError(t, err)
		require.Equal(t, 0, len(txs))
	})
	t.Run("bad hex", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		handler := mock.NewMockHandler(ctrl)
		handler.EXPECT().Get(gomock.Any(), endpoint, gomock.Any()).Return(nil).SetArg(
			2, structs.ProduceInclusionListResponse{Data: []string{"0xzz"}},
		).Times(1)
		client := &beaconApiValidatorClient{handler: handler}
		_, err := client.ProduceInclusionList(t.Context(), 42)
		require.ErrorContains(t, "could not decode inclusion list transaction 0", err)
	})
	t.Run("request error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		handler := mock.NewMockHandler(ctrl)
		handler.EXPECT().Get(gomock.Any(), endpoint, gomock.Any()).Return(errors.New("foo")).Times(1)
		client := &beaconApiValidatorClient{handler: handler}
		_, err := client.ProduceInclusionList(t.Context(), 42)
		require.ErrorContains(t, "could not get inclusion list: foo", err)
	})
}

func TestSubmitInclusionList(t *testing.T) {
	txs, err := enginev1.NewProgressiveTransactionList([][]byte{{0x02, 0x01}})
	require.NoError(t, err)
	signed := &ethpb.SignedInclusionList{
		Message: &ethpb.InclusionList{
			Slot:           5,
			ValidatorIndex: 7,
			DependentRoot:  bytesutil.PadTo([]byte{0xaa}, fieldparams.RootLength),
			Transactions:   txs,
		},
		Signature: bytesutil.PadTo([]byte{0xbb}, fieldparams.BLSSignatureLength),
	}
	body, err := json.Marshal(&structs.PublishInclusionListRequest{Data: structs.SignedInclusionListFromConsensus(signed)})
	require.NoError(t, err)

	t.Run("ok", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		handler := mock.NewMockHandler(ctrl)
		handler.EXPECT().Post(
			gomock.Any(),
			"/eth/v1/validator/inclusion_list",
			map[string]string{api.VersionHeader: "heze"},
			bytes.NewBuffer(body),
			nil,
		).Return(nil).Times(1)
		client := &beaconApiValidatorClient{handler: handler}
		require.NoError(t, client.SubmitInclusionList(t.Context(), signed))

		// The body round-trips to the same consensus value.
		var req structs.PublishInclusionListRequest
		require.NoError(t, json.Unmarshal(body, &req))
		got, err := req.Data.ToConsensus()
		require.NoError(t, err)
		require.DeepEqual(t, signed, got)
	})
	t.Run("nil", func(t *testing.T) {
		client := &beaconApiValidatorClient{}
		require.ErrorContains(t, "signed inclusion list is nil", client.SubmitInclusionList(t.Context(), nil))
		require.ErrorContains(t, "signed inclusion list is nil", client.SubmitInclusionList(t.Context(), &ethpb.SignedInclusionList{}))
	})
	t.Run("request error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		handler := mock.NewMockHandler(ctrl)
		handler.EXPECT().Post(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("foo")).Times(1)
		client := &beaconApiValidatorClient{handler: handler}
		require.ErrorContains(t, "foo", client.SubmitInclusionList(t.Context(), signed))
	})
}

func TestInclusionListDuties(t *testing.T) {
	const endpoint = "/eth/v1/validator/duties/inclusion_list/3"
	indicesBody, err := json.Marshal([]string{"1", "2"})
	require.NoError(t, err)
	dependentRoot := bytesutil.PadTo([]byte{0xdd}, fieldparams.RootLength)
	pubkey := bytesutil.PadTo([]byte{0xab}, fieldparams.BLSPubkeyLength)

	t.Run("ok", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		handler := mock.NewMockHandler(ctrl)
		handler.EXPECT().Post(
			gomock.Any(), endpoint, nil, bytes.NewBuffer(indicesBody), &structs.GetInclusionListDutiesResponse{},
		).Return(nil).SetArg(4, structs.GetInclusionListDutiesResponse{
			DependentRoot:       hexutil.Encode(dependentRoot),
			ExecutionOptimistic: true,
			Data:                []*structs.InclusionListDuty{{Pubkey: hexutil.Encode(pubkey), ValidatorIndex: "2", Slot: "100"}},
		}).Times(1)
		client := &beaconApiValidatorClient{handler: handler}
		resp, err := client.InclusionListDuties(t.Context(), 3, []primitives.ValidatorIndex{1, 2})
		require.NoError(t, err)
		require.DeepEqual(t, &iface.InclusionListDutiesResponse{
			DependentRoot:       dependentRoot,
			ExecutionOptimistic: true,
			Duties:              []*iface.InclusionListDuty{{Pubkey: pubkey, ValidatorIndex: 2, Slot: 100}},
		}, resp)
	})
	for _, tc := range []struct {
		name string
		resp structs.GetInclusionListDutiesResponse
		msg  string
	}{
		{name: "bad dependent root", resp: structs.GetInclusionListDutiesResponse{DependentRoot: "0xzz"}, msg: "could not decode dependent root"},
		{name: "nil duty", resp: structs.GetInclusionListDutiesResponse{DependentRoot: "0x00", Data: []*structs.InclusionListDuty{nil}}, msg: "inclusion list duty 0 is nil"},
		{name: "bad pubkey", resp: structs.GetInclusionListDutiesResponse{DependentRoot: "0x00", Data: []*structs.InclusionListDuty{{Pubkey: "x", ValidatorIndex: "1", Slot: "1"}}}, msg: "could not decode pubkey"},
		{name: "bad index", resp: structs.GetInclusionListDutiesResponse{DependentRoot: "0x00", Data: []*structs.InclusionListDuty{{Pubkey: "0x00", ValidatorIndex: "x", Slot: "1"}}}, msg: "could not parse validator index"},
		{name: "bad slot", resp: structs.GetInclusionListDutiesResponse{DependentRoot: "0x00", Data: []*structs.InclusionListDuty{{Pubkey: "0x00", ValidatorIndex: "1", Slot: "x"}}}, msg: "could not parse slot"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			handler := mock.NewMockHandler(ctrl)
			handler.EXPECT().Post(gomock.Any(), endpoint, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(4, tc.resp).Times(1)
			client := &beaconApiValidatorClient{handler: handler}
			_, err := client.InclusionListDuties(t.Context(), 3, []primitives.ValidatorIndex{1, 2})
			require.ErrorContains(t, tc.msg, err)
		})
	}
	t.Run("request error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		handler := mock.NewMockHandler(ctrl)
		handler.EXPECT().Post(gomock.Any(), endpoint, gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("foo")).Times(1)
		client := &beaconApiValidatorClient{handler: handler}
		_, err := client.InclusionListDuties(t.Context(), 3, []primitives.ValidatorIndex{1, 2})
		require.ErrorContains(t, "could not get inclusion list duties: foo", err)
		assert.NotNil(t, err)
	})
}
