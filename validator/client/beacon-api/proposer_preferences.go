package beacon_api

import (
	"context"
	"encoding/json"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
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

	sszFn := func() ([]byte, error) {
		return marshalSignedProposerPreferencesSSZ(prefs)
	}
	jsonFn := func() ([]byte, error) {
		return marshalSignedProposerPreferencesJSON(prefs)
	}

	return c.handler.PostSSZWithFallback(
		ctx,
		proposerPreferencesEndpoint,
		headers,
		sszFn,
		jsonFn,
	)
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
