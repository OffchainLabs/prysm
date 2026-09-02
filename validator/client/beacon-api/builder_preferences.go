package beacon_api

import (
	"context"
	"encoding/json"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

const builderPreferencesEndpoint = "/eth/v1/validator/builder_preferences"

func (c *beaconApiValidatorClient) submitBuilderPreferences(ctx context.Context, entries []*ethpb.BuilderPreferencesEntry) error {
	for i, e := range entries {
		if e == nil {
			return errors.Errorf("builder preferences entry at index %d is nil", i)
		}
	}

	headers := map[string]string{api.VersionHeader: version.String(version.Gloas)}

	sszFn := func() ([]byte, error) {
		return marshalBuilderPreferencesEntriesSSZ(entries)
	}
	jsonFn := func() ([]byte, error) {
		return marshalBuilderPreferencesEntriesJSON(entries)
	}

	return c.handler.PostSSZWithFallback(
		ctx,
		builderPreferencesEndpoint,
		headers,
		sszFn,
		jsonFn,
	)
}

// marshalBuilderPreferencesEntriesSSZ encodes entries as the SSZ
// List[BuilderPreferencesEntry]: an offset table followed by the variable-size elements.
func marshalBuilderPreferencesEntriesSSZ(entries []*ethpb.BuilderPreferencesEntry) ([]byte, error) {
	elements := make([][]byte, len(entries))
	for i, e := range entries {
		b, err := e.MarshalSSZ()
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal builder preferences entry ssz")
		}
		elements[i] = b
	}
	return ssz.MarshalVariableList(elements...), nil
}

func marshalBuilderPreferencesEntriesJSON(entries []*ethpb.BuilderPreferencesEntry) ([]byte, error) {
	jsonEntries := make([]*structs.BuilderPreferencesEntry, len(entries))
	for i, e := range entries {
		jsonEntries[i] = structs.BuilderPreferencesEntryFromConsensus(e)
	}
	body, err := json.Marshal(jsonEntries)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal builder preferences entries")
	}
	return body, nil
}
