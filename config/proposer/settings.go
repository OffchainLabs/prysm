package proposer

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/config"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
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
	return settings, nil
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

// BuilderConfig is the struct representation of the JSON config file set in the validator through the CLI.
// GasLimit is a number set to help the network decide on the maximum gas in each block.
type BuilderConfig struct {
	Enabled  bool             `json:"enabled" yaml:"enabled"`
	GasLimit validator.Uint64 `json:"gas_limit,omitempty" yaml:"gas_limit,omitempty"`
	Relays   []string         `json:"relays,omitempty" yaml:"relays,omitempty"`
}

// BuilderConfigFromConsensus converts protobuf to a builder config used in in-memory storage
func BuilderConfigFromConsensus(from *validatorpb.BuilderConfig) *BuilderConfig {
	if from == nil {
		return nil
	}
	c := &BuilderConfig{
		Enabled:  from.Enabled,
		GasLimit: from.GasLimit,
	}
	if from.Relays != nil {
		relays := make([]string, len(from.Relays))
		copy(relays, from.Relays)
		c.Relays = relays
	}
	return c
}

// Schema versions for proposer settings. SchemaV1Unset is the proto3 zero
// value — every existing v1 user has it, since the version field is new.
// Both SchemaV1Unset and SchemaV1 are legacy v1 inputs to the migration.
const (
	SchemaV1Unset uint32 = 0
	SchemaV1      uint32 = 1
	SchemaV2      uint32 = 2
)

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
// 3. defaultconfig is not nil, meaning it has at least fee recipient settings (this defines general settings for all validator keys but keys will use settings from propose config if available), propose config can be nil in this case
func (ps *Settings) ShouldBeSaved() bool {
	return ps != nil && (ps.ProposeConfig != nil || ps.DefaultConfig != nil && ps.DefaultConfig.FeeRecipientConfig != nil)
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
// GasLimit is the v2 home for the gas-limit signal (default_config only).
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
	var relays []string
	if bc.Relays != nil {
		relays = make([]string, len(bc.Relays))
		copy(relays, bc.Relays)
		c.Relays = relays
	}
	return c
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
	var relays []string
	if bc.Relays != nil {
		relays = make([]string, len(bc.Relays))
		copy(relays, bc.Relays)
		c.Relays = relays
	}
	c.GasLimit = bc.GasLimit
	return c
}

func (ps *Settings) isV2() bool {
	return ps != nil && ps.Version == SchemaV2
}

// UpgradeToV2 migrates v1 settings to v2 in place. Returns true if changed.
func (ps *Settings) UpgradeToV2() bool {
	if ps == nil || ps.isV2() {
		return false
	}
	if ps.DefaultConfig != nil && ps.DefaultConfig.BuilderConfig != nil {
		if ps.DefaultConfig.GasLimit == 0 {
			ps.DefaultConfig.GasLimit = ps.DefaultConfig.BuilderConfig.GasLimit
		}
		ps.DefaultConfig.BuilderConfig = nil
	}
	dropped := 0
	for _, opt := range ps.ProposeConfig {
		if opt == nil || opt.BuilderConfig == nil {
			continue
		}
		if opt.BuilderConfig.GasLimit != 0 {
			dropped++
		}
		opt.BuilderConfig = nil
	}
	if dropped > 0 {
		log.Warnf("Dropped per-validator builder.gas_limit on %d key(s) during v1->v2 upgrade; only default_config.gas_limit is honored.", dropped)
	}
	ps.Version = SchemaV2
	return true
}

// GasLimit returns the configured gas limit (gwei) for pubkey, or the chain
// default if no override is configured. v2 ignores the pubkey.
func (ps *Settings) GasLimit(pubkey [fieldparams.BLSPubkeyLength]byte) validator.Uint64 {
	chainDefault := validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit)
	if ps == nil {
		return chainDefault
	}
	if ps.isV2() {
		if ps.DefaultConfig != nil && ps.DefaultConfig.GasLimit != 0 {
			return ps.DefaultConfig.GasLimit
		}
		return chainDefault
	}
	if opt, ok := ps.ProposeConfig[pubkey]; ok && opt.BuilderConfig != nil {
		return opt.BuilderConfig.GasLimit
	}
	if ps.DefaultConfig != nil && ps.DefaultConfig.BuilderConfig != nil {
		return ps.DefaultConfig.BuilderConfig.GasLimit
	}
	return chainDefault
}

// SetGasLimit writes the gas limit. v2 writes DefaultConfig.GasLimit and
// ignores the pubkey; v1 requires existing settings with builder enabled.
func (ps *Settings) SetGasLimit(pubkey [fieldparams.BLSPubkeyLength]byte, gasLimit validator.Uint64) error {
	if ps == nil {
		return errors.New("No proposer settings were found to update")
	}
	if ps.isV2() {
		if ps.DefaultConfig == nil {
			ps.DefaultConfig = &Option{}
		}
		ps.DefaultConfig.GasLimit = gasLimit
		return nil
	}
	builderEnabled := func(o *Option) bool {
		return o != nil && o.BuilderConfig != nil && o.BuilderConfig.Enabled
	}
	if ps.ProposeConfig == nil {
		if !builderEnabled(ps.DefaultConfig) {
			return errors.New("Gas limit changes only apply when builder is enabled")
		}
		ps.ProposeConfig = make(map[[fieldparams.BLSPubkeyLength]byte]*Option)
		opt := ps.DefaultConfig.Clone()
		opt.BuilderConfig.GasLimit = gasLimit
		ps.ProposeConfig[pubkey] = opt
		return nil
	}
	if opt, found := ps.ProposeConfig[pubkey]; found {
		if !builderEnabled(opt) {
			return errors.New("Gas limit changes only apply when builder is enabled")
		}
		opt.BuilderConfig.GasLimit = gasLimit
		return nil
	}
	if !builderEnabled(ps.DefaultConfig) {
		return errors.New("Gas limit changes only apply when builder is enabled")
	}
	opt := ps.DefaultConfig.Clone()
	opt.BuilderConfig.GasLimit = gasLimit
	ps.ProposeConfig[pubkey] = opt
	return nil
}

// ResetGasLimit resets the gas limit to the chain default. v2 resets
// DefaultConfig.GasLimit; v1 resets the per-validator BuilderConfig.GasLimit.
// Returns false when there's nothing to reset.
func (ps *Settings) ResetGasLimit(pubkey [fieldparams.BLSPubkeyLength]byte) bool {
	if ps == nil {
		return false
	}
	chainDefault := validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit)
	if ps.isV2() {
		if ps.DefaultConfig == nil || ps.DefaultConfig.GasLimit == 0 {
			return false
		}
		ps.DefaultConfig.GasLimit = chainDefault
		return true
	}
	opt, found := ps.ProposeConfig[pubkey]
	if !found || opt.BuilderConfig == nil {
		return false
	}
	if ps.DefaultConfig != nil && ps.DefaultConfig.BuilderConfig != nil {
		opt.BuilderConfig.GasLimit = ps.DefaultConfig.BuilderConfig.GasLimit
	} else {
		opt.BuilderConfig.GasLimit = chainDefault
	}
	return true
}
