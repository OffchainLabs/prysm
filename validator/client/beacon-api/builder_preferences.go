package beacon_api

import (
	"encoding/json"
	"strconv"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// JIT builder-preferences POST body for produceBlockV4 (beacon-APIs #625).
type builderPreferencesBody struct {
	BuilderPreferences []*builderPreferenceReq `json:"builder_preferences"`
}

type builderPreferenceReq struct {
	SignedRequestAuth   *signedRequestAuthReq `json:"signed_request_auth"`
	MaxExecutionPayment string                `json:"max_execution_payment"`
	MinBid              string                `json:"min_bid,omitempty"`
	BuilderBoostFactor  string                `json:"builder_boost_factor,omitempty"`
}

type signedRequestAuthReq struct {
	Message   *requestAuthReq `json:"message"`
	Signature string          `json:"signature"`
}

type requestAuthReq struct {
	Data string `json:"data"`
	Slot string `json:"slot"`
}

// marshalBuilderPreferences encodes inline builder preferences into the #625 JSON body.
func marshalBuilderPreferences(prefs []*ethpb.BuilderPreferenceV1) ([]byte, error) {
	body := &builderPreferencesBody{BuilderPreferences: make([]*builderPreferenceReq, 0, len(prefs))}
	for _, p := range prefs {
		if p == nil || p.Request == nil || p.Request.Auth == nil || p.Request.Auth.Message == nil {
			continue
		}
		req := &builderPreferenceReq{
			MaxExecutionPayment: strconv.FormatUint(uint64(p.Request.Preferences.GetMaxExecutionPayment()), 10),
			SignedRequestAuth: &signedRequestAuthReq{
				Message: &requestAuthReq{
					Data: hexutil.Encode(p.Request.Auth.Message.Data),
					Slot: strconv.FormatUint(uint64(p.Request.Auth.Message.Slot), 10),
				},
				Signature: hexutil.Encode(p.Request.Auth.Signature),
			},
		}
		if p.MinBid != nil {
			req.MinBid = strconv.FormatUint(uint64(p.GetMinBid()), 10)
		}
		if p.BuilderBoostFactor != nil {
			req.BuilderBoostFactor = strconv.FormatUint(p.GetBuilderBoostFactor(), 10)
		}
		body.BuilderPreferences = append(body.BuilderPreferences, req)
	}
	return json.Marshal(body)
}
