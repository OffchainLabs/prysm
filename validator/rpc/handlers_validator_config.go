package rpc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/shared"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

const maxBuilderEntries = 64

// GetBuilders implements GET /eth/v1/validator/{pubkey}/builders (keymanager-APIs #88).
// It returns the key's builder configuration resolved against default_config so
// re-submitting the response reproduces the same behavior.
func (s *Server) GetBuilders(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "validator.keymanagerAPI.GetBuilders")
	defer span.End()

	if s.validatorService == nil {
		httputil.HandleError(w, "Validator service not ready.", http.StatusServiceUnavailable)
		return
	}
	_, pubkey, ok := shared.HexFromRoute(w, r, "pubkey", fieldparams.BLSPubkeyLength)
	if !ok {
		return
	}

	eff := resolveBuilders(s.validatorService.ProposerSettings(), bytesutil.ToBytes48(pubkey))
	out := builderConfigJSONFromConsensus(eff)
	// The resolved response always states a concrete builders list.
	if out.Builders == nil {
		out.Builders = []*BuilderEntryJson{}
	}
	for _, e := range out.Builders {
		if e.MinBid == nil {
			e.MinBid = out.MinBid
		}
		if e.BuilderBoostFactor == nil {
			e.BuilderBoostFactor = out.BuilderBoostFactor
		}
	}
	httputil.WriteJson(w, out)
}

// SetBuilders implements POST /eth/v1/validator/{pubkey}/builders. It replaces the
// key's builder configuration in full, leaving fee recipient, gas limit and graffiti
// untouched.
func (s *Server) SetBuilders(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.keymanagerAPI.SetBuilders")
	defer span.End()

	if s.validatorService == nil {
		httputil.HandleError(w, "Validator service not ready.", http.StatusServiceUnavailable)
		return
	}
	_, pubkey, ok := shared.HexFromRoute(w, r, "pubkey", fieldparams.BLSPubkeyLength)
	if !ok {
		return
	}

	var body BuilderConfigJson
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.HandleError(w, "Could not decode builder config: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Enabled == nil {
		httputil.HandleError(w, "enabled is required", http.StatusBadRequest)
		return
	}
	bc, err := builderConfigFromJSON(&body)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.proposerSettingsLock.Lock()
	defer s.proposerSettingsLock.Unlock()

	settings := s.validatorService.ProposerSettings()
	if settings == nil {
		settings = &proposer.Settings{Version: proposer.SchemaV2}
	} else {
		settings = settings.Clone()
		settings.UpgradeToV2()
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

	if err := s.validatorService.SetProposerSettings(ctx, settings); err != nil {
		httputil.HandleError(w, "Could not set proposer settings: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// DeleteBuilders implements DELETE /eth/v1/validator/{pubkey}/builders. It removes
// the key's builder configuration so the key follows the validator client defaults;
// this differs from enabled:false.
func (s *Server) DeleteBuilders(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.keymanagerAPI.DeleteBuilders")
	defer span.End()

	if s.validatorService == nil {
		httputil.HandleError(w, "Validator service not ready.", http.StatusServiceUnavailable)
		return
	}
	rawPubkey, pubkey, ok := shared.HexFromRoute(w, r, "pubkey", fieldparams.BLSPubkeyLength)
	if !ok {
		return
	}

	s.proposerSettingsLock.Lock()
	defer s.proposerSettingsLock.Unlock()

	settings := s.validatorService.ProposerSettings()
	key := bytesutil.ToBytes48(pubkey)
	if settings == nil || settings.ProposeConfig == nil {
		httputil.HandleError(w, fmt.Sprintf("No builder configuration found for pubkey %q", rawPubkey), http.StatusNotFound)
		return
	}
	opt, found := settings.ProposeConfig[key]
	if !found || opt == nil || opt.BuilderConfig == nil {
		httputil.HandleError(w, fmt.Sprintf("No builder configuration found for pubkey %q", rawPubkey), http.StatusNotFound)
		return
	}

	settings = settings.Clone()
	settings.ProposeConfig[key].BuilderConfig = nil
	if err := s.validatorService.SetProposerSettings(ctx, settings); err != nil {
		httputil.HandleError(w, "Could not set proposer settings: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func resolveBuilders(settings *proposer.Settings, key [fieldparams.BLSPubkeyLength]byte) *proposer.BuilderConfig {
	var perKey, def *proposer.BuilderConfig
	if settings != nil {
		if settings.DefaultConfig != nil {
			def = settings.DefaultConfig.BuilderConfig
		}
		if opt, ok := settings.ProposeConfig[key]; ok && opt != nil {
			perKey = opt.BuilderConfig
		}
	}
	if eff := proposer.EffectiveBuilderConfig(perKey, def); eff != nil {
		return eff
	}
	return &proposer.BuilderConfig{}
}

func builderConfigJSONFromConsensus(bc *proposer.BuilderConfig) *BuilderConfigJson {
	enabled := bc.Enabled
	out := &BuilderConfigJson{
		Enabled:            &enabled,
		MinBid:             optUintStrPtr(bc.MinBid),
		BuilderBoostFactor: optUintStrPtr(bc.BuilderBoostFactor),
	}
	if bc.Builders != nil {
		out.Builders = make([]*BuilderEntryJson, 0, len(bc.Builders))
		for _, b := range bc.Builders {
			out.Builders = append(out.Builders, builderEntryJSONFromConsensus(b))
		}
	}
	return out
}

func builderEntryJSONFromConsensus(be *proposer.BuilderEntry) *BuilderEntryJson {
	out := &BuilderEntryJson{
		MaxExecutionPayment: optUintStrPtr(be.MaxExecutionPayment),
		MinBid:              optUintStrPtr(be.MinBid),
		BuilderBoostFactor:  optUintStrPtr(be.BuilderBoostFactor),
	}
	if be.URL != "" {
		out.Url = strPtr(be.URL)
	}
	if len(be.Pubkey) != 0 {
		out.BuilderPubkey = strPtr(hexutil.Encode(be.Pubkey))
	}
	if len(be.AuthData) != 0 {
		out.AuthData = strPtr(hexutil.Encode(be.AuthData))
	}
	return out
}

func builderConfigFromJSON(in *BuilderConfigJson) (*proposer.BuilderConfig, error) {
	bc := &proposer.BuilderConfig{Enabled: in.Enabled != nil && *in.Enabled}
	if in.MinBid != nil {
		v, err := parseUint(*in.MinBid, "min_bid")
		if err != nil {
			return nil, err
		}
		bc.MinBid = &v
	}
	if in.BuilderBoostFactor != nil {
		v, err := parseUint(*in.BuilderBoostFactor, "builder_boost_factor")
		if err != nil {
			return nil, err
		}
		bc.BuilderBoostFactor = &v
	}
	if in.Builders == nil {
		return bc, nil
	}
	if len(in.Builders) > maxBuilderEntries {
		return nil, errors.Errorf("builders exceeds %d entries", maxBuilderEntries)
	}
	// Non-nil (possibly empty) list means "use exactly these builders", not "inherit".
	// Uniqueness is scoped by role: url entries by (url, auth_data),
	// url-less p2p-policy entries by builder_pubkey.
	bc.Builders = make([]*proposer.BuilderEntry, 0, len(in.Builders))
	seenURL := make(map[string]bool, len(in.Builders))
	seenP2P := make(map[string]bool, len(in.Builders))
	for i, entry := range in.Builders {
		be, err := builderEntryFromJSON(entry, i)
		if err != nil {
			return nil, err
		}
		if be.URL != "" {
			key := fmt.Sprintf("%s|%x", be.URL, be.AuthData)
			if seenURL[key] {
				return nil, errors.Errorf("builders[%d]: two entries share the same url and auth_data", i)
			}
			seenURL[key] = true
		} else if seenP2P[string(be.Pubkey)] {
			return nil, errors.Errorf("builders[%d]: two p2p-policy entries share the same builder_pubkey", i)
		} else {
			seenP2P[string(be.Pubkey)] = true
		}
		bc.Builders = append(bc.Builders, be)
	}
	return bc, nil
}

func builderEntryFromJSON(in *BuilderEntryJson, i int) (*proposer.BuilderEntry, error) {
	be := &proposer.BuilderEntry{}
	if in.Url != nil && *in.Url != "" {
		if u, err := url.Parse(*in.Url); err != nil || u.Scheme == "" || u.Host == "" {
			return nil, errors.Errorf("builders[%d].url is not a valid URL", i)
		}
		be.URL = *in.Url
	}
	if in.BuilderPubkey != nil {
		pk, err := hexutil.Decode(*in.BuilderPubkey)
		if err != nil || len(pk) != fieldparams.BLSPubkeyLength {
			return nil, errors.Errorf("builders[%d].builder_pubkey is not a valid BLS public key", i)
		}
		be.Pubkey = pk
	}
	if be.URL == "" && len(be.Pubkey) == 0 {
		return nil, errors.Errorf("builders[%d]: at least one of url and builder_pubkey is required", i)
	}
	if in.AuthData != nil {
		ad, err := hexutil.Decode(*in.AuthData)
		if err != nil {
			return nil, errors.Errorf("builders[%d].auth_data is not valid hex", i)
		}
		if len(ad) > 4096 {
			return nil, errors.Errorf("builders[%d].auth_data exceeds 4096 bytes", i)
		}
		be.AuthData = ad
	}
	if in.MaxExecutionPayment != nil {
		v, err := parseUint(*in.MaxExecutionPayment, "builders[].max_execution_payment")
		if err != nil {
			return nil, err
		}
		be.MaxExecutionPayment = &v
	}
	if in.MinBid != nil {
		v, err := parseUint(*in.MinBid, "builders[].min_bid")
		if err != nil {
			return nil, err
		}
		be.MinBid = &v
	}
	if in.BuilderBoostFactor != nil {
		v, err := parseUint(*in.BuilderBoostFactor, "builders[].builder_boost_factor")
		if err != nil {
			return nil, err
		}
		be.BuilderBoostFactor = &v
	}
	return be, nil
}

func parseUint(s, field string) (validator.Uint64, error) {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, errors.Errorf("%s is not a valid uint64", field)
	}
	return validator.Uint64(v), nil
}

func formatUint(v validator.Uint64) string {
	return strconv.FormatUint(uint64(v), 10)
}

func strPtr(s string) *string { return &s }

func optUintStrPtr(v *validator.Uint64) *string {
	if v == nil {
		return nil
	}
	return strPtr(formatUint(*v))
}
