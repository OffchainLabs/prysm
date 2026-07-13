package validator

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpbalpha "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SubmitBuilderPreferences forwards a proposer's per-builder preferences to the configured builder.
// Delegates to the gRPC server so the forwarding and caching logic stays in one place.
func (s *Server) SubmitBuilderPreferences(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.SubmitBuilderPreferences")
	defer span.End()

	versionHeader := r.Header.Get(api.VersionHeader)
	if versionHeader == "" {
		httputil.HandleError(w, api.VersionHeader+" header is required", http.StatusBadRequest)
		return
	}
	v, err := version.FromString(versionHeader)
	if err != nil {
		httputil.HandleError(w, "Invalid version: "+err.Error(), http.StatusBadRequest)
		return
	}
	if v < version.Gloas {
		httputil.HandleError(w, "Builder preferences are only supported from the gloas fork", http.StatusBadRequest)
		return
	}

	pubkey, err := bytesutil.DecodeHexWithLength(r.PathValue("pubkey"), fieldparams.BLSPubkeyLength)
	if err != nil {
		httputil.HandleError(w, "Invalid pubkey: "+err.Error(), http.StatusBadRequest)
		return
	}

	prefReq, err := decodeBuilderPreferencesRequest(r)
	if err != nil {
		if errors.Is(err, io.EOF) {
			httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		} else {
			httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		}
		return
	}

	req := &ethpbalpha.SubmitBuilderPreferencesRequest{ValidatorPubkey: pubkey, Request: prefReq}
	if _, err := s.V1Alpha1Server.SubmitBuilderPreferences(ctx, req); err != nil {
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.InvalidArgument:
				httputil.HandleError(w, st.Message(), http.StatusBadRequest)
			case codes.FailedPrecondition, codes.Unavailable:
				httputil.HandleError(w, st.Message(), http.StatusServiceUnavailable)
			default:
				httputil.HandleError(w, st.Message(), http.StatusInternalServerError)
			}
			return
		}
		httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
	}
}

func decodeBuilderPreferencesRequest(r *http.Request) (*ethpbalpha.BuilderPreferencesRequestV1, error) {
	if httputil.IsRequestSsz(r) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, errors.Wrap(err, "could not read request body")
		}
		if len(body) == 0 {
			return nil, io.EOF
		}
		m := &ethpbalpha.BuilderPreferencesRequestV1{}
		if err := m.UnmarshalSSZ(body); err != nil {
			return nil, errors.Wrap(err, "could not decode SSZ message")
		}
		return m, nil
	}
	var data structs.BuilderPreferencesRequestV1
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.ToConsensus()
}
