package validator

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

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
// of per-builder entries (beacon-APIs #630). proxy and pubkey are accepted but
// currently unused.
type builderEntryJson struct {
	Url                 string          `json:"url"`
	Proxy               string          `json:"proxy,omitempty"`
	Pubkey              string          `json:"pubkey,omitempty"`
	Auth                *signedAuthJson `json:"auth"`
	MaxExecutionPayment string          `json:"max_execution_payment,omitempty"`
	MinBid              string          `json:"min_bid,omitempty"`
	BuilderBoostFactor  string          `json:"builder_boost_factor,omitempty"`
}

type signedAuthJson struct {
	Message   *requestAuthJson `json:"message"`
	Signature string           `json:"signature"`
}

type requestAuthJson struct {
	Data string `json:"data"`
	Slot string `json:"slot"`
}

// parseBuilderPreferencesBody decodes the JIT produce-block POST body. Malformed
// entries are ignored per the spec; an empty body yields no preferences.
func parseBuilderPreferencesBody(r *http.Request) ([]*eth.BuilderPreferenceV1, error) {
	if r.Body == nil {
		return nil, nil
	}
	var entries []*builderEntryJson
	if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "could not decode builder preferences")
	}
	out := make([]*eth.BuilderPreferenceV1, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	for _, p := range entries {
		conv, err := p.toConsensus()
		if err != nil {
			log.WithError(err).Debug("Ignoring malformed builder entry")
			continue
		}
		// Duplicate urls invalidate the whole request, unlike malformed entries.
		if seen[conv.Url] {
			return nil, errors.Errorf("two builder entries share the same url")
		}
		seen[conv.Url] = true
		out = append(out, conv)
	}
	return out, nil
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
			Message:  "One or more preference submissions were rejected",
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
