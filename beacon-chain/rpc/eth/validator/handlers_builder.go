package validator

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/builder"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

// requireGloasVersionHeader validates the consensus version request header, writing
// the error response and returning false when it is absent, invalid, or pre-gloas.
func requireGloasVersionHeader(w http.ResponseWriter, r *http.Request, preGloasMsg string) bool {
	versionHeader := r.Header.Get(api.VersionHeader)
	if versionHeader == "" {
		httputil.HandleError(w, api.VersionHeader+" header is required", http.StatusBadRequest)
		return false
	}
	v, err := version.FromString(versionHeader)
	if err != nil {
		httputil.HandleError(w, "Invalid version: "+err.Error(), http.StatusBadRequest)
		return false
	}
	if v < version.Gloas {
		httputil.HandleError(w, preGloasMsg, http.StatusBadRequest)
		return false
	}
	return true
}

// SubmitBuilderPreferences forwards per-builder preference entries to their builders
// ahead of block production. Endpoint: POST /eth/v1/validator/builder_preferences
func (s *Server) SubmitBuilderPreferences(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.SubmitBuilderPreferences")
	defer span.End()

	if !requireGloasVersionHeader(w, r, "Builder preferences are only supported from the gloas fork") {
		return
	}

	var (
		decoded   []indexedPreferenceEntry
		failures  []*server.IndexedError
		decodeErr error
	)
	if httputil.IsRequestSsz(r) {
		decoded, failures, decodeErr = decodeBuilderPreferencesEntriesSSZ(r.Body)
	} else {
		decoded, failures, decodeErr = decodeBuilderPreferencesEntriesJSON(r.Body)
	}
	if decodeErr != nil {
		if errors.Is(decodeErr, io.EOF) {
			httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		} else {
			httputil.HandleError(w, "Could not decode request body: "+decodeErr.Error(), http.StatusBadRequest)
		}
		return
	}
	if len(decoded) == 0 && len(failures) == 0 {
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	}
	if len(decoded) > 0 {
		// Preconditions mean nothing was submitted, so they outrank per-entry reporting.
		if s.SyncChecker.Syncing() {
			httputil.HandleError(w, "Syncing to latest head, not ready to respond", http.StatusServiceUnavailable)
			return
		}
		// Not gated on Configured(), gloas builders are dialed per URL from the request rather than the endpoint flag.
		if s.BlockBuilder == nil {
			httputil.HandleError(w, "Builder is not configured", http.StatusInternalServerError)
			return
		}
		entries := make([]*eth.BuilderPreferencesEntry, len(decoded))
		for i, d := range decoded {
			entries[i] = d.entry
		}
		for pos, msg := range builder.SubmitPreferenceEntries(ctx, s.BlockBuilder, entries) {
			failures = append(failures, &server.IndexedError{Index: decoded[pos].index, Message: msg})
		}
	}
	// Well-formed entries were still submitted when others failed, per the spec's 400.
	if len(failures) > 0 {
		sort.Slice(failures, func(a, b int) bool { return failures[a].Index < failures[b].Index })
		httputil.WriteError(w, &server.IndexedErrorContainer{
			Code:     http.StatusBadRequest,
			Message:  server.ErrIndexedValidationFail,
			Failures: failures,
		})
		return
	}
}

// indexedPreferenceEntry pairs a decoded entry with its index in the request body.
type indexedPreferenceEntry struct {
	index int
	entry *eth.BuilderPreferencesEntry
}

// decodeBuilderPreferencesEntriesJSON decodes a JSON array of BuilderPreferencesEntry.
func decodeBuilderPreferencesEntriesJSON(r io.Reader) ([]indexedPreferenceEntry, []*server.IndexedError, error) {
	var data []*structs.BuilderPreferencesEntry
	if err := json.NewDecoder(r).Decode(&data); err != nil {
		return nil, nil, err
	}
	if len(data) > structs.MaxBuilderPreferencesList {
		return nil, nil, errors.Errorf("more than %d entries", structs.MaxBuilderPreferencesList)
	}
	entries := make([]indexedPreferenceEntry, 0, len(data))
	var failures []*server.IndexedError
	for i, item := range data {
		if item == nil {
			failures = append(failures, &server.IndexedError{Index: i, Message: "Entry is empty"})
			continue
		}
		consensusItem, err := item.ToConsensus()
		if err != nil {
			failures = append(failures, &server.IndexedError{Index: i, Message: err.Error()})
			continue
		}
		entries = append(entries, indexedPreferenceEntry{index: i, entry: consensusItem})
	}
	return entries, failures, nil
}

// decodeBuilderPreferencesEntriesSSZ decodes an SSZ List[BuilderPreferencesEntry].
func decodeBuilderPreferencesEntriesSSZ(r io.Reader) ([]indexedPreferenceEntry, []*server.IndexedError, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not read request body")
	}
	if len(body) == 0 {
		return nil, nil, io.EOF
	}
	elements, err := ssz.SplitVariableList(body, structs.MaxBuilderPreferencesList)
	if err != nil {
		return nil, nil, err
	}
	entries := make([]indexedPreferenceEntry, 0, len(elements))
	var failures []*server.IndexedError
	for i, elem := range elements {
		e := &eth.BuilderPreferencesEntry{}
		if err := e.UnmarshalSSZ(elem); err != nil {
			failures = append(failures, &server.IndexedError{Index: i, Message: "Could not decode SSZ message: " + err.Error()})
			continue
		}
		entries = append(entries, indexedPreferenceEntry{index: i, entry: e})
	}
	return entries, failures, nil
}
