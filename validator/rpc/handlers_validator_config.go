package rpc

import (
	"encoding/json"
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

const (
	maxBuilderEntries = 64   // MAX_BUILDER_ENTRIES
	maxBuilderURLSize = 2048 // MAX_BUILDER_URL_SIZE
	maxAuthDataSize   = 4096 // MAX_DATA_SIZE
)

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

	// The Effective* getters resolve unset values (no floor, neutral boost, trustless-only).
	eff := s.validatorService.ProposerSettings().EffectiveBuilderConfig(bytesutil.ToBytes48(pubkey))
	if eff == nil {
		eff = &proposer.BuilderConfig{}
	}
	out := builderConfigJSONFromConsensus(eff)
	// The resolved response always states a concrete builders list.
	if out.Builders == nil {
		out.Builders = []*BuilderEntryJson{}
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
	_, pubkey, ok := shared.HexFromRoute(w, r, "pubkey", fieldparams.BLSPubkeyLength)
	if !ok {
		return
	}

	s.proposerSettingsLock.Lock()
	defer s.proposerSettingsLock.Unlock()

	settings := s.validatorService.ProposerSettings()
	key := bytesutil.ToBytes48(pubkey)
	// Removing an absent configuration succeeds; the key already follows the defaults.
	if settings == nil || settings.ProposeConfig == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	opt, found := settings.ProposeConfig[key]
	if !found || opt == nil || opt.BuilderConfig == nil {
		w.WriteHeader(http.StatusNoContent)
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

func builderConfigJSONFromConsensus(bc *proposer.BuilderConfig) *BuilderConfigJson {
	enabled := bc.Enabled
	out := &BuilderConfigJson{
		Enabled:            &enabled,
		MinBid:             new(formatUint(bc.EffectiveMinBid())),
		BuilderBoostFactor: new(formatUint(bc.EffectiveBuilderBoostFactor())),
	}
	if bc.Builders != nil {
		out.Builders = make([]*BuilderEntryJson, 0, len(bc.Builders))
		for _, b := range bc.Builders {
			out.Builders = append(out.Builders, builderEntryJSONFromConsensus(b, bc))
		}
	}
	return out
}

func builderEntryJSONFromConsensus(be *proposer.BuilderEntry, bc *proposer.BuilderConfig) *BuilderEntryJson {
	out := &BuilderEntryJson{
		MaxExecutionPayment: new(formatUint(be.EffectiveMaxExecutionPayment(bc))),
		MinBid:              new(formatUint(be.EffectiveMinBid(bc))),
		BuilderBoostFactor:  new(formatUint(be.EffectiveBuilderBoostFactor(bc))),
	}
	if be.URL != "" {
		out.Url = be.URL
		out.AuthData = new(hexutil.Encode(be.EffectiveAuthData()))
	}
	if len(be.Pubkey) != 0 {
		out.BuilderPubkey = new(hexutil.Encode(be.Pubkey))
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
	// Omitted auth_data compares as its derived value, so it collides with the explicit form.
	bc.Builders = make([]*proposer.BuilderEntry, 0, len(in.Builders))
	seen := make(map[proposer.EntryIdentity]bool, len(in.Builders))
	for i, entry := range in.Builders {
		be, err := builderEntryFromJSON(entry, i)
		if err != nil {
			return nil, err
		}
		if seen[be.Identity()] {
			return nil, errors.Errorf("builders[%d]: two entries share the same url and auth_data", i)
		}
		seen[be.Identity()] = true
		bc.Builders = append(bc.Builders, be)
	}
	return bc, nil
}

func builderEntryFromJSON(in *BuilderEntryJson, i int) (*proposer.BuilderEntry, error) {
	be := &proposer.BuilderEntry{}
	if in.Url == "" {
		return nil, errors.Errorf("builders[%d].url is required", i)
	}
	if len(in.Url) > maxBuilderURLSize {
		return nil, errors.Errorf("builders[%d].url exceeds %d bytes", i, maxBuilderURLSize)
	}
	if u, err := url.Parse(in.Url); err != nil || u.Scheme == "" || u.Host == "" {
		return nil, errors.Errorf("builders[%d].url is not a valid URL", i)
	}
	be.URL = in.Url
	if in.BuilderPubkey != nil {
		pk, err := hexutil.Decode(*in.BuilderPubkey)
		if err != nil || len(pk) != fieldparams.BLSPubkeyLength {
			return nil, errors.Errorf("builders[%d].builder_pubkey is not a valid BLS public key", i)
		}
		be.Pubkey = pk
	}
	if in.AuthData != nil {
		ad, err := hexutil.Decode(*in.AuthData)
		if err != nil {
			return nil, errors.Errorf("builders[%d].auth_data is not valid hex", i)
		}
		if len(ad) > maxAuthDataSize {
			return nil, errors.Errorf("builders[%d].auth_data exceeds %d bytes", i, maxAuthDataSize)
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
