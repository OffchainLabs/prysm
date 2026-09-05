package beacon_api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
	"github.com/pkg/errors"
	"go.uber.org/mock/gomock"
)

func testClientBuilderPreferencesEntry() *ethpb.BuilderPreferencesEntry {
	return &ethpb.BuilderPreferencesEntry{
		ProposerPubkey: make([]byte, 48),
		Url:            []byte("http://builder.example"),
		Auth: &ethpb.SignedBuilderRequestAuth{
			Message:   &ethpb.BuilderRequestAuth{Data: []byte{0xaa}, Slot: 1},
			Signature: make([]byte, 96),
		},
		MaxExecutionPayment: 1000,
	}
}

func TestSubmitBuilderPreferences(t *testing.T) {
	entry := testClientBuilderPreferencesEntry()
	gloasHeaders := map[string]string{"Eth-Consensus-Version": "gloas"}

	t.Run("posts ssz list", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		entrySSZ, err := entry.MarshalSSZ()
		require.NoError(t, err)
		sszBody := ssz.MarshalVariableList(entrySSZ)

		handler := mock.NewMockHandler(ctrl)
		expectPostSSZWithFallback(handler)
		handler.EXPECT().PostSSZ(
			gomock.Any(),
			builderPreferencesEndpoint,
			gloasHeaders,
			bytes.NewBuffer(sszBody),
		).Return(nil).Times(1)

		client := &beaconApiValidatorClient{handler: handler}
		require.NoError(t, client.submitBuilderPreferences(t.Context(), []*ethpb.BuilderPreferencesEntry{entry}))
	})

	t.Run("falls back to json on 415", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		jsonBody, err := json.Marshal([]*structs.BuilderPreferencesEntry{structs.BuilderPreferencesEntryFromConsensus(entry)})
		require.NoError(t, err)

		handler := mock.NewMockHandler(ctrl)
		expectPostSSZWithFallback(handler)
		handler.EXPECT().PostSSZ(
			gomock.Any(),
			builderPreferencesEndpoint,
			gomock.Any(),
			gomock.Any(),
		).Return(&httputil.DefaultJsonError{Code: http.StatusUnsupportedMediaType, Message: "unsupported media type"}).Times(1)
		handler.EXPECT().Post(
			gomock.Any(),
			builderPreferencesEndpoint,
			gloasHeaders,
			bytes.NewBuffer(jsonBody),
			nil,
		).Return(nil).Times(1)

		client := &beaconApiValidatorClient{handler: handler}
		require.NoError(t, client.submitBuilderPreferences(t.Context(), []*ethpb.BuilderPreferencesEntry{entry}))
	})

	t.Run("non media type error no fallback", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		handler := mock.NewMockHandler(ctrl)
		expectPostSSZWithFallback(handler)
		handler.EXPECT().PostSSZ(
			gomock.Any(),
			builderPreferencesEndpoint,
			gloasHeaders,
			gomock.Any(),
		).Return(errors.New("foo error")).Times(1)

		client := &beaconApiValidatorClient{handler: handler}
		err := client.submitBuilderPreferences(t.Context(), []*ethpb.BuilderPreferencesEntry{entry})
		assert.ErrorContains(t, "foo error", err)
	})

	t.Run("nil entry", func(t *testing.T) {
		client := &beaconApiValidatorClient{}
		err := client.submitBuilderPreferences(t.Context(), []*ethpb.BuilderPreferencesEntry{nil})
		assert.ErrorContains(t, "is nil", err)
	})
}
