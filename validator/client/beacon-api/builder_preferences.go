package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

// builderEntryReq is one element of the produceBlockV4 POST body (beacon-APIs
// #630): a bare array of per-builder entries.
type builderEntryReq struct {
	Url                 string         `json:"url"`
	Auth                *signedAuthReq `json:"auth"`
	Pubkey              string         `json:"pubkey,omitempty"`
	MaxExecutionPayment string         `json:"max_execution_payment,omitempty"`
	MinBid              string         `json:"min_bid,omitempty"`
	BuilderBoostFactor  string         `json:"builder_boost_factor,omitempty"`
}

// builderPreferenceEntryReq is one element of the ahead-of-time preferences POST
// body for /eth/v1/validator/builder_preferences/{pubkey}.
type builderPreferenceEntryReq struct {
	Url                 string         `json:"url"`
	Auth                *signedAuthReq `json:"auth"`
	MaxExecutionPayment string         `json:"max_execution_payment"`
}

type signedAuthReq struct {
	Message   *requestAuthReq `json:"message"`
	Signature string          `json:"signature"`
}

type requestAuthReq struct {
	Data string `json:"data"`
	Slot string `json:"slot"`
}

func signedAuthFromConsensus(auth *ethpb.SignedRequestAuthV1) *signedAuthReq {
	return &signedAuthReq{
		Message: &requestAuthReq{
			Data: hexutil.Encode(auth.Message.Data),
			Slot: strconv.FormatUint(uint64(auth.Message.Slot), 10),
		},
		Signature: hexutil.Encode(auth.Signature),
	}
}

// marshalBuilderPreferences encodes inline builder preferences into the #630
// produce-block body: a bare array of builder entries.
func marshalBuilderPreferences(prefs []*ethpb.BuilderPreferenceV1) ([]byte, error) {
	entries := make([]*builderEntryReq, 0, len(prefs))
	for _, p := range prefs {
		if p == nil || p.Request == nil || p.Request.Auth == nil || p.Request.Auth.Message == nil {
			continue
		}
		entry := &builderEntryReq{
			Url:                 p.Url,
			Auth:                signedAuthFromConsensus(p.Request.Auth),
			MaxExecutionPayment: strconv.FormatUint(uint64(p.Request.Preferences.GetMaxExecutionPayment()), 10),
		}
		if len(p.Pubkey) > 0 {
			entry.Pubkey = hexutil.Encode(p.Pubkey)
		}
		if p.MinBid != nil {
			entry.MinBid = strconv.FormatUint(uint64(p.GetMinBid()), 10)
		}
		if p.BuilderBoostFactor != nil {
			entry.BuilderBoostFactor = strconv.FormatUint(p.GetBuilderBoostFactor(), 10)
		}
		entries = append(entries, entry)
	}
	return json.Marshal(entries)
}

// submitBuilderPreferences pushes one signed builder preference ahead of the
// proposal slot via POST /eth/v1/validator/builder_preferences/{pubkey}.
func (c *beaconApiValidatorClient) submitBuilderPreferences(ctx context.Context, in *ethpb.SubmitBuilderPreferencesRequest) error {
	if in == nil || in.Request == nil || in.Request.Auth == nil || in.Request.Auth.Message == nil {
		return errors.New("builder preferences request is empty")
	}
	body, err := json.Marshal([]*builderPreferenceEntryReq{{
		Url:                 in.Url,
		Auth:                signedAuthFromConsensus(in.Request.Auth),
		MaxExecutionPayment: strconv.FormatUint(uint64(in.Request.Preferences.GetMaxExecutionPayment()), 10),
	}})
	if err != nil {
		return errors.Wrap(err, "could not marshal builder preference entry")
	}
	return c.handler.Post(ctx, "/eth/v1/validator/builder_preferences/"+hexutil.Encode(in.ValidatorPubkey), nil, bytes.NewBuffer(body), nil)
}
