package validator

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api/server"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SubmitBuilderPreferences forwards per-builder preference entries to their builders
// ahead of block production. Endpoint: POST /eth/v1/validator/builder_preferences
func (s *Server) SubmitBuilderPreferences(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.SubmitBuilderPreferences")
	defer span.End()

	if !requireGloasVersionHeader(w, r, "Builder preferences are only supported from the gloas fork") {
		return
	}

	var (
		entries   []*eth.BuilderPreferencesEntry
		failures  []*server.IndexedError
		decodeErr error
	)
	if httputil.IsRequestSsz(r) {
		entries, failures, decodeErr = decodeBuilderPreferencesEntriesSSZ(r.Body)
	} else {
		entries, failures, decodeErr = decodeBuilderPreferencesEntriesJSON(r.Body)
	}
	if decodeErr != nil {
		if errors.Is(decodeErr, io.EOF) {
			httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		} else {
			httputil.HandleError(w, "Could not decode request body: "+decodeErr.Error(), http.StatusBadRequest)
		}
		return
	}
	if len(failures) > 0 {
		httputil.WriteError(w, &server.IndexedErrorContainer{
			Code:     http.StatusBadRequest,
			Message:  server.ErrIndexedValidationFail,
			Failures: failures,
		})
		return
	}
	if len(entries) == 0 {
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	}

	req := &eth.SubmitBuilderPreferencesRequest{Entries: entries}
	if _, err := s.V1Alpha1Server.SubmitBuilderPreferences(ctx, req); err != nil {
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.InvalidArgument:
				httputil.HandleError(w, st.Message(), http.StatusBadRequest)
			case codes.Unavailable:
				httputil.HandleError(w, st.Message(), http.StatusServiceUnavailable)
			default:
				httputil.HandleError(w, st.Message(), http.StatusInternalServerError)
			}
			return
		}
		httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// decodeBuilderPreferencesEntriesJSON decodes a JSON array of BuilderPreferencesEntry.
func decodeBuilderPreferencesEntriesJSON(r io.Reader) ([]*eth.BuilderPreferencesEntry, []*server.IndexedError, error) {
	var data []*structs.BuilderPreferencesEntry
	if err := json.NewDecoder(r).Decode(&data); err != nil {
		return nil, nil, err
	}
	if len(data) > structs.MaxBuilderPreferencesList {
		return nil, nil, errors.Errorf("more than %d entries", structs.MaxBuilderPreferencesList)
	}
	entries := make([]*eth.BuilderPreferencesEntry, len(data))
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
		entries[i] = consensusItem
	}
	return entries, failures, nil
}

// decodeBuilderPreferencesEntriesSSZ decodes an SSZ List[BuilderPreferencesEntry].
func decodeBuilderPreferencesEntriesSSZ(r io.Reader) ([]*eth.BuilderPreferencesEntry, []*server.IndexedError, error) {
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
	entries := make([]*eth.BuilderPreferencesEntry, len(elements))
	var failures []*server.IndexedError
	for i, elem := range elements {
		e := &eth.BuilderPreferencesEntry{}
		if err := e.UnmarshalSSZ(elem); err != nil {
			failures = append(failures, &server.IndexedError{Index: i, Message: "Could not decode SSZ message: " + err.Error()})
			continue
		}
		entries[i] = e
	}
	return entries, failures, nil
}
