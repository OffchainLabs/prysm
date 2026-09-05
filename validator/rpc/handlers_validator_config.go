package rpc

import (
	"encoding/json"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/shared"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
)

// requireGloasScheduled rejects builders configuration on networks that never
// activate it; the endpoints are meaningless without a gloas fork epoch.
func requireGloasScheduled(w http.ResponseWriter) bool {
	if params.GloasEnabled() {
		return true
	}
	httputil.HandleError(w, "Builders configuration requires a network with the gloas fork scheduled", http.StatusNotImplemented)
	return false
}

// GetBuilderConfig implements GET /eth/v1/validator/{pubkey}/builder_config: the key's config resolved against default_config, safe to re-submit.
func (s *Server) GetBuilderConfig(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "validator.keymanagerAPI.GetBuilderConfig")
	defer span.End()

	if !requireGloasScheduled(w) {
		return
	}
	if s.validatorService == nil {
		httputil.HandleError(w, "Validator service not ready.", http.StatusServiceUnavailable)
		return
	}
	_, pubkey, ok := shared.HexFromRoute(w, r, "pubkey", fieldparams.BLSPubkeyLength)
	if !ok {
		return
	}

	// The Effective* getters resolve unset values (no floor, neutral boost, trustless-only).
	eff := s.validatorService.ProposerSettings().EffectiveBuilderConfig(bytesutil.ToBytes48(pubkey))
	if eff == nil {
		eff = &proposer.BuilderConfig{}
	}
	out := builderConfigFromConsensus(eff)
	// The resolved response always states a concrete builders list.
	if out.Builders == nil {
		out.Builders = []*BuilderEntry{}
	}
	httputil.WriteJson(w, &GetBuilderConfigResponse{Data: out})
}

// SetBuilderConfig implements POST /eth/v1/validator/{pubkey}/builder_config, replacing the
// key's builder config in full; fee recipient, gas limit and graffiti are untouched.
func (s *Server) SetBuilderConfig(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.keymanagerAPI.SetBuilderConfig")
	defer span.End()

	if !requireGloasScheduled(w) {
		return
	}
	if s.validatorService == nil {
		httputil.HandleError(w, "Validator service not ready.", http.StatusServiceUnavailable)
		return
	}
	_, pubkey, ok := shared.HexFromRoute(w, r, "pubkey", fieldparams.BLSPubkeyLength)
	if !ok {
		return
	}

	var body BuilderConfig
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.HandleError(w, "Could not decode builder config: "+err.Error(), http.StatusBadRequest)
		return
	}
	bc, err := body.ToConsensus()
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Per spec an override-free body ({}) configures nothing: store no per-key
	// builder config, so the key keeps following the client defaults.
	if bc.Builders == nil && bc.MinBid == nil && bc.BuilderBoostFactor == nil {
		bc = nil
	}

	err = s.validatorService.UpdateProposerSettings(ctx, func(settings *proposer.Settings) (*proposer.Settings, error) {
		if settings == nil {
			settings = &proposer.Settings{Version: proposer.FreshSettingsVersion()}
		} else {
			// Stamp the storage format only: semantics are fork-keyed, so writing
			// builders never drops v1 content or changes other keys' behavior.
			settings.Version = proposer.SchemaV2
		}
		if settings.ProposeConfig == nil {
			settings.ProposeConfig = make(map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option)
		}
		key := bytesutil.ToBytes48(pubkey)
		opt := settings.ProposeConfig[key]
		if opt == nil {
			opt = &proposer.Option{}
			settings.ProposeConfig[key] = opt
		}
		opt.BuilderConfig = bc
		return settings, nil
	})
	if err != nil {
		httputil.HandleError(w, "Could not set proposer settings: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// DeleteBuilderConfig implements DELETE /eth/v1/validator/{pubkey}/builder_config: the key
// follows the validator client defaults again; not the same as builders: [].
func (s *Server) DeleteBuilderConfig(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.keymanagerAPI.DeleteBuilderConfig")
	defer span.End()

	if !requireGloasScheduled(w) {
		return
	}
	if s.validatorService == nil {
		httputil.HandleError(w, "Validator service not ready.", http.StatusServiceUnavailable)
		return
	}
	_, pubkey, ok := shared.HexFromRoute(w, r, "pubkey", fieldparams.BLSPubkeyLength)
	if !ok {
		return
	}

	err := s.validatorService.UpdateProposerSettings(ctx, func(settings *proposer.Settings) (*proposer.Settings, error) {
		// Removing an absent configuration succeeds; the key already follows the defaults.
		if settings == nil || settings.ProposeConfig == nil {
			return nil, nil
		}
		opt, found := settings.ProposeConfig[bytesutil.ToBytes48(pubkey)]
		if !found || opt == nil || opt.BuilderConfig == nil {
			return nil, nil
		}
		opt.BuilderConfig = nil
		return settings, nil
	})
	if err != nil {
		httputil.HandleError(w, "Could not set proposer settings: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
