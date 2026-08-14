package proposer

import (
	"fmt"
	"sync/atomic"

	"github.com/OffchainLabs/prysm/v7/config"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

// SettingFromConsensus converts struct to Settings while verifying the fields
func SettingFromConsensus(ps *validatorpb.ProposerSettingsPayload) (*Settings, error) {
	settings := &Settings{Version: ps.Version}
	if len(ps.ProposerConfig) != 0 {
		settings.ProposeConfig = make(map[[fieldparams.BLSPubkeyLength]byte]*Option)
		for key, optionPayload := range ps.ProposerConfig {
			decodedKey, err := hexutil.Decode(key)
			if err != nil {
				return nil, errors.Wrap(err, fmt.Sprintf("cannot decode public key %s", key))
			}
			if len(decodedKey) != fieldparams.BLSPubkeyLength {
				return nil, fmt.Errorf("%v is not a bls public key", key)
			}
			p := &Option{}
			if optionPayload.Graffiti != nil {
				p.GraffitiConfig = &GraffitiConfig{*optionPayload.Graffiti}
			}
			if optionPayload.FeeRecipient != "" {
				if err := verifyOption(key, optionPayload); err != nil {
					return nil, err
				}
				p.FeeRecipientConfig = &FeeRecipientConfig{FeeRecipient: common.HexToAddress(optionPayload.FeeRecipient)}
			}
			if optionPayload.Builder != nil {
				p.BuilderConfig = BuilderConfigFromConsensus(optionPayload.Builder)
			}
			p.GasLimit = optionPayload.GasLimit
			settings.ProposeConfig[bytesutil.ToBytes48(decodedKey)] = p
		}
	}
	if ps.DefaultConfig != nil {
		d := &Option{}
		if ps.DefaultConfig.FeeRecipient != "" {
			if !common.IsHexAddress(ps.DefaultConfig.FeeRecipient) {
				return nil, errors.New("default fee recipient is not a valid Ethereum address")
			}
			if err := config.WarnNonChecksummedAddress(ps.DefaultConfig.FeeRecipient); err != nil {
				return nil, err
			}
			d.FeeRecipientConfig = &FeeRecipientConfig{
				FeeRecipient: common.HexToAddress(ps.DefaultConfig.FeeRecipient),
			}
		}
		if ps.DefaultConfig.Builder != nil {
			d.BuilderConfig = BuilderConfigFromConsensus(ps.DefaultConfig.Builder)
		}
		d.GasLimit = ps.DefaultConfig.GasLimit
		settings.DefaultConfig = d
	}
	settings.dedupBuilders()
	return settings, nil
}

// Persisted configs may predate url-required and (url, auth_data) uniqueness;
// url-less entries are dropped and the first entry wins, matching POST validation.
func (ps *Settings) dedupBuilders() {
	dedup := func(opt *Option) {
		if opt == nil || opt.BuilderConfig == nil || len(opt.BuilderConfig.Builders) == 0 {
			return
		}
		seen := make(map[EntryIdentity]bool, len(opt.BuilderConfig.Builders))
		kept := opt.BuilderConfig.Builders[:0]
		for _, e := range opt.BuilderConfig.Builders {
			if e == nil || e.URL == "" || seen[e.Identity()] {
				continue
			}
			seen[e.Identity()] = true
			kept = append(kept, e)
		}
		if len(kept) != len(opt.BuilderConfig.Builders) {
			log.Warn("Removed url-less or duplicate builder entries from proposer settings")
			opt.BuilderConfig.Builders = kept
		}
	}
	dedup(ps.DefaultConfig)
	for _, opt := range ps.ProposeConfig {
		dedup(opt)
	}
}

func verifyOption(key string, option *validatorpb.ProposerOptionPayload) error {
	if option == nil {
		return fmt.Errorf("fee recipient is required for proposer %s", key)
	}
	if !common.IsHexAddress(option.FeeRecipient) {
		return errors.New("fee recipient is not a valid Ethereum address")
	}
	if err := config.WarnNonChecksummedAddress(option.FeeRecipient); err != nil {
		return err
	}
	return nil
}

// BuilderConfig is the in-memory builder settings.
type BuilderConfig struct {
	Enabled             bool              `json:"enabled" yaml:"enabled"`                                                 // legacy v1 (mev-boost); ignored by v2, dropped at the gloas cutover
	GasLimit            validator.Uint64  `json:"gas_limit,omitempty" yaml:"gas_limit,omitempty"`                         // legacy v1; v2 gas limits live on the option
	MaxExecutionPayment *validator.Uint64 `json:"max_execution_payment,omitempty" yaml:"max_execution_payment,omitempty"` // explicit 0 = trustless-only; unset inherits
	Builders            []*BuilderEntry   `json:"builders" yaml:"builders"`                                               // nil = inherit, [] = use none; no omitempty so the marker survives marshal
	MinBid              *validator.Uint64 `json:"min_bid,omitempty" yaml:"min_bid,omitempty"`
	BuilderBoostFactor  *validator.Uint64 `json:"builder_boost_factor,omitempty" yaml:"builder_boost_factor,omitempty"`
}

// BuilderEntry is one builder in a proposer's per-key builder list. Unset fields
// fall back to the enclosing BuilderConfig, then default_config.
type BuilderEntry struct {
	URL                 string            `json:"url" yaml:"url"`
	Pubkeys             [][]byte          `json:"builder_pubkeys,omitempty" yaml:"builder_pubkeys,omitempty"`
	AuthData            []byte            `json:"auth_data,omitempty" yaml:"auth_data,omitempty"`
	MinBid              *validator.Uint64 `json:"min_bid,omitempty" yaml:"min_bid,omitempty"`
	MaxExecutionPayment *validator.Uint64 `json:"max_execution_payment,omitempty" yaml:"max_execution_payment,omitempty"`
	BuilderBoostFactor  *validator.Uint64 `json:"builder_boost_factor,omitempty" yaml:"builder_boost_factor,omitempty"`
}

// EffectiveAuthData resolves omitted auth_data to the spec convention:
// the UTF-8 bytes of the builder's URL.
func (be *BuilderEntry) EffectiveAuthData() []byte {
	if len(be.AuthData) != 0 {
		return be.AuthData
	}
	return []byte(be.URL)
}

// EffectiveBuilderConfig resolves pubkey's builder config against default_config;
// nil when neither level configures a builder.
func (ps *Settings) EffectiveBuilderConfig(pubkey [fieldparams.BLSPubkeyLength]byte) *BuilderConfig {
	if ps == nil {
		return nil
	}
	var perKey, def *BuilderConfig
	if ps.DefaultConfig != nil {
		def = ps.DefaultConfig.BuilderConfig
	}
	if opt, ok := ps.ProposeConfig[pubkey]; ok && opt != nil {
		perKey = opt.BuilderConfig
	}
	return effectiveBuilderConfig(perKey, def)
}

// RegistrationFor resolves pubkey's mev-boost registration: fee recipient, gas
// limit, and participation. Registrations are pushed pre-gloas only.
func (ps *Settings) RegistrationFor(pubkey [fieldparams.BLSPubkeyLength]byte) (common.Address, validator.Uint64, bool) {
	feeRecipient := common.HexToAddress(params.BeaconConfig().EthBurnAddressHex)
	gasLimit := validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit)
	if ps == nil {
		return feeRecipient, gasLimit, false
	}
	hasFeeRecipient := false
	if ps.DefaultConfig != nil && ps.DefaultConfig.FeeRecipientConfig != nil {
		feeRecipient = ps.DefaultConfig.FeeRecipientConfig.FeeRecipient
		hasFeeRecipient = true
	}
	opt := ps.ProposeConfig[pubkey]
	if opt != nil && opt.FeeRecipientConfig != nil {
		feeRecipient = opt.FeeRecipientConfig.FeeRecipient
		hasFeeRecipient = true
	}
	enabled := false
	if ps.DefaultConfig != nil {
		if in, ok := ps.DefaultConfig.BuilderConfig.registrationEnabled(); ok {
			enabled = in
		}
	}
	// A per-key choice wins over the default's.
	if opt != nil {
		if in, ok := opt.BuilderConfig.registrationEnabled(); ok {
			enabled = in
		}
	}
	// Explicitly set option-level gas limits win; legacy builder-level values
	// are the pre-gloas fallback.
	switch {
	case opt != nil && opt.GasLimit != 0:
		gasLimit = opt.GasLimit
	case ps.DefaultConfig != nil && ps.DefaultConfig.GasLimit != 0:
		gasLimit = ps.DefaultConfig.GasLimit
	case opt != nil && opt.BuilderConfig != nil && opt.BuilderConfig.GasLimit != 0:
		gasLimit = opt.BuilderConfig.GasLimit
	case ps.DefaultConfig != nil && ps.DefaultConfig.BuilderConfig != nil && ps.DefaultConfig.BuilderConfig.GasLimit != 0:
		gasLimit = ps.DefaultConfig.BuilderConfig.GasLimit
	}
	return feeRecipient, gasLimit, enabled && hasFeeRecipient
}

// EntryIdentity is what makes a builder entry unique: its url compared as the
// exact string and its auth_data as the resolved bytes.
type EntryIdentity struct {
	URL  string
	Auth string
}

func (be *BuilderEntry) Identity() EntryIdentity {
	return EntryIdentity{URL: be.URL, Auth: string(be.EffectiveAuthData())}
}

// NeutralBuilderBoostFactor is the resolved boost when none is configured:
// pure profit maximization between builder and local payloads.
const NeutralBuilderBoostFactor = 100

// EffectiveMinBid resolves the config-level floor; unset means no floor.
func (bc *BuilderConfig) EffectiveMinBid() validator.Uint64 {
	if bc == nil || bc.MinBid == nil {
		return 0
	}
	return *bc.MinBid
}

// EffectiveBuilderBoostFactor resolves the config-level boost; unset means neutral.
func (bc *BuilderConfig) EffectiveBuilderBoostFactor() validator.Uint64 {
	if bc == nil || bc.BuilderBoostFactor == nil {
		return NeutralBuilderBoostFactor
	}
	return *bc.BuilderBoostFactor
}

// EffectiveMaxExecutionPayment resolves the ceiling; unset means trustless-only.
func (bc *BuilderConfig) EffectiveMaxExecutionPayment() validator.Uint64 {
	if bc == nil || bc.MaxExecutionPayment == nil {
		return 0
	}
	return *bc.MaxExecutionPayment
}

// EffectiveMinBid resolves this entry's floor, falling back to the enclosing config.
func (be *BuilderEntry) EffectiveMinBid(bc *BuilderConfig) validator.Uint64 {
	if be.MinBid != nil {
		return *be.MinBid
	}
	return bc.EffectiveMinBid()
}

// EffectiveBuilderBoostFactor resolves this entry's boost, falling back to the enclosing config.
func (be *BuilderEntry) EffectiveBuilderBoostFactor(bc *BuilderConfig) validator.Uint64 {
	if be.BuilderBoostFactor != nil {
		return *be.BuilderBoostFactor
	}
	return bc.EffectiveBuilderBoostFactor()
}

// EffectiveMaxExecutionPayment resolves this entry's ceiling, falling back to the enclosing config.
func (be *BuilderEntry) EffectiveMaxExecutionPayment(bc *BuilderConfig) validator.Uint64 {
	if be.MaxExecutionPayment != nil {
		return *be.MaxExecutionPayment
	}
	return bc.EffectiveMaxExecutionPayment()
}

// IsEnabled reports whether the legacy v1 builder path is explicitly enabled.
// v2 settings ignore the enabled field entirely.
func (bc *BuilderConfig) IsEnabled() bool {
	return bc != nil && bc.Enabled
}

// hasV2Content reports whether any v2 builder field is set; an explicit empty
// builders list counts.
func (bc *BuilderConfig) hasV2Content() bool {
	return bc != nil && (bc.Builders != nil || bc.MinBid != nil || bc.BuilderBoostFactor != nil || bc.MaxExecutionPayment != nil)
}

// registrationEnabled reports whether this config opts the key in or out of
// mev-boost registration. ok is false when the config says neither.
func (bc *BuilderConfig) registrationEnabled() (enabled, ok bool) {
	switch {
	case bc == nil:
		return false, false
	case len(bc.Builders) > 0 || bc.Enabled:
		// A nonempty v2 builders list opts in pre-gloas just like v1 enabled.
		return true, true
	case bc.Builders != nil:
		// An explicit empty list means self-build everywhere: no mev-boost either.
		return false, true
	case !bc.hasV2Content():
		// A pure-v1 config without enabled is the legacy wins-wholesale disable.
		return false, true
	default:
		// Gloas knobs (min_bid etc.) without a builders list: no registration choice.
		return false, false
	}
}

// effectiveBuilderConfig merges two config levels with field-level inheritance;
// the builders list replaces rather than merges.
func effectiveBuilderConfig(perKey, def *BuilderConfig) *BuilderConfig {
	if perKey == nil {
		return def
	}
	if def == nil {
		return perKey
	}
	eff := &BuilderConfig{
		GasLimit:            perKey.GasLimit,
		MaxExecutionPayment: coalesceUint64(perKey.MaxExecutionPayment, def.MaxExecutionPayment),
		MinBid:              coalesceUint64(perKey.MinBid, def.MinBid),
		BuilderBoostFactor:  coalesceUint64(perKey.BuilderBoostFactor, def.BuilderBoostFactor),
		Builders:            perKey.Builders,
	}
	if eff.GasLimit == 0 {
		eff.GasLimit = def.GasLimit
	}
	// A nil list inherits the default's; a non-nil empty list means "use no builders".
	if eff.Builders == nil {
		eff.Builders = def.Builders
	}
	return eff
}

func coalesceUint64(a, b *validator.Uint64) *validator.Uint64 {
	if a != nil {
		return a
	}
	return b
}

// BuilderConfigFromConsensus converts protobuf to a builder config used in in-memory storage
func BuilderConfigFromConsensus(from *validatorpb.BuilderConfig) *BuilderConfig {
	if from == nil {
		return nil
	}
	c := &BuilderConfig{
		Enabled:             from.Enabled,
		GasLimit:            from.GasLimit,
		MaxExecutionPayment: cloneUint64(from.MaxExecutionPayment),
		MinBid:              cloneUint64(from.MinBid),
		BuilderBoostFactor:  cloneUint64(from.BuilderBoostFactor),
	}
	if len(from.Builders) > 0 {
		c.Builders = make([]*BuilderEntry, 0, len(from.Builders))
		for _, b := range from.Builders {
			c.Builders = append(c.Builders, builderEntryFromConsensus(b))
		}
	} else if from.GetBuildersSet() {
		// An explicitly configured empty list means "use no builders".
		c.Builders = []*BuilderEntry{}
	}
	return c
}

func builderEntryFromConsensus(from *validatorpb.BuilderEntry) *BuilderEntry {
	if from == nil {
		return nil
	}
	e := &BuilderEntry{
		URL:                 from.Url,
		MinBid:              from.MinBid,
		MaxExecutionPayment: from.MaxExecutionPayment,
		BuilderBoostFactor:  from.BuilderBoostFactor,
	}
	// Treat empty as absent so bolt (nil) and filesystem (empty) round-trips agree.
	if len(from.Pubkeys) != 0 {
		e.Pubkeys = bytesutil.SafeCopy2dBytes(from.Pubkeys)
	}
	if len(from.AuthData) != 0 {
		e.AuthData = bytesutil.SafeCopyBytes(from.AuthData)
	}
	return e
}

// Schema versions for proposer settings. SchemaV1Unset is the proto3 zero value
// every pre-versioning user has; both it and SchemaV1 are legacy v1 inputs.
const (
	SchemaV1Unset uint32 = 0
	SchemaV1      uint32 = 1
	SchemaV2      uint32 = 2
)

// FreshSettingsVersion is the schema stamped on settings the keymanager APIs
// create from nothing: v2 once the network schedules gloas, legacy before.
func FreshSettingsVersion() uint32 {
	if params.GloasEnabled() {
		return SchemaV2
	}
	return SchemaV1Unset
}

// Settings is a Prysm internal representation of the fee recipient config on the validator client.
// validatorpb.ProposerSettingsPayload maps to Settings on import through the CLI.
type Settings struct {
	ProposeConfig map[[fieldparams.BLSPubkeyLength]byte]*Option
	DefaultConfig *Option
	Version       uint32
}

// ShouldBeSaved goes through checks to see if the value should be saveable
// Pseudocode: conditions for being saved into the database
// 1. settings are not nil
// 2. proposeconfig is not nil (this defines specific settings for each validator key), default config can be nil in this case and fall back to beacon node settings
// 3. defaultconfig is not nil, meaning it has at least fee recipient or gas limit settings (this defines general settings for all validator keys but keys will use settings from propose config if available), propose config can be nil in this case
func (ps *Settings) ShouldBeSaved() bool {
	return ps != nil && (ps.ProposeConfig != nil || ps.DefaultConfig != nil && (ps.DefaultConfig.FeeRecipientConfig != nil || ps.DefaultConfig.GasLimit != 0))
}

// ToConsensus converts struct to ProposerSettingsPayload
func (ps *Settings) ToConsensus() *validatorpb.ProposerSettingsPayload {
	if ps == nil {
		return nil
	}
	payload := &validatorpb.ProposerSettingsPayload{Version: ps.Version}
	if ps.ProposeConfig != nil {
		payload.ProposerConfig = make(map[string]*validatorpb.ProposerOptionPayload)
		for key, option := range ps.ProposeConfig {
			payload.ProposerConfig[hexutil.Encode(key[:])] = option.ToConsensus()
		}
	}
	if ps.DefaultConfig != nil {
		payload.DefaultConfig = ps.DefaultConfig.ToConsensus()
	}
	return payload
}

// FeeRecipientConfig is a prysm internal representation to see if the fee recipient was set.
type FeeRecipientConfig struct {
	FeeRecipient common.Address
}

// GraffitiConfig is a prysm internal representation to see if the graffiti was set.
type GraffitiConfig struct {
	Graffiti string
}

// Option is a Prysm internal representation of the ProposerOptionPayload on the validator client in bytes format instead of hex.
// GasLimit is the v2 home for the gas-limit signal, per-pubkey or in default_config.
type Option struct {
	FeeRecipientConfig *FeeRecipientConfig
	BuilderConfig      *BuilderConfig
	GraffitiConfig     *GraffitiConfig
	GasLimit           validator.Uint64
}

// Clone creates a deep copy of proposer option
func (po *Option) Clone() *Option {
	if po == nil {
		return nil
	}
	p := &Option{GasLimit: po.GasLimit}
	if po.FeeRecipientConfig != nil {
		p.FeeRecipientConfig = po.FeeRecipientConfig.Clone()
	}
	if po.BuilderConfig != nil {
		p.BuilderConfig = po.BuilderConfig.Clone()
	}
	if po.GraffitiConfig != nil {
		p.GraffitiConfig = po.GraffitiConfig.Clone()
	}
	return p
}

func (po *Option) ToConsensus() *validatorpb.ProposerOptionPayload {
	if po == nil {
		return nil
	}
	p := &validatorpb.ProposerOptionPayload{GasLimit: po.GasLimit}
	if po.FeeRecipientConfig != nil {
		p.FeeRecipient = po.FeeRecipientConfig.FeeRecipient.Hex()
	}
	if po.BuilderConfig != nil {
		p.Builder = po.BuilderConfig.ToConsensus()
	}
	if po.GraffitiConfig != nil {
		p.Graffiti = &po.GraffitiConfig.Graffiti
	}
	return p
}

// Clone creates a deep copy of the proposer settings
func (ps *Settings) Clone() *Settings {
	if ps == nil {
		return nil
	}
	clone := &Settings{Version: ps.Version}
	if ps.DefaultConfig != nil {
		clone.DefaultConfig = ps.DefaultConfig.Clone()
	}
	if ps.ProposeConfig != nil {
		clone.ProposeConfig = make(map[[fieldparams.BLSPubkeyLength]byte]*Option)
		for k, v := range ps.ProposeConfig {
			keyCopy := k
			valCopy := v.Clone()
			clone.ProposeConfig[keyCopy] = valCopy
		}
	}

	return clone
}

// Clone creates a deep copy of fee recipient config
func (fo *FeeRecipientConfig) Clone() *FeeRecipientConfig {
	if fo == nil {
		return nil
	}
	return &FeeRecipientConfig{fo.FeeRecipient}
}

// Clone creates a deep copy of builder config
func (bc *BuilderConfig) Clone() *BuilderConfig {
	if bc == nil {
		return nil
	}
	c := &BuilderConfig{}
	c.Enabled = bc.Enabled
	c.GasLimit = bc.GasLimit
	c.MaxExecutionPayment = cloneUint64(bc.MaxExecutionPayment)
	c.MinBid = cloneUint64(bc.MinBid)
	c.BuilderBoostFactor = cloneUint64(bc.BuilderBoostFactor)
	// Preserve nil vs empty: an empty list is the "use no builders" marker.
	if bc.Builders != nil {
		c.Builders = make([]*BuilderEntry, 0, len(bc.Builders))
		for _, b := range bc.Builders {
			c.Builders = append(c.Builders, b.Clone())
		}
	}
	return c
}

// Clone creates a deep copy of a builder entry
func (be *BuilderEntry) Clone() *BuilderEntry {
	if be == nil {
		return nil
	}
	return &BuilderEntry{
		URL:                 be.URL,
		Pubkeys:             bytesutil.SafeCopy2dBytes(be.Pubkeys),
		AuthData:            bytesutil.SafeCopyBytes(be.AuthData),
		MinBid:              cloneUint64(be.MinBid),
		MaxExecutionPayment: cloneUint64(be.MaxExecutionPayment),
		BuilderBoostFactor:  cloneUint64(be.BuilderBoostFactor),
	}
}

func cloneUint64(v *validator.Uint64) *validator.Uint64 {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

// Clone creates a deep copy of graffiti config
func (gc *GraffitiConfig) Clone() *GraffitiConfig {
	if gc == nil {
		return nil
	}
	return &GraffitiConfig{gc.Graffiti}
}

// ToConsensus converts Builder Config to the protobuf object
func (bc *BuilderConfig) ToConsensus() *validatorpb.BuilderConfig {
	if bc == nil {
		return nil
	}
	c := &validatorpb.BuilderConfig{}
	c.Enabled = bc.Enabled
	c.GasLimit = bc.GasLimit
	c.MaxExecutionPayment = cloneUint64(bc.MaxExecutionPayment)
	c.MinBid = cloneUint64(bc.MinBid)
	c.BuilderBoostFactor = cloneUint64(bc.BuilderBoostFactor)
	// BuildersSet preserves nil-vs-empty across the wire: an explicit empty
	// list is the "use no builders" marker and must survive persistence.
	c.BuildersSet = bc.Builders != nil
	if len(bc.Builders) > 0 {
		c.Builders = make([]*validatorpb.BuilderEntry, 0, len(bc.Builders))
		for _, b := range bc.Builders {
			c.Builders = append(c.Builders, b.toConsensus())
		}
	}
	return c
}

func (be *BuilderEntry) toConsensus() *validatorpb.BuilderEntry {
	if be == nil {
		return nil
	}
	return &validatorpb.BuilderEntry{
		Url:                 be.URL,
		Pubkeys:             bytesutil.SafeCopy2dBytes(be.Pubkeys),
		AuthData:            bytesutil.SafeCopyBytes(be.AuthData),
		MinBid:              cloneUint64(be.MinBid),
		MaxExecutionPayment: cloneUint64(be.MaxExecutionPayment),
		BuilderBoostFactor:  cloneUint64(be.BuilderBoostFactor),
	}
}

func (ps *Settings) isV2() bool {
	return ps != nil && ps.Version == SchemaV2
}

// WarnDeprecatedSchema logs a warning when legacy v1 builder content is loaded
// on a network with gloas scheduled, regardless of the schema stamp.
func (ps *Settings) WarnDeprecatedSchema() {
	if ps == nil || !params.GloasEnabled() {
		return
	}
	// Fee recipients and graffiti behave identically across schemas; only v1
	// builder fields give the cutover something to drop.
	if !ps.HasLegacyBuilderContent() {
		return
	}
	log.Warn("Proposer settings contain deprecated v1 builder fields (enabled, builder-level gas limits); they stop applying at the gloas fork and are replaced with defaults (fee recipients and graffiti carry over). Configure gloas builders via v2 settings or the keymanager API.")
}

// HasLegacyBuilderContent reports whether any level carries v1 builder fields,
// i.e. whether the gloas cutover has anything to drop.
func (ps *Settings) HasLegacyBuilderContent() bool {
	if ps == nil {
		return false
	}
	legacy := func(opt *Option) bool {
		return opt != nil && opt.BuilderConfig != nil && (opt.BuilderConfig.Enabled || opt.BuilderConfig.GasLimit != 0)
	}
	if legacy(ps.DefaultConfig) {
		return true
	}
	for _, opt := range ps.ProposeConfig {
		if legacy(opt) {
			return true
		}
	}
	return false
}

// UpgradeToV2 is the gloas cutover: legacy v1 builder fields are scrubbed
// wherever they appear — even under a v2 stamp — and the version is stamped.
func (ps *Settings) UpgradeToV2() bool {
	if ps == nil {
		return false
	}
	scrubbed := false
	scrub := func(opt *Option) {
		if opt == nil || opt.BuilderConfig == nil {
			return
		}
		bc := opt.BuilderConfig
		if bc.Enabled || bc.GasLimit != 0 {
			bc.Enabled = false
			bc.GasLimit = 0
			scrubbed = true
		}
		// A config left with no v2 content disappears entirely; an explicit
		// empty builders list is v2 content and survives.
		if !bc.hasV2Content() {
			opt.BuilderConfig = nil
			scrubbed = true
		}
	}
	scrub(ps.DefaultConfig)
	for _, opt := range ps.ProposeConfig {
		scrub(opt)
	}
	changed := scrubbed || ps.Version != SchemaV2
	ps.Version = SchemaV2
	if scrubbed {
		log.Warn("v1 builder settings, including gas limits, do not apply to gloas and were replaced with defaults; provide v2 proposer settings to configure builders")
	}
	return changed
}

// TargetGasLimit resolves pubkey's proposer-preference gas limit at epoch: the
// explicit operator value, else the EIP-8261 schedule, else the chain default.
func (ps *Settings) TargetGasLimit(pubkey [fieldparams.BLSPubkeyLength]byte, epoch primitives.Epoch) validator.Uint64 {
	scheduled, active := params.BeaconConfig().ScheduledGasLimit(epoch)
	operator, ok := ps.operatorGasLimit(pubkey)
	if !ok {
		if active {
			return validator.Uint64(scheduled)
		}
		return validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit)
	}
	if active && uint64(operator) > scheduled {
		warnGasLimitExceedsSchedule(uint64(operator), scheduled, epoch)
	}
	if active && uint64(operator) < scheduled {
		warnGasLimitBelowSchedule(uint64(operator), scheduled, epoch)
	}
	return operator
}

func (ps *Settings) operatorGasLimit(pubkey [fieldparams.BLSPubkeyLength]byte) (validator.Uint64, bool) {
	if ps == nil {
		return 0, false
	}
	if opt, ok := ps.ProposeConfig[pubkey]; ok && opt != nil && opt.GasLimit != 0 {
		return opt.GasLimit, true
	}
	if ps.DefaultConfig != nil && ps.DefaultConfig.GasLimit != 0 {
		return ps.DefaultConfig.GasLimit, true
	}
	return 0, false
}

var warnedGasLimitScheduleEpoch atomic.Uint64

func warnGasLimitExceedsSchedule(operator, scheduled uint64, epoch primitives.Epoch) {
	if !warnOncePerEpoch(&warnedGasLimitScheduleEpoch, epoch) {
		return
	}
	log.Warnf("Configured gas limit %d exceeds the recommended maximum of %d at epoch %d", operator, scheduled, epoch)
}

var warnedGasLimitBelowScheduleEpoch atomic.Uint64

func warnGasLimitBelowSchedule(operator, scheduled uint64, epoch primitives.Epoch) {
	if !warnOncePerEpoch(&warnedGasLimitBelowScheduleEpoch, epoch) {
		return
	}
	log.Warnf("Configured gas limit %d is below the scheduled network gas limit of %d at epoch %d; remove the explicit gas limit to follow the schedule", operator, scheduled, epoch)
}

// warnOncePerEpoch reports whether the caller won this epoch's single warning slot.
func warnOncePerEpoch(guard *atomic.Uint64, epoch primitives.Epoch) bool {
	e := uint64(epoch) + 1
	for {
		prev := guard.Load()
		if e <= prev {
			return false
		}
		if guard.CompareAndSwap(prev, e) {
			return true
		}
	}
}

// GasLimit resolves pubkey's gas limit: explicitly set option-level values win,
// legacy builder-level values are the fallback, else the chain default.
func (ps *Settings) GasLimit(pubkey [fieldparams.BLSPubkeyLength]byte) validator.Uint64 {
	chainDefault := validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit)
	if ps == nil {
		return chainDefault
	}
	opt := ps.ProposeConfig[pubkey]
	switch {
	case opt != nil && opt.GasLimit != 0:
		return opt.GasLimit
	case ps.DefaultConfig != nil && ps.DefaultConfig.GasLimit != 0:
		return ps.DefaultConfig.GasLimit
	case opt != nil && opt.BuilderConfig != nil && opt.BuilderConfig.GasLimit != 0:
		return opt.BuilderConfig.GasLimit
	case ps.DefaultConfig != nil && ps.DefaultConfig.BuilderConfig != nil && ps.DefaultConfig.BuilderConfig.GasLimit != 0:
		return ps.DefaultConfig.BuilderConfig.GasLimit
	}
	return chainDefault
}

// UpsertProposeOption returns pubkey's option, creating it if absent. A new
// option keeps BuilderConfig nil so it inherits default_config.
func (ps *Settings) UpsertProposeOption(pubkey [fieldparams.BLSPubkeyLength]byte) *Option {
	if ps.ProposeConfig == nil {
		ps.ProposeConfig = make(map[[fieldparams.BLSPubkeyLength]byte]*Option)
	}
	opt := ps.ProposeConfig[pubkey]
	if opt == nil {
		opt = &Option{}
		ps.ProposeConfig[pubkey] = opt
	}
	return opt
}

// SetGasLimit writes the per-pubkey gas limit at the option level, where both
// pre-gloas registrations and post-gloas preferences read it first.
func (ps *Settings) SetGasLimit(pubkey [fieldparams.BLSPubkeyLength]byte, gasLimit validator.Uint64) error {
	if ps == nil {
		return errors.New("No proposer settings were found to update")
	}
	ps.UpsertProposeOption(pubkey).GasLimit = gasLimit
	return nil
}

// ResetGasLimit reverts pubkey's gas limit to the configured default (or chain
// default). Returns false when there's nothing to reset.
func (ps *Settings) ResetGasLimit(pubkey [fieldparams.BLSPubkeyLength]byte) bool {
	if ps == nil {
		return false
	}
	chainDefault := validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit)
	opt, found := ps.ProposeConfig[pubkey]
	if !found || opt == nil {
		return false
	}
	reset := false
	if opt.GasLimit != 0 {
		if ps.DefaultConfig != nil && ps.DefaultConfig.GasLimit != 0 {
			opt.GasLimit = ps.DefaultConfig.GasLimit
		} else {
			opt.GasLimit = 0
		}
		reset = true
	}
	// Legacy per-key builder gas limits reset to the default's builder value.
	if opt.BuilderConfig != nil && opt.BuilderConfig.GasLimit != 0 {
		if ps.DefaultConfig != nil && ps.DefaultConfig.BuilderConfig != nil {
			opt.BuilderConfig.GasLimit = ps.DefaultConfig.BuilderConfig.GasLimit
		} else {
			opt.BuilderConfig.GasLimit = chainDefault
		}
		reset = true
	}
	return reset
}
