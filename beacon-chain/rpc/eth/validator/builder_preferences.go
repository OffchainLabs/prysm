package validator

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

// produceBlockV4Request is the POST body for the JIT produce-block endpoint
// (beacon-APIs #625): the proposer's per-builder preferences for this proposal.
type produceBlockV4Request struct {
	BuilderPreferences []*builderPreferenceJson `json:"builder_preferences"`
}

type builderPreferenceJson struct {
	Url                 string                 `json:"url"`
	SignedRequestAuth   *signedRequestAuthJson `json:"signed_request_auth"`
	MaxExecutionPayment string                 `json:"max_execution_payment"`
	MinBid              string                 `json:"min_bid,omitempty"`
	BuilderBoostFactor  string                 `json:"builder_boost_factor,omitempty"`
}

type signedRequestAuthJson struct {
	Message   *requestAuthJson `json:"message"`
	Signature string           `json:"signature"`
}

type requestAuthJson struct {
	Data string `json:"data"`
	Slot string `json:"slot"`
}

// parseBuilderPreferencesBody decodes the JIT builder-preferences POST body into
// consensus objects. An empty body yields no preferences.
func parseBuilderPreferencesBody(r *http.Request) ([]*eth.BuilderPreferenceV1, error) {
	if r.Body == nil {
		return nil, nil
	}
	var req produceBlockV4Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "could not decode builder preferences")
	}
	out := make([]*eth.BuilderPreferenceV1, 0, len(req.BuilderPreferences))
	for i, p := range req.BuilderPreferences {
		conv, err := p.toConsensus(i)
		if err != nil {
			return nil, err
		}
		out = append(out, conv)
	}
	return out, nil
}

func (p *builderPreferenceJson) toConsensus(i int) (*eth.BuilderPreferenceV1, error) {
	if p == nil || p.SignedRequestAuth == nil || p.SignedRequestAuth.Message == nil {
		return nil, errors.Errorf("builder_preferences[%d] missing signed_request_auth", i)
	}
	maxPayment, err := strconv.ParseUint(p.MaxExecutionPayment, 10, 64)
	if err != nil {
		return nil, errors.Errorf("builder_preferences[%d].max_execution_payment is not a valid uint64", i)
	}
	data, err := hexutil.Decode(p.SignedRequestAuth.Message.Data)
	if err != nil {
		return nil, errors.Errorf("builder_preferences[%d].signed_request_auth.message.data is not valid hex", i)
	}
	slot, err := strconv.ParseUint(p.SignedRequestAuth.Message.Slot, 10, 64)
	if err != nil {
		return nil, errors.Errorf("builder_preferences[%d].signed_request_auth.message.slot is not a valid uint64", i)
	}
	sig, err := hexutil.Decode(p.SignedRequestAuth.Signature)
	if err != nil {
		return nil, errors.Errorf("builder_preferences[%d].signed_request_auth.signature is not valid hex", i)
	}
	out := &eth.BuilderPreferenceV1{
		Url: p.Url,
		Request: &eth.BuilderPreferencesRequestV1{
			Preferences: &eth.BuilderPreferencesV1{MaxExecutionPayment: primitives.Gwei(maxPayment)},
			Auth: &eth.SignedRequestAuthV1{
				Message:   &eth.RequestAuthV1{Data: data, Slot: primitives.Slot(slot)},
				Signature: sig,
			},
		},
	}
	if p.MinBid != "" {
		v, err := strconv.ParseUint(p.MinBid, 10, 64)
		if err != nil {
			return nil, errors.Errorf("builder_preferences[%d].min_bid is not a valid uint64", i)
		}
		g := primitives.Gwei(v)
		out.MinBid = &g
	}
	if p.BuilderBoostFactor != "" {
		v, err := strconv.ParseUint(p.BuilderBoostFactor, 10, 64)
		if err != nil {
			return nil, errors.Errorf("builder_preferences[%d].builder_boost_factor is not a valid uint64", i)
		}
		out.BuilderBoostFactor = &v
	}
	return out, nil
}
