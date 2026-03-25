package validator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server"
	blockchainmock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	p2pMock "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestSubmitProposerPreferences_PreFork(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 100
	params.OverrideBeaconConfig(cfg)

	s := &Server{TimeFetcher: &blockchainmock.ChainService{}}
	request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/validator/proposer_preferences", strings.NewReader("[]"))
	request.Header.Set(api.VersionHeader, "gloas")
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.SubmitProposerPreferences(writer, request)
	assert.Equal(t, http.StatusBadRequest, writer.Code)
}

func TestSubmitProposerPreferences_OK(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	c := cache.NewProposerPreferencesCache()
	currentSlot := primitives.Slot(31)
	broadcaster := &p2pMock.MockBroadcaster{}

	s := &Server{
		ProposerPreferencesCache: c,
		TimeFetcher:              &blockchainmock.ChainService{Slot: &currentSlot},
		OperationNotifier:        &blockchainmock.MockOperationNotifier{},
		Broadcaster:              broadcaster,
	}

	feeRecipient := bytesutil.PadTo([]byte{0xaa}, 20)
	sig := bytesutil.PadTo([]byte{0xcc}, 96)
	body := fmt.Sprintf(`[{
		"message": {
			"proposal_slot": "32",
			"validator_index": "5",
			"fee_recipient": "%s",
			"gas_limit": "30000000"
		},
		"signature": "%s"
	}]`, hexutil.Encode(feeRecipient), hexutil.Encode(sig))

	request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/validator/proposer_preferences", strings.NewReader(body))
	request.Header.Set(api.VersionHeader, "gloas")
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.SubmitProposerPreferences(writer, request)
	assert.Equal(t, http.StatusOK, writer.Code)

	require.Equal(t, true, c.Has(32))
	pref, ok := c.Get(32)
	require.Equal(t, true, ok)
	assert.DeepEqual(t, feeRecipient, pref.Message.FeeRecipient)
	assert.Equal(t, uint64(30000000), pref.Message.GasLimit)
}

func TestSubmitProposerPreferences_MissingVersionHeader(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	s := &Server{TimeFetcher: &blockchainmock.ChainService{}}
	request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/validator/proposer_preferences", strings.NewReader("[]"))
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.SubmitProposerPreferences(writer, request)
	assert.Equal(t, http.StatusBadRequest, writer.Code)
}

func TestSubmitProposerPreferences_EmptyBody(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	s := &Server{TimeFetcher: &blockchainmock.ChainService{}}
	request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/validator/proposer_preferences", nil)
	request.Header.Set(api.VersionHeader, "gloas")
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.SubmitProposerPreferences(writer, request)
	assert.Equal(t, http.StatusBadRequest, writer.Code)
}

func TestSubmitProposerPreferences_WrongEpoch(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	c := cache.NewProposerPreferencesCache()
	currentSlot := primitives.Slot(31)

	s := &Server{
		ProposerPreferencesCache: c,
		TimeFetcher:              &blockchainmock.ChainService{Slot: &currentSlot},
	}

	feeRecipient := bytesutil.PadTo([]byte{0xaa}, 20)
	sig := bytesutil.PadTo([]byte{0xcc}, 96)
	body := fmt.Sprintf(`[{
		"message": {
			"proposal_slot": "0",
			"validator_index": "5",
			"fee_recipient": "%s",
			"gas_limit": "30000000"
		},
		"signature": "%s"
	}]`, hexutil.Encode(feeRecipient), hexutil.Encode(sig))

	request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/validator/proposer_preferences", strings.NewReader(body))
	request.Header.Set(api.VersionHeader, "gloas")
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.SubmitProposerPreferences(writer, request)
	assert.Equal(t, http.StatusBadRequest, writer.Code)

	e := &server.IndexedErrorContainer{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
	require.Equal(t, 1, len(e.Failures))
	assert.StringContains(t, "next epoch", e.Failures[0].Message)
}

func TestSubmitProposerPreferences_Duplicate(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	c := cache.NewProposerPreferencesCache()
	currentSlot := primitives.Slot(31)
	broadcaster := &p2pMock.MockBroadcaster{}

	sp := &ethpb.SignedProposerPreferences{
		Message: &ethpb.ProposerPreferences{
			ProposalSlot:   32,
			ValidatorIndex: 5,
			FeeRecipient:   bytesutil.PadTo([]byte{0xaa}, 20),
			GasLimit:       30000000,
		},
		Signature: bytesutil.PadTo([]byte{0xcc}, 96),
	}
	c.Add(32, sp)

	s := &Server{
		ProposerPreferencesCache: c,
		TimeFetcher:              &blockchainmock.ChainService{Slot: &currentSlot},
		OperationNotifier:        &blockchainmock.MockOperationNotifier{},
		Broadcaster:              broadcaster,
	}

	feeRecipient := bytesutil.PadTo([]byte{0xbb}, 20)
	sig := bytesutil.PadTo([]byte{0xdd}, 96)
	body := fmt.Sprintf(`[{
		"message": {
			"proposal_slot": "32",
			"validator_index": "6",
			"fee_recipient": "%s",
			"gas_limit": "30000001"
		},
		"signature": "%s"
	}]`, hexutil.Encode(feeRecipient), hexutil.Encode(sig))

	request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/validator/proposer_preferences", strings.NewReader(body))
	request.Header.Set(api.VersionHeader, "gloas")
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.SubmitProposerPreferences(writer, request)
	assert.Equal(t, http.StatusOK, writer.Code)

	pref, ok := c.Get(32)
	require.Equal(t, true, ok)
	assert.DeepEqual(t, bytesutil.PadTo([]byte{0xaa}, 20), pref.Message.FeeRecipient)
}
