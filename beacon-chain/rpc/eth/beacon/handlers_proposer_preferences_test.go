package beacon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestGetProposerPreferences_Empty(t *testing.T) {
	s := &Server{ProposerPreferencesCache: cache.NewProposerPreferencesCache()}
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.GetProposerPreferences(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	resp := &structs.GetProposerPreferencesResponse{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), resp))
	assert.Equal(t, 0, len(resp.Data))
}

func TestGetProposerPreferences_NilCache(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.GetProposerPreferences(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	resp := &structs.GetProposerPreferencesResponse{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), resp))
	assert.Equal(t, 0, len(resp.Data))
}

func TestGetProposerPreferences_Populated(t *testing.T) {
	c := cache.NewProposerPreferencesCache()
	c.Add(cache.ProposerPreference{
		DependentRoot:  [32]byte{0xaa},
		ProposalSlot:   primitives.Slot(32),
		ValidatorIndex: 1,
		FeeRecipient:   primitives.ExecutionAddress{0x01},
		TargetGasLimit: 30_000_000,
		Signature:      [96]byte{0xff},
	}, primitives.Slot(32))
	c.Add(cache.ProposerPreference{
		DependentRoot:  [32]byte{0xbb},
		ProposalSlot:   primitives.Slot(33),
		ValidatorIndex: 2,
		FeeRecipient:   primitives.ExecutionAddress{0x02},
		TargetGasLimit: 25_000_000,
		Signature:      [96]byte{0xee},
	}, primitives.Slot(33))

	s := &Server{ProposerPreferencesCache: c}
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	s.GetProposerPreferences(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	resp := &structs.GetProposerPreferencesResponse{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), resp))
	require.Equal(t, 2, len(resp.Data))

	bySlot := map[string]*structs.SignedProposerPreferences{}
	for _, p := range resp.Data {
		bySlot[p.Message.ProposalSlot] = p
	}
	assert.Equal(t, "1", bySlot["32"].Message.ValidatorIndex)
	assert.Equal(t, "30000000", bySlot["32"].Message.TargetGasLimit)
	assert.Equal(t, "2", bySlot["33"].Message.ValidatorIndex)
	assert.Equal(t, "25000000", bySlot["33"].Message.TargetGasLimit)
}
