package validator

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/shared"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

// builderEntryJson is one element of the produceBlockV4 POST body: a bare array
// of per-builder entries (beacon-APIs #630).
type builderEntryJson struct {
	Url                 string          `json:"url"`
	Pubkey              string          `json:"builder_pubkey,omitempty"`
	Auth                *signedAuthJson `json:"auth"`
	MaxExecutionPayment string          `json:"max_execution_payment,omitempty"`
	MinBid              string          `json:"min_bid,omitempty"`
	BuilderBoostFactor  string          `json:"builder_boost_factor,omitempty"`
}

// maxProduceBuilderEntries is MAX_BUILDER_ENTRIES from beacon-APIs #630.
const maxProduceBuilderEntries = 64

type signedAuthJson struct {
	Message   *requestAuthJson `json:"message"`
	Signature string           `json:"signature"`
}

type requestAuthJson struct {
	Data string `json:"data"`
	Slot string `json:"slot"`
}

// parseBuilderPreferencesBody decodes the JIT produce-block POST body, accepting
// SSZ (octet-stream) or JSON. Malformed entries are ignored per the spec; an empty
// body yields no preferences.
func parseBuilderPreferencesBody(r *http.Request) ([]*eth.BuilderPreferenceV1, error) {
	if r.Body == nil {
		return nil, nil
	}
	if strings.Contains(r.Header.Get("Content-Type"), api.OctetStreamMediaType) {
		return parseBuilderPreferencesSSZ(r)
	}
	return parseBuilderPreferencesJSON(r)
}

func parseBuilderPreferencesJSON(r *http.Request) ([]*eth.BuilderPreferenceV1, error) {
	var entries []*builderEntryJson
	if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "could not decode builder preferences")
	}
	if len(entries) > maxProduceBuilderEntries {
		return nil, errors.Errorf("builder preferences exceeds %d entries", maxProduceBuilderEntries)
	}
	prefs := make([]*eth.BuilderPreferenceV1, 0, len(entries))
	for _, p := range entries {
		conv, err := p.toConsensus()
		if err != nil {
			log.WithError(err).Debug("Ignoring malformed builder entry")
			continue
		}
		prefs = append(prefs, conv)
	}
	return prefs, nil
}

func parseBuilderPreferencesSSZ(r *http.Request) ([]*eth.BuilderPreferenceV1, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, errors.Wrap(err, "could not read builder preferences")
	}
	if len(body) == 0 {
		return nil, nil
	}
	list := &eth.ProduceBuilderEntryListV1{}
	if err := list.UnmarshalSSZ(body); err != nil {
		return nil, errors.Wrap(err, "could not decode builder preferences")
	}
	all := eth.BuilderPreferencesFromSSZ(list)
	if len(all) > maxProduceBuilderEntries {
		return nil, errors.Errorf("builder preferences exceeds %d entries", maxProduceBuilderEntries)
	}
	prefs := make([]*eth.BuilderPreferenceV1, 0, len(all))
	for _, p := range all {
		if p.Url == "" || p.Request == nil || p.Request.Auth == nil || p.Request.Auth.Message == nil {
			log.Debug("Ignoring malformed builder entry")
			continue
		}
		prefs = append(prefs, p)
	}
	return prefs, nil
}

func (p *builderEntryJson) toConsensus() (*eth.BuilderPreferenceV1, error) {
	if p == nil || p.Url == "" {
		return nil, errors.New("missing url")
	}
	if p.Auth == nil || p.Auth.Message == nil {
		return nil, errors.New("missing auth")
	}
	auth, err := p.Auth.toConsensus()
	if err != nil {
		return nil, err
	}
	out := &eth.BuilderPreferenceV1{
		Url:     p.Url,
		Request: &eth.BuilderPreferencesRequestV1{Preferences: &eth.BuilderPreferencesV1{}, Auth: auth},
	}
	if p.Pubkey != "" {
		pk, err := hexutil.Decode(p.Pubkey)
		if err != nil || len(pk) != fieldparams.BLSPubkeyLength {
			return nil, errors.New("pubkey is not a valid BLS public key")
		}
		out.Pubkey = pk
	}
	if p.MaxExecutionPayment != "" {
		v, err := strconv.ParseUint(p.MaxExecutionPayment, 10, 64)
		if err != nil {
			return nil, errors.New("max_execution_payment is not a valid uint64")
		}
		out.Request.Preferences.MaxExecutionPayment = primitives.Gwei(v)
	}
	if p.MinBid != "" {
		v, err := strconv.ParseUint(p.MinBid, 10, 64)
		if err != nil {
			return nil, errors.New("min_bid is not a valid uint64")
		}
		g := primitives.Gwei(v)
		out.MinBid = &g
	}
	if p.BuilderBoostFactor != "" {
		v, err := strconv.ParseUint(p.BuilderBoostFactor, 10, 64)
		if err != nil {
			return nil, errors.New("builder_boost_factor is not a valid uint64")
		}
		out.BuilderBoostFactor = &v
	}
	return out, nil
}

func (a *signedAuthJson) toConsensus() (*eth.SignedRequestAuthV1, error) {
	data, err := hexutil.Decode(a.Message.Data)
	if err != nil {
		return nil, errors.New("auth.message.data is not valid hex")
	}
	slot, err := strconv.ParseUint(a.Message.Slot, 10, 64)
	if err != nil {
		return nil, errors.New("auth.message.slot is not a valid uint64")
	}
	sig, err := hexutil.Decode(a.Signature)
	if err != nil {
		return nil, errors.New("auth.signature is not valid hex")
	}
	return &eth.SignedRequestAuthV1{
		Message:   &eth.RequestAuthV1{Data: data, Slot: primitives.Slot(slot)},
		Signature: sig,
	}, nil
}

// builderPreferenceEntryJson is one element of the ahead-of-time preferences POST
// body (beacon-APIs #630): url, required auth, and the signed max payment.
type builderPreferenceEntryJson struct {
	Url                 string          `json:"url"`
	Proxy               string          `json:"proxy,omitempty"`
	Auth                *signedAuthJson `json:"auth"`
	MaxExecutionPayment string          `json:"max_execution_payment"`
}

// SubmitBuilderPreferences implements POST /eth/v1/validator/builder_preferences/{pubkey}:
// it forwards each entry's signed preferences to its builder ahead of the proposal slot.
func (s *Server) SubmitBuilderPreferences(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.SubmitBuilderPreferences")
	defer span.End()

	if shared.IsSyncing(ctx, w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}
	_, pubkey, ok := shared.HexFromRoute(w, r, "pubkey", fieldparams.BLSPubkeyLength)
	if !ok {
		return
	}
	var entries []*builderPreferenceEntryJson
	if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
		httputil.HandleError(w, "Could not decode builder preference entries: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(entries) == 0 {
		httputil.HandleError(w, "No builder preference entries submitted", http.StatusBadRequest)
		return
	}

	var failures []*server.IndexedError
	for i, e := range entries {
		req, err := e.toSubmitRequest(pubkey)
		if err == nil {
			_, err = s.V1Alpha1Server.SubmitBuilderPreferences(ctx, req)
		}
		if err != nil {
			failures = append(failures, &server.IndexedError{Index: i, Message: err.Error()})
		}
	}
	if len(failures) > 0 {
		httputil.WriteError(w, &server.IndexedErrorContainer{
			Code:     http.StatusBadRequest,
			Message:  "Errors with one or more preference submissions; well-formed entries were still submitted",
			Failures: failures,
		})
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (e *builderPreferenceEntryJson) toSubmitRequest(pubkey []byte) (*eth.SubmitBuilderPreferencesRequest, error) {
	if e == nil || e.Url == "" {
		return nil, errors.New("missing url")
	}
	if e.Auth == nil || e.Auth.Message == nil {
		return nil, errors.New("missing auth")
	}
	auth, err := e.Auth.toConsensus()
	if err != nil {
		return nil, err
	}
	maxPayment, err := strconv.ParseUint(e.MaxExecutionPayment, 10, 64)
	if err != nil {
		return nil, errors.New("max_execution_payment is not a valid uint64")
	}
	return &eth.SubmitBuilderPreferencesRequest{
		ValidatorPubkey: pubkey,
		Url:             e.Url,
		Request: &eth.BuilderPreferencesRequestV1{
			Preferences: &eth.BuilderPreferencesV1{MaxExecutionPayment: primitives.Gwei(maxPayment)},
			Auth:        auth,
		},
	}, nil
}
