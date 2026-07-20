package rpc

import (
	"encoding/json"
	"net/http"
	"strconv"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

const (
	configStatusSet      = "set"
	configStatusNotFound = "not_found"
	configStatusError    = "error"
)

// GetValidatorConfig implements GET /eth/v1/validator/config from keymanager-APIs #87.
// It returns the explicitly configured per-key fragments plus the read-only default_config.
func (s *Server) GetValidatorConfig(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "validator.keymanagerAPI.GetValidatorConfig")
	defer span.End()

	if s.validatorService == nil {
		httputil.HandleError(w, "Validator service not ready.", http.StatusServiceUnavailable)
		return
	}

	filter, ok := parsePubkeyFilter(w, r)
	if !ok {
		return
	}

	settings := s.validatorService.ProposerSettings()
	resp := &GetValidatorConfigResponse{Data: &ValidatorConfigData{Configs: map[string]*ValidatorConfig{}}}
	if settings == nil {
		httputil.WriteJson(w, resp)
		return
	}
	if settings.DefaultConfig != nil {
		resp.Data.DefaultConfig = validatorConfigFromOption(settings.DefaultConfig)
	}
	for pk, opt := range settings.ProposeConfig {
		if filter != nil {
			if _, wanted := filter[pk]; !wanted {
				continue
			}
		}
		resp.Data.Configs[hexutil.Encode(pk[:])] = validatorConfigFromOption(opt)
	}
	httputil.WriteJson(w, resp)
}

// SetValidatorConfig implements POST /eth/v1/validator/config from keymanager-APIs #87.
// Each submitted document REPLACES the key's configured fragment. An empty document
// clears the key. Keys are applied independently with a per-key status.
func (s *Server) SetValidatorConfig(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.keymanagerAPI.SetValidatorConfig")
	defer span.End()

	if s.validatorService == nil {
		httputil.HandleError(w, "Validator service not ready.", http.StatusServiceUnavailable)
		return
	}

	var req SetValidatorConfigRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Configs == nil {
		httputil.HandleError(w, "No configs submitted", http.StatusBadRequest)
		return
	}

	km, err := s.validatorService.Keymanager()
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	known, err := km.FetchValidatingPublicKeys(ctx)
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	knownSet := make(map[[fieldparams.BLSPubkeyLength]byte]bool, len(known))
	for _, pk := range known {
		knownSet[pk] = true
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

	resp := &SetValidatorConfigResponse{Data: make(map[string]*ValidatorConfigStatus, len(req.Configs))}
	changed := false
	for rawPubkey, cfg := range req.Configs {
		pubkey, err := decodePubkey(rawPubkey)
		if err != nil {
			resp.Data[rawPubkey] = &ValidatorConfigStatus{Status: configStatusError, Message: err.Error()}
			continue
		}
		if !knownSet[pubkey] {
			resp.Data[rawPubkey] = &ValidatorConfigStatus{Status: configStatusNotFound}
			continue
		}
		if cfg == nil || cfg.isEmpty() {
			delete(settings.ProposeConfig, pubkey)
			resp.Data[rawPubkey] = &ValidatorConfigStatus{Status: configStatusSet}
			changed = true
			continue
		}
		opt, err := optionFromValidatorConfig(cfg)
		if err != nil {
			resp.Data[rawPubkey] = &ValidatorConfigStatus{Status: configStatusError, Message: err.Error()}
			continue
		}
		settings.ProposeConfig[pubkey] = opt
		resp.Data[rawPubkey] = &ValidatorConfigStatus{Status: configStatusSet}
		changed = true
	}

	if changed {
		if err := s.validatorService.SetProposerSettings(ctx, settings); err != nil {
			httputil.HandleError(w, "Could not set proposer settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	httputil.WriteJson(w, resp)
}

func parsePubkeyFilter(w http.ResponseWriter, r *http.Request) (map[[fieldparams.BLSPubkeyLength]byte]bool, bool) {
	raw := r.URL.Query()["pubkeys"]
	if len(raw) == 0 {
		return nil, true
	}
	filter := make(map[[fieldparams.BLSPubkeyLength]byte]bool, len(raw))
	for _, p := range raw {
		pubkey, err := decodePubkey(p)
		if err != nil {
			httputil.HandleError(w, "Invalid pubkey "+p+": "+err.Error(), http.StatusBadRequest)
			return nil, false
		}
		filter[pubkey] = true
	}
	return filter, true
}

func decodePubkey(raw string) ([fieldparams.BLSPubkeyLength]byte, error) {
	var out [fieldparams.BLSPubkeyLength]byte
	decoded, err := hexutil.Decode(raw)
	if err != nil {
		return out, errors.Wrap(err, "invalid public key")
	}
	if len(decoded) != fieldparams.BLSPubkeyLength {
		return out, errors.Errorf("public key %s is not %d bytes", raw, fieldparams.BLSPubkeyLength)
	}
	return bytesutil.ToBytes48(decoded), nil
}

func (c *ValidatorConfig) isEmpty() bool {
	return c.FeeRecipient == nil && c.TargetGasLimit == nil && c.Graffiti == nil && c.Builder == nil
}

// validatorConfigFromOption serializes only the explicitly configured fields, so
// tooling can tell an explicit value from an inherited one.
func validatorConfigFromOption(opt *proposer.Option) *ValidatorConfig {
	cfg := &ValidatorConfig{}
	if opt == nil {
		return cfg
	}
	if opt.FeeRecipientConfig != nil {
		addr := opt.FeeRecipientConfig.FeeRecipient.Hex()
		cfg.FeeRecipient = &addr
	}
	if opt.GasLimit != 0 {
		cfg.TargetGasLimit = strPtr(formatUint(opt.GasLimit))
	}
	if opt.GraffitiConfig != nil {
		g := opt.GraffitiConfig.Graffiti
		cfg.Graffiti = &g
	}
	if opt.BuilderConfig != nil {
		cfg.Builder = builderConfigJSONFromConsensus(opt.BuilderConfig)
	}
	return cfg
}

func builderConfigJSONFromConsensus(bc *proposer.BuilderConfig) *BuilderConfigJson {
	out := &BuilderConfigJson{
		Enabled:             bc.Enabled,
		Proxy:               bc.Proxy,
		MaxExecutionPayment: optUintStrPtr(bc.MaxExecutionPayment),
		MinBid:              optUintStrPtr(bc.MinBid),
		BuilderBoostFactor:  optUintStrPtr(bc.BuilderBoostFactor),
	}
	for _, b := range bc.Builders {
		out.Builders = append(out.Builders, builderEntryJSONFromConsensus(b))
	}
	return out
}

func builderEntryJSONFromConsensus(be *proposer.BuilderEntry) *BuilderEntryJson {
	out := &BuilderEntryJson{
		Url:                 be.URL,
		Proxy:               be.Proxy,
		MaxExecutionPayment: optUintStrPtr(be.MaxExecutionPayment),
		MinBid:              optUintStrPtr(be.MinBid),
		BuilderBoostFactor:  optUintStrPtr(be.BuilderBoostFactor),
	}
	if len(be.Pubkey) != 0 {
		out.Pubkey = strPtr(hexutil.Encode(be.Pubkey))
	}
	if len(be.AuthData) != 0 {
		out.AuthData = strPtr(hexutil.Encode(be.AuthData))
	}
	return out
}

// optionFromValidatorConfig builds a fresh Option from a submitted document. Every
// field is validated; unset fields stay nil so they inherit from default_config.
func optionFromValidatorConfig(cfg *ValidatorConfig) (*proposer.Option, error) {
	opt := &proposer.Option{}
	if cfg.FeeRecipient != nil {
		if !common.IsHexAddress(*cfg.FeeRecipient) {
			return nil, errors.Errorf("fee_recipient %s is not a valid execution address", *cfg.FeeRecipient)
		}
		opt.FeeRecipientConfig = &proposer.FeeRecipientConfig{FeeRecipient: common.HexToAddress(*cfg.FeeRecipient)}
	}
	if cfg.TargetGasLimit != nil {
		v, err := parseUint(*cfg.TargetGasLimit, "target_gas_limit")
		if err != nil {
			return nil, err
		}
		opt.GasLimit = v
	}
	if cfg.Graffiti != nil {
		opt.GraffitiConfig = &proposer.GraffitiConfig{Graffiti: *cfg.Graffiti}
	}
	if cfg.Builder != nil {
		bc, err := builderConfigFromJSON(cfg.Builder)
		if err != nil {
			return nil, err
		}
		opt.BuilderConfig = bc
	}
	return opt, nil
}

func builderConfigFromJSON(in *BuilderConfigJson) (*proposer.BuilderConfig, error) {
	bc := &proposer.BuilderConfig{Enabled: in.Enabled, Proxy: in.Proxy}
	if in.MaxExecutionPayment != nil {
		v, err := parseUint(*in.MaxExecutionPayment, "builder.max_execution_payment")
		if err != nil {
			return nil, err
		}
		bc.MaxExecutionPayment = &v
	}
	if in.MinBid != nil {
		v, err := parseUint(*in.MinBid, "builder.min_bid")
		if err != nil {
			return nil, err
		}
		bc.MinBid = &v
	}
	if in.BuilderBoostFactor != nil {
		v, err := parseUint(*in.BuilderBoostFactor, "builder.builder_boost_factor")
		if err != nil {
			return nil, err
		}
		bc.BuilderBoostFactor = &v
	}
	for i, entry := range in.Builders {
		be, err := builderEntryFromJSON(entry, i)
		if err != nil {
			return nil, err
		}
		bc.Builders = append(bc.Builders, be)
	}
	return bc, nil
}

func builderEntryFromJSON(in *BuilderEntryJson, i int) (*proposer.BuilderEntry, error) {
	if in.Url == "" {
		return nil, errors.Errorf("builder.builders[%d].url is required", i)
	}
	be := &proposer.BuilderEntry{URL: in.Url, Proxy: in.Proxy}
	if in.Pubkey != nil {
		pk, err := hexutil.Decode(*in.Pubkey)
		if err != nil || len(pk) != fieldparams.BLSPubkeyLength {
			return nil, errors.Errorf("builder.builders[%d].pubkey is not a valid BLS public key", i)
		}
		be.Pubkey = pk
	}
	if in.AuthData != nil {
		ad, err := hexutil.Decode(*in.AuthData)
		if err != nil {
			return nil, errors.Errorf("builder.builders[%d].auth_data is not valid hex", i)
		}
		if len(ad) > 4096 {
			return nil, errors.Errorf("builder.builders[%d].auth_data exceeds 4096 bytes", i)
		}
		be.AuthData = ad
	}
	if in.MaxExecutionPayment != nil {
		v, err := parseUint(*in.MaxExecutionPayment, "builder.builders[].max_execution_payment")
		if err != nil {
			return nil, err
		}
		be.MaxExecutionPayment = &v
	}
	if in.MinBid != nil {
		v, err := parseUint(*in.MinBid, "builder.builders[].min_bid")
		if err != nil {
			return nil, err
		}
		be.MinBid = &v
	}
	if in.BuilderBoostFactor != nil {
		v, err := parseUint(*in.BuilderBoostFactor, "builder.builders[].builder_boost_factor")
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
