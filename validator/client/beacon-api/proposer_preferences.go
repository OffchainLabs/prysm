package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

const proposerPreferencesEndpoint = "/eth/v1/validator/proposer_preferences"

func (c *beaconApiValidatorClient) submitSignedProposerPreferences(ctx context.Context, prefs []*ethpb.SignedProposerPreferences) error {
	for i, p := range prefs {
		if p == nil || p.Message == nil {
			return errors.Errorf("signed proposer preferences at index %d is nil", i)
		}
	}

	headers := map[string]string{api.VersionHeader: version.String(version.Gloas)}

	// Prefer SSZ; fall back to JSON if the beacon node does not accept octet-stream request bodies.
	sszBody, err := marshalSignedProposerPreferencesSSZ(prefs)
	if err != nil {
		return err
	}
	if _, _, err = c.handler.PostSSZ(ctx, proposerPreferencesEndpoint, headers, bytes.NewBuffer(sszBody)); err == nil {
		return nil
	}
	errJson := &httputil.DefaultJsonError{}
	if !errors.As(err, &errJson) || errJson.Code != http.StatusUnsupportedMediaType {
		return err
	}
	log.WithError(err).Warn("Beacon node does not accept SSZ proposer preferences, falling back to JSON")

	jsonBody, err := marshalSignedProposerPreferencesJSON(prefs)
	if err != nil {
		return err
	}
	return c.handler.Post(ctx, proposerPreferencesEndpoint, headers, bytes.NewBuffer(jsonBody), nil)
}

// marshalSignedProposerPreferencesSSZ encodes prefs as the SSZ List[SignedProposerPreferences],
// a concatenation of the fixed-size elements.
func marshalSignedProposerPreferencesSSZ(prefs []*ethpb.SignedProposerPreferences) ([]byte, error) {
	var body []byte
	for _, p := range prefs {
		b, err := p.MarshalSSZ()
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal signed proposer preferences ssz")
		}
		body = append(body, b...)
	}
	return body, nil
}

func marshalSignedProposerPreferencesJSON(prefs []*ethpb.SignedProposerPreferences) ([]byte, error) {
	jsonPrefs := make([]*structs.SignedProposerPreferences, len(prefs))
	for i, p := range prefs {
		jsonPrefs[i] = structs.SignedProposerPreferencesFromConsensus(p)
	}
	body, err := json.Marshal(jsonPrefs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal signed proposer preferences")
	}
	return body, nil
}
