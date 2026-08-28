package validator

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	builderTest "github.com/OffchainLabs/prysm/v7/beacon-chain/builder/testing"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/pkg/errors"
)

func testBuilderPreferencesEntry() *eth.BuilderPreferencesEntry {
	return &eth.BuilderPreferencesEntry{
		ProposerPubkey: make([]byte, 48),
		Url:            []byte("http://builder.example"),
		Auth: &eth.SignedRequestAuth{
			Message:   &eth.RequestAuth{Data: []byte{0xaa}, Slot: 1},
			Signature: make([]byte, 96),
		},
		MaxExecutionPayment: 1000,
	}
}

// sszEncodeBuilderPreferencesEntries encodes a bare SSZ List[BuilderPreferencesEntry].
func sszEncodeBuilderPreferencesEntries(t *testing.T, entries []*eth.BuilderPreferencesEntry) []byte {
	parts := make([][]byte, len(entries))
	for i, e := range entries {
		b, err := e.MarshalSSZ()
		require.NoError(t, err)
		parts[i] = b
	}
	return ssz.MarshalVariableList(parts...)
}

func TestSubmitBuilderPreferences(t *testing.T) {
	newServerWithBuilder := func(builderErr error) (*Server, *builderTest.MockBuilderService) {
		builder := &builderTest.MockBuilderService{HasConfigured: true, ErrSubmitBuilderPreferences: builderErr}
		return &Server{
			BlockBuilder: builder,
			SyncChecker:  &mockSync.Sync{IsSyncing: false},
		}, builder
	}
	newServer := func(builderErr error) *Server {
		s, _ := newServerWithBuilder(builderErr)
		return s
	}
	newRequest := func(body []byte) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/validator/builder_preferences", bytes.NewReader(body))
		req.Header.Set(api.VersionHeader, version.String(version.Gloas))
		return req
	}

	t.Run("json ok", func(t *testing.T) {
		body, err := json.Marshal([]*structs.BuilderPreferencesEntry{
			structs.BuilderPreferencesEntryFromConsensus(testBuilderPreferencesEntry()),
		})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}
		newServer(nil).SubmitBuilderPreferences(w, newRequest(body))
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("ssz ok", func(t *testing.T) {
		body := sszEncodeBuilderPreferencesEntries(t, []*eth.BuilderPreferencesEntry{
			testBuilderPreferencesEntry(),
			testBuilderPreferencesEntry(),
		})
		req := newRequest(body)
		req.Header.Set("Content-Type", api.OctetStreamMediaType)
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}
		newServer(nil).SubmitBuilderPreferences(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("missing version header", func(t *testing.T) {
		req := newRequest([]byte("[]"))
		req.Header.Del(api.VersionHeader)
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}
		newServer(nil).SubmitBuilderPreferences(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.StringContains(t, api.VersionHeader+" header is required", w.Body.String())
	})

	t.Run("pre-gloas version header", func(t *testing.T) {
		req := newRequest([]byte("[]"))
		req.Header.Set(api.VersionHeader, version.String(version.Fulu))
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}
		newServer(nil).SubmitBuilderPreferences(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.StringContains(t, "only supported from the gloas fork", w.Body.String())
	})

	t.Run("empty body", func(t *testing.T) {
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}
		newServer(nil).SubmitBuilderPreferences(w, newRequest(nil))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.StringContains(t, "No data submitted", w.Body.String())
	})

	t.Run("empty json list", func(t *testing.T) {
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}
		newServer(nil).SubmitBuilderPreferences(w, newRequest([]byte("[]")))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.StringContains(t, "No data submitted", w.Body.String())
	})

	t.Run("invalid entry reports indexed failure, valid entries still submit", func(t *testing.T) {
		bad := structs.BuilderPreferencesEntryFromConsensus(testBuilderPreferencesEntry())
		bad.ProposerPubkey = "0xff"
		good := structs.BuilderPreferencesEntryFromConsensus(testBuilderPreferencesEntry())
		body, err := json.Marshal([]*structs.BuilderPreferencesEntry{bad, good})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}
		s, builder := newServerWithBuilder(nil)
		s.SubmitBuilderPreferences(w, newRequest(body))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.StringContains(t, "failures", w.Body.String())
		require.DeepEqual(t, []string{"http://builder.example"}, builder.SubmittedPreferenceUrls())
	})

	t.Run("syncing maps to 503", func(t *testing.T) {
		body, err := json.Marshal([]*structs.BuilderPreferencesEntry{
			structs.BuilderPreferencesEntryFromConsensus(testBuilderPreferencesEntry()),
		})
		require.NoError(t, err)
		s := &Server{
			BlockBuilder: &builderTest.MockBuilderService{HasConfigured: true},
			SyncChecker:  &mockSync.Sync{IsSyncing: true},
		}
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}
		s.SubmitBuilderPreferences(w, newRequest(body))
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})

	t.Run("malformed ssz offset table", func(t *testing.T) {
		req := newRequest([]byte{0xff, 0xff, 0xff, 0xff})
		req.Header.Set("Content-Type", api.OctetStreamMediaType)
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}
		newServer(nil).SubmitBuilderPreferences(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("non-monotonic ssz offsets", func(t *testing.T) {
		body := sszEncodeBuilderPreferencesEntries(t, []*eth.BuilderPreferencesEntry{
			testBuilderPreferencesEntry(),
			testBuilderPreferencesEntry(),
		})
		// Second offset points before the first one.
		binary.LittleEndian.PutUint32(body[4:8], binary.LittleEndian.Uint32(body[:4])-1)
		req := newRequest(body)
		req.Header.Set("Content-Type", api.OctetStreamMediaType)
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}
		newServer(nil).SubmitBuilderPreferences(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.StringContains(t, "Could not decode request body", w.Body.String())
	})

	t.Run("ssz list over max entries", func(t *testing.T) {
		body := make([]byte, 4*(structs.MaxBuilderPreferencesList+1))
		binary.LittleEndian.PutUint32(body[:4], uint32(len(body)))
		req := newRequest(body)
		req.Header.Set("Content-Type", api.OctetStreamMediaType)
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}
		newServer(nil).SubmitBuilderPreferences(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.StringContains(t, "Could not decode request body", w.Body.String())
	})

	t.Run("builder submission failure reports indexed 400", func(t *testing.T) {
		body, err := json.Marshal([]*structs.BuilderPreferencesEntry{
			structs.BuilderPreferencesEntryFromConsensus(testBuilderPreferencesEntry()),
		})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}
		newServer(errors.New("boom")).SubmitBuilderPreferences(w, newRequest(body))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		resp := &server.IndexedErrorContainer{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), resp))
		require.Equal(t, 1, len(resp.Failures))
		assert.Equal(t, 0, resp.Failures[0].Index)
		assert.StringContains(t, "could not submit builder preferences", resp.Failures[0].Message)
	})

	t.Run("decode and submission failures merge with original indices", func(t *testing.T) {
		bad := structs.BuilderPreferencesEntryFromConsensus(testBuilderPreferencesEntry())
		bad.ProposerPubkey = "0xff"
		good := structs.BuilderPreferencesEntryFromConsensus(testBuilderPreferencesEntry())
		body, err := json.Marshal([]*structs.BuilderPreferencesEntry{good, bad, good})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}
		newServer(errors.New("boom")).SubmitBuilderPreferences(w, newRequest(body))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		resp := &server.IndexedErrorContainer{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), resp))
		require.Equal(t, 3, len(resp.Failures))
		for i, f := range resp.Failures {
			assert.Equal(t, i, f.Index)
		}
		assert.StringContains(t, "could not submit builder preferences", resp.Failures[0].Message)
		assert.StringContains(t, "could not submit builder preferences", resp.Failures[2].Message)
	})

	t.Run("syncing outranks decode failures", func(t *testing.T) {
		bad := structs.BuilderPreferencesEntryFromConsensus(testBuilderPreferencesEntry())
		bad.ProposerPubkey = "0xff"
		good := structs.BuilderPreferencesEntryFromConsensus(testBuilderPreferencesEntry())
		body, err := json.Marshal([]*structs.BuilderPreferencesEntry{bad, good})
		require.NoError(t, err)
		s := &Server{
			BlockBuilder: &builderTest.MockBuilderService{HasConfigured: true},
			SyncChecker:  &mockSync.Sync{IsSyncing: true},
		}
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}
		s.SubmitBuilderPreferences(w, newRequest(body))
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}
